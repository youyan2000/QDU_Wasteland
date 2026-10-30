package gerber

import (
	"strings"
	"testing"
)

const sampleGerber = `%FSLAX46Y46*%
%MOMM*%
%ADD10C,0.2000*%
%ADD11R,1.0000X2.0000*%
G04 顶层铜箔*
D10*
X1000000Y2000000D02*
X3000000Y2000000D01*
X3000000Y4000000D01*
M02*
`

func TestDetect(t *testing.T) {
	cases := []struct {
		name string
		head string
		want Kind
	}{
		{"top.gbr", "%FSLAX46Y46*%\n%MOMM*%", KindGerber},
		{"drill.drl", "M48\nMETRIC,TZ\nT1C0.800", KindExcellon},
		{"gerbers.zip", "PK\x03\x04", KindZip},
		{"readme.txt", "hello world", KindUnknown},
	}
	for _, c := range cases {
		if got := Detect(c.name, []byte(c.head)); got != c.want {
			t.Errorf("%s: want %s, got %s", c.name, c.want, got)
		}
	}
}

func TestParseGerberBasics(t *testing.T) {
	b, err := ParseGerber(strings.NewReader(sampleGerber))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if b.Unit != "mm" {
		t.Errorf("单位应为 mm，实际 %s", b.Unit)
	}
	if b.Format != (Format{Int: 4, Dec: 6}) {
		t.Errorf("坐标格式不符: %+v", b.Format)
	}
	if len(b.Apertures) != 2 {
		t.Fatalf("应解析到 2 个光圈，实际 %d", len(b.Apertures))
	}
	if ap := b.Apertures[11]; ap.Shape != "R" || len(ap.Params) != 2 || ap.Params[0] != 1.0 {
		t.Errorf("矩形光圈参数不符: %+v", ap)
	}
	if b.Points != 3 {
		t.Errorf("应识别 3 个坐标点，实际 %d", b.Points)
	}
	// 坐标 1000000 / 10^6 = 1.0mm 起，最大 3.0 x 4.0mm。
	if b.Box.MinX != 1.0 || b.Box.MinY != 2.0 || b.Box.MaxX != 3.0 || b.Box.MaxY != 4.0 {
		t.Errorf("包围盒不符: %+v", b.Box)
	}
	if b.Box.Width() != 2.0 || b.Box.Height() != 2.0 {
		t.Errorf("板框尺寸不符: %.2f x %.2f", b.Box.Width(), b.Box.Height())
	}
}

func TestParseGerberInch(t *testing.T) {
	src := `%FSLAX25Y25*%
%MOIN*%
X100000Y100000D02*
X200000Y200000D01*
M02*
`
	b, err := ParseGerber(strings.NewReader(src))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if b.Unit != "inch" {
		t.Fatalf("单位应为 inch，实际 %s", b.Unit)
	}
	// 1.00000 英寸（X25 = 2 位整数 + 5 位小数）→ 25.4mm
	if got := b.Box.MinX; got != 25.4 {
		t.Errorf("英寸换算不对: %.3f", got)
	}
}

func TestParseGerberRejectsEmpty(t *testing.T) {
	if _, err := ParseGerber(strings.NewReader("G04 nothing here*\nM02*")); err == nil {
		t.Fatal("没有任何坐标时应报错")
	}
}

func TestToMM(t *testing.T) {
	if got := toMM(1234567, Format{Int: 4, Dec: 6}); got != 1.234567 {
		t.Errorf("toMM 不符: %v", got)
	}
	if got := toMM(42, Format{Dec: 0}); got != 42 {
		t.Errorf("小数位为 0 时应原样返回: %v", got)
	}
}
