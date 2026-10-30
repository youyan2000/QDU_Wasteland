// ssrf_guard.go — 出网抓图的 SSRF 防护（P1-2）
//
// 规则：
//  1. 站内图片地址不再拼 "http://"+r.Host 回环请求，而是按白名单前缀直接读本地文件；
//  2. 站外地址只允许 http/https，且解析出的每个 IP 都必须是公网地址
//     （拒绝环回、RFC1918、链路本地 169.254/16、CGNAT 100.64/10、ULA、组播、未指定）；
//  3. 真正拨号时用的是「已校验过的 IP」，杜绝 DNS rebinding 在二次解析时换成内网地址；
//  4. 重定向最多 3 跳，每一跳都重新做同样的校验。
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// errInternalImage 表示站内路径不在允许的图片白名单里。
var errInternalImage = errors.New("站内图片地址不受支持")

// internalImageDirs 是允许被「保存到相册」的站内图片前缀。
var internalImageDirs = []struct {
	prefix string
	dir    string
}{
	{"/album-photo-file/", filepath.Join("uploads", "albums")},
	{"/forum-image-file/", filepath.Join("uploads", "forum-images")},
	{"/avatar-file/", filepath.Join("uploads", "avatars")},
}

// loadAlbumImageSource 按地址形态选择读取方式。
func loadAlbumImageSource(raw string) ([]byte, error) {
	if strings.HasPrefix(raw, "/") {
		return readInternalImage(raw)
	}
	return fetchRemoteImage(raw)
}

// readInternalImage 把站内图片 URL 映射成 uploads/ 下的真实文件读取。
func readInternalImage(p string) ([]byte, error) {
	for _, entry := range internalImageDirs {
		if !strings.HasPrefix(p, entry.prefix) {
			continue
		}
		name := strings.TrimPrefix(p, entry.prefix)
		if !isPhotoName(name) || strings.ContainsAny(name, `/\`) {
			return nil, errInternalImage
		}
		full := filepath.Join(entry.dir, name)
		info, err := os.Stat(full)
		if err != nil || !info.Mode().IsRegular() {
			return nil, errors.New("站内图片不存在")
		}
		if info.Size() > 8*1024*1024 {
			return nil, errors.New("图片超过 8MB")
		}
		return os.ReadFile(full)
	}
	return nil, errInternalImage
}

// extForImage 用魔数判断图片类型，返回扩展名；不是已知图片时返回空串。
func extForImage(data []byte) string {
	head := data
	if len(head) > 12 {
		head = head[:12]
	}
	switch {
	case strings.HasPrefix(string(head), "\xff\xd8"):
		return ".jpg"
	case strings.HasPrefix(string(head), "\x89PNG"):
		return ".png"
	case strings.HasPrefix(string(head), "GIF8"):
		return ".gif"
	case strings.HasPrefix(string(head), "RIFF") && len(head) > 8 && string(head[8:12]) == "WEBP":
		return ".webp"
	}
	return ""
}

// isPublicIP 判断一个解析后的 IP 是否可以访问。
func isPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		// 169.254.0.0/16 已由 IsLinkLocalUnicast 覆盖，这里再挡掉保留段。
		if v4[0] == 0 || v4[0] == 127 {
			return false
		}
		// CGNAT 100.64.0.0/10
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return false
		}
		return true
	}
	// IPv6：ULA fc00::/7
	if len(ip) == net.IPv6len && (ip[0]&0xfe) == 0xfc {
		return false
	}
	return true
}

// ssrfSafeClient 返回只允许访问公网地址的 HTTP 客户端。
func ssrfSafeClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy: nil, // 不继承环境代理，避免代理把请求带进内网
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if err != nil || len(ips) == 0 {
				return nil, errors.New("域名解析失败")
			}
			for _, ip := range ips {
				if !isPublicIP(ip) {
					return nil, errors.New("目标地址不允许访问")
				}
			}
			// 用已校验的 IP 拨号：即使解析器在两次查询间换了答案也不会绕过。
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		},
		TLSHandshakeTimeout: 10 * time.Second,
		MaxIdleConns:        8,
		IdleConnTimeout:     30 * time.Second,
	}
	return &http.Client{
		Timeout:   20 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("重定向次数过多")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("重定向地址协议不允许")
			}
			// 目标主机在拨号阶段仍会被校验；这里额外挡掉明显的本机名。
			host := strings.ToLower(req.URL.Hostname())
			if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".internal") {
				return fmt.Errorf("重定向目标不允许")
			}
			return nil
		},
	}
}

// fetchRemoteImage 抓取站外图片（受 SSRF 限制）。
func fetchRemoteImage(raw string) ([]byte, error) {
	u, err := parseHTTPURL(raw)
	if err != nil {
		return nil, err
	}
	resp, err := ssrfSafeClient().Get(u)
	if err != nil {
		return nil, errors.New("图片获取失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("图片获取失败（状态 %d）", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024+1))
	if err != nil {
		return nil, errors.New("图片获取失败")
	}
	if len(data) > 8*1024*1024 {
		return nil, errors.New("图片超过 8MB")
	}
	return data, nil
}

// parseHTTPURL 校验协议与主机名，返回规整后的地址。
func parseHTTPURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("图片地址不合法")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("图片地址不合法")
	}
	if u.Hostname() == "" {
		return "", errors.New("图片地址不合法")
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".internal") {
		return "", errors.New("目标地址不允许访问")
	}
	return u.String(), nil
}
