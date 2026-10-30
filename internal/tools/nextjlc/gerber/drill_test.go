package gerber

import (
	"strings"
	"testing"
)

const sampleDrill = `M48
;FILE_FORMAT=3:3
;TYPE=PLATED
METRIC,TZ
T1C0.800
T2C3.200
%
G90
G05
T1
X1.000Y2.000
X3.000Y2.000
T2
X5.500Y6.500
M30
`

func TestParseExcellonDecimal(t *testing.T) {
	d, err := ParseExcellon(strings.NewReader(sampleDrill))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if d.Unit != "mm" || !d.Plated {
		t.Errorf("单位/镀通属性不符: %s plated=%v", d.Unit, d.Plated)
	}
	if len(d.Tools) != 2 {
		t.Fatalf("应解析到 2 把钻头，实际 %d", len(d.Tools))
	}
	if got := d.Tools[2].Diameter; got != 3.2 {
		t.Errorf("T2 直径应为 3.2，实际 %v", got)
	}
	if len(d.Holes) != 3 {
		t.Fatalf("应有 3 个孔，实际 %d", len(d.Holes))
	}
	if d.ByTool[1] != 2 || d.ByTool[2] != 1 {
		t.Errorf("按钻头统计不符: %+v", d.ByTool)
	}
	if d.ByDia[0.8] != 2 || d.ByDia[3.2] != 1 {
		t.Errorf("按孔径统计不符: %+v", d.ByDia)
	}
	// 包围盒应覆盖 1..5.5 / 2..6.5
	if d.Box.MinX != 1.0 || d.Box.MaxX != 5.5 || d.Box.MinY != 2.0 || d.Box.MaxY != 6.5 {
		t.Errorf("包围盒不符: %+v", d.Box)
	}
}

func TestParseExcellonNonPlated(t *testing.T) {
	src := `M48
;TYPE=NON_PLATED
METRIC,TZ
T1C3.200
%
T1
X1.0Y1.0
M30
`
	d, err := ParseExcellon(strings.NewReader(src))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if d.Plated {
		t.Error("头部声明 NON_PLATED 时应标记为非金属化孔")
	}
}

func TestParseExcellonIntegerCoords(t *testing.T) {
	src := `M48
;FILE_FORMAT=2:4
METRIC,LZ
T1C0.5
%
T1
X001234Y005678
M30
`
	d, err := ParseExcellon(strings.NewReader(src))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	// 001234 去掉前导零后按 4 位小数 → 0.1234mm
	if got := d.Holes[0].X; got != 0.1234 {
		t.Errorf("整数坐标换算不符: %v", got)
	}
	if got := d.Holes[0].Y; got != 0.5678 {
		t.Errorf("整数坐标换算不符: %v", got)
	}
}

func TestParseExcellonInch(t *testing.T) {
	src := `M48
INCH,LZ
T1C0.04
%
T1
X1.0Y1.0
M30
`
	d, err := ParseExcellon(strings.NewReader(src))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if d.Unit != "inch" {
		t.Fatalf("单位应为 inch，实际 %s", d.Unit)
	}
	if got := d.Holes[0].X; got != 25.4 {
		t.Errorf("英寸坐标应换算成毫米: %v", got)
	}
	if got := d.Tools[1].Diameter; got != 1.016 {
		t.Errorf("英寸孔径应换算成毫米: %v", got)
	}
}

func TestParseExcellonNoHoles(t *testing.T) {
	if _, err := ParseExcellon(strings.NewReader("M48\nMETRIC,TZ\n%\nM30\n")); err == nil {
		t.Fatal("没有坐标时应报错")
	}
}

func TestParseExcellonUnknownPrecision(t *testing.T) {
	// 没有小数点、也没有 FILE_FORMAT：宁可报错，也不要猜精度算错板子。
	src := "M48\nMETRIC,TZ\nT1C0.5\n%\nT1\nX001234Y005678\nM30\n"
	if _, err := ParseExcellon(strings.NewReader(src)); err == nil {
		t.Fatal("缺少精度声明且坐标无小数点时应报错")
	}
}

func TestInsertDecimal(t *testing.T) {
	if got := insertDecimal("01234", 3); got != "01.234" {
		t.Errorf("insertDecimal 不符: %q", got)
	}
	if got := insertDecimal("-01234", 3); got != "-01.234" {
		t.Errorf("负数应保留符号: %q", got)
	}
}
