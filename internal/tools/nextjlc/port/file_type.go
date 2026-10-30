// 直译自 src/file_type.rs —— 按内容关键字识别 EDA 软件，顺序敏感。
package port

import "strings"

// IdentifySoftware 返回 "Altium" / "KiCad" / "EasyEDA" / ""（未识别）。
// 原实现的优先级就是 altium → kicad → easyeda，不要改成"最匹配"。
func IdentifySoftware(content string) string {
	lower := strings.ToLower(content)
	if strings.Contains(lower, "altium") {
		return "Altium"
	}
	if strings.Contains(lower, "kicad") {
		return "KiCad"
	}
	if strings.Contains(lower, "easyeda") {
		return "EasyEDA"
	}
	return ""
}
