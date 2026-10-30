package gerber

import (
	"strings"
	"testing"
)

const boardA = `%FSLAX46Y46*%
%MOMM*%
%TF.FileFunction,Profile,NP*%
X0Y0D02*
X10000000Y0D01*
X10000000Y10000000D01*
X0Y10000000D01*
M02*
`

// 同一块板内的一块铜箔（坐标落在 0..10mm 内）。
const copperA = `%FSLAX46Y46*%
%MOMM*%
%TF.FileFunction,Copper,L1,Top*%
X1000000Y1000000D02*
X2000000Y2000000D01*
M02*
`

// 落在板框外的铜箔（用于验证兜底归组不丢文件）。
const copperFar = `%FSLAX46Y46*%
%MOMM*%
X50000000Y50000000D02*
X51000000Y51000000D01*
M02*
`

func mustGerber(t *testing.T, src string) *Board {
	t.Helper()
	b, err := ParseGerber(strings.NewReader(src))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	return b
}

func TestIsOutline(t *testing.T) {
	cases := []struct {
		name string
		file File
		want bool
	}{
		{"按属性", File{Name: "a.gbr", Gerber: mustGerber(t, boardA)}, true},
		{"按扩展名", File{Name: "board.GKO"}, true},
		{"按文件名", File{Name: "my-edge_cuts.gbr"}, true},
		{"铜箔", File{Name: "top.gbr", Gerber: mustGerber(t, copperA)}, false},
	}
	for _, c := range cases {
		if got := IsOutline(c.file); got != c.want {
			t.Errorf("%s: want %v, got %v", c.name, c.want, got)
		}
	}
}

func TestGroupAttachesMembersToOutline(t *testing.T) {
	files := []File{
		{Name: "edge_cuts.gbr", Gerber: mustGerber(t, boardA)},
		{Name: "top-copper.gbr", Gerber: mustGerber(t, copperA)},
	}
	bundles := Group(files)
	if len(bundles) != 1 {
		t.Fatalf("应归成 1 块板，实际 %d", len(bundles))
	}
	if bundles[0].Outline == nil {
		t.Fatal("应识别出板框")
	}
	if len(bundles[0].Files) != 2 {
		t.Errorf("板框组应含 2 个文件，实际 %d", len(bundles[0].Files))
	}
	if bundles[0].Box.Width() != 10.0 || bundles[0].Box.Height() != 10.0 {
		t.Errorf("板框尺寸不符: %.2f x %.2f", bundles[0].Box.Width(), bundles[0].Box.Height())
	}
}

func TestGroupKeepsFarFileSeparate(t *testing.T) {
	files := []File{
		{Name: "edge_cuts.gbr", Gerber: mustGerber(t, boardA)},
		{Name: "top-copper.gbr", Gerber: mustGerber(t, copperA)},
		{Name: "other.gbr", Gerber: mustGerber(t, copperFar)},
	}
	bundles := Group(files)
	if len(bundles) != 2 {
		t.Fatalf("远处的文件应自成一组，实际 %d 组", len(bundles))
	}
	total := 0
	for _, b := range bundles {
		total += len(b.Files)
	}
	if total != 3 {
		t.Errorf("不应丢文件：期望 3 个，实际 %d", total)
	}
}

func TestGroupWithoutOutlineGroupsByStem(t *testing.T) {
	files := []File{
		{Name: "gerbv_example-boardA-F.Cu.gbr", Gerber: mustGerber(t, copperA)},
		{Name: "gerbv_example-boardA-B.Cu.gbr", Gerber: mustGerber(t, copperA)},
		{Name: "boardB-F.Cu.gbr", Gerber: mustGerber(t, copperA)},
	}
	bundles := Group(files)
	if len(bundles) != 2 {
		t.Fatalf("应按文件名主干分成 2 组，实际 %d", len(bundles))
	}
	names := []string{bundles[0].Name, bundles[1].Name}
	joined := strings.Join(names, "|")
	if !strings.Contains(joined, "gerbv_example-boardA") || !strings.Contains(joined, "boardB") {
		t.Errorf("分组名不符: %v", names)
	}
}

func TestStemKey(t *testing.T) {
	cases := map[string]string{
		"boardA-F.Cu.gbr":  "boardA",
		"boardA-B.Cu.gbr":  "boardA",
		"boardA-mask.gbr":  "boardA",
		"plain.gbr":        "plain",
	}
	for in, want := range cases {
		if got := stemKey(in); got != want {
			t.Errorf("stemKey(%q)=%q, want %q", in, got, want)
		}
	}
}
