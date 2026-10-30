// 直译自 src/header.rs —— 生成随机化的 Gerber / 钻孔文件头。
package port

import (
	"fmt"
	"math/rand"
	"time"
)

// headerInfo 对应 Rust 的 HeaderInfo。
type headerInfo struct {
	software  string
	version   string
	timestamp string
}

// generateHeaderInfo 对应 generate_header_info：随机的软件名与版本号 + 当前时间。
func generateHeaderInfo() headerInfo {
	software := "EasyEDA"
	if rand.Intn(2) == 0 { // random_bool(0.5)
		software = "EasyEDA Pro"
	}
	major := 2 + rand.Intn(2)      // 2..=3
	minor := 1 + rand.Intn(5)      // 1..=5
	patch := 1 + rand.Intn(42)     // 1..=42
	build := rand.Intn(3)          // 0..=2
	return headerInfo{
		software:  software,
		version:   fmt.Sprintf("v%d.%d.%d.%d", major, minor, patch, build),
		timestamp: time.Now().Format("2006-01-02 15:04:05"),
	}
}

// GetGerberHeader 对应 get_gerber_header：G04 注释格式。
func GetGerberHeader() string {
	info := generateHeaderInfo()
	return fmt.Sprintf("G04 %s %s, %s*\nG04 Gerber Generator version 0.3*\n",
		info.software, info.version, info.timestamp)
}

// GetDrillHeader 对应 get_drill_header：分号注释格式。
// holeType 为 "PLATED" / "NON_PLATED"，layerName 如 "PTH_Through" / "NPTH_Through"。
func GetDrillHeader(holeType, layerName string) string {
	info := generateHeaderInfo()
	return fmt.Sprintf(";TYPE=%s\n;Layer: %s\n;%s %s, %s\n;Gerber Generator version 0.3\n",
		holeType, layerName, info.software, info.version, info.timestamp)
}

// GetOrderGuideText 对应 get_order_guide_text（原文照抄）。
func GetOrderGuideText() string {
	return `如何进行PCB下单

请查看：
https://prodocs.lceda.cn/cn/pcb/order-order-pcb/index.html`
}
