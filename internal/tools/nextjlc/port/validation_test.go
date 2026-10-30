package port

import (
	"strings"
	"testing"
)

func TestValidateRequiresCoreFiles(t *testing.T) {
	report, errs := ValidateGerberFiles([]string{"Gerber_TopLayer.GTL"})
	if len(errs) == 0 {
		t.Fatal("缺板框与阻焊层必须报错")
	}
	joined := strings.Join(errs, "\n")
	if !strings.Contains(joined, "Gerber_BoardOutlineLayer") {
		t.Errorf("应提示缺板框: %v", errs)
	}
	if !strings.Contains(joined, "Gerber_TopSolderMaskLayer") {
		t.Errorf("应提示缺顶层阻焊: %v", errs)
	}
	if report.LayerCount != 0 {
		t.Errorf("失败时层数应为 0，实际 %d", report.LayerCount)
	}
}

func TestValidateCountsLayersAndWarns(t *testing.T) {
	files := []string{
		"Gerber_BoardOutlineLayer.GKO",
		"Gerber_TopLayer.GTL",
		"Gerber_TopSolderMaskLayer.GTS",
		"Gerber_BottomLayer.GBL",
		"Gerber_BottomSolderMaskLayer.GBS",
		"Gerber_InnerLayer1.G1",
		"Gerber_InnerLayer2.G2",
	}
	report, errs := ValidateGerberFiles(files)
	if len(errs) != 0 {
		t.Fatalf("不该报错: %v", errs)
	}
	if report.LayerCount != 4 { // 顶 + 底 + 2 内层
		t.Errorf("层数应为 4，实际 %d", report.LayerCount)
	}
	// 缺顶层丝印/钢网 → 警告而非错误
	if len(report.Warnings) == 0 {
		t.Error("应给出缺丝印/钢网的警告")
	}
}

func TestValidateRejectsTopInnerWithoutBottom(t *testing.T) {
	files := []string{
		"Gerber_BoardOutlineLayer.GKO",
		"Gerber_TopLayer.GTL",
		"Gerber_TopSolderMaskLayer.GTS",
		"Gerber_InnerLayer1.G1",
	}
	_, errs := ValidateGerberFiles(files)
	joined := strings.Join(errs, "\n")
	if !strings.Contains(joined, "layer stackup") {
		t.Errorf("应报层叠错误: %v", errs)
	}
}

const fingerprintSample = `%FSLAX46Y46*%
%MOMM*%
%ADD10C,0.1000*%
%ADD11C,0.2000*%
%ADD12C,0.3000*%
%ADD13C,0.4000*%
%ADD14C,0.5000*%
%ADD15C,0.6000*%
%ADD16C,0.7000*%
G01*
D10*
X0Y0D02*
M02*
`

func TestAddFingerprintInsertsAperture(t *testing.T) {
	got := AddFingerprint(fingerprintSample, false)
	if got == fingerprintSample {
		t.Fatal("应插入指纹光圈")
	}
	// 原实现取「第 6 个编号」作为注入点：10..16 的第 6 个是 15。
	if !strings.Contains(got, "%ADD15C,0.42") {
		t.Errorf("指纹光圈尺寸格式不对（期望 0.42xx）:\n%s", got)
	}
	// 原来的 16 号应被顶到 17，给指纹让位
	if !strings.Contains(got, "%ADD17C,0.7000*%") {
		t.Errorf("原有高编号光圈应整体 +1:\n%s", got)
	}
}

func TestAddFingerprintSkipsSmallFiles(t *testing.T) {
	// 光圈少于 5 个时原实现直接返回，不注入。
	small := "%FSLAX46Y46*%\n%MOMM*%\n%ADD10C,0.1*%\n%ADD11C,0.2*%\nG01*\nD10*\nX0Y0D02*\n"
	if got := AddFingerprint(small, false); got != small {
		t.Errorf("少于 5 个光圈时不应改动内容")
	}
}

func TestAddFingerprintDeterministic(t *testing.T) {
	a := AddFingerprint(fingerprintSample, false)
	b := AddFingerprint(fingerprintSample, false)
	if a != b {
		t.Error("同样内容应得到同样的指纹")
	}
	if a == AddFingerprint(fingerprintSample, true) {
		t.Error("外部板标记不同时指纹应不同")
	}
}
