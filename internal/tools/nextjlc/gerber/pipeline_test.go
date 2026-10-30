package gerber

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func outlineFile(t *testing.T) File {
	return File{Name: "edge_cuts.gbr", Kind: KindGerber, Gerber: mustGerber(t, boardA), Content: []byte(boardA)}
}

func copperFile(t *testing.T, name string) File {
	return File{Name: name, Kind: KindGerber, Gerber: mustGerber(t, copperA), Content: []byte(copperA)}
}

func drillFile(t *testing.T, plated bool) File {
	d, err := ParseExcellon(strings.NewReader(sampleDrill))
	if err != nil {
		t.Fatal(err)
	}
	d.Plated = plated
	return File{Name: "drill.drl", Kind: KindExcellon, Drill: d, Content: []byte(sampleDrill)}
}

func TestFingerprintStableAndSizeSensitive(t *testing.T) {
	a := Bundle{Name: "a", Box: Box{MinX: 0, MinY: 0, MaxX: 10, MaxY: 10},
		Files: []File{outlineFile(t), copperFile(t, "a-F.Cu.gbr")}}
	b := Bundle{Name: "b", Box: Box{MinX: 0, MinY: 0, MaxX: 10, MaxY: 10},
		Files: []File{outlineFile(t), copperFile(t, "b-F.Cu.gbr")}}
	if Fingerprint(a) != Fingerprint(b) {
		t.Error("同尺寸同层类型的板应得到同一指纹（文件名不参与）")
	}
	c := Bundle{Name: "c", Box: Box{MinX: 0, MinY: 0, MaxX: 20, MaxY: 10},
		Files: []File{outlineFile(t), copperFile(t, "a-F.Cu.gbr")}}
	if Fingerprint(a) == Fingerprint(c) {
		t.Error("尺寸不同的板不应同指纹")
	}
}

func TestDedupeRemovesRepeatedBoard(t *testing.T) {
	mk := func(name string) Bundle {
		return Bundle{Name: name, Box: Box{MaxX: 10, MaxY: 10},
			Files: []File{outlineFile(t), copperFile(t, name+"-F.Cu.gbr")}}
	}
	got, removed := Dedupe([]Bundle{mk("a"), mk("b")})
	if len(got) != 1 || removed != 1 {
		t.Fatalf("应去掉 1 块重复板，实际剩 %d 去 %d", len(got), removed)
	}
	if got[0].Name != "a" {
		t.Errorf("应保留第一块，实际 %s", got[0].Name)
	}
}

func TestPlanNamesLayers(t *testing.T) {
	b := Bundle{Name: "demo board", Box: Box{MaxX: 10, MaxY: 10},
		Files: []File{outlineFile(t), copperFile(t, "top-copper.gbr"), drillFile(t, true)}}
	plan := Plan(b)
	got := map[string]string{}
	for _, r := range plan {
		got[r.From] = r.To
	}
	if got["edge_cuts.gbr"] != "demo-board-Outline.gbr" {
		t.Errorf("板框命名不符: %s", got["edge_cuts.gbr"])
	}
	if got["drill.drl"] != "demo-board-Drill-PTH.drl" {
		t.Errorf("钻孔命名不符: %s", got["drill.drl"])
	}
	if !strings.HasPrefix(got["top-copper.gbr"], "demo-board-Copper") {
		t.Errorf("铜层命名不符: %s", got["top-copper.gbr"])
	}
}

func TestPlanAvoidsDuplicateNames(t *testing.T) {
	b := Bundle{Name: "b", Box: Box{MaxX: 10, MaxY: 10},
		Files: []File{copperFile(t, "x1.gbr"), copperFile(t, "x2.gbr")}}
	plan := Plan(b)
	if len(plan) != 2 {
		t.Fatalf("应有 2 条计划，实际 %d", len(plan))
	}
	if plan[0].To == plan[1].To {
		t.Fatalf("重名未被消解: %s", plan[0].To)
	}
}

func TestValidateCatchesMissingOutlineAndDrill(t *testing.T) {
	b := Bundle{Name: "b", Box: Box{MaxX: 10, MaxY: 10},
		Files: []File{copperFile(t, "top.gbr")}}
	issues := Validate(b)
	if !HasError(issues) {
		t.Fatal("缺板框应判为 error")
	}
	var hasDrillWarn bool
	for _, i := range issues {
		if i.Level == "warn" && strings.Contains(i.Message, "钻孔") {
			hasDrillWarn = true
		}
	}
	if !hasDrillWarn {
		t.Error("缺钻孔应给 warn")
	}
}

func TestValidateFlagsMixedDrillUnits(t *testing.T) {
	d1 := drillFile(t, true)
	d2 := drillFile(t, false)
	d2.Drill.Unit = "inch"
	b := Bundle{Name: "b", Box: Box{MaxX: 10, MaxY: 10},
		Files: []File{outlineFile(t), d1, d2}}
	issues := Validate(b)
	var mixed bool
	for _, i := range issues {
		if i.Level == "error" && strings.Contains(i.Message, "单位不一致") {
			mixed = true
		}
	}
	if !mixed {
		t.Error("钻孔单位混用应判为 error")
	}
}

func TestWriteZipLayout(t *testing.T) {
	b := Bundle{Name: "demo", Box: Box{MaxX: 10, MaxY: 10},
		Files: []File{outlineFile(t), copperFile(t, "top.gbr")}}
	var buf bytes.Buffer
	if err := WriteZip(&buf, b, Plan(b)); err != nil {
		t.Fatalf("打包失败: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip 不可读: %v", err)
	}
	names := []string{}
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	if len(names) != 2 {
		t.Fatalf("应有 2 个条目，实际 %d: %v", len(names), names)
	}
	for _, n := range names {
		if !strings.HasPrefix(n, "demo/") {
			t.Errorf("条目应以板名开头: %s", n)
		}
	}
}

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		"demo board":  "demo-board",
		"../evil/x":   "evil-x",
		"中文板子":        "",
		"ok-name_1":   "ok-name_1",
	}
	for in, want := range cases {
		if got := sanitize(in); got != want {
			t.Errorf("sanitize(%q)=%q, want %q", in, got, want)
		}
	}
}
