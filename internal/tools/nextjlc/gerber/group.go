// group.go — 把一堆制造文件归成「一块板」。
//
// 真实压缩包里常见十几份文件：顶层铜、阻焊、丝印、钻孔、板框……
// 归组规则按可信度从高到低：
//  1. %TF.FileFunction,Profile 或 GKO/Edge_Cuts 之类的名字 → 板框
//  2. 文件名里带 outline/profile/board/edge 的也算板框
//  3. 其余文件按「中心点落在哪个板框内」归组
//  4. 落不进任何板框的，自己单独成组（不丢文件）
package gerber

import (
	"math"
	"path"
	"sort"
	"strings"
)

// File 是一份参与归组的文件。
type File struct {
	Name    string
	Kind    Kind
	Gerber  *Board
	Drill   *Drill
	Content []byte // 原始内容，打包时直接写出
}

// Box 返回该文件的外接矩形。
func (f File) Box() Box {
	if f.Gerber != nil {
		return f.Gerber.Box
	}
	if f.Drill != nil {
		return f.Drill.Box
	}
	return Box{}
}

// Bundle 是一块板：一个板框 + 归属它的其它文件。
type Bundle struct {
	Name    string
	Outline *File
	Files   []File
	Box     Box
}

// IsOutline 判断这份文件是否是板框层。
func IsOutline(f File) bool {
	if f.Gerber != nil && f.Gerber.Function != "" {
		if strings.EqualFold(f.Gerber.Function, "Profile") {
			return true
		}
	}
	name := strings.ToLower(path.Base(f.Name))
	switch path.Ext(name) {
	case ".gko", ".gm1", ".gml":
		return true
	}
	for _, hint := range []string{"edge_cuts", "edge-cuts", "outline", "profile", "boardoutline", "board-outline"} {
		if strings.Contains(name, hint) {
			return true
		}
	}
	return false
}

// Group 把文件归成若干块板。
func Group(files []File) []Bundle {
	// 先挑出板框，其余作为候选成员。
	var outlines, rest []File
	for _, f := range files {
		if IsOutline(f) {
			outlines = append(outlines, f)
		} else {
			rest = append(rest, f)
		}
	}

	// 一个板框都没有：按文件名主干分组，保证不丢文件。
	if len(outlines) == 0 {
		return groupByStem(rest)
	}

	// 板框按左上角排序，保证输出顺序稳定。
	sort.Slice(outlines, func(i, j int) bool {
		a, b := outlines[i].Box(), outlines[j].Box()
		if a.MinY != b.MinY {
			return a.MinY > b.MinY
		}
		return a.MinX < b.MinX
	})

	bundles := make([]Bundle, len(outlines))
	for i, o := range outlines {
		box := o.Box()
		name := strings.TrimSuffix(path.Base(o.Name), path.Ext(o.Name))
		bundles[i] = Bundle{Name: name, Outline: &outlines[i], Files: []File{o}, Box: box}
	}

	for _, f := range rest {
		box := f.Box()
		idx := containingBundle(bundles, box, f.Name)
		if idx < 0 {
			idx = nearestWithinMargin(bundles, box)
		}
		if idx < 0 {
			// 没有可用板框（例如板框解析失败）：自己成组。
			bundles = append(bundles, Bundle{
				Name:  strings.TrimSuffix(path.Base(f.Name), path.Ext(f.Name)),
				Files: []File{f}, Box: box,
			})
			continue
		}
		bundles[idx].Files = append(bundles[idx].Files, f)
	}
	return bundles
}

// containingBundle 返回中心点落在哪个板框里。
//
// 两块板的板框完全重叠时（同一个 zip 里重复导出很常见），几何信息无法区分，
// 这时用「文件名与板名的公共前缀」做偏好，避免所有文件都被塞进第一块板。
func containingBundle(bundles []Bundle, box Box, name string) int {
	cx, cy := center(box)
	lowerName := strings.ToLower(name)
	best, bestScore, bestArea := -1, -1, math.MaxFloat64
	for i, b := range bundles {
		if b.Outline == nil {
			continue
		}
		if cx < b.Box.MinX || cx > b.Box.MaxX || cy < b.Box.MinY || cy > b.Box.MaxY {
			continue
		}
		area := (b.Box.MaxX - b.Box.MinX) * (b.Box.MaxY - b.Box.MinY)
		score := commonPrefixLen(lowerName, strings.ToLower(b.Name))
		if score > bestScore || (score == bestScore && area > 0 && area < bestArea) {
			best, bestScore, bestArea = i, score, area
		}
	}
	return best
}

// commonPrefixLen 返回两个字符串的公共前缀长度（忽略大小写）。
func commonPrefixLen(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

// nearestWithinMargin 在中心点不落在板框内时，只在「一个板框尺寸」的余量内兜底，
// 超出就让它单独成组 —— 否则远景的无关文件会被硬塞进某块板。
func nearestWithinMargin(bundles []Bundle, box Box) int {
	cx, cy := center(box)
	best, bestDist := -1, math.MaxFloat64
	for i, b := range bundles {
		if b.Outline == nil {
			continue
		}
		w, h := b.Box.Width(), b.Box.Height()
		if w <= 0 && h <= 0 {
			continue
		}
		margin := math.Max(w, h)
		if cx < b.Box.MinX-margin || cx > b.Box.MaxX+margin ||
			cy < b.Box.MinY-margin || cy > b.Box.MaxY+margin {
			continue
		}
		bcx, bcy := center(b.Box)
		d := math.Hypot(cx-bcx, cy-bcy)
		if d < bestDist {
			best, bestDist = i, d
		}
	}
	return best
}

func center(b Box) (float64, float64) {
	return (b.MinX + b.MaxX) / 2, (b.MinY + b.MaxY) / 2
}

// groupByStem 在没有板框时按文件名主干分组（如 "boardA-F.Cu.gbr" → "boardA"）。
func groupByStem(files []File) []Bundle {
	order := []string{}
	byKey := map[string]*Bundle{}
	for _, f := range files {
		key := stemKey(f.Name)
		b, ok := byKey[key]
		if !ok {
			b = &Bundle{Name: key, Files: []File{}, Box: f.Box()}
			byKey[key] = b
			order = append(order, key)
		}
		b.Files = append(b.Files, f)
		box := f.Box()
		if b.Box.Width() == 0 && b.Box.Height() == 0 {
			b.Box = box
		} else {
			b.Box.MinX = minF(b.Box.MinX, box.MinX)
			b.Box.MinY = minF(b.Box.MinY, box.MinY)
			b.Box.MaxX = maxF(b.Box.MaxX, box.MaxX)
			b.Box.MaxY = maxF(b.Box.MaxY, box.MaxY)
		}
	}
	out := make([]Bundle, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	return out
}

// stemKey 取文件名里「属于同一块板」的那一段：
// "gerbv_example-boardA-F.Cu.gbr" → 去掉扩展名与常见层后缀后剩下的部分。
func stemKey(name string) string {
	base := strings.TrimSuffix(path.Base(name), path.Ext(name))
	lower := strings.ToLower(base)
	for _, sep := range []string{"-f.", "-b.", "_f.", "_b.", "-cu", "_cu", "-mask", "_mask", "-silk", "_silk"} {
		if i := strings.Index(lower, sep); i > 0 {
			return base[:i]
		}
	}
	return base
}
