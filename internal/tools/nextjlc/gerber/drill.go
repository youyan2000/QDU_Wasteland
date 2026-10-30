// drill.go — Excellon 钻孔文件解析（与 Gerber 分开，因为它的语法完全不同）。
//
// 只处理制造文件里真正用到的部分：头部的单位与工具表、坐标、以及 PTH/NPTH 属性。
// 坐标精度来源优先级：显式小数点 > 头部 FILE_FORMAT > 头部 METRIC,000.000 形式；
// 三者都没有时报错，而不是猜一个精度把板子算错。
package gerber

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Tool 是一把钻头。
type Tool struct {
	ID       int
	Diameter float64 // 毫米
}

// Hole 是一个钻孔位置。
type Hole struct {
	X, Y     float64 // 毫米
	Tool     int
	Diameter float64
}

// Drill 是一份钻孔文件的解析结果。
type Drill struct {
	Unit     string // mm | inch
	Plated   bool   // 是否金属化孔（PTH）
	Tools    map[int]Tool
	Holes    []Hole
	Box      Box
	ByTool   map[int]int     // 每个钻头的孔数
	ByDia    map[float64]int // 每个孔径的孔数
}

// ParseExcellon 解析 Excellon 钻孔文件。
func ParseExcellon(r io.Reader) (*Drill, error) {
	raw, err := io.ReadAll(io.LimitReader(r, 32<<20))
	if err != nil {
		return nil, err
	}
	d := &Drill{
		Unit:   "mm",
		Plated: true,
		Tools:  map[int]Tool{},
		ByTool: map[int]int{},
		ByDia:  map[float64]int{},
	}
	scale := 1.0
	decimals := -1 // -1 表示未知

	inHeader := false
	curTool := 0

	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		upper := strings.ToUpper(trimmed)

		// 注释里可能带出关键信息。
		if strings.HasPrefix(upper, ";") {
			switch {
			case strings.Contains(upper, "TYPE=NON_PLATED"), strings.Contains(upper, "TYPE=NONPLATED"):
				d.Plated = false
			case strings.Contains(upper, "TYPE=PLATED"):
				d.Plated = true
			}
			if n, m, ok := parseFileFormat(upper); ok {
				decimals = m
				_ = n
			}
			continue
		}

		switch {
		case upper == "M48":
			inHeader = true
			continue
		case upper == "%" || upper == "M95":
			inHeader = false
			continue
		case upper == "M30" || upper == "M00":
			// 文件结束
			goto done
		case strings.HasPrefix(upper, "METRIC"):
			d.Unit, scale = "mm", 1.0
			if n := headerDecimals(upper); n >= 0 {
				decimals = n
			}
			continue
		case strings.HasPrefix(upper, "INCH"):
			d.Unit, scale = "inch", 25.4
			if n := headerDecimals(upper); n >= 0 {
				decimals = n
			}
			continue
		case upper == "M71":
			d.Unit, scale = "mm", 1.0
			continue
		case upper == "M72":
			d.Unit, scale = "inch", 25.4
			continue
		}

		// 工具定义：T1C0.800（头部）
		if inHeader && strings.HasPrefix(upper, "T") {
			id, dia, ok := parseToolDef(upper)
			if ok {
				d.Tools[id] = Tool{ID: id, Diameter: dia * scale}
				continue
			}
		}
		// 工具选择：T1
		if strings.HasPrefix(upper, "T") && !strings.Contains(upper, "C") {
			if id, err := strconv.Atoi(upper[1:]); err == nil {
				curTool = id
				continue
			}
		}
		// 坐标行
		if strings.HasPrefix(upper, "X") || strings.HasPrefix(upper, "Y") {
			x, y, ok := parseDrillCoord(upper, decimals)
			if !ok {
				continue
			}
			hole := Hole{X: x * scale, Y: y * scale, Tool: curTool}
			if t, ok := d.Tools[curTool]; ok {
				hole.Diameter = t.Diameter
			}
			d.Holes = append(d.Holes, hole)
			d.ByTool[curTool]++
			d.ByDia[hole.Diameter]++
			if len(d.Holes) == 1 {
				d.Box = Box{MinX: hole.X, MinY: hole.Y, MaxX: hole.X, MaxY: hole.Y}
			} else {
				d.Box.MinX = minF(d.Box.MinX, hole.X)
				d.Box.MinY = minF(d.Box.MinY, hole.Y)
				d.Box.MaxX = maxF(d.Box.MaxX, hole.X)
				d.Box.MaxY = maxF(d.Box.MaxY, hole.Y)
			}
		}
	}

done:
	if len(d.Holes) == 0 {
		return d, errors.New("钻孔文件里没有可识别的坐标")
	}
	// 坐标没有小数点、也没有精度声明时，前面的解析会失败；这里再兜底一次。
	if decimals < 0 && d.Unit == "mm" && len(d.Tools) == 0 {
		return d, errors.New("无法确定钻孔坐标精度（缺少 FILE_FORMAT 或 METRIC,0.000 声明）")
	}
	return d, nil
}

// parseToolDef 解析 "T1C0.800"。
func parseToolDef(line string) (int, float64, bool) {
	i := 1
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i == 1 || i >= len(line) {
		return 0, 0, false
	}
	id, err := strconv.Atoi(line[1:i])
	if err != nil {
		return 0, 0, false
	}
	rest := line[i:]
	if !strings.HasPrefix(rest, "C") {
		return 0, 0, false
	}
	diaStr := rest[1:]
	if j := strings.IndexAny(diaStr, "FSBHT"); j >= 0 {
		diaStr = diaStr[:j]
	}
	dia, err := strconv.ParseFloat(strings.TrimSpace(diaStr), 64)
	if err != nil {
		return 0, 0, false
	}
	return id, dia, true
}

// parseDrillCoord 解析 "X1.234Y5.678" 或 "X01234Y05678"。
func parseDrillCoord(line string, decimals int) (float64, float64, bool) {
	var xs, ys string
	hasDot := strings.Contains(line, ".")
	for i := 0; i < len(line); i++ {
		c := line[i]
		if c != 'X' && c != 'Y' {
			continue
		}
		j := i + 1
		start := j
		for j < len(line) && (line[j] == '-' || line[j] == '+' || line[j] == '.' ||
			(line[j] >= '0' && line[j] <= '9')) {
			j++
		}
		if c == 'X' {
			xs = line[start:j]
		} else {
			ys = line[start:j]
		}
		i = j - 1
	}
	if xs == "" || ys == "" {
		return 0, 0, false
	}
	if !hasDot {
		if decimals < 0 {
			return 0, 0, false
		}
		xs = insertDecimal(xs, decimals)
		ys = insertDecimal(ys, decimals)
	}
	x, err1 := strconv.ParseFloat(xs, 64)
	y, err2 := strconv.ParseFloat(ys, 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return x, y, true
}

// insertDecimal 把 "01234" + 3 位小数 变成 "12.34"。
func insertDecimal(s string, decimals int) string {
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	if len(s) <= decimals {
		s = strings.Repeat("0", decimals-len(s)+1) + s
	}
	out := s[:len(s)-decimals] + "." + s[len(s)-decimals:]
	if neg {
		out = "-" + out
	}
	return out
}

// headerDecimals 从 "METRIC,000.000" / "METRIC,TZ" 里取小数位数。
func headerDecimals(line string) int {
	if i := strings.Index(line, ","); i >= 0 {
		tail := line[i+1:]
		if j := strings.Index(tail, "."); j >= 0 {
			return len(tail) - j - 1
		}
	}
	return -1
}

// parseFileFormat 解析 ";FILE_FORMAT=2:4"。
func parseFileFormat(line string) (int, int, bool) {
	i := strings.Index(line, "FILE_FORMAT=")
	if i < 0 {
		return 0, 0, false
	}
	tail := line[i+len("FILE_FORMAT="):]
	parts := strings.SplitN(strings.TrimSpace(tail), ":", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	n, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	m, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || m < 0 || m > 6 {
		return 0, 0, false
	}
	return n, m, true
}

func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// String 便于日志输出。
func (d *Drill) String() string {
	return fmt.Sprintf("drill(%s, plated=%v, holes=%d, tools=%d)", d.Unit, d.Plated, len(d.Holes), len(d.Tools))
}
