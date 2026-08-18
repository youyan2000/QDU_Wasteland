// api.go — HTTP 接口 (V1+) / V14 课程多维检索
package main

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
)

// apiJSON 统一 JSON 响应
func apiJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// listCoursesDTO 列表项
type listCoursesDTO struct {
	Code       string   `json:"code"`
	Name       string   `json:"name"`
	College    string   `json:"college"`
	Colleges   []string `json:"colleges"`
	Tag        string   `json:"tag"`
	Credit     string   `json:"credit"`
	Exam       string   `json:"exam"`
	Nature     string   `json:"nature"`
	BigType    string   `json:"bigType"`
	General    string   `json:"general"`
	Summary    string   `json:"summary"`
	Majors     []string `json:"majors"`
	Teachers   []string `json:"teachers,omitempty"`
	ClassGroup string   `json:"classGroup,omitempty"`
	Terms      []string `json:"-"`
	FileCount  int      `json:"-"`
	ReviewCount int     `json:"-"`
	Category   string   `json:"category,omitempty"` // 新分类体系（必修/选修下的路径）
}

// codeToCourses 缓存：code -> 对应 course(s)（byCode 已在 db 提供单映射）
// 供课次筛选（teacher/time/major）反查

// handleCourses 课程列表 + 多维搜索 + 筛选
// 参数：
//   q        课程名/编号
//   teacher  任课老师（子串）
//   college  开课院系
//   major    上课专业（子串）
//   cat      课程类别：专业课/公共必修课/公共选修课/通选课/体育课/实验课/实践
//   time     上课时间周几（周一~周日，或"1,2节"）
func handleCourses(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if id := strings.TrimPrefix(r.URL.Path, "/api/courses"); len(id) > 1 {
			handleCourseDetail(db, strings.TrimPrefix(id, "/"))(w, r)
			return
		}
		q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
		tag := strings.TrimSpace(r.URL.Query().Get("tag"))
		teacher := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("teacher")))
		college := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("college")))
		major := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("major")))
		cat := strings.TrimSpace(r.URL.Query().Get("cat"))
		tw := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("time")))
		sem := strings.TrimSpace(r.URL.Query().Get("semester"))
		creditMin := strings.TrimSpace(r.URL.Query().Get("creditMin"))
		creditMax := strings.TrimSpace(r.URL.Query().Get("creditMax"))
		exam := strings.TrimSpace(r.URL.Query().Get("exam"))
		sortBy := strings.TrimSpace(r.URL.Query().Get("sort"))

		// 若需要按班次维度筛选（teacher/time/major），先扫 offerings 收集命中 code 集合
		var hitCodes map[string]bool
		if teacher != "" || tw != "" || major != "" {
			hitCodes = map[string]bool{}
			for _, o := range db.Offerings {
				if teacher != "" && !strings.Contains(strings.ToLower(o.Teacher), teacher) {
					continue
				}
				if tw != "" && !strings.Contains(strings.ToLower(o.Time), tw) {
					continue
				}
				if major != "" {
					found := false
					for _, m := range majorsFromClass(o.ClassGroup, o.Term) {
						if strings.Contains(strings.ToLower(m), major) { found = true; break }
					}
					if !found { continue }
				}
				if sem != "" && o.Term != sem {
					continue
				}
				hitCodes[o.CourseCode] = true
			}
		} else if sem != "" {
			// 按学期筛选（无班次维度时也扫班次）
			hitCodes = map[string]bool{}
			for _, o := range db.Offerings {
				if o.Term == sem {
					hitCodes[o.CourseCode] = true
				}
			}
		}

		out := make([]listCoursesDTO, 0, len(db.Courses))
		for _, c := range db.Courses {
			// tag（传统课程标签，仍保留兼容）
			if tag != "" && c.Tag != tag { continue }
			// cat 新分类体系匹配（如 "必修"、"必修/专业核心课"、"选修/通识教育选修课/核心课"）
			if cat != "" {
				full := courseCategory(c)
				if !catMatch(full, cat) { continue }
			}
			// 考核方式
			if exam != "" && c.ExamType != exam { continue }
			// 学分范围
			if creditMin != "" || creditMax != "" {
				credit := parseCredit(c.Credit)
				if creditMin != "" && credit < parseCredit(creditMin) { continue }
				if creditMax != "" && credit > parseCredit(creditMax) { continue }
			}
			// 班次维度命中
			if hitCodes != nil {
				inAny := false
				for _, code := range c.Codes {
					if hitCodes[code] { inAny = true; break }
				}
				if !inAny { continue }
			}
			// 开课院系
			if college != "" && !strings.Contains(strings.ToLower(c.College), college) { continue }
			// q 名称/编号
			if q != "" {
				hay := strings.ToLower(c.Name + " " + c.Code + " " + c.GeneralCat + " " + c.Tag + " " + c.College + " " + c.Nature)
				if !strings.Contains(hay, q) { continue }
			}
			out = append(out, listCoursesDTO{
				Code: c.Code, Name: c.Name, College: c.College, Colleges: c.Colleges,
				Tag: c.Tag, Credit: c.Credit,
				Exam: c.ExamType, Nature: c.Nature, BigType: c.BigType,
				General: c.GeneralCat, Summary: c.FeaturedText, Majors: c.Majors, Terms: c.Terms,
				FileCount: courseFileCount(db, c.Code), ReviewCount: courseReviewCount(db, c.Code),
				Category: courseCategory(c),
			})
		}
		// J4：排序
		applyCourseSort(out, sortBy)
		apiJSON(w, 200, map[string]any{"total": len(out), "courses": out})
	}
}

// parseCredit 解析学分为数字（如 "3.0" -> 3，解析失败返回 -1）
func parseCredit(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" { return -1 }
	var n float64
	var dec float64
	var inDec bool
	scale := 1.0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '.' { inDec = true; continue }
		if ch < '0' || ch > '9' { continue }
		if inDec { scale *= 0.1; dec += float64(ch-'0')*scale } else { n = n*10 + float64(ch-'0') }
	}
	return n + dec
}

// maxTermScore 取课程所有学期中的最高学期分（用于 newest 排序）
func maxTermScore(terms []string) int {
	best := 0
	for _, t := range terms {
		if s := termScore(t); s > best {
			best = s
		}
	}
	return best
}

// courseFileCount 某课程的资料数量（正常状态）
func courseFileCount(db *DB, code string) int {
	var n int
	authStore.db.QueryRow("SELECT COUNT(*) FROM files WHERE course_code=? AND status='正常'", code).Scan(&n)
	return n
}

// courseReviewCount 某课程的评价数量（正常状态）
func courseReviewCount(db *DB, code string) int {
	var n int
	authStore.db.QueryRow("SELECT COUNT(*) FROM reviews WHERE course_code=? AND status='正常'", code).Scan(&n)
	return n
}

// applyCourseSort 按 sort 参数对课程列表排序（J4）
func applyCourseSort(courses []listCoursesDTO, sortBy string) {
	switch sortBy {
	case "credit": // 学分从高到低
		sort.SliceStable(courses, func(i, j int) bool {
			return parseCredit(courses[i].Credit) > parseCredit(courses[j].Credit)
		})
	case "files": // 按资料数量（多→少）
		sort.SliceStable(courses, func(i, j int) bool {
			if courses[i].FileCount != courses[j].FileCount { return courses[i].FileCount > courses[j].FileCount }
			return courses[i].Name < courses[j].Name
		})
	case "hot": // 按热度（评价数*2 + 资料数，多→少）
		sort.SliceStable(courses, func(i, j int) bool {
			hi, hj := courses[i].ReviewCount*2+courses[i].FileCount, courses[j].ReviewCount*2+courses[j].FileCount
			if hi != hj { return hi > hj }
			return courses[i].Name < courses[j].Name
		})
	case "college": // 按院系
		sort.SliceStable(courses, func(i, j int) bool {
			if courses[i].College != courses[j].College { return courses[i].College < courses[j].College }
			return courses[i].Name < courses[j].Name
		})
	case "newest": // 按最新开课学期排序（学期分数大的在前）
		sort.SliceStable(courses, func(i, j int) bool {
			ti, tj := maxTermScore(courses[i].Terms), maxTermScore(courses[j].Terms)
			if ti != tj { return ti > tj }
			return courses[i].Name < courses[j].Name
		})
	default: // name: 按名称排序
		sort.SliceStable(courses, func(i, j int) bool {
			a, b := courses[i], courses[j]
			if a.Name != b.Name { return a.Name < b.Name }
			return a.College < b.College
		})
	}
}

// catMatch 判断课程分类路径 full 是否匹配用户选择的筛选 cat（支持一级/二级/三级）
func catMatch(full, cat string) bool {
	if full == cat { return true }
	// 前缀匹配（选了"必修"则匹配所有必修下的子类）
	if strings.HasPrefix(full, cat+"/") { return true }
	return false
}

// matchCat 课程类别映射（基于课表字段）
func matchCat(cat string, c Course) bool {
	switch cat {
	case "专业课":
		return c.Tag == "专业课"
	case "公共必修课":
		return c.Tag == "公共必修课"
	case "公共选修课":
		return c.Tag == "公共选修课"
	case "体育课", "体育":
		return c.BigType == "体育课" || c.Nature == "体育课" || strings.Contains(c.Name, "体育")
	case "实验课", "实验":
		return c.BigType == "实验课" || strings.Contains(c.Nature, "实验")
	case "实践", "实践环节":
		return c.BigType == "实习" || strings.Contains(c.Nature, "实践")
	case "美育课", "美育":
		return strings.Contains(c.GeneralCat, "美育") || strings.Contains(c.BigType, "美育")
	case "通选课", "通识":
		return c.BigType == "通选课" || c.GeneralCat != "" || c.Tag == "公共选修课"
	default:
		return true
	}
}

// courseDetailDTO 详情含班次
type courseDetailDTO struct {
	listCoursesDTO
	Terms     []string   `json:"terms"`
	Colleges  []string   `json:"colleges"`
	Offerings []Offering `json:"offerings"`
}

func handleCourseDetail(db *DB, code string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, ok := db.byCode[code]
		if !ok {
			apiJSON(w, 404, map[string]string{"error": "课程不存在"})
			return
		}
		// 收集该课程所有班次
		var offs []Offering
		seen := map[string]bool{}
		for _, o := range db.Offerings {
			if !codeset(o.CourseCode, c.Codes) { continue }
			// 去重（同老师同班次同时间）
			k := o.Term + "|" + o.Teacher + "|" + o.ClassGroup + "|" + o.Time
			if seen[k] { continue }
			seen[k] = true
			offs = append(offs, o)
		}
		apiJSON(w, 200, courseDetailDTO{
			listCoursesDTO: listCoursesDTO{
				Code: c.Code, Name: c.Name, College: c.College, Colleges: c.Colleges,
				Tag: c.Tag, Credit: c.Credit, Exam: c.ExamType, Nature: c.Nature,
				BigType: c.BigType, General: c.GeneralCat, Summary: c.FeaturedText,
				Majors: c.Majors,
			},
			Terms: c.Terms, Colleges: c.Colleges, Offerings: offs,
		})
	}
}

// handleTags 返回课程标签分类
func handleTags(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := map[string]bool{}
		for _, c := range db.Courses {
			if c.Tag != "" { m[c.Tag] = true }
		}
		out := make([]string, 0, len(m))
		for k := range m { out = append(out, k) }
		apiJSON(w, 200, map[string]any{"tags": out})
	}
}

// handleSemesters 返回所有开课学期（J4 高级检索下拉），按时间倒序
func handleSemesters(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := map[string]bool{}
		for _, o := range db.Offerings {
			if o.Term != "" { m[o.Term] = true }
		}
		out := make([]string, 0, len(m))
		for k := range m { out = append(out, k) }
		sort.Slice(out, func(i, j int) bool { return termScore(out[i]) > termScore(out[j]) })
		apiJSON(w, 200, map[string]any{"semesters": out})
	}
}

// handleStats 统计信息
func handleStats(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cs := map[string]bool{}
		for _, o := range db.Offerings { if o.CourseCode != "" { cs[o.CourseCode] = true } }
		apiJSON(w, 200, map[string]any{
			"courseCount":   len(db.Courses),
			"offeringCount": len(db.Offerings),
			"collegeCount":  len(db.CollegeSet),
		})
	}
}

// handleCourseCategories 返回课程新分类树（供前端高级检索下拉）
func handleCourseCategories() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apiJSON(w, 200, map[string]any{"categories": courseCategoryTree()})
	}
}