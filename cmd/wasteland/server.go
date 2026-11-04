package main

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// 本项目的公网入口应由反向代理负责，应用仅接受数字回环地址。
func newHTTPServer(addr string, handler http.Handler) (*http.Server, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		addr = "127.0.0.1:3000"
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("PORT=%q 格式无效（应为 127.0.0.1:3000）: %w", addr, err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return nil, fmt.Errorf("PORT=%q 不是回环地址：请在 systemd 的 Environment/EnvironmentFile 里改成 127.0.0.1:3000（例如 PORT=127.0.0.1:3000），不要在变量前加 Environment= 前缀", addr)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return nil, fmt.Errorf("PORT 必须是 1..65535 的数字端口")
	}
	return &http.Server{
		Addr: addr, Handler: handler,
		ReadHeaderTimeout: 10 * time.Second,
		// 200 MiB upload/downloads need more than five minutes on slower but valid links.
		// Nginx separately bounds idle gaps; this is an absolute upper bound.
		ReadTimeout:    30 * time.Minute,
		WriteTimeout:   30 * time.Minute,
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}, nil
}
