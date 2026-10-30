// category.go — 课程分类体系（按用户要求重构 v2）
// 一级：必修 / 选修
//   必修：通识教育必修课 / 大类专业必修课 / 专业基础课 / 专业核心课 / 集中实践 / 实验课
//   选修：通识教育选修课(核心课/普通课/美育课) / 体育课
// 说明（用户调整）：
//   - 课内实验与独立实验课合并为"实验课"，归入必修。
//   - "体育课"仅指 体育Ⅰ/体育Ⅱ 这类公共体育课（名称=体育+罗马数字/数字），
//     其他含"体育"但实为专业课/选修课的（如体育社会学、运动解剖学）不归体育。
// 无课表课程：由后台手动补充（不在课表内，如网课/军训等）
package main

import "strings"

// isPESportCourse 判断是否公共体育课：名称以"体育"开头且紧跟 Ⅰ/Ⅱ/Ⅲ/1/2 等编号
func isPESportCourse(name string) bool {
	n := strings.TrimSpace(name)
	if !strings.HasPrefix(n, "体育") {
		return false
	}
	rest := strings.TrimPrefix(n, "体育")
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return false
	}
	// 编号：罗马数字 ⅠⅡⅢⅣⅤ 或 阿拉伯数字
	roman := []rune("ⅠⅡⅢⅣⅤⅥⅦⅧⅨⅩ")
	for _, r := range roman {
		if []rune(rest)[0] == r {
			return true
		}
	}
	if rest[0] >= '0' && rest[0] <= '9' {
		return true
	}
	// 也接受 "体育（一）" 这类
	if strings.HasPrefix(rest, "（") || strings.HasPrefix(rest, "(") {
		return true
	}
	return false
}

// courseCategory 返回课程的新分类路径（一级/二级/三级，用 / 分隔）
func courseCategory(c Course) string {
	name := c.Name
	// 集中实践优先（实习/实践环节/毕业论文：军训、社会实践、实习、课程设计等）
	if c.BigType == "实习" || strings.Contains(c.Nature, "实践") || c.Nature == "毕业论文" {
		return "必修/集中实践"
	}
	// 美育
	if strings.Contains(c.Nature, "美育") || strings.Contains(c.GeneralCat, "美育") {
		return "选修/通识教育选修课/美育课"
	}
	// 公共体育课（仅体育Ⅰ/体育Ⅱ 这类）→ 选修/体育课
	if isPESportCourse(name) || c.BigType == "体育课" && isPESportCourse(name) {
		return "选修/体育课"
	}
	// 实验课（课内实验 + 独立实验课合并）→ 必修/实验课
	if c.BigType == "实验课" || strings.Contains(c.Nature, "实验") || strings.Contains(name, "实验") {
		return "必修/实验课"
	}
	// 通识选修（核心/普通）
	if c.Tag == "公共选修课" || c.GeneralCat != "" || c.BigType == "通选课" {
		if strings.Contains(c.GeneralCat, "核心") {
			return "选修/通识教育选修课/核心课"
		}
		return "选修/通识教育选修课/普通课"
	}
	// 公共必修 → 通识教育必修
	if c.Tag == "公共必修课" {
		return "必修/通识教育必修课"
	}
	// 专业课：按属性区分核心/基础/大类
	if c.Tag == "专业课" {
		switch {
		case strings.Contains(name, "基础") && !strings.Contains(name, "核心"):
			return "必修/专业基础课"
		case c.Attribute == "必修" || strings.Contains(name, "核心"):
			return "必修/专业核心课"
		default:
			return "必修/大类专业必修课"
		}
	}
	// 兜底
	return "必修/大类专业必修课"
}

// courseCategoryTree 返回完整的分类树（供前端筛选展示）
func courseCategoryTree() []map[string]any {
	return []map[string]any{
		{"name": "必修", "children": []string{"通识教育必修课", "大类专业必修课", "专业基础课", "专业核心课", "集中实践", "实验课"}},
		{"name": "选修", "children": []string{"通识教育选修课/核心课", "通识教育选修课/普通课", "通识教育选修课/美育课", "体育课"}},
	}
}
