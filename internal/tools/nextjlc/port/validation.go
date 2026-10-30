// 直译自 src/validation.rs —— 对「已重命名」的标准文件名做下单前校验。
//
// 注意：这个模块只认标准化后的名字（Gerber_TopLayer…），所以必须在 rename 之后调用。
package port

import (
	"fmt"
	"strings"
)

// ValidationReport 对应 Rust 的 ValidationReport。
type ValidationReport struct {
	LayerCount uint32
	Warnings   []string
}

// requiredPrefixes 对应 REQUIRED_PREFIXES。
var requiredPrefixes = []string{
	"Gerber_BoardOutlineLayer",
	"Gerber_TopLayer",
	"Gerber_TopSolderMaskLayer",
}

func anyPrefix(files []string, prefix string) bool {
	for _, f := range files {
		if strings.HasPrefix(f, prefix) {
			return true
		}
	}
	return false
}

// ValidateGerberFiles 对应 validate_gerber_files：
// 全部关键规则通过时返回 (报告, nil)，否则返回 (零值, 错误列表)。
func ValidateGerberFiles(files []string) (ValidationReport, []string) {
	var errors []string
	var warnings []string

	// 1. 必需文件
	for _, prefix := range requiredPrefixes {
		if !anyPrefix(files, prefix) {
			errors = append(errors, fmt.Sprintf("Missing required file starting with: '%s'", prefix))
		}
	}

	// 2. 收集事实
	hasTopCopper := anyPrefix(files, "Gerber_TopLayer")
	hasBottomCopper := anyPrefix(files, "Gerber_BottomLayer")
	var innerLayerCount uint32
	for _, f := range files {
		if strings.HasPrefix(f, "Gerber_InnerLayer") {
			innerLayerCount++
		}
	}

	// 3. 条件检查
	if hasTopCopper {
		if !anyPrefix(files, "Gerber_TopSolderMaskLayer") {
			already := false
			for _, e := range errors {
				if strings.Contains(e, "Gerber_TopSolderMaskLayer") {
					already = true
					break
				}
			}
			if !already {
				errors = append(errors, "Top copper layer is present, but the required 'Gerber_TopSolderMaskLayer' is missing.")
			}
		}
		if !anyPrefix(files, "Gerber_TopSilkscreenLayer") {
			warnings = append(warnings, "Warning: 'Gerber_TopSilkscreenLayer' is missing for the top side.")
		}
		if !anyPrefix(files, "Gerber_TopPasteMaskLayer") {
			warnings = append(warnings, "Warning: 'Gerber_TopPasteMaskLayer' is missing. This is usually needed for SMD components.")
		}
	}

	if hasBottomCopper {
		if !anyPrefix(files, "Gerber_BottomSolderMaskLayer") {
			errors = append(errors, "Bottom copper layer is present, but 'Gerber_BottomSolderMaskLayer' is missing.")
		}
		if !anyPrefix(files, "Gerber_BottomSilkscreenLayer") {
			warnings = append(warnings, "Warning: 'Gerber_BottomSilkscreenLayer' is missing for the bottom side.")
		}
		if !anyPrefix(files, "Gerber_BottomPasteMaskLayer") {
			warnings = append(warnings, "Warning: 'Gerber_BottomPasteMaskLayer' is missing for the bottom side.")
		}
	}

	// 多层板（顶层 + 内层）必须有底层
	if hasTopCopper && innerLayerCount > 0 && !hasBottomCopper {
		errors = append(errors, "Invalid layer stackup: A board with top and inner copper layers must also have a bottom copper layer.")
	}

	// 4. 层数
	var total uint32
	if hasTopCopper {
		total++
	}
	if hasBottomCopper {
		total++
	}
	total += innerLayerCount

	// 5. 返回
	if len(errors) == 0 {
		return ValidationReport{LayerCount: total, Warnings: warnings}, nil
	}
	return ValidationReport{}, errors
}
