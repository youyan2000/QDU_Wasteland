package port

import (
	"strings"
	"testing"
)

const adDrill = `M48
;FILE_FORMAT=2:4
;TYPE=PLATED
METRIC,TZ
T01F00S00C0.800
T02F00S00C3.200
%
G90
G05
T01
X10000Y20000
X30000Y20000
T02
X50000Y60000
M30
`

const kicadPTHDrill = `M48
;DRILL file {KiCad 8.0.0} date 2025-01-01
;FORMAT={-:-/ absolute / metric / decimal}
; #@! TF.CreationDate,2025-01-01T00:00:00
FMAT,2
METRIC
T1C0.800
%
G90
G05
T1
X1.000Y2.000
X3.000Y2.000
M30
`

const kicadNPTHDrill = `M48
;DRILL file {KiCad 8.0.0} date 2025-01-01
; #@! TF.FileFunction,NonPlated,1,2,NPTH
FMAT,2
METRIC
T1C3.200
%
G90
G05
T1
X5.000Y6.000
M30
`

func TestIsDrillFileAndThrough(t *testing.T) {
	cases := []struct {
		name     string
		isDrill  bool
		isThru   bool
	}{
		{"board.drl", true, true},
		{"board-holes.txt", true, true},
		{"Board-NPTH.TX1", true, false}, // 盲埋孔
		{"board.gtl", false, true},
	}
	for _, c := range cases {
		if got := IsDrillFile(c.name); got != c.isDrill {
			t.Errorf("IsDrillFile(%q)=%v, want %v", c.name, got, c.isDrill)
		}
		if got := IsThroughDrill(c.name); got != c.isThru {
			t.Errorf("IsThroughDrill(%q)=%v, want %v", c.name, got, c.isThru)
		}
	}
}

func TestDetectDrillEDA(t *testing.T) {
	if got := DetectDrillEDA(kicadPTHDrill); got != DrillEDAKiCad {
		t.Errorf("应识别为 KiCad，实际 %v", got)
	}
	// 原实现的 ^T\d+F\d+S\d+C 没有 (?m)，只能匹配整份内容的开头；
	// 真实 AD 文件因此落到 Unknown 分支，仍然走 parse_ad_excellon（行为等价）。
	if got := DetectDrillEDA(adDrill); got != DrillEDAUnknown {
		t.Errorf("按原实现语义应为 Unknown，实际 %v", got)
	}
	if got := DetectDrillEDA("T01F00S00C0.8\nM48\n%\nT01\nX1Y1\n"); got != DrillEDAAltium {
		t.Errorf("内容以 T01F00S00C 开头时应识别为 Altium，实际 %v", got)
	}
	if got := DetectDrillEDA("M48\nMETRIC\n%\nT1C0.8\nM30\n"); got != DrillEDAUnknown {
		t.Errorf("应识别为 Unknown，实际 %v", got)
	}
}

func TestParseADExcellonCoordinates(t *testing.T) {
	f := ParseADExcellon(adDrill)
	if len(f.Operations) != 2 {
		t.Fatalf("应有 2 把刀，实际 %d", len(f.Operations))
	}
	// FILE_FORMAT=2:4 + TZ：X10000 → 1.0000mm
	op := f.Operations[0]
	if op.Diameter != 0.8 {
		t.Errorf("T01 直径应为 0.8，实际 %v", op.Diameter)
	}
	if len(op.Commands) != 2 {
		t.Fatalf("T01 应有 2 个孔，实际 %d", len(op.Commands))
	}
	if op.Commands[0].X != 1.0 || op.Commands[0].Y != 2.0 {
		t.Errorf("坐标换算不符: (%.4f, %.4f)", op.Commands[0].X, op.Commands[0].Y)
	}
	if op.HoleType != HolePlated {
		t.Error("TYPE=PLATED 应判为金属化孔")
	}
}

func TestParseKiCadSeparatesHoleType(t *testing.T) {
	pth, ht := ParseKiCadExcellon(kicadPTHDrill)
	if ht != HolePlated {
		t.Errorf("PTH 文件应判为金属化，实际 %v", ht)
	}
	if len(pth.Operations) != 1 || len(pth.Operations[0].Commands) != 2 {
		t.Fatalf("PTH 解析结果不符: %+v", pth)
	}

	npth, ht2 := ParseKiCadExcellon(kicadNPTHDrill)
	if ht2 != HoleNonPlated {
		t.Errorf("NPTH 文件应判为非金属化，实际 %v", ht2)
	}
	if len(npth.Operations) != 1 || npth.Operations[0].Diameter != 3.2 {
		t.Fatalf("NPTH 解析结果不符: %+v", npth)
	}
}

func TestMergeAndSplitDrills(t *testing.T) {
	pthFile, _ := ParseKiCadExcellon(kicadPTHDrill)
	npthFile, _ := ParseKiCadExcellon(kicadNPTHDrill)
	ad := ParseADExcellon(adDrill)

	pthMerged, npthMerged := MergeAndSplitDrills([]DrillFile{pthFile, npthFile, ad})
	if pthMerged == nil || npthMerged == nil {
		t.Fatal("两侧都应有内容")
	}
	// PTH：KiCad 的 0.8 与 AD 的 0.8 应合并为同一把刀（按直径）
	var total int
	for _, op := range pthMerged.Operations {
		total += len(op.Commands)
	}
	// KiCad 的 2 个 0.8mm + AD 的 T01 2 个 + AD 的 T02 1 个（AD 文件里同为 PLATED）
	if total != 5 {
		t.Errorf("PTH 孔数应为 5，实际 %d", total)
	}
	if len(npthMerged.Operations) != 1 {
		t.Errorf("NPTH 应只有 1 把刀，实际 %d", len(npthMerged.Operations))
	}
}

func TestProcessDrillFilesProducesBothOutputs(t *testing.T) {
	res := ProcessDrillFiles(
		[]string{kicadPTHDrill, kicadNPTHDrill},
		[]string{"board-PTH.drl", "board-NPTH.drl"},
	)
	if res.PTHContent == nil || res.NPTHContent == nil {
		t.Fatalf("应同时产出 PTH 与 NPTH：%+v", res)
	}
	if !strings.Contains(*res.PTHContent, ";TYPE=PLATED") {
		t.Error("PTH 文件头不对")
	}
	if !strings.Contains(*res.PTHContent, ";Layer: PTH_Through") {
		t.Error("PTH 层名不对")
	}
	if !strings.Contains(*res.PTHContent, "METRIC,LZ,0000.00000") {
		t.Error("缺少 JLC 要求的单位行")
	}
	if !strings.Contains(*res.PTHContent, "T01C0.80000") {
		t.Errorf("工具定义格式不对:\n%s", *res.PTHContent)
	}
	if !strings.Contains(*res.PTHContent, "X1.00000Y2.00000") {
		t.Errorf("坐标精度应为 5 位小数:\n%s", *res.PTHContent)
	}
	if !strings.HasSuffix(*res.PTHContent, "M30\n") {
		t.Error("应以 M30 结尾")
	}
	if !strings.Contains(*res.NPTHContent, ";TYPE=NON_PLATED") {
		t.Error("NPTH 文件头不对")
	}
}

func TestProcessDrillFilesSkipsBlindVia(t *testing.T) {
	res := ProcessDrillFiles([]string{adDrill}, []string{"board.TX1"})
	if res.PTHContent != nil {
		t.Error("盲埋孔文件应被跳过")
	}
	if len(res.Warnings) == 0 {
		t.Error("应给出跳过警告")
	}
}

func TestSlotBecomesG85(t *testing.T) {
	// M15/G00/G01 路由 → X..Y..G85X..Y.. 槽孔
	src := `M48
;FILE_FORMAT=2:4
METRIC,TZ
T01F00S00C0.800
%
T01
G00X10000Y20000
M15
G01X30000Y20000
M16
M30
`
	res := ProcessDrillFiles([]string{src}, []string{"board.drl"})
	if res.PTHContent == nil {
		t.Fatal("应产出 PTH")
	}
	if !strings.Contains(*res.PTHContent, "G85") {
		t.Errorf("槽孔应输出 G85 格式:\n%s", *res.PTHContent)
	}
}
