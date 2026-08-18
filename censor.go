// censor.go — V8 关键词自动审查拦截
// 敏感词存于 SQLite 表 censor_words（可后台增删），启动时加载到内存，
// 发布内容（发帖/评论/评价/上传标题与描述）时自动检查，命中则拒绝提交。
package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"path"
	"strings"
	"time"
)

// 内存敏感词表（启动时从库加载）
var censorWords []string

// 默认敏感词（首次启动时写入 censor_words 表）
// 已按「色情低俗/暴力血腥/辱骂歧视/违禁交易/政治敏感/诈骗广告/赌博毒品」分类扩充 (J3)
var defaultCensorWords = []string{
	// 色情低俗
	"色情", "卖淫", "嫖娼", "裸聊", "约炮", "一夜情", "援交", "招妓", "强奸",
	"黄片", "三级片", "AV", "成人视频", "黄色网站", "情色", "色诱", "露点", "性暗示",
	// 暴力血腥
	"杀人", "自杀", "跳楼", "割腕", "服毒", "恐怖袭击", "砍人", "群殴", "行凶", "爆炸物",
	// 辱骂歧视
	"傻逼", "傻叉", "操你妈", "草泥马", "妈的逼", "狗日的", "狗逼", "蠢猪",
	"去死", "贱人", "婊子", "脑残", "弱智", "白痴",
	"汉奸", "卖国贼", "台独", "港独", "藏独", "疆独", "支那",
	// 违禁交易
	"代开发票", "办证", "假文凭", "假证", "刷单", "刷好评", "洗钱", "走私",
	"代考", "代写论文", "代写作业", "学位买卖", "买答案", "卖答案", "枪手",
	// 贪污受贿
	"行贿", "受贿", "回扣",
	// 赌博毒品
	"博彩", "赌博", "赌球", "六合彩", "开赌场", "投注", "荷官",
	"毒品", "冰毒", "海洛因", "大麻", "摇头丸", "K粉", "制毒", "吸毒", "毒贩", "麻黄素",
	// 骗局诈骗
	"裸贷", "校园贷", "传销", "庞氏骗局", "资金盘", "刷流水", "刷单赚钱",
	"加我微信", "加我QQ", "加我威信", "二维码加群", "进群领红包", "兼职日结",
	"恭喜您中奖", "退款请加", "银行卡验证", "裸聊",
	// 政治敏感（极端）
	"分裂国家", "邪教", "法轮功", "颠覆国家",
	"暴动", "煽动",
	// 隐私与骚扰
	"人肉搜索", "曝光隐私",
}

// initCensor 建表 + 种子默认词 + 加载内存词表
func initCensor(db *sql.DB) {
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS censor_words (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		word       TEXT NOT NULL UNIQUE,
		created_at TEXT NOT NULL
	)`)
	for _, w := range defaultCensorWords {
		var cnt int
		db.QueryRow(`SELECT COUNT(*) FROM censor_words WHERE word=?`, w).Scan(&cnt)
		if cnt == 0 {
			_, _ = db.Exec(`INSERT INTO censor_words (word, created_at) VALUES (?,?)`,
				w, time.Now().Format("2006-01-02 15:04:05"))
		}
	}
	loadCensorWords(db)
}

func loadCensorWords(db *sql.DB) {
	rows, err := db.Query(`SELECT word FROM censor_words ORDER BY id`)
	if err != nil {
		return
	}
	defer rows.Close()
	var words []string
	for rows.Next() {
		var w string
		rows.Scan(&w)
		words = append(words, w)
	}
	censorWords = words
}

// hitWords 返回文本命中哪些敏感词（小写化比较，兼容英文）
func hitWords(text string) []string {
	if len(censorWords) == 0 || text == "" {
		return nil
	}
	lower := strings.ToLower(text)
	var hits []string
	for _, w := range censorWords {
		if w == "" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(w)) {
			hits = append(hits, w)
		}
	}
	return hits
}

// ---- 后台 API ----

// handleAdminCensor 敏感词管理
// GET  /api/admin/censor           列表
// POST /api/admin/censor           body: {"word":"..."} 新增
// DELETE /api/admin/censor/{id}    删除
func handleAdminCensor(auth *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(auth, w, r) == 0 {
			return
		}
		switch r.Method {
		case http.MethodPost:
			var body struct{ Word string `json:"word"` }
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				apiErr(w, 400, "请求格式错误")
				return
			}
			word := strings.TrimSpace(body.Word)
			if word == "" {
				apiErr(w, 400, "敏感词不能为空")
				return
			}
			var cnt int
			auth.db.QueryRow(`SELECT COUNT(*) FROM censor_words WHERE word=?`, word).Scan(&cnt)
			if cnt > 0 {
				apiErr(w, 400, "该敏感词已存在")
				return
			}
			_, err := auth.db.Exec(`INSERT INTO censor_words (word, created_at) VALUES (?,?)`,
				word, time.Now().Format("2006-01-02 15:04:05"))
			if err != nil {
				apiErr(w, 500, "添加失败")
				return
			}
			loadCensorWords(auth.db)
			apiJSON(w, 200, map[string]any{"ok": true, "word": word})
		case http.MethodDelete:
			idStr := strings.TrimPrefix(r.URL.Path, "/api/admin/censor/")
			id := parseID(path.Base(idStr))
			if id == 0 {
				apiErr(w, 400, "无效的敏感词ID")
				return
			}
			_, err := auth.db.Exec(`DELETE FROM censor_words WHERE id=?`, id)
			if err != nil {
				apiErr(w, 500, "删除失败")
				return
			}
			loadCensorWords(auth.db)
			apiJSON(w, 200, map[string]any{"ok": true, "id": id})
		default: // GET
			page := atoi(r.URL.Query().Get("page"))
			if page < 1 { page = 1 }
			pageSize := atoi(r.URL.Query().Get("pageSize"))
			if pageSize < 1 || pageSize > 100 { pageSize = 30 }
			var total int
			auth.db.QueryRow(`SELECT COUNT(*) FROM censor_words`).Scan(&total)
			rows, err := auth.db.Query(`SELECT id, word, created_at FROM censor_words ORDER BY id DESC LIMIT ? OFFSET ?`, pageSize, (page-1)*pageSize)
			if err != nil {
				apiErr(w, 500, "查询失败")
				return
			}
			defer rows.Close()
			type item struct {
				ID        int64  `json:"id"`
				Word      string `json:"word"`
				CreatedAt string `json:"createdAt"`
			}
			out := []item{}
			for rows.Next() {
				it := item{}
				rows.Scan(&it.ID, &it.Word, &it.CreatedAt)
				out = append(out, it)
			}
			apiJSON(w, 200, map[string]any{"total": total, "page": page, "pageSize": pageSize, "words": out})
		}
	}
}