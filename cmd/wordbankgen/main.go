// Command wordbankgen 把 Word Atlas 词库转换成站点内置的精简词库。
//
// 用法：
//
//	go run ./cmd/wordbankgen -src ../word-atlas/data/word-atlas.json -out internal/wordbank/words.json
//	go run ./cmd/wordbankgen -src https://raw.githubusercontent.com/designfuchen/word-atlas/main/data/word-atlas.json
//
// 词库内容版权归 Word Atlas contributors，按 CC BY 4.0 使用。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type sourceEntry struct {
	Word   string `json:"w"`
	UK     string `json:"uk"`
	US     string `json:"us"`
	Senses []struct {
		Pos string `json:"pos"`
		ZH  string `json:"zh"`
		EN  string `json:"en"`
	} `json:"senses"`
}

// Word 是站点实际使用的精简结构（短字段名控制网络体积）。
type Word struct {
	W   string `json:"w"`
	Ph  string `json:"ph,omitempty"`
	Pos string `json:"pos,omitempty"`
	EN  string `json:"en"`
	ZH  string `json:"zh,omitempty"`
}

func main() {
	src := flag.String("src", "", "词库 JSON 的本地路径或 https URL")
	out := flag.String("out", "internal/wordbank/words.json", "输出文件")
	flag.Parse()
	if *src == "" {
		fmt.Fprintln(os.Stderr, "缺少 -src")
		os.Exit(2)
	}
	raw, err := read(*src)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取词库失败:", err)
		os.Exit(1)
	}
	var entries []sourceEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		fmt.Fprintln(os.Stderr, "解析词库失败:", err)
		os.Exit(1)
	}
	seen := make(map[string]bool, len(entries))
	words := make([]Word, 0, len(entries))
	for _, e := range entries {
		w := strings.TrimSpace(e.Word)
		if w == "" || seen[strings.ToLower(w)] {
			continue
		}
		if len(e.Senses) == 0 || strings.TrimSpace(e.Senses[0].EN) == "" {
			continue
		}
		seen[strings.ToLower(w)] = true
		s := e.Senses[0]
		ph := strings.TrimSpace(e.UK)
		if ph == "" {
			ph = strings.TrimSpace(e.US)
		}
		words = append(words, Word{
			W:   w,
			Ph:  ph,
			Pos: strings.TrimSpace(s.Pos),
			EN:  strings.TrimSpace(s.EN),
			ZH:  strings.TrimSpace(s.ZH),
		})
	}
	if len(words) < 100 {
		fmt.Fprintf(os.Stderr, "词条过少（%d），疑似数据源异常，拒绝覆盖\n", len(words))
		os.Exit(1)
	}
	body, err := json.Marshal(words)
	if err != nil {
		fmt.Fprintln(os.Stderr, "序列化失败:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, body, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "写入失败:", err)
		os.Exit(1)
	}
	fmt.Printf("已写入 %s：%d 个词，%.1f KB\n", *out, len(words), float64(len(body))/1024)
}

func read(src string) ([]byte, error) {
	if strings.HasPrefix(src, "https://") || strings.HasPrefix(src, "http://") {
		client := &http.Client{Timeout: 60 * time.Second}
		resp, err := client.Get(src)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	}
	return os.ReadFile(src)
}
