// admin_course.go — F5 课程后台管理：课程手动纠错(持久化覆盖) + CSV 导入
//
// 持久化方案：
//   course_overrides 表（key=课程名）存后台修正字段；启动时 loadAll 后 applyCourseOverrides 覆盖内存课程。
// 接口：
//   GET    /api/admin/courses?q=&page=       后台课程列表（含是否被覆盖）
//   GET    /api/admin/courses/{name}         单个课程当前值（含覆盖）
//   POST   /api/admin/courses/{name}/override 保存纠错 body:{field:value,...}（空值=还原该字段）
//   POST   /api/admin/courses/import         上传 csv（multipart file=xxx）到 CSV_DIR 并重载
package main

import (
	"database/sql"
	"sort"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ---- course_overrides 表 ----
func migrateCourseOverrides(db *sql.DB) {
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS course_overrides (
		course_name TEXT PRIMARY KEY,
		code        TEXT NOT NULL DEFAULT '',
		college     TEXT NOT NULL DEFAULT '',
		tag         TEXT NOT NULL DEFAULT '',
		credit      TEXT NOT NULL DEFAULT '',
		exam_type   TEXT NOT NULL DEFAULT '',
		big_type    TEXT NOT NULL DEFAULT '',
		nature      TEXT NOT NULL DEFAULT '',
		attribute   TEXT NOT NULL DEFAULT '',
		general_cat TEXT NOT NULL DEFAULT '',
		updated_at  TEXT NOT NULL DEFAULT ''
	)`)
}

// applyCourseOverrides 把 SQLite 里的纠错覆盖应用到内存课程（启动时调用）
func applyCourseOverrides(db *DB, sdb *sql.DB) {
	rows, err := sdb.Query(`SELECT course_name,code,college,tag,credit,exam_type,big_type,nature,attribute,general_cat FROM course_overrides`)
	if err != nil {
		return
	}
	defer rows.Close()
	type ov struct {
		name, code, college, tag, credit, exam, big, nature, attr, gen string
	}
	var list []ov
	for rows.Next() {
		o := ov{}
		_ = rows.Scan(&o.name, &o.code, &o.college, &o.tag, &o.credit, &o.exam, &o.big, &o.nature, &o.attr, &o.gen)
		list = append(list, o)
	}
	rows.Close() // ⚠️ SQLite 单连接：先关闭 rows

	for _, o := range list {
		c, ok := db.byKey[o.name]
		if !ok {
			continue
		}
		if o.code != "" {
			c.Code = o.code
		}
		if o.college != "" {
			c.College = o.college
		}
		if o.tag != "" {
			c.Tag = o.tag
		}
		if o.credit != "" {
			c.Credit = o.credit
		}
		if o.exam != "" {
			c.ExamType = o.exam
		}
		if o.big != "" {
			c.BigType = o.big
		}
		if o.nature != "" {
			c.Nature = o.nature
		}
		if o.attr != "" {
			c.Attribute = o.attr
		}
		if o.gen != "" {
			c.GeneralCat = o.gen
		}
	}
	// 修改的是 byKey 源数据，需重建 Courses 副本切片让前台查询生效
	db.rebuildCourses()
}

// ---- 后台：课程列表 ----
func handleAdminListCourses(db *DB, auth *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(auth, w, r) == 0 {
			return
		}
		// 路由分支（Go ServeMux 前缀匹配，这里手动分流）：
		//   /api/admin/courses/import              -> CSV 导入
		//   /api/admin/courses/{name}/override     -> 保存纠错
		//   /api/admin/courses/{name}              -> 单课详情
		p := strings.TrimPrefix(r.URL.Path, "/api/admin/courses/")
		if p == "import" && r.Method == http.MethodPost {
			handleAdminImportCSV(db, auth)(w, r)
			return
		}
		if strings.HasSuffix(p, "/override") {
			handleAdminSaveCourseOverride(db, auth)(w, r)
			return
		}
		if p != "" && p != r.URL.Path && !strings.Contains(p, "/") {
			handleAdminGetCourse(db, w, r, p)
			return
		}
		q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
		page := atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		pageSize := atoi(r.URL.Query().Get("pageSize"))
		if pageSize < 1 || pageSize > 200 {
			pageSize = 50
		}
		// 先过滤出全部匹配的课程（用于 total 与分页）
		type row struct {
			name, code, college, tag, credit, exam, big, nature, attr, gen string
		}
		var all []row
		for _, c := range db.Courses {
			if q != "" {
				hay := strings.ToLower(c.Name + " " + c.Code + " " + c.College + " " + c.Tag + " " + c.Nature + " " + c.GeneralCat)
				if !strings.Contains(hay, q) {
					continue
				}
			}
			all = append(all, row{c.Name, c.Code, c.College, c.Tag, c.Credit, c.ExamType, c.BigType, c.Nature, c.Attribute, c.GeneralCat})
		}
		total := len(all)
		// 分页切片
		start := (page - 1) * pageSize
		if start > total {
			start = total
		}
		end := start + pageSize
		if end > total {
			end = total
		}
		out := make([]map[string]any, 0, end-start)
		for _, c := range all[start:end] {
			out = append(out, map[string]any{
				"name": c.name, "code": c.code, "college": c.college,
				"tag": c.tag, "credit": c.credit, "examType": c.exam,
				"bigType": c.big, "nature": c.nature,
				"attribute": c.attr, "generalCat": c.gen,
				"overridden": courseOverridden(auth.db, c.name),
			})
		}
		apiJSON(w, 200, map[string]any{"total": total, "page": page, "pageSize": pageSize, "courses": out})
	}
}

// courseOverridden 判断课程是否已有覆盖记录
func courseOverridden(sdb *sql.DB, name string) bool {
	var cnt int
	_ = sdb.QueryRow(`SELECT COUNT(*) FROM course_overrides WHERE course_name=?`, name).Scan(&cnt)
	return cnt > 0
}

// ---- 后台：单个课程（含覆盖）----
func handleAdminGetCourse(db *DB, w http.ResponseWriter, r *http.Request, name string) {
	c, ok := db.byKey[name]
	if !ok {
		apiErr(w, 404, "课程不存在")
		return
	}
	apiJSON(w, 200, map[string]any{"course": map[string]any{
		"name": c.Name, "code": c.Code, "college": c.College,
		"tag": c.Tag, "credit": c.Credit, "examType": c.ExamType,
		"bigType": c.BigType, "nature": c.Nature,
		"attribute": c.Attribute, "generalCat": c.GeneralCat,
		"overridden": courseOverridden(authStore.db, c.Name),
	}})
}

// ---- 后台：保存纠错 ----
func handleAdminSaveCourseOverride(db *DB, auth *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(auth, w, r) == 0 {
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/api/admin/courses/")
		name := strings.TrimSuffix(p, "/override")
		if name == "" {
			apiErr(w, 400, "缺少课程名")
			return
		}
		c, ok := db.byKey[name]
		if !ok {
			apiErr(w, 404, "课程不存在")
			return
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiErr(w, 400, "请求格式错误")
			return
		}

		// 空值 = 还原该字段（删除覆盖里的对应字段值）
		setOrClear := func(field string, v string, cur *string) {
			if vv, has := body[field]; has {
				if vv != "" {
					*cur = vv
				}
			}
		}
		setOrClear("code", body["code"], &c.Code)
		setOrClear("college", body["college"], &c.College)
		setOrClear("tag", body["tag"], &c.Tag)
		setOrClear("credit", body["credit"], &c.Credit)
		setOrClear("examType", body["examType"], &c.ExamType)
		setOrClear("bigType", body["bigType"], &c.BigType)
		setOrClear("nature", body["nature"], &c.Nature)
		setOrClear("attribute", body["attribute"], &c.Attribute)
		setOrClear("generalCat", body["generalCat"], &c.GeneralCat)

		// 改名支持：body.name 若存在且与当前 name 不同，则迁移 byKey 映射与覆盖记录
		newName := strings.TrimSpace(body["name"])
		if newName != "" && newName != name {
			// 检查新名字是否已被占用
			if _, exists := db.byKey[newName]; exists {
				apiErr(w, 400, "已存在同名课程: "+newName)
				return
			}
			// 迁移 byKey 映射
			delete(db.byKey, name)
			c.Name = newName
			db.byKey[newName] = c
			// 迁移覆盖记录主键
			var cnt int
			_ = auth.db.QueryRow(`SELECT COUNT(*) FROM course_overrides WHERE course_name=?`, name).Scan(&cnt)
			if cnt > 0 {
				_, _ = auth.db.Exec(`UPDATE course_overrides SET course_name=? WHERE course_name=?`, newName, name)
			}
			name = newName
		}

		// 若所有字段都空 → 删除覆盖；否则 upsert
		allEmpty := true
		for _, v := range body {
			if v != "" {
				allEmpty = false
				break
			}
		}
		if allEmpty {
			_, _ = auth.db.Exec(`DELETE FROM course_overrides WHERE course_name=?`, name)
		} else {
			_, _ = auth.db.Exec(`INSERT INTO course_overrides (course_name,code,college,tag,credit,exam_type,big_type,nature,attribute,general_cat,updated_at)
				VALUES (?,?,?,?,?,?,?,?,?,?,?)
				ON CONFLICT(course_name) DO UPDATE SET
					code=excluded.code,college=excluded.college,tag=excluded.tag,
					credit=excluded.credit,exam_type=excluded.exam_type,big_type=excluded.big_type,
					nature=excluded.nature,attribute=excluded.attribute,general_cat=excluded.general_cat,
					updated_at=excluded.updated_at`,
				name, c.Code, c.College, c.Tag, c.Credit, c.ExamType, c.BigType,
				c.Nature, c.Attribute, c.GeneralCat, nowStr())
		}
		// db.Courses 是 rebuildCourses 生成的副本切片，需重建以反映 byKey 的最新值
		db.rebuildCourses()
		logAudit(auth.db, currentUserID(r), "编辑课程", "课程="+name)
		apiJSON(w, 200, map[string]any{"ok": true, "name": name})
	}
}

// ---- 后台：CSV 导入（上传到 CSV_DIR 并重载）----
func handleAdminImportCSV(db *DB, auth *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(auth, w, r) == 0 {
			return
		}
		r.ParseMultipartForm(32 << 20) // 32MB
		file, _, err := r.FormFile("file")
		if err != nil {
			apiErr(w, 400, "缺少文件字段 file")
			return
		}
		defer file.Close()

		csvDir := os.Getenv("CSV_DIR")
		if csvDir == "" {
			csvDir = filepath.Join("..", "_csv")
		}
		if st, err := os.Stat(csvDir); err != nil || !st.IsDir() {
			apiErr(w, 500, "CSV 目录不可用: "+csvDir)
			return
		}
		dst := filepath.Join(csvDir, "import_"+nowFileTag()+".csv")
		out, err := os.Create(dst)
		if err != nil {
			apiErr(w, 500, "无法写入 CSV 目录")
			return
		}
		_, err = ioCopy(out, file)
		out.Close()
		if err != nil {
			apiErr(w, 500, "文件写入失败")
			return
		}
		// 重载课表
		if err := db.loadAll(csvDir); err != nil {
			apiErr(w, 500, "导入失败: "+err.Error())
			return
		}
		apiJSON(w, 200, map[string]any{"ok": true, "saved": dst, "courses": len(db.Courses)})
	}
}

// ---- 工具 ----
func nowStr() string { return timeNow() }
func nowFileTag() string { return fileTag() }

func timeNow() string { return time.Now().Format("2006-01-02 15:04:05") }
func fileTag() string { return time.Now().Format("20060102_150405") }
func ioCopy(dst io.Writer, src io.Reader) (int64, error) { return io.Copy(dst, src) }


// ---- 后台：开课班次列表 ----
// GET /api/admin/offerings?q=&code=&limit=  按课程名/编号/老师搜索班次
func handleAdminListOfferings(db *DB, auth *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(auth, w, r) == 0 {
			return
		}
		q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
		page := atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		pageSize := atoi(r.URL.Query().Get("pageSize"))
		if pageSize < 1 || pageSize > 500 {
			pageSize = 100
		}
		// 先过滤出全部匹配（用于 total + 分页）
		type row struct {
			code, term, college, teacher, class, timev, loc string
		}
		var all []row
		for _, o := range db.Offerings {
			if q != "" {
				hay := strings.ToLower(o.CourseCode + " " + o.Teacher + " " + o.ClassGroup + " " + o.College)
				if !strings.Contains(hay, q) {
					continue
				}
			}
			all = append(all, row{o.CourseCode, o.Term, o.College, o.Teacher, o.ClassGroup, o.Time, o.Location})
		}
		total := len(all)
		// 按时间排序：学期从新到旧（主），同学期再按星期+节次（次）
		sort.SliceStable(all, func(i, j int) bool {
			ti, tj := termScore(all[i].term), termScore(all[j].term)
			if ti != tj {
				return ti > tj // 学期新→旧
			}
			return offeringTimeScore(all[i].timev) < offeringTimeScore(all[j].timev)
		})
		start := (page - 1) * pageSize
		if start > total {
			start = total
		}
		end := start + pageSize
		if end > total {
			end = total
		}
		out := make([]map[string]any, 0, end-start)
		for _, o := range all[start:end] {
			out = append(out, map[string]any{
				"courseCode": o.code, "term": o.term, "college": o.college,
				"teacher": o.teacher, "classGroup": o.class,
				"time": o.timev, "location": o.loc,
			})
		}
		apiJSON(w, 200, map[string]any{"total": total, "page": page, "pageSize": pageSize, "offerings": out})
	}
}

// ---- 后台：学院列表（覆盖学院）----
// GET /api/admin/colleges  返回全部开课院系（按名称排序），附该院系课程数
func handleAdminListColleges(db *DB, auth *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(auth, w, r) == 0 {
			return
		}
		// 以官方 25 学院（orgColleges）为主，补充课表课程数
		names := make([]string, 0, len(orgColleges))
		names = append(names, orgColleges...)
		sortStrings(names)
		out := make([]map[string]any, 0, len(names))
		for _, n := range names {
			cnt := 0
			for _, c := range db.Courses {
				if c.College == n {
					cnt++
				}
			}
			disp, sched, cat, majors := collegeAdmin(auth.db, n)
			out = append(out, map[string]any{
				"name": n, "courseCount": cnt,
				"displayName": disp, "scheduleName": sched, "category": cat, "majors": majors,
			})
		}
		apiJSON(w, 200, map[string]any{"total": len(out), "colleges": out})
	}
}

// sortStrings 简单字符串排序（避免依赖 sort 包时遗漏导入）
func sortStrings(sl []string) {
	for i := 1; i < len(sl); i++ {
		for j := i; j > 0 && sl[j] < sl[j-1]; j-- {
			sl[j], sl[j-1] = sl[j-1], sl[j]
		}
	}
}


// ---- 班次管理：持久化（offering_overrides / offering_deletes）----
func migrateOfferingTables(db *sql.DB) {
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS offering_overrides (
		code        TEXT NOT NULL,
		term        TEXT NOT NULL,
		class_group TEXT NOT NULL,
		teacher     TEXT NOT NULL DEFAULT '',
		time        TEXT NOT NULL DEFAULT '',
		location    TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (code, term, class_group)
	)`)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS offering_deletes (
		code        TEXT NOT NULL,
		term        TEXT NOT NULL,
		class_group TEXT NOT NULL,
		PRIMARY KEY (code, term, class_group)
	)`)
}

// applyOfferingOverrides 启动时应用班次覆盖与删除到内存 db.Offerings
func applyOfferingOverrides(db *DB, sdb *sql.DB) {
	// 收集覆盖
	type ov struct {
		code, term, class, teacher, timev, loc string
	}
	rows, err := sdb.Query(`SELECT code,term,class_group,teacher,time,location FROM offering_overrides`)
	if err == nil {
		var list []ov
		for rows.Next() {
			o := ov{}
			_ = rows.Scan(&o.code, &o.term, &o.class, &o.teacher, &o.timev, &o.loc)
			list = append(list, o)
		}
		rows.Close()
		// 应用覆盖
		for i := range db.Offerings {
			for _, o := range list {
				if db.Offerings[i].CourseCode == o.code && db.Offerings[i].Term == o.term && db.Offerings[i].ClassGroup == o.class {
					if o.teacher != "" {
						db.Offerings[i].Teacher = o.teacher
					}
					if o.timev != "" {
						db.Offerings[i].Time = o.timev
					}
					if o.loc != "" {
						db.Offerings[i].Location = o.loc
					}
				}
			}
		}
	}
	// 收集删除标记
	deleted := map[[3]string]bool{}
	rows2, err := sdb.Query(`SELECT code,term,class_group FROM offering_deletes`)
	if err == nil {
		for rows2.Next() {
			var c, t, g string
			_ = rows2.Scan(&c, &t, &g)
			deleted[[3]string{c, t, g}] = true
		}
		rows2.Close()
	}
	// 应用删除
	out := db.Offerings[:0]
	for _, o := range db.Offerings {
		if !deleted[[3]string{o.CourseCode, o.Term, o.ClassGroup}] {
			out = append(out, o)
		}
	}
	db.Offerings = out
}

// ---- 后台：编辑班次 ----
// POST /api/admin/offerings/edit  body:{code,term,classGroup,teacher,time,location}
func handleAdminEditOffering(db *DB, auth *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(auth, w, r) == 0 {
			return
		}
		var body struct {
			Code       string `json:"code"`
			Term       string `json:"term"`
			ClassGroup string `json:"classGroup"`
			Teacher    string `json:"teacher"`
			Time       string `json:"time"`
			Location   string `json:"location"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiErr(w, 400, "请求格式错误")
			return
		}
		if body.Code == "" || body.Term == "" || body.ClassGroup == "" {
			apiErr(w, 400, "缺少 code/term/classGroup")
			return
		}
		// 写持久化覆盖
		_, _ = auth.db.Exec(`INSERT INTO offering_overrides (code,term,class_group,teacher,time,location) VALUES (?,?,?,?,?,?)
			ON CONFLICT(code,term,class_group) DO UPDATE SET teacher=excluded.teacher, time=excluded.time, location=excluded.location`,
			body.Code, body.Term, body.ClassGroup, body.Teacher, body.Time, body.Location)
		// 应用到内存
		for i := range db.Offerings {
			o := &db.Offerings[i]
			if o.CourseCode == body.Code && o.Term == body.Term && o.ClassGroup == body.ClassGroup {
				if body.Teacher != "" {
					o.Teacher = body.Teacher
				}
				if body.Time != "" {
					o.Time = body.Time
				}
				if body.Location != "" {
					o.Location = body.Location
				}
			}
		}
		logAudit(auth.db, currentUserID(r), "编辑班次", "编号="+body.Code+" 学期="+body.Term+" 班级="+body.ClassGroup)
		apiJSON(w, 200, map[string]any{"ok": true})
	}
}

// ---- 后台：删除班次 ----
// POST /api/admin/offerings/delete  body:{code,term,classGroup}
func handleAdminDeleteOffering(db *DB, auth *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(auth, w, r) == 0 {
			return
		}
		var body struct {
			Code       string `json:"code"`
			Term       string `json:"term"`
			ClassGroup string `json:"classGroup"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiErr(w, 400, "请求格式错误")
			return
		}
		_, _ = auth.db.Exec(`INSERT OR IGNORE INTO offering_deletes (code,term,class_group) VALUES (?,?,?)`,
			body.Code, body.Term, body.ClassGroup)
		// 从内存删除
		out := db.Offerings[:0]
		for _, o := range db.Offerings {
			if !(o.CourseCode == body.Code && o.Term == body.Term && o.ClassGroup == body.ClassGroup) {
				out = append(out, o)
			}
		}
		db.Offerings = out
		apiJSON(w, 200, map[string]any{"ok": true})
	}
}


// ---- 学院管理（colleges_admin 表：展示名/课表名/专业缩写）----
func migrateCollegesAdmin(db *sql.DB) {
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS colleges_admin (
		name          TEXT PRIMARY KEY,
		display_name  TEXT NOT NULL DEFAULT '',
		schedule_name TEXT NOT NULL DEFAULT '',
		category      TEXT NOT NULL DEFAULT '',
		majors        TEXT NOT NULL DEFAULT '',
		updated_at    TEXT NOT NULL DEFAULT ''
	)`)
	// 兼容旧库：若缺 category 列则补上
	var hasCat int
	_ = db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('colleges_admin') WHERE name='category'`).Scan(&hasCat)
	if hasCat == 0 {
		_, _ = db.Exec(`ALTER TABLE colleges_admin ADD COLUMN category TEXT NOT NULL DEFAULT ''`)
	}
	// 种子：官方 25 学院（幂等——仅当记录不存在或 majors 为空时填充，避免覆盖管理员自定义）
	now := nowStr()
	for name, majors := range orgCollegeData {
		var cnt int
		var curMajors string
		_ = db.QueryRow(`SELECT COUNT(*), COALESCE(majors,'') FROM colleges_admin WHERE name=?`, name).Scan(&cnt, &curMajors)
		if cnt == 0 {
			_, _ = db.Exec(`INSERT INTO colleges_admin (name,display_name,schedule_name,category,majors,updated_at) VALUES (?,?,?,?,?,?)`,
				name, name, "", collegeCategory(name), strings.Join(majors, ","), now)
		} else {
			// majors 已有：仅回填空 category（不覆盖管理员自定义的专业）
			var curCat string
			_ = db.QueryRow(`SELECT COALESCE(category,'') FROM colleges_admin WHERE name=?`, name).Scan(&curCat)
			if strings.TrimSpace(curCat) == "" {
				_, _ = db.Exec(`UPDATE colleges_admin SET category=?, updated_at=? WHERE name=?`,
					collegeCategory(name), now, name)
			}
		}
	}
}

// collegeCategory 官方学科类别（来源：青岛大学专业统计.md）
func collegeCategory(name string) string {
	switch name {
	case "数学与统计学院", "物理科学学院", "化学化工学院", "生命科学学院", "机电工程学院",
		"材料科学与工程学院", "自动化学院", "电气工程学院", "电子信息学院", "计算机科学技术学院",
		"环境与地理科学学院", "纺织服装学院":
		return "理工类"
	case "青岛医学院":
		return "医学类"
	case "马克思主义学院", "历史学院", "经济学院", "法学院", "政治与公共管理学院",
		"教育科学学院", "体育学院", "文学与新闻传播学院", "外语学院", "商学院":
		return "人文社科类"
	case "艺术学院":
		return "艺术类"
	case "青岛大学德雷克联合学院":
		return "中外合作办学"
	}
	return ""
}
// ---- 后台：编辑学院 ----
// POST /api/admin/college  body:{name, displayName, scheduleName, majors}
func handleAdminEditCollege(auth *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(auth, w, r) == 0 {
			return
		}
		var body struct {
			Name         string `json:"name"`
			DisplayName  string `json:"displayName"`
			ScheduleName string `json:"scheduleName"`
			Category     string `json:"category"`
			Majors       string `json:"majors"` // 逗号分隔：专业(缩写)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiErr(w, 400, "请求格式错误")
			return
		}
		if body.Name == "" {
			apiErr(w, 400, "缺少学院名")
			return
		}
		if body.Category == "" {
			body.Category = collegeCategory(body.Name)
		}
		_, _ = auth.db.Exec(`INSERT INTO colleges_admin (name,display_name,schedule_name,category,majors,updated_at) VALUES (?,?,?,?,?,?)
			ON CONFLICT(name) DO UPDATE SET display_name=excluded.display_name, schedule_name=excluded.schedule_name, category=excluded.category, majors=excluded.majors, updated_at=excluded.updated_at`,
			body.Name, body.DisplayName, body.ScheduleName, body.Category, body.Majors, nowStr())
		logAudit(auth.db, currentUserID(r), "编辑学院", "学院="+body.Name)
		apiJSON(w, 200, map[string]any{"ok": true})
	}
}

// collegeAdmin 获取学院的 admin 补充信息（无则返回空结构）
func collegeAdmin(sdb *sql.DB, name string) (display, schedule, category, majors string) {
	_ = sdb.QueryRow(`SELECT display_name,schedule_name,category,majors FROM colleges_admin WHERE name=?`, name).
		Scan(&display, &schedule, &category, &majors)
	return
}


// offeringTimeScore 把班次 Time 时间描述解析成排序分数（星期最大权重，其次是节次）
// 例："周一第5、6节..." -> weekday=1, period=5
func offeringTimeScore(t string) int {
	week := 0
	if strings.Contains(t, "周一") { week = 1 }
	if strings.Contains(t, "周二") { week = 2 }
	if strings.Contains(t, "周三") { week = 3 }
	if strings.Contains(t, "周四") { week = 4 }
	if strings.Contains(t, "周五") { week = 5 }
	if strings.Contains(t, "周六") { week = 6 }
	if strings.Contains(t, "周日") { week = 7 }
	period := 0
	for _, p := range []string{"第1、2节", "第1,2节", "第1、2", "第1,2"} {
		if strings.Contains(t, p) { period = 1; break }
	}
	if period == 0 {
		for _, p := range []string{"第3、4节", "第3,4节", "第3、4", "第3,4"} {
			if strings.Contains(t, p) { period = 2; break }
		}
	}
	if period == 0 {
		for _, p := range []string{"第5、6节", "第5,6节", "第5、6", "第5,6"} {
			if strings.Contains(t, p) { period = 3; break }
		}
	}
	if period == 0 {
		for _, p := range []string{"第7、8节", "第7,8节", "第7、8", "第7,8"} {
			if strings.Contains(t, p) { period = 4; break }
		}
	}
	if period == 0 {
		for _, p := range []string{"第9、10节", "第9,10节", "第9、10", "第9,10"} {
			if strings.Contains(t, p) { period = 5; break }
		}
	}
	return week*100 + period
}

// termScore 解析学期字符串为排序分数（越大越新），如 "2026春"->326, "2023秋"->223
func termScore(term string) int {
	var year int
	for _, r := range term {
		if r < '0' || r > '9' { break }
		year = year*10 + int(r-'0')
	}
	score := year * 10
	if strings.Contains(term, "秋") { score += 2 }
	if strings.Contains(term, "夏") { score += 1 }
	// 春 = +0
	return score
}
