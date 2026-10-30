// 包 gerber 解析 PCB 制造文件：Gerber RS-274X 与 Excellon 钻孔。
//
// 只依赖标准库：这两种格式都是公开规范，不需要引入上游的任何代码。
// 解析结果用于「文件识别 → 板框匹配 → 归类重命名 → 打包下载」这条流水线。
package gerber

import (
	"bytes"
	"errors"
	"io"
	"strconv"
	"strings"
)

// Kind 是文件类型。
type Kind string

const (
	KindGerber   Kind = "gerber"
	KindExcellon Kind = "excellon"
	KindZip      Kind = "zip"
	KindUnknown  Kind = "unknown"
)

// ErrUnsupported 表示既不是 Gerber 也不是钻孔文件。
var ErrUnsupported = errors.New("不是受支持的 Gerber/Excellon 文件")

// Detect 根据文件名与内容头部判断类型。
func Detect(name string, head []byte) Kind {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".zip") {
		return KindZip
	}
	if len(head) >= 4 && bytes.HasPrefix(head, []byte("PK\x03\x04")) {
		return KindZip
	}
	text := string(head)
	// Gerber 必有 %FS 或 G04 + D 码；用几个特征交叉判断。
	if strings.Contains(text, "%FS") || strings.Contains(text, "%MO") {
		return KindGerber
	}
	// Excellon 头部是 M48，且带 T 码与 METRIC/INCH。
	if strings.Contains(text, "M48") && (strings.Contains(text, "METRIC") || strings.Contains(text, "INCH") ||
		strings.Contains(text, "T1C")) {
		return KindExcellon
	}
	// 没有头部但像 Gerber：出现坐标 + D01/D02/D03。
	if strings.Contains(text, "D0") && (strings.Contains(text, "X") && strings.Contains(text, "Y")) {
		return KindGerber
	}
	return KindUnknown
}

// Box 是毫米单位的包围盒。
type Box struct {
	MinX, MinY, MaxX, MaxY float64
}

// Valid 表示是否至少有一个点。
func (b Box) Valid() bool { return b.MaxX >= b.MinX && b.MaxY >= b.MinY && b.MaxX != 0 || b.MaxY != 0 }

// Width/Height 返回尺寸（毫米）。
func (b Box) Width() float64  { return b.MaxX - b.MinX }
func (b Box) Height() float64 { return b.MaxY - b.MinY }

// Format 是 Gerber 的坐标格式声明（整数位、小数位）。
type Format struct{ Int, Dec int }

// Aperture 是一个光圈定义。
type Aperture struct {
	Code    int
	Shape   string  // C 圆 / R 矩形 / O 椭圆 / P 多边形
	Params  []float64
}

// Board 是一份 Gerber 的解析结果。
type Board struct {
	Format    Format
	Unit      string // mm | inch
	Apertures map[int]Aperture
	Box       Box
	Points    int
	Layers    int    // 见过的 D 码层数（D10 起）
	Function  string // %TF.FileFunction,Profile,NP 里的层用途（Profile/Soldermask/Copper…）
}

// ParseGerber 解析 RS-274X。
func ParseGerber(r io.Reader) (*Board, error) {
	raw, err := io.ReadAll(io.LimitReader(r, 64<<20))
	if err != nil {
		return nil, err
	}
	b := &Board{Apertures: map[int]Aperture{}, Unit: "mm"}
	fs := Format{Int: 2, Dec: 4}
	scale := 1.0 // 到毫米的系数

	// Gerber 以 * 结束每条命令，扩展命令包在 %...% 之间。
	body := string(raw)
	segments := strings.Split(body, "*")
	first := true
	for _, seg := range segments {
		seg = strings.TrimSpace(strings.ReplaceAll(seg, "%", ""))
		if seg == "" {
			continue
		}
		switch {
		case strings.HasPrefix(seg, "FS"):
			// FSLAX46Y46
			if f, ok := parseFormat(seg); ok {
				fs = f
				b.Format = f
			}
		case strings.HasPrefix(seg, "MO"):
			if strings.Contains(seg, "IN") {
				b.Unit, scale = "inch", 25.4
			} else {
				b.Unit, scale = "mm", 1.0
			}
		case strings.HasPrefix(seg, "AD"):
			if ap, ok := parseAperture(seg); ok {
				b.Apertures[ap.Code] = ap
				if ap.Code >= 10 {
					b.Layers++
				}
			}
		case strings.HasPrefix(seg, "TF.FileFunction"):
			// 形如 TF.FileFunction,Profile,NP → 取第一个逗号后的用途
			parts := strings.Split(seg, ",")
			if len(parts) > 1 {
				b.Function = strings.TrimSpace(parts[1])
			}
		case strings.HasPrefix(seg, "G04"):
			continue
		default:
			if x, y, ok := parseCoord(seg); ok {
				nx, ny := toMM(x, fs)*scale, toMM(y, fs)*scale
				if first {
					b.Box = Box{MinX: nx, MinY: ny, MaxX: nx, MaxY: ny}
					first = false
				} else {
					if nx < b.Box.MinX {
						b.Box.MinX = nx
					}
					if ny < b.Box.MinY {
						b.Box.MinY = ny
					}
					if nx > b.Box.MaxX {
						b.Box.MaxX = nx
					}
					if ny > b.Box.MaxY {
						b.Box.MaxY = ny
					}
				}
				b.Points++
			}
		}
	}
	if b.Points == 0 {
		return b, ErrUnsupported
	}
	return b, nil
}

// parseFormat 解析 FS 命令里的坐标格式，例如 FSLAX46Y46 → 4 位整数、6 位小数。
func parseFormat(seg string) (Format, bool) {
	i := strings.Index(seg, "X")
	if i < 0 || i+2 >= len(seg) {
		return Format{}, false
	}
	d := seg[i+1 : i+3]
	iv, err1 := strconv.Atoi(d[:1])
	dv, err2 := strconv.Atoi(d[1:])
	if err1 != nil || err2 != nil {
		return Format{}, false
	}
	return Format{Int: iv, Dec: dv}, true
}

// parseAperture 解析 ADD10C,0.5 → 圆光圈直径 0.5。
func parseAperture(seg string) (Aperture, bool) {
	rest := strings.TrimPrefix(strings.TrimPrefix(seg, "AD"), "D") // ADD10C,0.5 → 10C,0.5
	end := strings.IndexAny(rest, "CR OPO")
	_ = end
	// 形如 "10C,0.5" 或 "11R,1.0X2.0"
	i := 0
	for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
		i++
	}
	if i == 0 || i >= len(rest) {
		return Aperture{}, false
	}
	code, err := strconv.Atoi(rest[:i])
	if err != nil {
		return Aperture{}, false
	}
	shape := string(rest[i])
	tail := ""
	if j := strings.Index(rest[i:], ","); j >= 0 {
		tail = rest[i+j+1:]
	}
	var params []float64
	for _, p := range strings.FieldsFunc(tail, func(r rune) bool { return r == 'X' || r == ',' }) {
		if v, err := strconv.ParseFloat(strings.TrimSpace(p), 64); err == nil {
			params = append(params, v)
		}
	}
	return Aperture{Code: code, Shape: shape, Params: params}, true
}

// parseCoord 从 "X1000Y2000D03" 里取出原始整数坐标。
func parseCoord(seg string) (int64, int64, bool) {
	var xs, ys string
	gotX, gotY := false, false
	for i := 0; i < len(seg); i++ {
		c := seg[i]
		if c == 'X' || c == 'Y' {
			j := i + 1
			start := j
			for j < len(seg) && (seg[j] == '-' || seg[j] == '+' || (seg[j] >= '0' && seg[j] <= '9')) {
				j++
			}
			val := seg[start:j]
			if c == 'X' {
				xs, gotX = val, true
			} else {
				ys, gotY = val, true
			}
			i = j - 1
		}
	}
	if !gotX || !gotY {
		return 0, 0, false
	}
	x, err1 := strconv.ParseInt(xs, 10, 64)
	y, err2 := strconv.ParseInt(ys, 10, 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return x, y, true
}

// toMM 把原始整数坐标按格式换算成单位长度（未乘单位系数）。
func toMM(raw int64, f Format) float64 {
	if f.Dec <= 0 {
		return float64(raw)
	}
	div := 1.0
	for i := 0; i < f.Dec; i++ {
		div *= 10
	}
	return float64(raw) / div
}
