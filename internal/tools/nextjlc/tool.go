// tool.go — NeXTJLC：一比一复刻 upstream web/workflow.tsx 的流程。
//
// 处理顺序（与 workflow.tsx 完全一致）：
//  1. 识别每个文件的软件（Altium/KiCad/EasyEDA）
//  2. 按排序后的第一个文件确定 primaryEda（决定命名规则与下载后缀）
//  3. 每个 Gerber：\r\n→\n → 拼上同一个 Gerber 头 → D 码补 G54（KiCad=false/其余=true）
//     → 写入指纹
//  4. 钻孔：合并拆分出 PTH/NPTH，名称为 Drill_PTH_Through.DRL / Drill_NPTH_Through.DRL
//  5. 校验（失败即整体失败）
//  6. 追加「PCB下单必读.txt」到导出列表
//  7. 下载名 = 原 zip 名去扩展 + "-AD|-Ki|-ED" + "-L层数.zip"
package nextjlc

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"wasteland/internal/tools"
	"wasteland/internal/tools/nextjlc/port"
)

func init() { tools.Register(tool{}) }

type tool struct{}

func (tool) Meta() tools.Meta {
	return tools.Meta{
		Slug:    "nextjlc",
		Title:   "NeXTJLC",
		Summary: "把 Gerber 与钻孔文件整理成 JLC 下单格式：JLC 命名、分层校验、PTH/NPTH 分离，一键下载。",
		Label:   "PCB / GERBER",
		Access:  tools.Public,
		Enabled: true,
	}
}

// 导出包里附带的须知文件名（与 workflow.tsx 一致）。
const guideName = "PCB下单必读.txt"

// ---------- 会话 ----------

type entry struct {
	Name string
	Data []byte
}

type session struct {
	entries []entry
	raw     map[string][]byte // 原始文件内容，供「复制到导出列表」
	rename  map[string]string // 原名 → JLC 文件名
	eda     string
	layers  uint32
	zipName string
	expires time.Time
}

var (
	sessMu      sync.Mutex
	sessions    = map[string]*session{}
	sessionTTL  = 10 * time.Minute
	maxSessions = 32
)

func putSession(s *session) string {
	buf := make([]byte, 12)
	_, _ = rand.Read(buf)
	id := hex.EncodeToString(buf)
	sessMu.Lock()
	now := time.Now()
	for k, v := range sessions {
		if v.expires.Before(now) {
			delete(sessions, k)
		}
	}
	if len(sessions) >= maxSessions {
		for k := range sessions {
			delete(sessions, k)
			break
		}
	}
	s.expires = now.Add(sessionTTL)
	sessions[id] = s
	sessMu.Unlock()
	return id
}

func getSession(id string) *session {
	sessMu.Lock()
	defer sessMu.Unlock()
	s, ok := sessions[id]
	if !ok || s.expires.Before(time.Now()) {
		return nil
	}
	return s
}

// ---------- 路由 ----------

func (tool) Register(mux *http.ServeMux, _ tools.Env) {
	mux.HandleFunc("/api/process", handleProcess)
	mux.HandleFunc("/api/copy", handleCopy)
	mux.HandleFunc("/api/remove", handleRemove)
	mux.HandleFunc("/api/download", handleDownload)
	mux.Handle("/", tools.Static{FS: webFS(), CacheControl: "public, max-age=60"})
}

// ---------- 响应 ----------

type fileDTO struct {
	OriginalName string `json:"originalName"`
	NewName      string `json:"newName"`
}

type inputDTO struct {
	Name     string `json:"name"`
	Software string `json:"software"`
}

type processResponse struct {
	Session    string     `json:"session"`
	Inputs     []inputDTO `json:"inputs"`
	Files      []fileDTO `json:"files"`
	PrimaryEDA string    `json:"primaryEda"`
	Layers     uint32    `json:"layers"`
	Errors     []string  `json:"errors"`
	Warnings   []string  `json:"warnings"`
	ZipName    string    `json:"zipName"`
}

func toDTO(s *session) []fileDTO {
	out := make([]fileDTO, 0, len(s.entries))
	for _, e := range s.entries {
		out = append(out, fileDTO{OriginalName: e.Name, NewName: e.Name})
	}
	return out
}

// ---------- 处理 ----------

func handleProcess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeErr(w, 405, "只支持 POST")
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, 400, "上传内容过大或格式错误")
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()

	var files []uploaded
	zipBase := ""
	var total int64
	for _, fhs := range r.MultipartForm.File {
		for _, fh := range fhs {
			src, err := fh.Open()
			if err != nil {
				writeErr(w, 400, "读取上传文件失败")
				return
			}
			data, err := io.ReadAll(io.LimitReader(src, maxUploadBytes))
			_ = src.Close()
			if err != nil {
				writeErr(w, 400, "读取上传文件失败")
				return
			}
			total += int64(len(data))
			if total > 2*maxUploadBytes {
				writeErr(w, 400, "本次上传总量过大，请分批")
				return
			}
			if zipBase == "" && strings.HasSuffix(strings.ToLower(fh.Filename), ".zip") {
				zipBase = strings.TrimSuffix(fh.Filename, ".zip")
				zipBase = strings.TrimSuffix(zipBase, ".ZIP")
			}
			items, err := collectUpload(fh.Filename, data)
			if err != nil {
				writeErr(w, 400, err.Error())
				return
			}
			files = append(files, items...)
		}
	}
	if len(files) == 0 {
		writeErr(w, 400, "没有收到文件")
		return
	}

	// 逐文件识别软件
	raw := map[string][]byte{}
	software := map[string]string{}
	for _, f := range files {
		raw[f.Name] = f.Data
		sw := port.IdentifySoftware(string(headOf(f.Data)))
		if sw == "" {
			// 内容里没有关键字时按命名习惯兜底：KiCad 用 *-F_Cu/*-Edge_Cuts，
			// Altium 用 .GTL/.GBL/.GM1 这类三字母扩展名。
			sw = softwareByName(f.Name)
		}
		if sw == "" {
			sw = "None"
		}
		software[f.Name] = sw
	}

	// primaryEda = 排序后第一个文件的软件（与 workflow.tsx 一致）
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, f.Name)
	}
	ordered := port.SortGerberFiles(names)
	primaryEDA := "None"
	for _, name := range ordered {
		if sw := software[name]; sw != "None" && sw != "" {
			primaryEDA = sw
			break
		}
	}
	if primaryEDA != "Altium" && primaryEDA != "KiCad" && primaryEDA != "EasyEDA" {
		writeErr(w, 400, fmt.Sprintf(
			"Processing is currently only supported for Altium, KiCad, and EasyEDA projects. The primary type detected was %q.",
			primaryEDA))
		return
	}
	edaForMapping := port.EDAAltium
	if primaryEDA == "KiCad" {
		edaForMapping = port.EDAKiCad
	}
	renameMap := port.MapFilenames(names, edaForMapping)

	// 生成一次共用头（原版 sharedHeader 就是在流程里生成一次）
	sharedHeader := port.GetGerberHeader()

	var gerbers, drills []uploaded
	for _, f := range files {
		if port.IsDrillFile(f.Name) {
			drills = append(drills, f)
		} else {
			gerbers = append(gerbers, f)
		}
	}

	var entries []entry
	var warnings []string

	for _, g := range gerbers {
		content := strings.ReplaceAll(string(g.Data), "\r\n", "\n")
		sw := software[g.Name]
		if sw == "Altium" || sw == "KiCad" || sw == "EasyEDA" {
			content = sharedHeader + content
		}
		if sw == "KiCad" {
			content = port.ProcessDCodes(content, port.FlavorKiCad)
		} else if sw == "Altium" || sw == "EasyEDA" {
			content = port.ProcessDCodes(content, port.FlavorAltium)
		}
		if sw == "Altium" || sw == "KiCad" || sw == "EasyEDA" {
			content = port.AddFingerprint(content, false)
		}
		name := renameMap[g.Name]
		if name == "" {
			name = g.Name
		}
		entries = append(entries, entry{Name: name, Data: []byte(content)})
	}

	if len(drills) > 0 {
		contents := make([]string, 0, len(drills))
		dnames := make([]string, 0, len(drills))
		for _, d := range drills {
			contents = append(contents, strings.ReplaceAll(string(d.Data), "\r\n", "\n"))
			dnames = append(dnames, d.Name)
		}
		res := port.ProcessDrillFiles(contents, dnames)
		warnings = append(warnings, res.Warnings...)
		if res.PTHContent != nil {
			entries = append(entries, entry{Name: "Drill_PTH_Through.DRL", Data: []byte(*res.PTHContent)})
		}
		if res.NPTHContent != nil {
			entries = append(entries, entry{Name: "Drill_NPTH_Through.DRL", Data: []byte(*res.NPTHContent)})
		}
	}

	// 校验（用导出名）
	outNames := make([]string, 0, len(entries))
	for _, e := range entries {
		outNames = append(outNames, e.Name)
	}
	report, errs := port.ValidateGerberFiles(outNames)
	if errs != nil {
		writeJSON(w, 200, processResponse{
			Files: []fileDTO{}, PrimaryEDA: primaryEDA,
			Errors: errs, Warnings: []string{},
		})
		return
	}

	// 追加下单须知（导出包里的一个文件）
	hasGuide := false
	for _, e := range entries {
		if e.Name == guideName {
			hasGuide = true
		}
	}
	if !hasGuide {
		entries = append(entries, entry{Name: guideName, Data: []byte(port.GetOrderGuideText())})
	}

	// 下载名：原名-AD|Ki|ED[-L层]
	suffix := "AD"
	switch primaryEDA {
	case "KiCad":
		suffix = "Ki"
	case "EasyEDA":
		suffix = "ED"
	}
	base := zipBase
	if base == "" {
		base = "Gerber"
	}
	zipName := base + "-" + suffix
	if report.LayerCount > 0 {
		zipName += fmt.Sprintf("-L%d", report.LayerCount)
	}
	zipName += ".zip"

	s := &session{
		entries: entries, raw: raw, rename: renameMap,
		eda: primaryEDA, layers: report.LayerCount, zipName: zipName,
	}
	if warnings == nil {
		warnings = []string{}
	}
	inputs := make([]inputDTO, 0, len(files))
	for _, f := range files {
		inputs = append(inputs, inputDTO{Name: f.Name, Software: software[f.Name]})
	}
	writeJSON(w, 200, processResponse{
		Session: putSession(s), Inputs: inputs, Files: toDTO(s), PrimaryEDA: primaryEDA,
		Layers: report.LayerCount, Errors: []string{}, Warnings: warnings, ZipName: zipName,
	})
}

// ---------- 复制单文件到导出列表 / 从导出列表移除 ----------

func handleCopy(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Session string `json:"session"`
		Name    string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	s := getSession(body.Session)
	if s == nil {
		writeErr(w, 409, "结果已过期，请重新上传")
		return
	}
	data, ok := s.raw[body.Name]
	if !ok {
		writeErr(w, 404, "文件不存在")
		return
	}
	name := s.rename[body.Name]
	if name == "" {
		name = body.Name
	}
	for _, e := range s.entries {
		if e.Name == name {
			writeJSON(w, 200, map[string]any{"files": toDTO(s)})
			return
		}
	}
	s.entries = append(s.entries, entry{Name: name, Data: data})
	writeJSON(w, 200, map[string]any{"files": toDTO(s)})
}

func handleRemove(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Session string `json:"session"`
		Name    string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	s := getSession(body.Session)
	if s == nil {
		writeErr(w, 409, "结果已过期，请重新上传")
		return
	}
	kept := s.entries[:0]
	for _, e := range s.entries {
		if e.Name != body.Name {
			kept = append(kept, e)
		}
	}
	s.entries = kept
	writeJSON(w, 200, map[string]any{"files": toDTO(s)})
}

// ---------- 下载 ----------

func handleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeErr(w, 405, "只支持 POST")
		return
	}
	var body struct {
		Session string `json:"session"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	s := getSession(body.Session)
	if s == nil {
		writeErr(w, 409, "结果已过期，请重新上传")
		return
	}
	if len(s.entries) == 0 {
		writeErr(w, 400, "导出列表为空")
		return
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	names := make([]string, 0, len(s.entries))
	byName := map[string][]byte{}
	for _, e := range s.entries {
		names = append(names, e.Name)
		byName[e.Name] = e.Data
	}
	sort.Strings(names)
	for _, name := range names {
		fw, err := zw.Create(name)
		if err != nil {
			writeErr(w, 500, "打包失败")
			return
		}
		if _, err := fw.Write(byName[name]); err != nil {
			writeErr(w, 500, "打包失败")
			return
		}
	}
	if err := zw.Close(); err != nil {
		writeErr(w, 500, "打包失败")
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, s.zipName))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(buf.Bytes())
}

// ---------- 小工具 ----------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// softwareByName 按命名习惯判断 EDA（内容里没有关键字时用）。
func softwareByName(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "f_cu"), strings.Contains(lower, "b_cu"),
		strings.Contains(lower, "edge_cuts"), strings.Contains(lower, "f_mask"),
		strings.Contains(lower, "b_mask"), strings.Contains(lower, "f_silkscreen"),
		strings.Contains(lower, "b_silkscreen"), strings.HasSuffix(lower, ".gbrjob"),
		strings.Contains(lower, "in1_cu"), strings.Contains(lower, "in2_cu"):
		return "KiCad"
	case strings.HasSuffix(lower, ".gtl"), strings.HasSuffix(lower, ".gbl"),
		strings.HasSuffix(lower, ".gts"), strings.HasSuffix(lower, ".gbs"),
		strings.HasSuffix(lower, ".gto"), strings.HasSuffix(lower, ".gbo"),
		strings.HasSuffix(lower, ".gtp"), strings.HasSuffix(lower, ".gbp"),
		strings.HasSuffix(lower, ".gm1"), strings.HasSuffix(lower, ".gm13"),
		strings.HasSuffix(lower, ".tx1"), strings.HasSuffix(lower, ".drr"):
		return "Altium"
	}
	return ""
}