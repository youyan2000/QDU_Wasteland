package nextjlc

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wasteland/internal/tools"
)

// 一套 Altium 命名：GM1 板框 + 顶/底铜 + 阻焊 + 丝印 + 钻孔
const (
	tGM1 = "%FSLAX46Y46*%\nG04 Altium Designer*\n%MOMM*%\nX0Y0D02*\nX10000000Y0D01*\nX10000000Y10000000D01*\nX0Y10000000D01*\nM02*\n"
	tGTL = "%FSLAX46Y46*%\nG04 Altium Designer*\n%MOMM*%\nD10*\nX1000000Y1000000D02*\nX2000000Y2000000D01*\nM02*\n"
	tGBL = "%FSLAX46Y46*%\nG04 Altium Designer*\n%MOMM*%\nX3000000Y3000000D02*\nM02*\n"
	tGTS = "%FSLAX46Y46*%\nG04 Altium Designer*\n%MOMM*%\nX1000000Y1000000D03*\nM02*\n"
	tGBS = "%FSLAX46Y46*%\nG04 Altium Designer*\n%MOMM*%\nX3000000Y3000000D03*\nM02*\n"
	tDRL = "M48\n;TYPE=PLATED\n;FILE_FORMAT=2:4\nMETRIC,TZ\nT01F00S00C0.800\n%\nT01\nX10000Y20000\nM30\n"
)

func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	tool{}.Register(mux, tools.Env{})
	return mux
}

func multipartBody(t *testing.T, files map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for name, content := range files {
		fw, err := w.CreateFormFile("files", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, w.FormDataContentType()
}

func process(t *testing.T, mux *http.ServeMux, files map[string]string) (int, processResponse, string) {
	t.Helper()
	body, ctype := multipartBody(t, files)
	req := httptest.NewRequest(http.MethodPost, "/api/process", body)
	req.Header.Set("Content-Type", ctype)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var out processResponse
	if rec.Code == 200 {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("响应不是 JSON: %v\n%s", err, rec.Body.String())
		}
	}
	return rec.Code, out, rec.Body.String()
}

func TestProcessProducesJLCNamesAndGuide(t *testing.T) {
	mux := newMux()
	code, res, raw := process(t, mux, map[string]string{
		"board.GM1": tGM1, "board.GTL": tGTL, "board.GBL": tGBL,
		"board.GTS": tGTS, "board.GBS": tGBS, "board-holes.txt": tDRL,
	})
	if code != 200 {
		t.Fatalf("process 失败: %d %s", code, raw)
	}
	if res.PrimaryEDA != "Altium" {
		t.Errorf("primaryEda 应为 Altium，实际 %s", res.PrimaryEDA)
	}
	names := map[string]bool{}
	for _, f := range res.Files {
		names[f.NewName] = true
	}
	for _, want := range []string{
		"Gerber_BoardOutlineLayer.GKO", "Gerber_TopLayer.GTL", "Gerber_BottomLayer.GBL",
		"Drill_PTH_Through.DRL", "PCB下单必读.txt",
	} {
		if !names[want] {
			t.Errorf("导出列表应有 %s，实际 %v", want, names)
		}
	}
	if !strings.Contains(res.ZipName, "-AD") {
		t.Errorf("下载名应带 -AD：%s", res.ZipName)
	}
}

func TestProcessAddsSharedHeaderAndFingerprint(t *testing.T) {
	mux := newMux()
	_, res, _ := process(t, mux, map[string]string{
		"b.GM1": tGM1, "b.GTL": tGTL, "b.GTS": tGTS,
	})
	payload, _ := json.Marshal(map[string]string{"session": res.Session})
	req := httptest.NewRequest(http.MethodPost, "/api/download", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("不是有效 zip: %v", err)
	}
	for _, f := range zr.File {
		if f.Name != "Gerber_TopLayer.GTL" {
			continue
		}
		rc, _ := f.Open()
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(rc)
		_ = rc.Close()
		body := buf.String()
		if !strings.HasPrefix(body, "G04 ") {
			t.Errorf("Gerber 应以共享头开头:\n%.80s", body)
		}
		if !strings.Contains(body, "G54D10") {
			t.Errorf("Altium 应补 G54 前缀:\n%s", body)
		}
	}
}

func TestProcessRejectsUnsupportedSoftware(t *testing.T) {
	mux := newMux()
	code, _, raw := process(t, mux, map[string]string{"weird.gbr": "%FSLAX46Y46*%\n%MOMM*%\nX0Y0D02*\nM02*\n"})
	if code != 400 {
		t.Fatalf("未识别软件应 400，实际 %d %s", code, raw)
	}
	if !strings.Contains(raw, "only supported for Altium, KiCad, and EasyEDA") {
		t.Errorf("错误文案应与原版一致: %s", raw)
	}
}

func TestCopyAndRemove(t *testing.T) {
	mux := newMux()
	_, res, _ := process(t, mux, map[string]string{"b.GM1": tGM1, "b.GTL": tGTL, "b.GTS": tGTS})

	// 移除须知
	body, _ := json.Marshal(map[string]string{"session": res.Session, "name": "PCB下单必读.txt"})
	req := httptest.NewRequest(http.MethodPost, "/api/remove", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var removed struct {
		Files []fileDTO `json:"files"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &removed)
	for _, f := range removed.Files {
		if f.NewName == "PCB下单必读.txt" {
			t.Fatal("移除失败")
		}
	}

	// 复制原文件回去
	body2, _ := json.Marshal(map[string]string{"session": res.Session, "name": "b.GTL"})
	req2 := httptest.NewRequest(http.MethodPost, "/api/copy", bytes.NewReader(body2))
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)
	if rec2.Code != 200 {
		t.Fatalf("复制失败: %d %s", rec2.Code, rec2.Body.String())
	}
}