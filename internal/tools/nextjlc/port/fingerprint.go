// 直译自 src/fingerprint.rs —— 往 Gerber 里插入一个"指纹"光圈，用于溯源。
package port

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// 常量与正则对应 Rust 中的定义。
const fingerprintMaxLen = 30_000_000

var (
	apertureLineRe = regexp.MustCompile(`^%ADD(\d{2,4})\D.*`)
	renumberRe     = regexp.MustCompile(`(?m)^(%ADD|G54D)(\d{2,4})`)
	sizeRe         = regexp.MustCompile(`,([\d.]+)`)
)

// AddFingerprint 对应 add_fingerprint。
func AddFingerprint(gerberContent string, isForeignBoardFile bool) string {
	normalized := strings.ReplaceAll(gerberContent, "\r\n", "\n")
	normalized = strings.TrimPrefix(normalized, "\uFEFF")

	if len(normalized) > fingerprintMaxLen {
		return normalized
	}

	definitions, numbers := scanForApertureDefinitions(normalized)
	if len(definitions) < 5 {
		return normalized
	}

	template, targetNumber, originalNumber := selectInjectionTemplate(definitions, numbers)
	shifted := renumberApertures(normalized, originalNumber)
	dimension := generateHashedDimension(shifted, isForeignBoardFile)
	line := createFingerprintApertureLine(template, targetNumber, originalNumber, dimension)
	return insertNewApertureLine(shifted, line, targetNumber)
}

// scanForApertureDefinitions 只看前 200 行里的 %ADD 定义。
func scanForApertureDefinitions(content string) ([]string, []uint32) {
	var definitions []string
	var numbers []uint32
	lines := strings.Split(content, "\n")
	if len(lines) > 200 {
		lines = lines[:200]
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		m := apertureLineRe.FindStringSubmatch(trimmed)
		if m == nil {
			continue
		}
		var num uint32
		if _, err := fmt.Sscanf(m[1], "%d", &num); err != nil {
			continue
		}
		definitions = append(definitions, trimmed)
		numbers = append(numbers, num)
	}
	return definitions, numbers
}

// selectInjectionTemplate 对应 select_injection_template：
// 按数字分组的第 6 个 id（不足取最后一个），模板取字典序最小的那条。
func selectInjectionTemplate(definitions []string, numbers []uint32) (template string, target, original uint32) {
	byNumber := map[uint32][]string{}
	for i, num := range numbers {
		if i >= len(definitions) {
			break
		}
		byNumber[num] = append(byNumber[num], definitions[i])
	}
	keys := make([]uint32, 0, len(byNumber))
	for k := range byNumber {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	if len(keys) > 5 {
		original = keys[5]
	} else {
		original = keys[len(keys)-1]
	}
	candidates := byNumber[original]
	sort.Strings(candidates)
	template = candidates[0]
	target = original
	return template, target, original
}

// renumberApertures 把 >= originalNumber 的光圈编号整体 +1，给指纹腾位置。
func renumberApertures(content string, originalNumber uint32) string {
	return renumberRe.ReplaceAllStringFunc(content, func(match string) string {
		sub := renumberRe.FindStringSubmatch(match)
		if sub == nil {
			return match
		}
		prefix := sub[1]
		var number uint32
		_, _ = fmt.Sscanf(sub[2], "%d", &number)
		if number >= originalNumber {
			return fmt.Sprintf("%s%d", prefix, number+1)
		}
		return match
	})
}

// generateHashedDimension 用内容哈希算出一个 0.42xx 的尺寸。
func generateHashedDimension(content string, isForeign bool) string {
	hashContent := content
	if isForeign {
		hashContent = "494d" + content
	}
	sum := md5.Sum([]byte(hashContent))
	hexStr := hex.EncodeToString(sum[:])
	lastTwo := hexStr[len(hexStr)-2:]
	var hashNumber uint32
	_, _ = fmt.Sscanf(lastTwo, "%x", &hashNumber)
	hashNumber %= 100
	suffix := fmt.Sprintf("%02d", hashNumber)

	const baseSizeInt = 42
	finalSizeInt := fmt.Sprintf("%02d%s", baseSizeInt, suffix)
	final := fmt.Sprintf("0.%s%s", finalSizeInt[:2], finalSizeInt[2:])
	if final == "0.0000" {
		return "0.0100"
	}
	return final
}

// createFingerprintApertureLine 复制模板形状，替换尺寸与编号。
func createFingerprintApertureLine(template string, targetNumber, originalNumber uint32, finalSize string) string {
	var aperture string
	if loc := sizeRe.FindString(template); loc != "" {
		aperture = strings.ReplaceAll(template, loc, ","+finalSize)
	} else {
		aperture = fmt.Sprintf("%%ADD%dC,%s*%%", targetNumber, finalSize)
	}
	idRe := regexp.MustCompile(fmt.Sprintf("ADD%d", originalNumber))
	return idRe.ReplaceAllString(aperture, fmt.Sprintf("ADD%d", targetNumber))
}

// insertNewApertureLine 把指纹光圈插到相邻定义之后。
func insertNewApertureLine(content, fingerprintLine string, targetNumber uint32) string {
	lines := strings.Split(content, "\n")
	anchor := fmt.Sprintf("%%ADD%d", targetNumber-1)
	out := make([]string, 0, len(lines)+1)
	inserted := false
	for _, line := range lines {
		out = append(out, line)
		if line == anchor || strings.HasPrefix(line, anchor) {
			out = append(out, fingerprintLine)
			inserted = true
		}
	}
	if inserted {
		return strings.Join(out, "\n")
	}

	// 兜底：找到 %MO 之后第一行 %LP 或 G 开头的位置插入。
	final := make([]string, 0, len(lines)+1)
	moFound := false
	inserted = false
	for _, line := range lines {
		if !moFound && strings.HasPrefix(line, "%MO") {
			moFound = true
		} else if moFound && !inserted && (strings.HasPrefix(line, "%LP") || strings.HasPrefix(line, "G")) {
			final = append(final, fingerprintLine)
			inserted = true
		}
		final = append(final, line)
	}
	if !inserted {
		final = append(final, fingerprintLine)
	}
	return strings.Join(final, "\n")
}
