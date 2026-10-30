// 直译自 src/outline.rs —— 按固定优先级给 Gerber 文件排序。
package port

import (
	"sort"
	"strings"
)

// kicadNames 对应 KICAD_NAMES，顺序即优先级。
var kicadNames = []string{"Edge_Cuts", "F_Cu", "F_Mask"}

// gerberExtensions 对应 GERBER_EXTENSIONS，顺序即优先级。
var gerberExtensions = []string{"gto", "gtl", "gbl", "gm13", "gm1", "gko"}

// filePriority 对应 get_file_priority：数字越小越靠前，未匹配给最大。
func filePriority(path string) int {
	for i, name := range kicadNames {
		if strings.Contains(path, name) {
			return i
		}
	}
	lower := strings.ToLower(path)
	for i, ext := range gerberExtensions {
		if strings.HasSuffix(lower, "."+ext) {
			return i + len(kicadNames)
		}
	}
	return int(^uint(0) >> 1) // isize::MAX
}

// SortGerberFiles 对应 sort_gerber_files：返回排序后的新切片（稳定排序）。
func SortGerberFiles(files []string) []string {
	out := make([]string, len(files))
	copy(out, files)
	sort.SliceStable(out, func(i, j int) bool {
		return filePriority(out[i]) < filePriority(out[j])
	})
	return out
}
