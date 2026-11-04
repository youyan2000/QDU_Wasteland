package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// 仅检查配置及合法正常流程，不发送慢请求或复现安全漏洞。
func TestNetworkServerRejectsNonLoopbackConfiguration(t *testing.T) {
	for _, addr := range []string{":3000", "0.0.0.0:3000", "192.0.2.1:3000", "localhost:3000"} {
		if _, err := newHTTPServer(addr, http.NotFoundHandler()); err == nil {
			t.Errorf("expected non-loopback address %q to be rejected", addr)
		}
	}
}

func TestClientRateLimitKeyUsesCanonicalIPNotUserAgent(t *testing.T) {
	first := httptest.NewRequest(http.MethodPost, "/", nil)
	first.RemoteAddr = "203.0.113.10:1000"
	first.Header.Set("User-Agent", "browser-one")
	second := httptest.NewRequest(http.MethodPost, "/", nil)
	second.RemoteAddr = first.RemoteAddr
	second.Header.Set("User-Agent", "browser-two")
	if fpKey(first) != fpKey(second) {
		t.Fatal("rate-limit identity must not vary with User-Agent")
	}
}

func TestNetworkServerConfiguration(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	for _, addr := range []string{"", "127.0.0.1:3000", "[::1]:3000"} {
		srv, err := newHTTPServer(addr, h)
		if err != nil {
			t.Fatal(err)
		}
		if srv.ReadHeaderTimeout != 10*time.Second || srv.ReadTimeout != 30*time.Minute || srv.WriteTimeout != 30*time.Minute || srv.IdleTimeout != 120*time.Second || srv.MaxHeaderBytes != 1<<20 {
			t.Fatalf("unexpected server configuration: %+v", srv)
		}
		rr := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
		if rr.Code != http.StatusNoContent {
			t.Fatal(rr.Code)
		}
	}
}

func TestNetworkLimiterNormalUseAndMaintenance(t *testing.T) {
	rlMu.Lock()
	oldRL, oldMail := rlData, mailQ
	rlData, mailQ = map[string]*rlEntry{}, map[int64]mailQuotaEntry{}
	rlMu.Unlock()
	t.Cleanup(func() { rlMu.Lock(); rlData, mailQ = oldRL, oldMail; rlMu.Unlock() })
	if ok, count := rateLimitOK("normal-client|act", 30); !ok || count != 1 {
		t.Fatalf("normal use: %v %d", ok, count)
	}
	if ok, count := mailQuotaUse(100); !ok || count != 1 {
		t.Fatalf("normal quota: %v %d", ok, count)
	}
	cleanupRateLimits(time.Now().Add(2 * time.Hour))
	rlMu.Lock()
	defer rlMu.Unlock()
	if len(rlData) != 0 || len(mailQ) != 0 {
		t.Fatal("maintenance should remove expired records")
	}
}

func TestRateLimitClearsAfterSuccessfulLogin(t *testing.T) {
	rlMu.Lock()
	old := rlData
	rlData = map[string]*rlEntry{}
	rlMu.Unlock()
	t.Cleanup(func() { rlMu.Lock(); rlData = old; rlMu.Unlock() })

	key := "203.0.113.77|auth"
	for i := 0; i < 20; i++ {
		if ok, _ := rateLimitOK(key, 20); !ok {
			t.Fatalf("第 %d 次应放行", i+1)
		}
	}
	if ok, _ := rateLimitOK(key, 20); ok {
		t.Fatal("超过 20 次应拒绝")
	}
	clearRateLimit(key)
	if ok, _ := rateLimitOK(key, 20); !ok {
		t.Fatal("登录成功后应清零计数")
	}
}
