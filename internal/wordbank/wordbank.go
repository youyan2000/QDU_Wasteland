// Package wordbank 提供内置的雅思词库（来自 Word Atlas，CC BY 4.0）。
//
// 词库在编译期嵌入二进制，运行时不依赖任何本地目录或外部网络；
// 需要更新时用 cmd/wordbankgen 重新生成 words.json 再重新编译。
package wordbank

import (
	"crypto/rand"
	_ "embed"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
)

//go:embed words.json
var raw []byte

// Word 是一条精简词条。
type Word struct {
	W   string `json:"w"`
	Ph  string `json:"ph,omitempty"`
	Pos string `json:"pos,omitempty"`
	EN  string `json:"en"`
	ZH  string `json:"zh,omitempty"`
}

var words []Word

func init() {
	if err := json.Unmarshal(raw, &words); err != nil {
		panic("wordbank: 内置词库损坏: " + err.Error())
	}
	kept := words[:0]
	for _, w := range words {
		if strings.TrimSpace(w.W) != "" && strings.TrimSpace(w.EN) != "" {
			kept = append(kept, w)
		}
	}
	words = kept
}

// Count 返回可用词条数量。
func Count() int { return len(words) }

// Pick 随机取 n 个互不相同的词条；n 超过词库容量时返回全部（顺序随机）。
func Pick(n int) []Word {
	if n <= 0 || len(words) == 0 {
		return nil
	}
	if n > len(words) {
		n = len(words)
	}
	idx := make([]int, len(words))
	for i := range idx {
		idx[i] = i
	}
	out := make([]Word, 0, n)
	for i := 0; i < n; i++ {
		j := randInt(len(idx))
		out = append(out, words[idx[j]])
		idx[j] = idx[len(idx)-1]
		idx = idx[:len(idx)-1]
	}
	return out
}

func randInt(max int) int {
	if max <= 1 {
		return 0
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0
	}
	return int(v.Int64())
}

// MustJSON 仅用于调试输出。
func MustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
