// security_s2_test.go — S2 授权与隐私修复的回归测试。
//
// 覆盖：
//   P1-3 隐私开关语义（显式关闭 vs 从未设置）、读取侧可见性
//   P1-4 待复核/已下架内容不得被直接读取
//   P1-5 封禁即踢会话、后台路径解析（禁言按钮此前完全失效）
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ---------- P1-3 隐私开关 ----------

func TestPrivacyFlagDistinguishesUnsetFromOff(t *testing.T) {
	priv := map[string]any{"posts": float64(0)}

	if got := privacyFlag(priv, "posts", 1); got != 0 {
		t.Fatalf("用户显式关闭(0)必须生效，got %v", got)
	}
	if got := privacyFlag(priv, "articles", 1); got != 1 {
		t.Fatalf("未设置的项应取默认值 1，got %v", got)
	}
	if got := privacyFlag(priv, "age", 0); got != 0 {
		t.Fatalf("未设置的年龄应默认不公开，got %v", got)
	}
	if got := privacyFlag(map[string]any{"age": float64(1)}, "age", 0); got != 1 {
		t.Fatalf("用户显式公开(1)必须生效，got %v", got)
	}
	// JSON 解析出来的数字也可能是 int，不能因此掉回默认值。
	if got := privacyFlag(map[string]any{"posts": 0}, "posts", 1); got != 0 {
		t.Fatalf("int 形式的 0 也应被识别为关闭，got %v", got)
	}
}

// ---------- P1-4 读取侧可见性 ----------

func TestContentVisibleToAnonymousViewer(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/files/1/download", nil)

	cases := []struct {
		status string
		want   bool
	}{
		{"", true},
		{"正常", true},
		{"待复核", false},
		{"已下架", false},
		{"已驳回", false},
		{"draft", false},
	}
	for _, c := range cases {
		if got := contentVisibleTo(r, c.status, 42, 0); got != c.want {
			t.Errorf("状态 %q：期望 %v，实际 %v", c.status, c.want, got)
		}
	}
}

func TestContentVisibleToHidesBannedAuthor(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/forum/post/1", nil)
	if contentVisibleTo(r, "正常", 42, 1) {
		t.Fatal("作者被封禁时，即使状态正常也不应对外可见")
	}
}

// ---------- P1-5 会话失效 ----------

func TestDestroyUserSessionsOnlyTargetUser(t *testing.T) {
	sessionsMu.Lock()
	exp := time.Now().Add(time.Hour)
	sessions["s-a"] = sessionInfo{uid: 101, expires: exp}
	sessions["s-b"] = sessionInfo{uid: 101, expires: exp}
	sessions["s-c"] = sessionInfo{uid: 202, expires: exp}
	sessionsMu.Unlock()
	t.Cleanup(func() {
		sessionsMu.Lock()
		delete(sessions, "s-a")
		delete(sessions, "s-b")
		delete(sessions, "s-c")
		sessionsMu.Unlock()
	})

	if n := destroyUserSessions(101); n != 2 {
		t.Fatalf("应清除 2 个会话，实际 %d", n)
	}
	sessionsMu.Lock()
	_, stillThere := sessions["s-a"]
	_, other := sessions["s-c"]
	sessionsMu.Unlock()
	if stillThere {
		t.Fatal("目标用户的会话未被清除")
	}
	if !other {
		t.Fatal("其他用户的会话不应被清除")
	}
	if n := destroyUserSessions(0); n != 0 {
		t.Fatalf("uid=0 不应清除任何会话，got %d", n)
	}
}

// ---------- 后台路径解析（禁言按钮此前完全失效） ----------

func TestParseUserbanID(t *testing.T) {
	cases := []struct {
		path, action string
		want         int64
		ok           bool
	}{
		// 禁言：当前注册的写法 + 旧写法
		{"/api/admin/userban/mute/5", "mute", 5, true},
		{"/api/admin/userban/5/mute", "mute", 5, true},
		{"/api/admin/userban/mute/5/", "mute", 5, true},
		// 封禁
		{"/api/admin/userban/7", "ban", 7, true},
		{"/api/admin/userban/7/ban", "ban", 7, true},
		// 非法
		{"/api/admin/userban/mute/abc", "mute", 0, false},
		{"/api/admin/userban/", "ban", 0, false},
		{"/api/admin/userban/mute", "mute", 0, false},
		{"/api/admin/userban/-3", "ban", 0, false},
	}
	for _, c := range cases {
		got, ok := parseUserbanID(c.path, c.action)
		if ok != c.ok || got != c.want {
			t.Errorf("%s (action=%s)：期望 (%d,%v)，实际 (%d,%v)", c.path, c.action, c.want, c.ok, got, ok)
		}
	}
}
