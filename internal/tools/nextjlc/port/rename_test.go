package port

import "testing"

func TestIdentifySoftware(t *testing.T) {
	cases := map[string]string{
		"G04 Altium Designer*":            "Altium",
		"G04 KiCad PCB*":                  "KiCad",
		"G04 EasyEDA*":                    "EasyEDA",
		"G04 altium kicad easyeda 都有*":   "Altium", // 顺序优先
		"G04 nothing*":                    "",
	}
	for content, want := range cases {
		if got := IdentifySoftware(content); got != want {
			t.Errorf("IdentifySoftware(%q)=%q, want %q", content, got, want)
		}
	}
}

func TestMapFilenamesAltium(t *testing.T) {
	cases := map[string]string{
		"Board.GTL":            "Gerber_TopLayer.GTL",
		"Board.GBL":            "Gerber_BottomLayer.GBL",
		"Board.GTS":            "Gerber_TopSolderMaskLayer.GTS",
		"Board.GBS":            "Gerber_BottomSolderMaskLayer.GBS",
		"Board.GTO":            "Gerber_TopSilkscreenLayer.GTO",
		"Board.GBO":            "Gerber_BottomSilkscreenLayer.GBO",
		"Board.GTP":            "Gerber_TopPasteMaskLayer.GTP",
		"Board.GBP":            "Gerber_BottomPasteMaskLayer.GBP",
		"Board.GM1":            "Gerber_BoardOutlineLayer.GKO",
		"Board.GM13":           "Gerber_BoardOutlineLayer.GKO",
		"Board.G1":             "Gerber_InnerLayer1.G1",
		"Board.G6":             "Gerber_InnerLayer6.G6",
		"Board-holes.TXT":      "Drill_Source.DRL", // 规则要求名字里含 hole(s)
		"Board-NPTH.TX1":       "Drill_Source.DRL",
		"Board.DRR":            "Drill_MAP_GBR.DRR",
		"Board.LDP":            "Gerber_GBR_JOB.LDP",
		"Board.unknown":        "Board.unknown", // 未命中保持原名
		"board.gtl":            "Gerber_TopLayer.GTL", // 大小写不敏感
	}
	got := MapFilenames(keys(cases), EDAAltium)
	for in, want := range cases {
		if got[in] != want {
			t.Errorf("AD %q → %q, want %q", in, got[in], want)
		}
	}
}

func TestMapFilenamesKiCad(t *testing.T) {
	cases := map[string]string{
		"board-F_Cu.gbr":          "Gerber_TopLayer.GTL",
		"board-B_Cu.gbr":          "Gerber_BottomLayer.GBL",
		"board-F_Silkscreen.gbr":  "Gerber_TopSilkscreenLayer.GTO",
		"board-B_Silkscreen.gbr":  "Gerber_BottomSilkscreenLayer.GBO",
		"board-F_Mask.gbr":        "Gerber_TopSolderMaskLayer.GTS",
		"board-B_Mask.gbr":        "Gerber_BottomSolderMaskLayer.GBS",
		"board-F_Paste.gbr":       "Gerber_TopPasteMaskLayer.GTP",
		"board-B_Paste.gbr":       "Gerber_BottomPasteMaskLayer.GBP",
		"board-Edge_Cuts.gbr":     "Gerber_BoardOutlineLayer.GKO",
		"board-In1_Cu.gbr":        "Gerber_InnerLayer1.G1",
		"board-In6_Cu.gbr":        "Gerber_InnerLayer6.G6",
		"board.drl":               "Drill_Source.DRL",
		"board-drl_map.gbr":       "Drill_MAP_GBR.GBR",
		"board.gbrjob":            "Gerber_GBR_JOB.GBRJOB",
	}
	got := MapFilenames(keys(cases), EDAKiCad)
	for in, want := range cases {
		if got[in] != want {
			t.Errorf("KiCad %q → %q, want %q", in, got[in], want)
		}
	}
}

func TestRuleOrderMatters(t *testing.T) {
	// GM1 必须先命中 BoardOutline 而不是 DocumentLayer（.GM 规则在后面）。
	if got := LogicalName("board.GM1", EDAAltium); got != "Gerber_BoardOutlineLayer" {
		t.Errorf("GM1 应命中 BoardOutlineLayer，实际 %s", got)
	}
	if got := LogicalName("board.GM", EDAAltium); got != "Gerber_DocumentLayer" {
		t.Errorf("GM 应命中 DocumentLayer，实际 %s", got)
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
