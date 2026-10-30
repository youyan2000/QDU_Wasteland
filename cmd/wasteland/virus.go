// virus.go — V12 上传文件安全防护
// 三层防护：
//   1. 危险扩展名黑名单（exe/dll/bat/vbs/… 直接拒绝）
//   2. 文件头魔数嗅探（伪装成图片/PDF 的可执行文件截获）
//   3. ClamAV 病毒扫描（可选：设置环境变量 CLAMAV_CMD 指向 clamscan 后自动启用）
package main

import (
	"archive/zip"
	"bytes"
	"context"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// 危险扩展名黑名单（可执行 / 脚本 / 宏载体）
var dangerousExts = map[string]bool{
	".exe": true, ".dll": true, ".com": true, ".scr": true, ".pif": true,
	".bat": true, ".cmd": true, ".msi": true, ".msp": true, ".mst": true,
	".vbs": true, ".vbe": true, ".js": true, ".jse": true, ".wsf": true, ".wsh": true,
	".ps1": true, ".psm1": true, ".sh": true, ".bash": true, ".csh": true, ".ksh": true,
	".hta": true, ".reg": true, ".lnk": true, ".gadget": true, ".jar": true, ".apk": true,
	".html": true, ".htm": true, ".svg": true, // 可携带脚本的文本格式，防止 XSS 劫持
	// Office 宏文档（可内嵌 VBA 宏，即使无杀毒库也必须拦截）
	".docm": true, ".dotm": true, ".xlsm": true, ".xltm": true, ".xlam": true,
	".pptm": true, ".potm": true, ".ppam": true, ".sldm": true, ".xlsb": true,
}

// 危险文件头魔数（防伪装）
var dangerousMagics = []struct {
	head []byte
	name string
}{
	{[]byte("MZ"), "Windows 可执行程序(PE)"},          // 0x4D 0x5A
	{[]byte{0x7F, 'E', 'L', 'F'}, "Linux 可执行文件(ELF)"},
	{[]byte{0xCA, 0xFE, 0xBA, 0xBE}, "Java 字节码"},
	{[]byte{0xFE, 0xED, 0xFA, 0xCE}, "macOS 可执行文件(Mach-O)"},
	{[]byte{0xFE, 0xED, 0xFA, 0xCF}, "macOS 可执行文件(Mach-O)"},
	{[]byte("#!"), "脚本文件(Shebang)"},
	{[]byte{0x4F, 0x67, 0x67, 0x53}, "Ogg(可含视频/恶意Payload)"}, // 保守拦截,防OGG漏洞投递
}

// checkDangerousFile 返回 (是否危险, 原因)。优先文件名和头部双检。
func checkDangerousFile(fileName string, data []byte) (bool, string) {
	ext := strings.ToLower(filepath.Ext(fileName))
	if dangerousExts[ext] {
		return true, "禁止上传 " + ext + " 可执行/脚本文件"
	}
	// 魔数嗅探：只对"声称无害但头部危险"的文件拦截
	head := data
	if len(head) > 16 {
		head = head[:16]
	}
	for _, m := range dangerousMagics {
		if bytes.HasPrefix(head, m.head) {
			// shebang 特例：仅当扩展名是脚本类时才拦，
			// 避免把"以 #! 开头的教学笔记(.md/.txt)"误拦
			if m.name == "脚本文件(Shebang)" {
				scriptExt := map[string]bool{
					".sh": true, ".bash": true, ".csh": true, ".ksh": true,
					".py": true, ".pl": true, ".rb": true, ".php": true,
					".awk": true, ".tcl": true, ".zsh": true, ".fish": true,
				}
				if !scriptExt[ext] {
					continue
				}
				return true, "文件内容为 " + m.name + "，禁止上传"
			}
			return true, "文件内容为 " + m.name + "，禁止上传"
		}
	}
	// zip 容器（docx/xlsx/pptx/zip…）深度嗅探 OLE 宏条目
	if zipHasMacro(data) {
		return true, "检测到 Office 宏(vbaProject)，禁止上传"
	}
	return false, ""
}

// zipHasMacro 检查 zip 包里是否含 OLE 宏条目（vbaProject.bin / vbaProjectSignature.bin）
func zipHasMacro(data []byte) bool {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false // 不是合法 zip，不判宏
	}
	low := func(s string) string { return strings.ToLower(s) }
	for _, f := range zr.File {
		n := low(f.Name)
		if strings.Contains(n, "vbaproject") || strings.Contains(n, "vba_data.xml") || strings.HasSuffix(n, ".bin") && (strings.Contains(n, "/macros/") || strings.Contains(n, "macros/")) {
			return true
		}
	}
	return false
}

// ---- ClamAV 可选集成 ----
// 环境变量 CLAMAV_CMD（如 "/usr/bin/clamscan"）设置后启用；
// 未设置时跳过扫描（记日志），不阻塞上传。
// 返回 (是否干净, 描述)
func clamavScan(data []byte, fileName string) (bool, string) {
	cmdPath := os.Getenv("CLAMAV_CMD")
	if cmdPath == "" {
		return true, ""
	}
	// 写到临时文件再扫描（clamscan 需要文件路径）
	tmp, err := os.CreateTemp("", "qdu-scan-*")
	if err != nil {
		log.Println("[ClamAV] 临时文件创建失败:", err)
		return true, ""
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if _, err := tmp.Write(data); err != nil {
		log.Println("[ClamAV] 写入临时文件失败:", err)
		return true, ""
	}
	tmp.Sync()

	// 超时保护：大文件扫描可能很慢，60 秒超时后放行并记录（避免阻塞请求）
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, cmdPath, "--no-summary", "--quiet", tmp.Name()).CombinedOutput()
	// clamscan 退出码: 0=干净 1=发现病毒 2=错误
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if ok && exitErr.ExitCode() == 1 {
			msg := strings.TrimSpace(string(out))
			if msg == "" {
				msg = "检测到病毒或恶意内容"
			}
			return false, msg
		}
		// 超时：记录并放行（避免长时间阻塞上传请求）
		if ctx.Err() != nil {
			log.Printf("[ClamAV] 扫描超时已放行: %s", fileName)
			return true, ""
		}
		// 其他错误（2=引擎错误等）：保守起见 记录但放行（避免误伤正常使用）
		log.Printf("[ClamAV] 扫描出错(%v) 已放行: %s", err, fileName)
		return true, ""
	}
	return true, ""
}