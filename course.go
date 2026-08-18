// course.go — 课表导入与课程模型 (V1)
// 负责：解析三份课程总表 CSV → 生成两类数据：
//   courses  课程档案（按课程编号去重，一份稳定记录）
//   offerings 开课班次（每个班级/老师/时间的排课记录）
package main

import (
	"encoding/csv"
	"os"
	"sort"
	"strings"
)

// Course 一门课程的档案（按 课程名+开课院系 合并去重）
type Course struct {
	Code          string            // 主课程编号（取第一个）
	Codes         []string          // 合并进来的所有编号
	Name          string            // 课程名称
	College       string            // 开课院系（班次最多的主开课院系）
	Colleges      []string          // 上课院系（给哪些学院学生上课，去重）
	Majors        []string          // 上课专业（从班次班级名推导，如"微纳(大一)"）
	CollegeCount  map[string]int    // 各开课院系班次数（用于取主开课院系）
	Tag           string            // 课程标签：专业课/公共选修课/公共必修课
	Credit        string            // 学分
	ExamType      string            // 考核方式：考试/考查
	BigType       string            // 课程大类：普通课/通选课/实验课/体育课/实习
	Nature        string            // 课程性质：课程/实践环节/美育课/毕业论文
	Attribute     string            // 课程属性：必修/任选/限选
	GeneralCat    string            // 通选课类别
	FeaturedText  string            // 展示用摘要
	Terms         []string          // 开课过的学期
}

// Offering 一个开课班次（一门课某学期某老师某班级）
type Offering struct {
	CourseCode    string `json:"courseCode"`
	Term          string `json:"term"`
	College       string `json:"college"`        // 开课院系（谁开的课）
	EnrollCollege string `json:"enrollCollege"` // 上课院系（给哪些学院的学生上）
	Teacher       string `json:"teacher"`
	ClassGroup    string `json:"classGroup"`
	Time          string `json:"time"`
	Location      string `json:"location"`
}

// DB 简单内存库（V1 用，后续迁移到 MySQL）
type DB struct {
	Courses   []Course            // 合并去重后的课程
	byKey     map[string]*Course  // key = 课程名|开课院系
	byCode    map[string]*Course  // 编号 -> 归属课程（详情跳转）
	Offerings []Offering
	CollegeSet map[string]bool
}

// NewDB 创建一个空库并初始化映射
func NewDB() *DB {
	return &DB{
		byKey:  map[string]*Course{},
		byCode: map[string]*Course{},
		CollegeSet: map[string]bool{},
	}
}

// importCSV 解析一份课表 CSV，并入库
// 表头在第3行（前两行是标题/制表说明）
func (db *DB) importCSV(path, term string) error {
	f, err := os.Open(path)
	if err != nil { return err }
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1 // 允许可变列

	records, err := r.ReadAll()
	if err != nil { return err }

	// 找到表头行（含"通知单号"）
	headIdx := -1
	for i, rec := range records {
		if len(rec) > 0 && strings.Contains(rec[0], "通知单号") {
			headIdx = i
			break
		}
	}
	if headIdx < 0 { return nil }

	header := records[headIdx]
	col := func(name string) int {
		for i, h := range header {
			if strings.TrimSpace(h) == name { return i }
		}
		return -1
	}
	// 需要的列索引
	ciCode := col("课程编号")
	ciName := col("课程名称")
	ciTag := col("课程标签")
	ciCredit := col("学分")
	ciExam := col("考核方式")
	ciBig := col("课程大类")
	ciNature := col("课程性质")
	ciAttr := col("课程属性")
	ciGen := col("通选课类别")
	ciCollege := col("开课院系")
	ciEnroll := col("上课院系")
	ciTeacher := col("教师姓名")
	ciClass := col("上课班级")
	ciTime := col("上课时间")
	ciLoc := col("上课地点")

	// 遍历表头之后的每一行（一个班次）
	for i := headIdx + 1; i < len(records); i++ {
		rec := records[i]
		if len(rec) == 0 || strings.TrimSpace(rec[0]) == "" { continue }
		get := func(idx int) string {
			if idx >= 0 && idx < len(rec) { return strings.TrimSpace(rec[idx]) }
			return ""
		}
		code := get(ciCode)
		if code == "" { continue }
		name := get(ciName)
		college := get(ciCollege)

		// 记录开课院系集合
		if college != "" { db.CollegeSet[college] = true }

		// 合并键：仅"课程名"（同名即同一门课，合并所有老师/院系的班次）
		// 注：学校编号是跟"开设课次"走的；内容相同的课名字一样，应合并为1门。
		//     名字带序号(如"C语言(1)/C语言(二)")则本身不同名，自然分成不同课。
		key := name
		c, ok := db.byKey[key]
		if !ok {
			c = &Course{
				Name: name,
				Credit: get(ciCredit), ExamType: get(ciExam),
				BigType: get(ciBig), Nature: get(ciNature),
				Attribute: get(ciAttr), GeneralCat: get(ciGen),
			}
			db.byKey[key] = c
		}
		// 开课院系计数（重建时取班次最多的作主开课院系）
		if college != "" {
			if c.CollegeCount == nil { c.CollegeCount = map[string]int{} }
			c.CollegeCount[college]++
		}
		// 上课院系（给哪些学院的学生上）去重收集（源数据可能用逗号分隔多个院系）
		if enroll := get(ciEnroll); enroll != "" {
			for _, one := range splitComma(enroll) {
				if !containsStr(c.Colleges, one) { c.Colleges = append(c.Colleges, one) }
			}
		}
		// 补充字段（以第一个非空为准）
		if c.Tag == "" { c.Tag = get(ciTag) }
		if c.Credit == "" { c.Credit = get(ciCredit) }
		if c.ExamType == "" { c.ExamType = get(ciExam) }
		if c.BigType == "" { c.BigType = get(ciBig) }
		if c.Nature == "" { c.Nature = get(ciNature) }
		if c.Attribute == "" { c.Attribute = get(ciAttr) }
		if c.GeneralCat == "" { c.GeneralCat = get(ciGen) }
		// 记录编号（第一个作主编号）+ 编号映射
		if !containsStr(c.Codes, code) { c.Codes = append(c.Codes, code); if c.Code == "" { c.Code = code } }
		db.byCode[code] = c
		// 学期
		if !containsStr(c.Terms, term) { c.Terms = append(c.Terms, term) }

		// 2) 记录班次（含上课院系）
		db.Offerings = append(db.Offerings, Offering{
			CourseCode: code, Term: term,
			College: college, EnrollCollege: get(ciEnroll),
			Teacher: get(ciTeacher),
			ClassGroup: get(ciClass), Time: get(ciTime), Location: get(ciLoc),
		})
	}
	return nil
}

// rebuildCourses 把映射整理成稳定的 Courses 切片（含排序）
func (db *DB) rebuildCourses() {
	db.Courses = db.Courses[:0]
	for _, c := range db.byKey {
		c.FeaturedText = firstNonEmpty(c.Tag, c.BigType, c.GeneralCat, c.ExamType)
		// 主开课院系 = 班次最多的开课院系
		c.College = ""
		max := 0
		for clg, n := range c.CollegeCount {
			if n > max { max = n; c.College = clg }
		}
		// 是否全校可选的课（通选/公共选修/公共必修 或 班次含"临班"）：上课专业显示"全校"
		isGeneral := c.Tag == "公共选修课" || c.Tag == "公共必修课" || c.BigType == "通选课" || c.GeneralCat != ""
		// 临班（面向全校各专业临时班级）也视为全校
		if hasLinban(db, c.Codes) {
			isGeneral = true
		}
		c.Majors = nil
		if isGeneral {
			c.Majors = []string{"全校"}
		} else {
			seen := map[string]bool{}
			for _, o := range db.Offerings {
				if !codeset(o.CourseCode, c.Codes) { continue }
				for _, m := range majorsFromClass(o.ClassGroup, o.Term) {
					if !seen[m] { seen[m] = true; c.Majors = append(c.Majors, m) }
				}
			}
			// 专业数过多（明显面向全校多专业）→ 折叠为"全校"
			if len(c.Majors) >= 6 {
				c.Majors = []string{"全校"}
			}
		}
		db.Courses = append(db.Courses, *c)
	}
	// 按名称排序（先名后学院，同名课程排在相邻，靠学院区分）
	sort.Slice(db.Courses, func(i, j int) bool {
		a, b := db.Courses[i], db.Courses[j]
		if a.Name != b.Name { return a.Name < b.Name }
		return a.College < b.College
	})
}

// loadAll 加载全部历年课表（8 份：2023春夏~2026春）
func (db *DB) loadAll(csvDir string) error {
	files := []struct{ path, term string }{
		{csvDir + "/2023春_utf8.csv", "2023春"},
		{csvDir + "/2023秋_utf8.csv", "2023秋"},
		{csvDir + "/2024春_utf8.csv", "2024春"},
		{csvDir + "/2024夏_utf8.csv", "2024夏"},
		{csvDir + "/2024秋_utf8.csv", "2024秋"},
		{csvDir + "/2025spring_utf8.csv", "2025春"},
		{csvDir + "/2025autumn_utf8.csv", "2025秋"},
		{csvDir + "/2026spring_utf8.csv", "2026春"},
	}
	for _, f := range files {
		if err := db.importCSV(f.path, f.term); err != nil {
			return err
		}
	}
	db.rebuildCourses()
	return nil
}

// ------- 小工具 -------
func containsStr(sl []string, s string) bool {
	for _, v := range sl { if v == s { return true } }
	return false
}
func firstNonEmpty(vals ...string) string {
	for _, v := range vals { if v != "" { return v } }
	return ""
}

// splitComma 把 CPU/全角逗号分隔的串拆成多个去空白的字段
func splitComma(s string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '，' }) {
		t := strings.TrimSpace(part)
		if t != "" { out = append(out, t) }
	}
	return out
}

// codeset 判断 code 是否在 codes 列表中
func codeset(code string, codes []string) bool { return containsStr(codes, code) }

// majorsFromClass 从班级名 + 学期 推导"专业(中文年级)"
// 例：term=2026春, 25机械03班 -> "机械(大一)"
// 中文年级规则：学年初(秋/次年春)年份 Y，入学年份 G 的学生为 第(Y-G+1) 年。第1年=大一、2=大二、3=大三、4=大四、>4=研/更高。
func majorsFromClass(class, term string) []string {
	var out []string
	// 从学期推断学年起始年
	academicYear := 0
	termYear := 0
	if len(term) >= 4 && term[:4] == "2026" { termYear = 2026 }
	if len(term) >= 4 && term[:4] == "2025" { termYear = 2025 }
	if len(term) >= 4 && term[:4] == "2024" { termYear = 2024 }
	if len(term) >= 4 && term[:4] == "2023" { termYear = 2023 }
	season := "春"
	if strings.HasSuffix(term, "秋") { season = "秋" } else if strings.HasSuffix(term, "夏") { season = "夏" }
	if termYear != 0 {
		if season == "春" { academicYear = termYear - 1 } else { academicYear = termYear }
	}

	for _, one := range splitComma(class) {
		s := strings.TrimSpace(one)
		if s == "" || !strings.HasSuffix(s, "班") { continue }
		body := strings.TrimSuffix(s, "班")
		// 提取前2位年级数字（入学年份末两位）
		gradeNum := ""
		for i := 0; i < len(body) && i < 2; i++ {
			if body[i] >= '0' && body[i] <= '9' { gradeNum += body[i:i+1] } else { gradeNum = ""; break }
		}
		noGrade := body
		if gradeNum != "" { noGrade = strings.TrimPrefix(body, gradeNum) }
		// 专业 = 到第一个 '[' 或数字前的部分
		major := noGrade
		for i := 0; i < len(noGrade); i++ {
			ch := noGrade[i]
			if ch == '[' || (ch >= '0' && ch <= '9') { major = noGrade[:i]; break }
		}
		major = strings.TrimSpace(major)
		if major == "" { continue }

		// 计算中文年级
		if academicYear != 0 && len(gradeNum) == 2 {
			enroll := 2000 + atoi(gradeNum)
			year := academicYear - enroll + 1
			label := gradeLabel(year)
			out = append(out, major+"("+label+")")
		} else {
			out = append(out, major)
		}
	}
	return out
}

// atoi 简易转数字
func atoi(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' { return n }
		n = n*10 + int(s[i]-'0')
	}
	return n
}

// gradeLabel 数字年级 -> 中文
func gradeLabel(y int) string {
	switch {
	case y <= 1: return "大一"
	case y == 2: return "大二"
	case y == 3: return "大三"
	case y == 4: return "大四"
	default:     return "高年级"
	}
}


// fixNameCodeSwap 修正课表导入时的 name/code 错位（通用清洗）。
// 现象：某课程 name 是纯编号（如 4101130402265），而 code 是中文课程名（如 透视与构图）。
// 判定：name 全是数字（或数字+字母编号形态）且 code 含中文字符，则交换二者。
func fixNameCodeSwap(db *DB) {
	for key, c := range db.byKey {
		if isNumericName(c.Name) && containsChinese(c.Code) {
			oldName, oldCode := c.Name, c.Code
			c.Name, c.Code = oldCode, oldName
			// 重建 byKey 映射
			delete(db.byKey, key)
			db.byKey[c.Name] = c
			// byCode 映射也更新
			if old, ok := db.byCode[oldCode]; ok && old == c {
				delete(db.byCode, oldCode)
			}
			db.byCode[oldCode] = c
		}
	}
}

// isNumericName 判断字符串是否"编号形态"（主要由数字组成，或含连字符/字母的编号）。
func isNumericName(s string) bool {
	if s == "" {
		return false
	}
	hasDigit := false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			hasDigit = true
			continue
		}
		// 编号常见分隔符
		if r == '-' || r == '_' || r == '.' || r == '/' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			continue
		}
		return false // 遇到中文字符等 → 不是编号形态
	}
	return hasDigit
}

// containsChinese 判断字符串是否包含中文字符
func containsChinese(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}

// hasLinban 判断课程的班次班级名里是否含"临班"（面向全校各专业的临时班级）
func hasLinban(db *DB, codes []string) bool {
	for _, o := range db.Offerings {
		if codeset(o.CourseCode, codes) && strings.Contains(o.ClassGroup, "临班") {
			return true
		}
	}
	return false
}
