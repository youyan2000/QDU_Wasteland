// 直译自 src/rename.rs —— 把 EDA 导出的文件名映射成 JLC 认得的固定文件名。
//
// 规则表、匹配顺序、最终文件名全部照抄原实现；这是"转换"最核心的一步：
// JLC 只认 Gerber_TopLayer.GTL 这类名字，改错一个字母就识别不到层。
package port

import (
	"regexp"
	"sort"
)

// EdaType 对应 Rust 的 EdaType。
type EdaType int

const (
	// EDAAltium 对应 EdaType::Ad（Altium Designer）。
	EDAAltium EdaType = iota
	// EDAKiCad 对应 EdaType::KiCad。
	EDAKiCad
)

// rule 对应 Rust 的 Rule：逻辑名 + 已编译正则。
type rule struct {
	logical string
	re      *regexp.Regexp
}

func mustRule(logical, pattern string) rule {
	return rule{logical: logical, re: regexp.MustCompile(pattern)}
}

// adRules 对应 AD_RULES（顺序即优先级）。
var adRules = []rule{
	mustRule("Gerber_BoardOutlineLayer", `(?i)\.GM(1|13)$`),
	mustRule("Gerber_DocumentLayer", `(?i)\.GM$`),
	mustRule("Gerber_TopLayer", `(?i)\.GTL$`),
	mustRule("Gerber_TopSilkscreenLayer", `(?i)\.GTO$`),
	mustRule("Gerber_TopSolderMaskLayer", `(?i)\.GTS$`),
	mustRule("Gerber_TopPasteMaskLayer", `(?i)\.GTP$`),
	mustRule("Gerber_BottomLayer", `(?i)\.GBL$`),
	mustRule("Gerber_BottomSilkscreenLayer", `(?i)\.GBO$`),
	mustRule("Gerber_BottomSolderMaskLayer", `(?i)\.GBS$`),
	mustRule("Gerber_BottomPasteMaskLayer", `(?i)\.GBP$`),
	mustRule("Gerber_InnerLayer1", `(?i)\.G1$`),
	mustRule("Gerber_InnerLayer2", `(?i)\.G2$`),
	mustRule("Gerber_InnerLayer3", `(?i)\.G3$`),
	mustRule("Gerber_InnerLayer4", `(?i)\.G4$`),
	mustRule("Gerber_InnerLayer5", `(?i)\.G5$`),
	mustRule("Gerber_InnerLayer6", `(?i)\.G6$`),
	mustRule("Drill_Source", `(?i).*holes?.*\.txt$`),
	mustRule("Drill_Source", `(?i)\.tx[1-9]$`),
	mustRule("Drill_PTH_Through_GBR", `(?i)\.GD1$`),
	mustRule("Drill_PTH_Through_Via_GBR", `(?i)\.GG1$`),
	mustRule("Drill_Report", `(?i)\.DRR$`),
	mustRule("Gerber_Layer_Drawing_Parameters", `(?i)\.LDP$`),
}

// kicadRules 对应 KICAD_RULES（顺序即优先级）。
var kicadRules = []rule{
	mustRule("Gerber_BoardOutlineLayer", `(?i).*Edge_Cuts.*`),
	mustRule("Gerber_DocumentLayer", `(?i).*GM.*`),
	mustRule("Gerber_TopLayer", `(?i).*F_Cu.*`),
	mustRule("Gerber_TopSilkscreenLayer", `(?i).*F_Silkscreen.*`),
	mustRule("Gerber_TopSolderMaskLayer", `(?i).*F_Mask.*`),
	mustRule("Gerber_TopPasteMaskLayer", `(?i).*F_Paste.*`),
	mustRule("Gerber_BottomLayer", `(?i).*B_Cu.*`),
	mustRule("Gerber_BottomSilkscreenLayer", `(?i).*B_Silkscreen.*`),
	mustRule("Gerber_BottomSolderMaskLayer", `(?i).*B_Mask.*`),
	mustRule("Gerber_BottomPasteMaskLayer", `(?i).*B_Paste.*`),
	mustRule("Gerber_InnerLayer1", `(?i).*In1_Cu.*`),
	mustRule("Gerber_InnerLayer2", `(?i).*In2_Cu.*`),
	mustRule("Gerber_InnerLayer3", `(?i).*In3_Cu.*`),
	mustRule("Gerber_InnerLayer4", `(?i).*In4_Cu.*`),
	mustRule("Gerber_InnerLayer5", `(?i).*In5_Cu.*`),
	mustRule("Gerber_InnerLayer6", `(?i).*In6_Cu.*`),
	mustRule("Drill_Source", `(?i)\.DRL$`),
	mustRule("Drill_MAP_GBR", `(?i).*[-_]drl_map(?:\.GBR)?$`),
	mustRule("Gerber_GBR_JOB", `(?i)\.gbrjob$`),
}

// finalFilename 对应 get_final_filename。
func finalFilename(logical string) string {
	switch logical {
	case "Gerber_TopSolderMaskLayer":
		return "Gerber_TopSolderMaskLayer.GTS"
	case "Gerber_TopSilkscreenLayer":
		return "Gerber_TopSilkscreenLayer.GTO"
	case "Gerber_TopPasteMaskLayer":
		return "Gerber_TopPasteMaskLayer.GTP"
	case "Gerber_TopLayer":
		return "Gerber_TopLayer.GTL"
	case "Gerber_InnerLayer1":
		return "Gerber_InnerLayer1.G1"
	case "Gerber_InnerLayer2":
		return "Gerber_InnerLayer2.G2"
	case "Gerber_InnerLayer3":
		return "Gerber_InnerLayer3.G3"
	case "Gerber_InnerLayer4":
		return "Gerber_InnerLayer4.G4"
	case "Gerber_InnerLayer5":
		return "Gerber_InnerLayer5.G5"
	case "Gerber_InnerLayer6":
		return "Gerber_InnerLayer6.G6"
	case "Gerber_BottomSolderMaskLayer":
		return "Gerber_BottomSolderMaskLayer.GBS"
	case "Gerber_BottomSilkscreenLayer":
		return "Gerber_BottomSilkscreenLayer.GBO"
	case "Gerber_BottomPasteMaskLayer":
		return "Gerber_BottomPasteMaskLayer.GBP"
	case "Gerber_BottomLayer":
		return "Gerber_BottomLayer.GBL"
	case "Gerber_BoardOutlineLayer":
		return "Gerber_BoardOutlineLayer.GKO"
	case "Drill_PTH_Through":
		return "Drill_PTH_Through.DRL"
	case "Drill_PTH_Through_Via":
		return "Drill_PTH_Through_Via.DRL"
	case "Drill_NPTH_Through":
		return "Drill_NPTH_Through.DRL"
	case "Drill_Source":
		return "Drill_Source.DRL"
	case "Drill_MAP_GBR":
		return "Drill_MAP_GBR.GBR"
	case "Gerber_GBR_JOB":
		return "Gerber_GBR_JOB.GBRJOB"
	case "Drill_Report":
		return "Drill_MAP_GBR.DRR"
	case "Gerber_Layer_Drawing_Parameters":
		return "Gerber_GBR_JOB.LDP"
	}
	return logical + ".GBR"
}

// MapFilenames 对应 map_filenames：返回「原文件名 → 标准文件名」的映射。
// 未匹配任何规则的文件保持原名。
func MapFilenames(files []string, eda EdaType) map[string]string {
	rules := adRules
	if eda == EDAKiCad {
		rules = kicadRules
	}
	out := make(map[string]string, len(files))
	for _, file := range files {
		newName := file
		for _, r := range rules {
			if r.re.MatchString(file) {
				newName = finalFilename(r.logical)
				break // 首个命中即生效
			}
		}
		out[file] = newName
	}
	return out
}

// MapFilenamesSorted 与 MapFilenames 相同，但按原文件名排序，便于稳定输出。
func MapFilenamesSorted(files []string, eda EdaType) [][2]string {
	m := MapFilenames(files, eda)
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([][2]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, [2]string{k, m[k]})
	}
	return out
}

// LogicalName 返回文件命中的逻辑名（未命中返回空串）；drill 模块需要用到。
func LogicalName(file string, eda EdaType) string {
	rules := adRules
	if eda == EDAKiCad {
		rules = kicadRules
	}
	for _, r := range rules {
		if r.re.MatchString(file) {
			return r.logical
		}
	}
	return ""
}
