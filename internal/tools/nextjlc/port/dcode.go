// 直译自 src/dcode.rs —— 给 D 码补 G54 前缀（KiCad / Altium 两种口味）。
package port

import (
	"regexp"
	"strings"
)

// GerberFlavor 对应 Rust 的 GerberFlavor。
type GerberFlavor int

const (
	// FlavorKiCad 对应 GerberFlavor::KiCad。
	FlavorKiCad GerberFlavor = iota
	// FlavorAltium 对应 GerberFlavor::Altium。
	FlavorAltium
)

// dcodeRegex 对应 DCODE_REGEX：行首的 D 码（D + 2~4 位数字 + *）。
var dcodeRegex = regexp.MustCompile(`^(D\d{2,4}\*)`)

// ProcessDCodes 对应 process_d_codes。
func ProcessDCodes(gerberData string, flavor GerberFlavor) string {
	lines := strings.Split(gerberData, "\n")
	out := make([]string, 0, len(lines))

	for _, line := range lines {
		skip := false
		switch flavor {
		case FlavorKiCad:
			if strings.Contains(line, "%ADD") || strings.Contains(line, "G54D") {
				skip = true
			}
		case FlavorAltium:
			if strings.Contains(line, "G54D") {
				skip = true
			}
			if strings.HasPrefix(line, "G01") || strings.HasPrefix(line, "G02") ||
				strings.HasPrefix(line, "G36") || strings.HasPrefix(line, "G37") {
				trimmed := strings.TrimSpace(line)
				if trimmed == "D01*" || trimmed == "D02*" || trimmed == "D03*" ||
					strings.Contains(trimmed, "X") || strings.Contains(trimmed, "Y") {
					skip = true
				}
			}
		}
		if skip {
			out = append(out, line)
			continue
		}
		out = append(out, dcodeRegex.ReplaceAllString(line, "G54$1"))
	}
	return strings.Join(out, "\n")
}
