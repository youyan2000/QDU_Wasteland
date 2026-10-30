// pipeline.go — 指纹去重、重命名计划、校验、打包。
//
// 这四步串起来就是工具的主流程：一堆文件 → 归组 → 去重 → 改名 → 体检 → 出 zip。
package gerber

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
)

// Renamed 是一条重命名记录（From 是原文件名，To 是包内路径）。
type Renamed struct {
	From string
	To   string
}

// Issue 是体检结论。Level 为 "error" 时不应下单，"warn" 只提示。
type Issue struct {
	Level   string `json:"level"`
	Message string `json:"message"`
	File    string `json:"file,omitempty"`
}

// ---------- 4/9 指纹 ----------

// Fingerprint 给一块板算指纹：板框尺寸（0.1mm 精度）+ 成员层类型集合。
// 同一块板被重复打包（例如工程师导出了两遍）时会得到同一个指纹，从而被去重。
func Fingerprint(b Bundle) string {
	size := fmt.Sprintf("%.1fx%.1f", b.Box.Width(), b.Box.Height())
	layers := make([]string, 0, len(b.Files))
	for _, f := range b.Files {
		layers = append(layers, LayerOf(f))
	}
	sort.Strings(layers)
	sum := sha256.Sum256([]byte(size + "|" + strings.Join(layers, ",")))
	return hex.EncodeToString(sum[:8])
}

// Dedupe 去掉重复的板（保留第一块），返回去重结果与被去掉的块数。
func Dedupe(bundles []Bundle) ([]Bundle, int) {
	seen := map[string]bool{}
	out := make([]Bundle, 0, len(bundles))
	removed := 0
	for _, b := range bundles {
		fp := Fingerprint(b)
		if seen[fp] {
			removed++
			continue
		}
		seen[fp] = true
		out = append(out, b)
	}
	return out, removed
}

// ---------- 5/9 重命名 ----------

// LayerOf 推断一份文件属于哪一层（命名与页面展示共用）。
func LayerOf(f File) string {
	if f.Drill != nil {
		if f.Drill.Plated {
			return "Drill-PTH"
		}
		return "Drill-NPTH"
	}
	fn := ""
	if f.Gerber != nil {
		fn = f.Gerber.Function
	}
	low := strings.ToLower(fn)
	switch {
	case strings.Contains(low, "profile"):
		return "Outline"
	case strings.Contains(low, "copper"):
		return "Copper-" + sideOf(fn, f.Name)
	case strings.Contains(low, "soldermask"):
		return "Mask-" + sideOf(fn, f.Name)
	case strings.Contains(low, "legend"), strings.Contains(low, "silkscreen"):
		return "Silk-" + sideOf(fn, f.Name)
	case strings.Contains(low, "paste"):
		return "Paste-" + sideOf(fn, f.Name)
	case strings.Contains(low, "drill"):
		return "Drill"
	}
	// 没有属性时退回扩展名（Altium/KiCad/Protel 的命名习惯）。
	if byExt := layerByExt(path.Ext(strings.ToLower(f.Name))); byExt != "" {
		return byExt
	}
	// 再退回文件名线索。
	name := strings.ToLower(path.Base(f.Name))
	switch {
	case strings.Contains(name, "edge"), strings.Contains(name, "outline"), strings.Contains(name, "profile"):
		return "Outline"
	case strings.Contains(name, "mask"):
		return "Mask-" + sideOf("", name)
	case strings.Contains(name, "silk"), strings.Contains(name, "legend"):
		return "Silk-" + sideOf("", name)
	case strings.Contains(name, "paste"):
		return "Paste-" + sideOf("", name)
	case strings.Contains(name, "cu"), strings.Contains(name, "copper"):
		return "Copper-" + sideOf("", name)
	}
	return "Layer"
}

// sideOf 判断是顶层还是底层；判断不出时用 L1/L2 之外的通用名。
func sideOf(function, name string) string {
	hay := strings.ToLower(function + " " + path.Base(name))
	switch {
	case strings.Contains(hay, "top"), strings.Contains(hay, "-f"), strings.Contains(hay, "_f"), strings.Contains(hay, "l1"):
		return "Top"
	case strings.Contains(hay, "bot"), strings.Contains(hay, "-b"), strings.Contains(hay, "_b"), strings.Contains(hay, "l2"):
		return "Bottom"
	}
	return "Side"
}

// Plan 生成这块板的重命名计划：<板名>-<层>.<扩展名>，重名时追加序号。
func Plan(b Bundle) []Renamed {
	base := sanitize(b.Name)
	if base == "" {
		base = "board"
	}
	used := map[string]int{}
	out := make([]Renamed, 0, len(b.Files))
	for _, f := range b.Files {
		ext := strings.ToLower(path.Ext(f.Name))
		switch f.Kind {
		case KindGerber:
			ext = ".gbr"
		case KindExcellon:
			ext = ".drl"
		default:
			if ext == "" {
				ext = ".dat"
			}
		}
		target := fmt.Sprintf("%s-%s%s", base, LayerOf(f), ext)
		if n, ok := used[target]; ok {
			used[target] = n + 1
			target = fmt.Sprintf("%s-%s-%d%s", base, LayerOf(f), n+1, ext)
		} else {
			used[target] = 0
		}
		out = append(out, Renamed{From: f.Name, To: target})
	}
	return out
}

// sanitize 只保留安全字符，避免路径穿越与不可读文件名。
func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ' || r == '.' || r == '/':
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// ---------- 6/9 校验 ----------

// Validate 对一块板做下单前体检。
func Validate(b Bundle) []Issue {
	var issues []Issue
	if b.Outline == nil {
		issues = append(issues, Issue{Level: "error", Message: "缺少板框层，无法确定板子尺寸"})
	}
	if len(b.Files) == 0 {
		issues = append(issues, Issue{Level: "error", Message: "这块板没有任何文件"})
	}

	hasDrill := false
	units := map[string]bool{}
	for _, f := range b.Files {
		if f.Drill != nil {
			hasDrill = true
			units[f.Drill.Unit] = true
			if len(f.Drill.Holes) == 0 {
				issues = append(issues, Issue{Level: "error", Message: "钻孔文件里没有孔", File: f.Name})
			}
		}
		if f.Gerber != nil && f.Gerber.Points == 0 {
			issues = append(issues, Issue{Level: "error", Message: "Gerber 里没有有效坐标", File: f.Name})
		}
		// 文件中心落在板框外 → 大概率放错了板。
		if b.Outline != nil && f.Name != b.Outline.Name {
			cx, cy := center(f.Box())
			if cx < b.Box.MinX || cx > b.Box.MaxX || cy < b.Box.MinY || cy > b.Box.MaxY {
				issues = append(issues, Issue{Level: "warn", Message: "该文件不在板框范围内", File: f.Name})
			}
		}
	}
	if len(units) > 1 {
		issues = append(issues, Issue{Level: "error", Message: "钻孔文件单位不一致（同时存在 mm 与 inch）"})
	}
	if !hasDrill {
		issues = append(issues, Issue{Level: "warn", Message: "没有钻孔文件，可能只导出了 Gerber"})
	}
	// 重名检查。
	seen := map[string]bool{}
	for _, r := range Plan(b) {
		if seen[r.To] {
			issues = append(issues, Issue{Level: "error", Message: "重命名后仍有重名", File: r.From})
		}
		seen[r.To] = true
	}
	return issues
}

// HasError 判断体检结果里是否有阻断项。
func HasError(issues []Issue) bool {
	for _, i := range issues {
		if i.Level == "error" {
			return true
		}
	}
	return false
}

// ---------- 7/9 打包 ----------

// WriteZip 把一块板按重命名计划打成一个 zip（顶层目录 = 板名）。
func WriteZip(w io.Writer, b Bundle, plan []Renamed) error {
	zw := zip.NewWriter(w)
	byName := map[string]File{}
	for _, f := range b.Files {
		byName[f.Name] = f
	}
	dir := sanitize(b.Name)
	if dir == "" {
		dir = "board"
	}
	for _, r := range plan {
		f, ok := byName[r.From]
		if !ok {
			continue
		}
		fw, err := zw.Create(dir + "/" + r.To)
		if err != nil {
			return err
		}
		if _, err := fw.Write(f.Content); err != nil {
			return err
		}
	}
	return zw.Close()
}

// layerByExt 按常见 EDA 导出的扩展名推断层。
func layerByExt(ext string) string {
	switch ext {
	case ".gtl", ".cmp", ".top":
		return "Copper-Top"
	case ".gbl", ".sol", ".bot":
		return "Copper-Bottom"
	case ".g2", ".g3", ".g4":
		return "Copper-Inner" + strings.TrimPrefix(ext, ".g")
	case ".gts", ".stc", ".smt":
		return "Mask-Top"
	case ".gbs", ".sts", ".smb":
		return "Mask-Bottom"
	case ".gto", ".sst", ".sstc":
		return "Silk-Top"
	case ".gbo", ".ssb", ".sstb":
		return "Silk-Bottom"
	case ".gtp", ".gpt":
		return "Paste-Top"
	case ".gbp", ".gpb":
		return "Paste-Bottom"
	case ".gko", ".gm1", ".gml", ".outline":
		return "Outline"
	case ".drl", ".xln", ".nc", ".ncd", ".txt":
		return "Drill"
	}
	return ""
}