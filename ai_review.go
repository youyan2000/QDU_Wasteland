// ai_review.go — 后台 AI 内容审查（V16）
// 通过可配置的 LLM API（OpenAI 兼容）审查文本/文件内容，帮助站长判断违规。
// 环境变量：
//   AI_API_URL  默认 https://api.openai.com/v1/chat/completions
//   AI_API_KEY  API 密钥（未配置时接口返回提示，功能不可用）
//   AI_MODEL    默认 gpt-4o-mini（可换 deepseek-chat 等）
// 接口：POST /api/admin/ai-review  body:{content}
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// aiReviewRequest 审查请求
// POST /api/admin/ai-review  body:{content, type?}
func handleAiReview() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(authStore, w, r) == 0 { return }
		apiKey := os.Getenv("AI_API_KEY")
		if apiKey == "" {
			apiErr(w, 400, "AI 审查未配置：请设置环境变量 AI_API_KEY（以及可选的 AI_API_URL / AI_MODEL）")
			return
		}
		var body struct {
			Content string `json:"content"`
			Type    string `json:"type"` // post/comment/review/article/file
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiErr(w, 400, "请求格式错误"); return
		}
		body.Content = trimMaxRunes(body.Content, 3000)
		if body.Content == "" {
			apiErr(w, 400, "内容不能为空"); return
		}

		apiURL := os.Getenv("AI_API_URL")
		if apiURL == "" { apiURL = "https://api.openai.com/v1/chat/completions" }
		model := os.Getenv("AI_MODEL")
		if model == "" { model = "gpt-4o-mini" }

		system := "你是校园内容安全审查助手。请审查用户提交的内容，判断是否违规。违规类别：色情、淫秽、暴力、侮辱辱骂、广告营销、诈骗、政治敏感、版权侵权、人身攻击。请输出 JSON：{\"ok\":true/false,\"categories\":[\"命中类别\"],\"reason\":\"简短中文理由\",\"suggestion\":\"建议（通过/下架/删除）\"}。只输出 JSON，不要其他文字。"

		payload := map[string]any{
			"model": model,
			"messages": []map[string]string{
				{"role": "system", "content": system},
				{"role": "user", "content": "内容类型：" + body.Type + "\n内容：\n" + body.Content},
			},
			"temperature": 0.2,
		}
		jsonData, _ := json.Marshal(payload)
		req, err := http.NewRequest("POST", apiURL, bytes.NewReader(jsonData))
		if err != nil { apiErr(w, 500, "请求构造失败"); return }
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		client := &http.Client{Timeout: 60 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			apiErr(w, 502, "AI 服务连接失败："+err.Error()); return
		}
		defer resp.Body.Close()
		respData, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
		if resp.StatusCode != 200 {
			apiErr(w, 502, "AI 服务返回错误 "+fmt.Sprint(resp.StatusCode)+": "+string(respData)); return
		}
		var cr struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(respData, &cr); err != nil || len(cr.Choices) == 0 {
			apiErr(w, 502, "AI 响应解析失败"); return
		}
		aiOut := cr.Choices[0].Message.Content
		// 提取 JSON（模型可能包在 ```json 里）
		aiOut = extractJSONBlock(aiOut)
		var verdict struct {
			Ok         bool     `json:"ok"`
			Categories []string `json:"categories"`
			Reason     string   `json:"reason"`
			Suggestion string   `json:"suggestion"`
		}
		if err := json.Unmarshal([]byte(aiOut), &verdict); err != nil {
			// 解析失败也返回原始输出
			apiJSON(w, 200, map[string]any{"raw": aiOut})
			return
		}
		logAudit(authStore.db, currentUserID(r), "AI审查", "类型="+body.Type+" 结果="+verdict.Suggestion)
		apiJSON(w, 200, map[string]any{"verdict": verdict})
	}
}

// extractJSONBlock 从模型输出提取第一个 {...} JSON 块（去掉 ```json 包裹）
func extractJSONBlock(s string) string {
	i := 0
	for i < len(s) {
		if s[i] == '{' { break }
		i++
	}
	if i >= len(s) { return s }
	j := len(s)
	for j > i {
		if s[j-1] == '}' { break }
		j--
	}
	if j <= i { return s }
	return s[i:j]
}

// trimMaxRunes 截断到 maxLen 个 rune
func trimMaxRunes(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) > maxLen {
		return string(runes[:maxLen])
	}
	return s
}

// ---- V16.1 两级批量审查（DeepSeek 接入）----
// POST /api/admin/ai-review/auto  body:{limit?}
// 策略：拉取候选内容（帖子/评论/评价/文章），AI 批量判断：
//   clear      → 放行（不改动）
//   spam       → 明显恶意（反复出现/空洞/恶俗）→ 自动下架
//   borderline → 模糊 → 返回列表供站长人工复核
// 结果：{processed, removed:[...], borderline:[...], errors}

func handleAiReviewAuto() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(authStore, w, r) == 0 { return }
		apiKey := os.Getenv("AI_API_KEY")
		if apiKey == "" {
			apiErr(w, 400, "AI 审查未配置：请设置环境变量 AI_API_KEY（可用 DeepSeek：AI_API_URL=https://api.deepseek.com/v1/chat/completions AI_MODEL=deepseek-chat）")
			return
		}
		var body struct {
			Limit int `json:"limit"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Limit < 1 || body.Limit > 100 { body.Limit = 30 }

		// 1) 收集候选：最近且状态正常的帖子/评论/评价/文章
		type cand struct {
			Kind    string `json:"kind"`
			ID      int64  `json:"id"`
			Content string `json:"content"`
			Author  string `json:"author"`
		}
		var candidates []cand
		add := func(kind string, id int64, content, author string) {
			content = trimMaxRunes(content, 400)
			if content != "" {
				candidates = append(candidates, cand{kind, id, content, author})
			}
		}
		// 帖子
		if rows, err := authStore.db.Query(`SELECT p.id, p.title, p.content, COALESCE(u.nickname,'') FROM posts p LEFT JOIN users u ON u.id=p.user_id WHERE p.status='正常' ORDER BY p.id DESC LIMIT ?`, body.Limit); err == nil {
			for rows.Next() { var id int64; var t, c, n string; rows.Scan(&id, &t, &c, &n); add("post", id, t+" "+c, n) }
			rows.Close()
		}
		// 评论
		if rows, err := authStore.db.Query(`SELECT c.id, c.content, COALESCE(u.nickname,'') FROM comments c LEFT JOIN users u ON u.id=c.user_id ORDER BY c.id DESC LIMIT ?`, body.Limit); err == nil {
			for rows.Next() { var id int64; var c, n string; rows.Scan(&id, &c, &n); add("comment", id, c, n) }
			rows.Close()
		}
		// 评价
		if rows, err := authStore.db.Query(`SELECT r.id, r.content, COALESCE(u.nickname,'') FROM reviews r LEFT JOIN users u ON u.id=r.user_id WHERE r.status='正常' ORDER BY r.id DESC LIMIT ?`, body.Limit); err == nil {
			for rows.Next() { var id int64; var c, n string; rows.Scan(&id, &c, &n); add("review", id, c, n) }
			rows.Close()
		}
		// 文章
		if rows, err := authStore.db.Query(`SELECT a.id, a.title, a.content, COALESCE(u.nickname,'') FROM articles a LEFT JOIN users u ON u.id=a.user_id WHERE a.status='正常' ORDER BY a.id DESC LIMIT ?`, body.Limit); err == nil {
			for rows.Next() { var id int64; var t, c, n string; rows.Scan(&id, &t, &c, &n); add("article", id, t+" "+c, n) }
			rows.Close()
		}
		if len(candidates) == 0 {
			apiJSON(w, 200, map[string]any{"processed": 0, "removed": []any{}, "borderline": []any{}})
			return
		}

		// 2) 分批送 AI（每批 ≤ 10 条）
		apiURL := os.Getenv("AI_API_URL")
		if apiURL == "" { apiURL = "https://api.openai.com/v1/chat/completions" }
		model := os.Getenv("AI_MODEL")
		if model == "" { model = "gpt-4o-mini" }

		var removed, borderline []map[string]any
		batchSize := 10
		for start := 0; start < len(candidates); start += batchSize {
			end := start + batchSize
			if end > len(candidates) { end = len(candidates) }
			batch := candidates[start:end]

			var sb strings.Builder
			for i, c := range batch {
				fmt.Fprintf(&sb, "[%d] 类型=%s id=%d 作者=%s\n内容：%s\n---\n", i, c.Kind, c.ID, c.Author, c.Content)
			}
			system := "你是校园内容安全审查助手。对下面每条内容（用 [序号] 标记）判断违规等级，只输出 JSON 数组，每项：{\"index\":序号,\"verdict\":\"clear|spam|borderline\",\"reason\":\"简短理由\"}。clear=正常；spam=明显恶意（反复出现的广告/垃圾/恶俗/色情/辱骂/诈骗，内容空洞无意义）；borderline=模糊、不确定是否违规。不要输出其他文字。"
			payload := map[string]any{
				"model": model,
				"messages": []map[string]string{
					{"role": "system", "content": system},
					{"role": "user", "content": sb.String()},
				},
				"temperature": 0.1,
			}
			jsonData, _ := json.Marshal(payload)
			req, err := http.NewRequest("POST", apiURL, bytes.NewReader(jsonData))
			if err != nil { continue }
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+apiKey)
			client := &http.Client{Timeout: 90 * time.Second}
			resp, err := client.Do(req)
			if err != nil { continue }
			respData, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
			resp.Body.Close()
			if resp.StatusCode != 200 { continue }
			var cr struct {
				Choices []struct {
					Message struct{ Content string `json:"content"` } `json:"message"`
				} `json:"choices"`
			}
			if json.Unmarshal(respData, &cr) != nil || len(cr.Choices) == 0 { continue }
			aiOut := extractJSONBlock(cr.Choices[0].Message.Content)
			var verdicts []struct {
				Index   int    `json:"index"`
				Verdict string `json:"verdict"`
				Reason  string `json:"reason"`
			}
			if json.Unmarshal([]byte(aiOut), &verdicts) != nil { continue }

			for _, v := range verdicts {
				if v.Index < 0 || v.Index >= len(batch) { continue }
				c := batch[v.Index]
				item := map[string]any{"kind": c.Kind, "id": c.ID, "content": trimMaxRunes(c.Content, 120), "author": c.Author, "reason": v.Reason}
				switch v.Verdict {
				case "spam":
					// 自动下架
					removeByKind(c.Kind, c.ID)
					item["action"] = "已下架"
					removed = append(removed, item)
				case "borderline":
					item["action"] = "待复核"
					borderline = append(borderline, item)
				}
			}
		}
		logAudit(authStore.db, currentUserID(r), "AI自动审查", fmt.Sprintf("处理%d条 下架%d 模糊%d", len(candidates), len(removed), len(borderline)))
		apiJSON(w, 200, map[string]any{"processed": len(candidates), "removed": removed, "borderline": borderline})
	}
}

// removeByKind 按类型下架内容
func removeByKind(kind string, id int64) {
	switch kind {
	case "post":
		_, _ = authStore.db.Exec("UPDATE posts SET status='已下架' WHERE id=?", id)
	case "comment":
		// 评论无 status，直接删除
		_, _ = authStore.db.Exec("DELETE FROM comments WHERE id=?", id)
	case "review":
		_, _ = authStore.db.Exec("UPDATE reviews SET status='已下架' WHERE id=?", id)
	case "article":
		_, _ = authStore.db.Exec("UPDATE articles SET status='已下架' WHERE id=?", id)
	}
}