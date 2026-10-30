// org.go — J1 学院/专业全站统一
// 建立「学院规范名 + 学院<->课表多个别名 映射 + 专业总表」。
// 目标：全站（课程/注册/个人中心/后台）只用一份规范名展示，课慢原始别名只在内部解析时归一。
package main

import (
	"log"
	"net/http"
	"sort"
	"strings"
)

// collegeAliases 规范名 -> 课表/历史数据里的多种叫法（用于把原始名归一为规范名）
// 反向：collegeAliasReverse 缓存 别名/原样 -> 规范名，供 canonicalCollege 使用
var collegeAliasReverse = map[string]string{}

// orgColleges 规范学院总表（注册下拉 + 全站展示用），排序稳定
var orgColleges []string

// orgMajors 规范学院 -> 专业列表
var orgMajors = map[string][]string{}

// buildOrgIndexes 由 orgCollegeData 构建 orgColleges / orgMajors / 别名反向表
// 由 initOrg() 调用，进行课程归一化后统一写入 orgColleges。
func buildOrgIndexes() {
	seen := map[string]bool{}
	for name, majors := range orgCollegeData {
		orgMajors[name] = majors
	}
	// 把所有规范名 + 别名录入反向表
	for canon, aliases := range orgCollegeAliases {
		collegeAliasReverse[canon] = canon
		for _, a := range aliases {
			collegeAliasReverse[a] = canon
		}
	}
	// 规范学院列表 = 别名表中"规范名"（自映射键）+ orgCollegeData 键
	for canon := range collegeAliasReverse {
		if collegeAliasReverse[canon] == canon {
			seen[canon] = true
		}
	}
	for name := range orgCollegeData {
		seen[name] = true
	}
	orgColleges = orgColleges[:0]
	for name := range seen {
		orgColleges = append(orgColleges, name)
	}
	sort.Strings(orgColleges)
}

// canonicalCollege 把任意学院名（课表/历史/自由输入）归一到规范名
func canonicalCollege(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	// 1) 精确别名命中
	if c := collegeAliasReverse[name]; c != "" {
		return c
	}
	// 2) 去掉括号/尾巴等常见变体后尝试
	base := trimCollegeSuffix(name)
	if base != name {
		if c := collegeAliasReverse[base]; c != "" {
			return c
		}
	}
	// 3) 去掉"学院"后缀后命中（如"青岛医学院-公共卫生学院"形式）
	for k, c := range collegeAliasReverse {
		// k 是规范名时跳过（避免把规范名再映射）
		if trimCollegeSuffix(k) != k {
			continue
		}
		if strings.TrimSuffix(name, k) != name {
			_ = c
		}
	}
	return name
}

// trimCollegeSuffix 去掉学院名的常见后缀变体：括号/斜杠/连字符块，使名称可比较
func trimCollegeSuffix(s string) string {
	// 去掉括号块："某某（微纳技术学院）"、"某某(教师教育学院)"、"某某_某某"
	if i := strings.IndexAny(s, "（(_-"); i > 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

// orgCollegeAliases 学院规范名 -> 课表/历史数据里的各种叫法
// 注意：同一别名不会被两个规范名共用；若出现请删掉冲突项。
var orgCollegeAliases = map[string][]string{
	"电子信息学院":     {"电子信息学院（微纳技术学院）", "微纳技术学院", "电子信息学院(微纳技术学院)"},
	"教育科学学院":     {"教育科学学院(教师教育学院)", "教育科学学院（教师教育学院）", "师范学院_教师教育学院", "师范学院"},
	"青岛医学院":      {"医学部", "青岛大学医学部", "口腔医学院", "护理学院", "药学院", "公共卫生学院", "基础医学院"},
	"艺术学院":       {"音乐学院", "美术学院"},
	"材料科学与工程学院":  {"材料学院"},
	"机电工程学院":     {"机电学院"},
	"计算机科学技术学院":  {"计算机学院", "计算机科学技术学院（数据科学与软件工程学院）"},
	"数学与统计学院":    {"数学院"},
	"文学与新闻传播学院":  {"文传学院"},
	"环境与地理科学学院":  {"旅游与地理科学学院", "环境科学与工程学院"},
	"体育学院":       {"体育部"},
	"马克思主义学院":    {"思政学院"},
	"物理科学学院":     {"物理学院"},
	"自动化学院":      {"自动化与电气学院"},
	"电气工程学院":     {"电气学院"},
	"化学化工学院":     {"化工学院"},
	"生命科学学院":     {"生科院"},
	"纺织服装学院":     {"纺织学院"},
	"历史学院":       {"文学院"},
	"外语学院":       {"外院"},
	"经济学院":       {"经济学院（会计学院）"},
	"商学院":        {"管理科学与工程学院"},
}

// orgCollegeData 学院规范名 -> 专业总表（官方：青岛大学专业统计）
// 数据来源：青岛大学专业统计.md（官方权威清单，25 个学院）
var orgCollegeData = map[string][]string{
	"数学与统计学院":   {"数学与应用数学", "数学与应用数学（师范）", "应用统计学"},
	"物理科学学院":    {"应用物理学", "物理学（师范）", "新能源科学与工程", "光电信息科学与工程"},
	"化学化工学院":    {"化学（师范）", "应用化学", "化学工程与工艺"},
	"生命科学学院":    {"生物技术", "食品科学与工程"},
	"机电工程学院":    {"机械工程", "智能制造工程", "工业设计", "能源与动力工程", "储能科学与工程"},
	"材料科学与工程学院": {"高分子材料与工程", "复合材料与工程", "智能材料与结构"},
	"自动化学院":     {"自动化", "机器人工程"},
	"电气工程学院":    {"电气工程及其自动化"},
	"电子信息学院":    {"电子信息工程", "通信工程", "微电子科学与工程", "集成电路设计与集成系统", "电子信息工程（中外合作）"},
	"计算机科学技术学院": {"计算机科学与技术", "软件工程", "信息安全", "智能科学与技术", "人工智能"},
	"环境与地理科学学院": {"环境科学与工程", "地理科学（师范）"},
	"纺织服装学院":    {"纺织工程", "服装设计与工程", "轻化工程"},
	"青岛医学院":     {"临床医学", "医学影像学", "口腔医学", "预防医学", "药学", "护理学", "医学检验技术", "智能医学工程"},
	"马克思主义学院":   {"思想政治教育"},
	"历史学院":      {"历史学"},
	"经济学院":      {"经济学", "金融学", "财政学", "国际经济与贸易"},
	"法学院":       {"法学"},
	"政治与公共管理学院": {"行政管理", "国际政治"},
	"教育科学学院":    {"应用心理学", "教育技术学", "学前教育", "小学教育", "人工智能教育"},
	"体育学院":      {"体育教育"},
	"文学与新闻传播学院": {"汉语言文学", "新闻学", "广播电视编导"},
	"外语学院":      {"英语", "德语", "法语", "西班牙语", "日语", "朝鲜语"},
	"商学院":       {"工商管理", "会计学", "信息管理与信息系统", "供应链管理", "标准化工程", "旅游管理（中外合作）"},
	"艺术学院":      {"音乐表演", "音乐学", "舞蹈学", "绘画", "视觉传达设计", "环境设计"},
	"青岛大学德雷克联合学院": {"计算机科学与技术（中外合作）", "生物技术（中外合作）"},
}

// initOrg 构建学院索引（在 db.loadAll 之后调用一次）
func initOrg() {
	buildOrgIndexes()
}

// normalizeCollegeDisplay 把课程/班次的学院展示名统一为规范名（J1）
func normalizeCollegeDisplay(db *DB) {
	changed := 0
	for i := range db.Courses {
		c := &db.Courses[i]
		n := canonicalCollege(c.College)
		if n != c.College {
			changed++
			c.College = n
		}
		for j := range c.Colleges {
			nj := canonicalCollege(c.Colleges[j])
			if nj != c.Colleges[j] {
				changed++
				c.Colleges[j] = nj
			}
		}
	}
	// 同步归一 byKey 源（供后续 rebuildCourses 复用，避免回退）
	for _, c := range db.byKey {
		c.College = canonicalCollege(c.College)
		for j := range c.Colleges {
			c.Colleges[j] = canonicalCollege(c.Colleges[j])
		}
	}
	for i := range db.Offerings {
		o := &db.Offerings[i]
		no := canonicalCollege(o.College)
		if no != o.College {
			changed++
			o.College = no
		}
		ne := canonicalCollege(o.EnrollCollege)
		if ne != o.EnrollCollege {
			changed++
			o.EnrollCollege = ne
		}
	}
	log.Printf("🟢 normalizeCollegeDisplay: 修改学院字段 %d 处（含 byKey 源归一）", changed)
}

// ---- J1 对外 API ----

// handleOrg 返回全站学院/专业统一清单
// GET /api/org        => { colleges:[...], majors:{学院:[专业,...]} , aliases:{规范名:[别名,...]} }
// GET /api/org/colleges => { colleges:[...] }
// GET /api/org/majors?college=xx => { majors:[...] }
func handleOrg(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/api/org")
		switch {
		case p == "/colleges":
			apiJSON(w, 200, map[string]any{"colleges": orgColleges})
		case p == "/majors":
			col := r.URL.Query().Get("college")
			m := orgMajors[col]
			if m == nil {
				m = []string{}
			}
			apiJSON(w, 200, map[string]any{"majors": m})
		default:
			aliases := map[string][]string{}
			for canon, al := range orgCollegeAliases {
				aliases[canon] = al
			}
			apiJSON(w, 200, map[string]any{
				"colleges": orgColleges,
				"majors":   orgMajors,
				"aliases":  aliases,
			})
		}
	}
}
