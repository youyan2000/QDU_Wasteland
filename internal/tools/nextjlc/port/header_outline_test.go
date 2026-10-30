package port

import (
	"regexp"
	"strings"
	"testing"
)

func TestGetGerberHeaderFormat(t *testing.T) {
	h := GetGerberHeader()
	re := regexp.MustCompile(`^G04 (EasyEDA Pro|EasyEDA) v\d+\.\d+\.\d+\.\d+, \d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\*\nG04 Gerber Generator version 0\.3\*\n$`)
	if !re.MatchString(h) {
		t.Fatalf("Gerber 头格式不符: %q", h)
	}
}

func TestGetDrillHeaderFormat(t *testing.T) {
	h := GetDrillHeader("NON_PLATED", "NPTH_Through")
	if !strings.HasPrefix(h, ";TYPE=NON_PLATED\n;Layer: NPTH_Through\n;") {
		t.Fatalf("钻孔头格式不符: %q", h)
	}
	if !strings.Contains(h, ";Gerber Generator version 0.3\n") {
		t.Errorf("缺少版本行: %q", h)
	}
}

func TestSortGerberFilesPriority(t *testing.T) {
	in := []string{"board.gko", "board.gm1", "board.gtl", "board.gto", "board-Edge_Cuts.gbr", "other.txt"}
	got := SortGerberFiles(in)
	// KiCad 名字优先（Edge_Cuts 最高），然后按扩展名 gto→gtl→…→gko，未匹配最后。
	want := []string{"board-Edge_Cuts.gbr", "board.gto", "board.gtl", "board.gm1", "board.gko", "other.txt"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("排序不符:\n got=%v\nwant=%v", got, want)
		}
	}
	if in[0] != "board.gko" {
		t.Error("不应修改入参切片")
	}
}

func TestProcessDCodesKiCad(t *testing.T) {
	in := "%ADD10C,0.5*%\nD10*\nG54D11*\nX0Y0D02*\n"
	got := ProcessDCodes(in, FlavorKiCad)
	want := "%ADD10C,0.5*%\nG54D10*\nG54D11*\nX0Y0D02*\n"
	if got != want {
		t.Fatalf("KiCad D 码处理不符:\n got=%q\nwant=%q", got, want)
	}
}

func TestProcessDCodesAltium(t *testing.T) {
	in := "D10*\nG01X100Y200D01*\nG01D02*\nG36*\nD03*\n"
	got := ProcessDCodes(in, FlavorAltium)
	// D10* 补前缀；带坐标的 G01 行跳过；G01D02* 跳过；G36* 补前缀；D03* 补前缀。
	// 正则锚定行首的 D 码，G36* 不以 D 开头，因此不会被改写。
	want := "G54D10*\nG01X100Y200D01*\nG01D02*\nG36*\nG54D03*\n"
	if got != want {
		t.Fatalf("Altium D 码处理不符:\n got=%q\nwant=%q", got, want)
	}
}
