// 直译自 src/drill.rs —— 钻孔文件解析、合并、按 PTH/NPTH 拆分，输出 JLC 格式。
//
// 这是原项目最关键的一块：JLC 下单要求「金属化孔」和「非金属化孔」分成两个 DRL，
// 并且文件头、工具表、坐标精度都有固定格式。
//
// 注意：Go 的 regexp 不支持 (?:...) 非捕获组，这里改成捕获组并相应调整下标，
// 匹配语义与原实现保持一致。
package port

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// HoleType 对应 HoleType。
type HoleType int

const (
	// HolePlated 对应 HoleType::Plated（PTH）。
	HolePlated HoleType = iota
	// HoleNonPlated 对应 HoleType::NonPlated（NPTH）。
	HoleNonPlated
)

// DrillUnit 对应 DrillUnit。
type DrillUnit int

const (
	// UnitInch 对应 DrillUnit::Inch。
	UnitInch DrillUnit = iota
	// UnitMetric 对应 DrillUnit::Metric。
	UnitMetric
)

// DrillCommand 对应 DrillCommand；坐标一律毫米。
type DrillCommand struct {
	IsSlot                           bool
	X, Y, StartX, StartY, EndX, EndY float64
}

// DrillOperation 对应 DrillOperation。
type DrillOperation struct {
	Diameter float64 // 毫米
	HoleType HoleType
	Commands []DrillCommand
}

// DrillFile 对应 DrillFile。
type DrillFile struct {
	Operations []DrillOperation
}

// DrillResult 对应 DrillResult。
type DrillResult struct {
	PTHContent  *string
	NPTHContent *string
	Warnings    []string
}

const inchToMM = 25.4

var (
	adToolRe     = regexp.MustCompile(`^T(\d+)F\d+S\d+C([\d.]+)`)
	kicadToolRe  = regexp.MustCompile(`^T(\d+)C([\d.]+)`)
	coordRe      = regexp.MustCompile(`^(X([\d.-]+))?(Y([\d.-]+))?$`)
	toolSelectRe = regexp.MustCompile(`^T(\d+)$`)
	routeStartRe = regexp.MustCompile(`^G00(X([\d.-]+))?(Y([\d.-]+))?`)
	routeToRe    = regexp.MustCompile(`^G01(X([\d.-]+))?(Y([\d.-]+))?`)
	fileFormatRe = regexp.MustCompile(`FILE_FORMAT=(\d+):(\d+)`)
	// 有的导出（EasyEDA、部分 Altium）刀表就是 "T1C0.800"，且出现在内容中间。
	toolDefLoose = regexp.MustCompile(`(?m)^T\d+C[\d.]+`)
	// AD 刀表（T01F00S00C0.8）也常常不在文件第一行。
	adToolLoose = regexp.MustCompile(`(?m)^T\d+F\d+S\d+C[\d.]+`)
)

// IsDrillFile 对应 is_drill_file。
func IsDrillFile(filename string) bool {
	lower := strings.ToLower(filename)
	if strings.HasSuffix(lower, ".drl") {
		return true
	}
	if strings.HasSuffix(lower, ".txt") && (strings.Contains(lower, "hole") || strings.Contains(lower, "drill")) {
		return true
	}
	for _, ext := range []string{".tx1", ".tx2", ".tx3", ".tx4", ".tx5", ".tx6"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// LooksLikeExcellon 兜底识别：Altium 导出的钻孔常直接叫 board.TXT / PCB1.TXT，
// 名字里没有 "hole"/"drill"，只按文件名会被当成 Gerber，导致整包缺钻孔。
// 这里按内容判断：以 M48 开头，且带 Excellon 的典型标记。
func LooksLikeExcellon(name string, data []byte) bool {
	lower := strings.ToLower(name)
	if !(strings.HasSuffix(lower, ".txt") || strings.HasSuffix(lower, ".drl") || strings.HasSuffix(lower, ".nc")) {
		return false
	}
	head := strings.ToUpper(strings.TrimPrefix(string(headBytes(data, 4096)), "\ufeff"))
	head = strings.TrimSpace(head)
	if !strings.HasPrefix(head, "M48") {
		return false
	}
	return strings.Contains(head, "METRIC") || strings.Contains(head, "INCH") ||
		strings.Contains(head, "FMAT") || strings.Contains(head, ";TYPE=")
}

func headBytes(data []byte, n int) []byte {
	if len(data) > n {
		return data[:n]
	}
	return data
}

// IsThroughDrill 对应 is_through_drill：TX1-TX6 是盲埋孔，JLC 不支持。
func IsThroughDrill(filename string) bool {
	lower := strings.ToLower(filename)
	for _, ext := range []string{".tx1", ".tx2", ".tx3", ".tx4", ".tx5", ".tx6"} {
		if strings.HasSuffix(lower, ext) {
			return false
		}
	}
	return true
}

// DrillEdaType 对应 DrillEdaType。
type DrillEdaType int

const (
	// DrillEDAAltium 对应 DrillEdaType::Altium。
	DrillEDAAltium DrillEdaType = iota
	// DrillEDAKiCad 对应 DrillEdaType::KiCad。
	DrillEDAKiCad
	// DrillEDAUnknown 对应 DrillEdaType::Unknown。
	DrillEDAUnknown
)

// DetectDrillEDA 对应 detect_drill_eda（注意顺序：kicad → AD 工具格式 → altium）。
// 额外兜底：EasyEDA / 部分 Altium 版本的刀表写成 "T1C0.800"，没有 F00S00 段，
// 只认 adToolRe 会把它们当 Unknown，最后钻孔全丢。
func DetectDrillEDA(content string) DrillEdaType {
	lower := strings.ToLower(content)
	if strings.Contains(lower, "kicad") {
		return DrillEDAKiCad
	}
	if adToolRe.MatchString(content) || adToolLoose.MatchString(content) {
		return DrillEDAAltium
	}
	if toolDefLoose.MatchString(content) {
		return DrillEDAAltium
	}
	if strings.Contains(lower, "altium") {
		return DrillEDAAltium
	}
	return DrillEDAUnknown
}

// parseADCoordinate 对应 parse_ad_coordinate：按 FILE_FORMAT 与 LZ/TZ 解析坐标。
func parseADCoordinate(coord string, integerPlaces, decimalPlaces uint32, isLZ bool, unit DrillUnit) float64 {
	if strings.Contains(coord, ".") {
		var val float64
		_, _ = fmt.Sscanf(coord, "%f", &val)
		if unit == UnitInch {
			return val * inchToMM
		}
		return val
	}
	sign := 1.0
	if strings.HasPrefix(coord, "-") {
		sign = -1
	}
	abs := strings.TrimLeft(coord, "+-")

	var val float64
	if isLZ {
		splitPos := int(integerPlaces)
		if len(abs) <= splitPos {
			_, _ = fmt.Sscanf(abs, "%f", &val)
		} else {
			intPart := abs[:splitPos]
			decPart := abs[splitPos:]
			var iv float64
			_, _ = fmt.Sscanf(intPart, "%f", &iv)
			var dv float64
			if decPart != "" {
				var raw float64
				_, _ = fmt.Sscanf(decPart, "%f", &raw)
				dv = raw / pow10(len(decPart))
			}
			val = iv + dv
		}
	} else {
		var v int64
		if _, err := fmt.Sscanf(abs, "%d", &v); err == nil {
			val = float64(v) / pow10(int(decimalPlaces))
		}
	}
	result := sign * val
	if unit == UnitInch {
		return result * inchToMM
	}
	return result
}

func pow10(n int) float64 {
	out := 1.0
	for i := 0; i < n; i++ {
		out *= 10
	}
	return out
}

// ParseADExcellon 对应 parse_ad_excellon。
func ParseADExcellon(content string) DrillFile {
	toolMap := map[uint32]struct {
		diameter float64
		holeType HoleType
	}{}
	var toolKeys []uint32
	currentHoleType := HolePlated
	inHeader := true

	unit := UnitMetric
	integerPlaces := uint32(2)
	decimalPlaces := uint32(4)
	isLZ := true

	lines := strings.Split(content, "\n")

	// 第一遍：头部（单位、格式、工具表）
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "%" {
			inHeader = false
			continue
		}
		if !inHeader {
			continue
		}
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, "INCH") {
			unit = UnitInch
			if strings.Contains(upper, "LZ") {
				isLZ = true
			} else if strings.Contains(upper, "TZ") {
				isLZ = false
			}
		} else if strings.HasPrefix(upper, "METRIC") {
			unit = UnitMetric
			if strings.Contains(upper, "LZ") {
				isLZ = true
			} else if strings.Contains(upper, "TZ") {
				isLZ = false
			}
		}
		if m := fileFormatRe.FindStringSubmatch(line); m != nil {
			_, _ = fmt.Sscanf(m[1], "%d", &integerPlaces)
			_, _ = fmt.Sscanf(m[2], "%d", &decimalPlaces)
		}
		if strings.Contains(line, "TYPE=PLATED") && !strings.Contains(line, "NON_PLATED") {
			currentHoleType = HolePlated
		} else if strings.Contains(line, "TYPE=NON_PLATED") {
			currentHoleType = HoleNonPlated
		}
		if m := adToolRe.FindStringSubmatch(line); m != nil {
			var toolNum uint32
			_, _ = fmt.Sscanf(m[1], "%d", &toolNum)
			var dia float64
			_, _ = fmt.Sscanf(m[2], "%f", &dia)
			if unit == UnitInch {
				dia *= inchToMM
			}
			if _, ok := toolMap[toolNum]; !ok {
				toolKeys = append(toolKeys, toolNum)
			}
			toolMap[toolNum] = struct {
				diameter float64
				holeType HoleType
			}{dia, currentHoleType}
		} else if m := kicadToolRe.FindStringSubmatch(line); m != nil {
			var toolNum uint32
			_, _ = fmt.Sscanf(m[1], "%d", &toolNum)
			var dia float64
			_, _ = fmt.Sscanf(m[2], "%f", &dia)
			if unit == UnitInch {
				dia *= inchToMM
			}
			if _, ok := toolMap[toolNum]; !ok {
				toolKeys = append(toolKeys, toolNum)
			}
			toolMap[toolNum] = struct {
				diameter float64
				holeType HoleType
			}{dia, currentHoleType}
		}
	}

	toolOps := map[uint32]*DrillOperation{}
	for num, t := range toolMap {
		toolOps[num] = &DrillOperation{Diameter: t.diameter, HoleType: t.holeType}
	}

	// 第二遍：钻带命令
	var currentTool uint32
	hasTool := false
	inRoute := false
	var lastX, lastY float64
	inHeader = true

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "%" {
			inHeader = false
			continue
		}
		if inHeader || line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		if m := toolSelectRe.FindStringSubmatch(line); m != nil {
			var num uint32
			_, _ = fmt.Sscanf(m[1], "%d", &num)
			if _, ok := toolMap[num]; ok {
				currentTool, hasTool = num, true
			}
			continue
		}
		if m := routeStartRe.FindStringSubmatch(line); m != nil {
			if m[2] != "" {
				lastX = parseADCoordinate(m[2], integerPlaces, decimalPlaces, isLZ, unit)
			}
			if m[4] != "" {
				lastY = parseADCoordinate(m[4], integerPlaces, decimalPlaces, isLZ, unit)
			}
			continue
		}
		if line == "M15" {
			inRoute = true
			continue
		}
		if inRoute {
			if m := routeToRe.FindStringSubmatch(line); m != nil {
				startX, startY := lastX, lastY
				if m[2] != "" {
					lastX = parseADCoordinate(m[2], integerPlaces, decimalPlaces, isLZ, unit)
				}
				if m[4] != "" {
					lastY = parseADCoordinate(m[4], integerPlaces, decimalPlaces, isLZ, unit)
				}
				if hasTool {
					if op, ok := toolOps[currentTool]; ok {
						op.Commands = append(op.Commands, DrillCommand{
							IsSlot: true, StartX: startX, StartY: startY, EndX: lastX, EndY: lastY,
						})
					}
				}
				continue
			}
		}
		if line == "M16" {
			inRoute = false
			continue
		}
		if m := coordRe.FindStringSubmatch(line); m != nil {
			if m[2] == "" && m[4] == "" {
				continue
			}
			if hasTool {
				if op, ok := toolOps[currentTool]; ok {
					if m[2] != "" {
						lastX = parseADCoordinate(m[2], integerPlaces, decimalPlaces, isLZ, unit)
					}
					if m[4] != "" {
						lastY = parseADCoordinate(m[4], integerPlaces, decimalPlaces, isLZ, unit)
					}
					op.Commands = append(op.Commands, DrillCommand{X: lastX, Y: lastY})
				}
			}
		}
	}

	out := DrillFile{}
	for _, num := range sortedUint32(toolKeys) {
		op := toolOps[num]
		if op != nil && len(op.Commands) > 0 {
			out.Operations = append(out.Operations, *op)
		}
	}
	// 头部里没登记的工具（理论上不会出现）也要按编号稳定输出。
	if len(out.Operations) != len(toolOps) {
		var rest []uint32
		seen := map[uint32]bool{}
		for _, n := range toolKeys {
			seen[n] = true
		}
		for n := range toolOps {
			if !seen[n] {
				rest = append(rest, n)
			}
		}
		for _, n := range sortedUint32(rest) {
			if op := toolOps[n]; op != nil && len(op.Commands) > 0 {
				out.Operations = append(out.Operations, *op)
			}
		}
	}
	return out
}

// ParseKiCadExcellon 对应 parse_kicad_excellon：KiCad 坐标已是十进制毫米。
func ParseKiCadExcellon(content string) (DrillFile, HoleType) {
	holeType := HolePlated
	if strings.Contains(content, "NonPlated") || strings.Contains(content, "NPTH") {
		holeType = HoleNonPlated
	}

	toolMap := map[uint32]float64{}
	var toolKeys []uint32
	inHeader := true
	lines := strings.Split(content, "\n")

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "%" {
			inHeader = false
			continue
		}
		if !inHeader {
			continue
		}
		if m := kicadToolRe.FindStringSubmatch(line); m != nil {
			var num uint32
			_, _ = fmt.Sscanf(m[1], "%d", &num)
			var dia float64
			_, _ = fmt.Sscanf(m[2], "%f", &dia)
			if _, ok := toolMap[num]; !ok {
				toolKeys = append(toolKeys, num)
			}
			toolMap[num] = dia
		}
	}

	toolOps := map[uint32]*DrillOperation{}
	for num, dia := range toolMap {
		toolOps[num] = &DrillOperation{Diameter: dia, HoleType: holeType}
	}

	var currentTool uint32
	hasTool := false
	inRoute := false
	var routeStartX, routeStartY float64
	hasRouteStart := false
	var lastY float64
	inHeader = true

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "%" {
			inHeader = false
			continue
		}
		if inHeader || line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		if m := toolSelectRe.FindStringSubmatch(line); m != nil {
			var num uint32
			_, _ = fmt.Sscanf(m[1], "%d", &num)
			if _, ok := toolMap[num]; ok {
				currentTool, hasTool = num, true
			}
			continue
		}
		if m := routeStartRe.FindStringSubmatch(line); m != nil {
			x, y := 0.0, 0.0
			if m[2] != "" {
				_, _ = fmt.Sscanf(m[2], "%f", &x)
			}
			if m[4] != "" {
				_, _ = fmt.Sscanf(m[4], "%f", &y)
			}
			routeStartX, routeStartY, hasRouteStart = x, y, true
			lastY = y
			continue
		}
		if line == "M15" {
			inRoute = true
			continue
		}
		if inRoute {
			if m := routeToRe.FindStringSubmatch(line); m != nil {
				endX := 0.0
				if m[2] != "" {
					_, _ = fmt.Sscanf(m[2], "%f", &endX)
				}
				endY := lastY
				if m[4] != "" {
					_, _ = fmt.Sscanf(m[4], "%f", &endY)
				}
				if hasTool && hasRouteStart {
					if op, ok := toolOps[currentTool]; ok {
						op.Commands = append(op.Commands, DrillCommand{
							IsSlot: true, StartX: routeStartX, StartY: routeStartY, EndX: endX, EndY: endY,
						})
					}
				}
				lastY = endY
				continue
			}
		}
		if line == "M16" {
			inRoute = false
			hasRouteStart = false
			continue
		}
		if strings.HasPrefix(line, "X") && strings.Contains(line, "Y") {
			if m := regexp.MustCompile(`^X([\d.-]+)Y([\d.-]+)`).FindStringSubmatch(line); m != nil {
				if hasTool {
					if op, ok := toolOps[currentTool]; ok {
						var x, y float64
						_, _ = fmt.Sscanf(m[1], "%f", &x)
						_, _ = fmt.Sscanf(m[2], "%f", &y)
						op.Commands = append(op.Commands, DrillCommand{X: x, Y: y})
					}
				}
			}
		}
	}

	out := DrillFile{}
	for _, num := range sortedUint32(toolKeys) {
		if op := toolOps[num]; op != nil && len(op.Commands) > 0 {
			out.Operations = append(out.Operations, *op)
		}
	}
	for n := range toolOps {
		found := false
		for _, k := range toolKeys {
			if k == n {
				found = true
				break
			}
		}
		if !found && toolOps[n] != nil && len(toolOps[n].Commands) > 0 {
			out.Operations = append(out.Operations, *toolOps[n])
		}
	}
	return out, holeType
}

// MergeAndSplitDrills 对应 merge_and_split_drills。
func MergeAndSplitDrills(files []DrillFile) (*DrillFile, *DrillFile) {
	var pth, npth []DrillOperation
	for _, f := range files {
		for _, op := range f.Operations {
			if op.HoleType == HolePlated {
				pth = append(pth, op)
			} else {
				npth = append(npth, op)
			}
		}
	}
	pthMerged := mergeOperationsByDiameter(pth)
	npthMerged := mergeOperationsByDiameter(npth)

	var pthFile, npthFile *DrillFile
	if len(pthMerged) > 0 {
		pthFile = &DrillFile{Operations: pthMerged}
	}
	if len(npthMerged) > 0 {
		npthFile = &DrillFile{Operations: npthMerged}
	}
	return pthFile, npthFile
}

// mergeOperationsByDiameter 对应 merge_operations_by_diameter：
// 直径 ×100000 取整作为 key（浮点比较），结果按 key 升序。
func mergeOperationsByDiameter(ops []DrillOperation) []DrillOperation {
	m := map[uint64]*DrillOperation{}
	for i := range ops {
		op := ops[i]
		key := uint64(op.Diameter * 100000.0)
		if existing, ok := m[key]; ok {
			existing.Commands = append(existing.Commands, op.Commands...)
		} else {
			copied := op
			m[key] = &copied
		}
	}
	keys := make([]uint64, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	out := make([]DrillOperation, 0, len(keys))
	for _, k := range keys {
		out = append(out, *m[k])
	}
	return out
}

// GenerateJLCExcellon 对应 generate_jlc_excellon。
func GenerateJLCExcellon(drill *DrillFile, holeType HoleType) string {
	var b strings.Builder

	typeStr, layerName := "PLATED", "PTH_Through"
	if holeType == HoleNonPlated {
		typeStr, layerName = "NON_PLATED", "NPTH_Through"
	}
	b.WriteString(GetDrillHeader(typeStr, layerName))

	b.WriteString("M48\n")
	b.WriteString("METRIC,LZ,0000.00000\n")

	for i, op := range drill.Operations {
		toolNum := i + 1
		fmt.Fprintf(&b, ";Hole size %d = %.5f METRIC\n", toolNum, op.Diameter)
		fmt.Fprintf(&b, "T%02dC%.5f\n", toolNum, op.Diameter)
	}

	b.WriteString("%\n")
	b.WriteString("G05\n")
	b.WriteString("G90\n")

	for i, op := range drill.Operations {
		toolNum := i + 1
		fmt.Fprintf(&b, "T%02d\n", toolNum)
		for _, cmd := range op.Commands {
			if cmd.IsSlot {
				fmt.Fprintf(&b, "X%.5fY%.5fG85X%.5fY%.5f\n", cmd.StartX, cmd.StartY, cmd.EndX, cmd.EndY)
			} else {
				fmt.Fprintf(&b, "X%.5fY%.5f\n", cmd.X, cmd.Y)
			}
		}
	}
	b.WriteString("M30\n")
	return b.String()
}

// ProcessDrillFiles 对应 process_drill_files。
func ProcessDrillFiles(contents, filenames []string) DrillResult {
	var allFiles []DrillFile
	var warnings []string
	hasKiCadPTH, hasKiCadNPTH := false, false
	var kicadPTH, kicadNPTH *string

	n := len(contents)
	if len(filenames) < n {
		n = len(filenames)
	}
	for i := 0; i < n; i++ {
		content, filename := contents[i], filenames[i]
		if !IsThroughDrill(filename) {
			warnings = append(warnings, fmt.Sprintf(
				"Skipped blind/buried via file: %s. JLC only supports through holes.", filename))
			continue
		}
		switch DetectDrillEDA(content) {
		case DrillEDAKiCad:
			file, holeType := ParseKiCadExcellon(content)
			if holeType == HolePlated {
				if !hasKiCadPTH {
					s := GenerateJLCExcellon(&file, HolePlated)
					kicadPTH = &s
					hasKiCadPTH = true
				} else {
					allFiles = append(allFiles, file)
				}
			} else {
				if !hasKiCadNPTH {
					s := GenerateJLCExcellon(&file, HoleNonPlated)
					kicadNPTH = &s
					hasKiCadNPTH = true
				} else {
					allFiles = append(allFiles, file)
				}
			}
		default: // Altium / Unknown
			allFiles = append(allFiles, ParseADExcellon(content))
		}
	}

	if len(allFiles) > 0 {
		pthFile, npthFile := MergeAndSplitDrills(allFiles)
		var pthContent, npthContent *string
		if pthFile != nil {
			s := GenerateJLCExcellon(pthFile, HolePlated)
			pthContent = &s
		}
		if npthFile != nil {
			s := GenerateJLCExcellon(npthFile, HoleNonPlated)
			npthContent = &s
		}
		finalPTH := pthContent
		if finalPTH == nil {
			finalPTH = kicadPTH
		}
		finalNPTH := npthContent
		if finalNPTH == nil {
			finalNPTH = kicadNPTH
		}
		return DrillResult{PTHContent: finalPTH, NPTHContent: finalNPTH, Warnings: warnings}
	}
	return DrillResult{PTHContent: kicadPTH, NPTHContent: kicadNPTH, Warnings: warnings}
}

func sortedUint32(in []uint32) []uint32 {
	out := make([]uint32, len(in))
	copy(out, in)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
