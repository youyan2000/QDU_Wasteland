package hotdigest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// curatorPromptVersion is part of the rewrite cache key; bump it when the
// prompts below change.
const curatorPromptVersion = "2026-10-30.2"

// translationBatch is how many items one translation call carries.
const translationBatch = 8

// modelRetries is how many times a dropped or throttled call is retried. The
// gateway closes the connection when too many requests overlap, so retries are
// what keep a run from silently losing sections.
const modelRetries = 3

// postChat sends one chat completion and returns the assistant text. Network
// drops and 429/5xx answers are retried with a growing pause.
func (e *Enricher) postChat(ctx context.Context, payload map[string]any) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	client := e.Client
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	var lastErr error
	for attempt := 0; attempt < modelRetries; attempt++ {
		if attempt > 0 {
			delay := e.RetryDelay
			if delay <= 0 {
				delay = 4 * time.Second
			}
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(attempt) * delay):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.APIURL, bytes.NewReader(raw))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+e.APIKey)
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("model returned %d", resp.StatusCode)
			if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
				continue
			}
			return "", lastErr
		}
		var completion struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(data, &completion); err != nil {
			lastErr = err
			continue
		}
		if len(completion.Choices) == 0 {
			lastErr = errors.New("model returned no choices")
			continue
		}
		return completion.Choices[0].Message.Content, nil
	}
	if lastErr == nil {
		lastErr = errors.New("model call failed")
	}
	return "", lastErr
}

// curatorMaxCandidates bounds what one field sends to the model.
const curatorMaxCandidates = 30

// curatorPicks is how many rows the model is asked for; the rule selection
// fills the rest of the section.
const curatorPicks = 15

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// fieldBrief tells the model what "worth reading" means in each field. This is
// the editorial KnowHow the rules cannot express: a religion of recency cannot
// tell an advanced paper from a routine one, or a good essay from a listicle.
var fieldBrief = map[string]string{
	GroupPhilosophy: "哲学理论：优先真正提出新论证的同行评议论文（原创论文优先于书评、会议综述、悼文、征稿）；" +
		"偏好形而上学、认识论、语言哲学、心灵哲学、伦理学与政治哲学中的前沿工作；不要选纯书讯、期刊通告。",
	GroupEssay: "思想散文：优先有具体叙事或洞见的长文，偏好被多个平台转载、被读书类精选榜（Longreads、Arts & Letters Daily、Literary Hub）收录的文章；" +
		"排除清单体、鸡汤、营销软文、单纯的新闻转述。",
	GroupPower: "电力电子：优先宽禁带器件（SiC/GaN）、新拓扑、数字控制、磁性元件、封装与热管理上的实质进展；" +
		"论文与产业报道都要，但排除纯财报、纯人事、纯营销。",
	GroupEmbedded: "嵌入式：优先新的 MCU/SoC 架构、RISC-V、RTOS 与工具链、低功耗与边缘 AI 加速、功能安全与验证；" +
		"排除纯消费电子评测、众筹预热、教程搬运。",
	GroupAI: "AI：优先模型能力、训练与推理方法上的实质进展、开源权重、评测与安全研究；" +
		"排除纯融资、纯人事、纯产品营销、单纯榜单名次。",
	GroupMusic: "潮流音乐：优先新专辑与新单曲的实质评述、场景与流派动向、制作技术与设备创新；" +
		"排除榜单流水账、纯八卦、票务信息。",
}

// Curate asks the model to pick and write the best items of one field. It
// returns nil when the model is unavailable or its answer is unusable, and the
// caller keeps the rule-based selection.
func (e *Enricher) Curate(ctx context.Context, group string, candidates []Item, want int) []Item {
	if e == nil || len(candidates) == 0 || want <= 0 {
		return nil
	}
	brief, ok := fieldBrief[group]
	if !ok {
		return nil
	}
	if len(candidates) > curatorMaxCandidates {
		candidates = candidates[:curatorMaxCandidates]
	}
	// The model only has to order the very best few; the rules already chose a
	// full section, and asking for thirty rows in one answer makes a smaller
	// model drift away from JSON.
	ask := want
	if ask > curatorPicks {
		ask = curatorPicks
	}

	rewrites, err := e.requestCuration(ctx, group, brief, candidates, ask)
	if err != nil {
		// One shorter retry: a smaller prompt usually gets valid JSON back.
		short := candidates
		if len(short) > 12 {
			short = short[:12]
		}
		if len(short) < len(candidates) {
			if retry, retryErr := e.requestCuration(ctx, group, brief, short, minInt(ask, 8)); retryErr == nil {
				rewrites, err = retry, nil
			}
		}
		if err != nil {
			log.Printf("hotdigest: curation for %q failed: %v", group, err)
			return nil
		}
	}

	byID := make(map[string]Item, len(candidates))
	for _, it := range candidates {
		byID[it.ID] = it
	}
	out := make([]Item, 0, len(rewrites))
	for _, rw := range rewrites {
		base, ok := byID[rw.ID]
		if !ok {
			continue
		}
		chinese := targetIsChinese(base)
		title := oneLine(rw.Title, 200)
		summary := oneLine(rw.Summary, 320)
		if title == "" {
			title = base.Title
		}
		if chinese != hasHan(title) {
			title = base.Title
			summary = base.Summary
		}
		item := base
		if title != base.Title {
			item.TitleOrig = base.Title
			item.Title = title
			item.Rewritten = true
			item.Translated = hasHan(base.Title) != hasHan(title)
		}
		if summary != "" && chinese == hasHan(summary) {
			item.Summary = summary
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil
	}
	if len(out) > want {
		out = out[:want]
	}
	return out
}

// curationReply is one row of the model's JSON answer.
type curationReply struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

func (e *Enricher) requestCuration(ctx context.Context, group, brief string, candidates []Item, want int) ([]curationReply, error) {
	var body strings.Builder
	for i, it := range candidates {
		kind := "新闻"
		if it.Kind == KindPaper {
			kind = "论文"
		}
		lang := it.Lang
		if lang == "" {
			lang = "en"
		}
		fmt.Fprintf(&body, "[%s] 类型=%s 来源=%s 语言=%s\n标题：%s\n", it.ID, kind, it.Source, lang, it.Title)
		if it.Summary != "" {
			fmt.Fprintf(&body, "摘要：%s\n", it.Summary)
		}
		body.WriteString("---\n")
		if i >= curatorMaxCandidates {
			break
		}
	}

	system := "你是「热点日报」的编辑，从一个行业的候选条目里挑出最值得读的那些，并写好标题与摘要。\n\n" +
		"语言规则（最重要）：原文是中文的，标题和摘要用中文；原文是其他任何语言的（法语、德语、西班牙语、日语……），" +
		"标题和摘要都必须译成英文。不要保留原文语言，人名、机构名、书名可以保留原名。\n\n" +
		"行业标准：" + brief + "\n\n" +
		fmt.Sprintf("只输出 JSON 数组，按推荐顺序排列，最多 %d 条：{\"id\":\"候选编号\",\"title\":\"...\",\"summary\":\"...\"}。"+
			"标题要具体、自足，不添加原文没有的公司名或数字；摘要一到两句，先给结论，不重复标题。不要输出其他文字。", want)

	payload := map[string]any{
		"model": e.Model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": body.String()},
		},
		"temperature": 0.3,
	}
	content, err := e.postChat(ctx, payload)
	if err != nil {
		return nil, err
	}
	return parseCuration(content)
}

// parseCuration accepts the documented array and tolerates a fenced answer.
func parseCuration(text string) ([]curationReply, error) {
	start := strings.Index(text, "[")
	end := strings.LastIndex(text, "]")
	if start < 0 || end <= start {
		return nil, errors.New("model output has no JSON array")
	}
	var rows []curationReply
	if err := json.Unmarshal([]byte(text[start:end+1]), &rows); err != nil {
		return nil, err
	}
	out := make([]curationReply, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.ID) == "" {
			continue
		}
		out = append(out, row)
	}
	if len(out) == 0 {
		return nil, errors.New("model output has no usable rows")
	}
	return out, nil
}

// Translate rewrites every non-Chinese title and summary that is still in its
// original language. The curation prompt asks for English, but a model that
// echoes a French or German title would otherwise pass every language check
// that only looks for Han characters, so this second, explicit pass runs after
// selection and is the one that guarantees English output.
func (e *Enricher) Translate(ctx context.Context, items []Item) []Item {
	if e == nil || len(items) == 0 {
		return items
	}
	idx := make([]int, 0, len(items))
	for i, it := range items {
		if it.Lang == "zh" || hasHan(it.Title) {
			continue
		}
		if !looksEnglish(it.Title) || (it.Summary != "" && !looksEnglish(it.Summary)) {
			idx = append(idx, i)
		}
	}
	if len(idx) == 0 {
		return items
	}

	for start := 0; start < len(idx); start += translationBatch {
		end := start + translationBatch
		if end > len(idx) {
			end = len(idx)
		}
		batch := idx[start:end]
		translated, err := e.requestTranslation(ctx, items, batch)
		if err != nil {
			log.Printf("hotdigest: translation failed: %v", err)
			continue
		}
		for local, entry := range translated {
			if local < 0 || local >= len(batch) {
				continue
			}
			i := batch[local]
			title := oneLine(entry.Title, 200)
			if title == "" || !looksEnglish(title) {
				continue
			}
			out := items[i]
			if title != out.Title {
				if out.TitleOrig == "" {
					out.TitleOrig = out.Title
				}
				out.Title = title
				out.Rewritten = true
				out.Translated = true
			}
			if summary := oneLine(entry.Summary, 320); summary != "" && looksEnglish(summary) {
				out.Summary = summary
			}
			items[i] = out
		}
	}
	return items
}

func (e *Enricher) requestTranslation(ctx context.Context, items []Item, batch []int) (map[int]curationReply, error) {
	var body strings.Builder
	for local, i := range batch {
		it := items[i]
		fmt.Fprintf(&body, "[%d] 语言=%s\n标题：%s\n", local, it.Lang, it.Title)
		if it.Summary != "" {
			fmt.Fprintf(&body, "摘要：%s\n", it.Summary)
		}
		body.WriteString("---\n")
	}
	system := "把下面每一条的标题和摘要译成英文。只翻译，不要改写、不要补充原文没有的信息、不要评价。" +
		"人名、机构名、书名保留原名。只输出 JSON 数组：{\"index\":编号,\"title\":\"英文标题\",\"summary\":\"英文摘要\"}，不要输出其他文字。"

	payload := map[string]any{
		"model": e.Model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": body.String()},
		},
		"temperature": 0.1,
	}
	content, err := e.postChat(ctx, payload)
	if err != nil {
		return nil, err
	}
	start := strings.Index(content, "[")
	end := strings.LastIndex(content, "]")
	if start < 0 || end <= start {
		return nil, errors.New("translation has no JSON array")
	}
	var rows []struct {
		Index   int    `json:"index"`
		Title   string `json:"title"`
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(content[start:end+1]), &rows); err != nil {
		return nil, err
	}
	out := make(map[int]curationReply, len(rows))
	for _, row := range rows {
		out[row.Index] = curationReply{Title: row.Title, Summary: row.Summary}
	}
	if len(out) == 0 {
		return nil, errors.New("translation has no usable rows")
	}
	return out, nil
}

// candidatesForCuration returns the freshest pool entries, which are what the
// model gets to choose from.
func candidatesForCuration(pool []Item, limit int) []Item {
	if limit <= 0 || limit > len(pool) {
		limit = len(pool)
	}
	return pool[:limit]
}
