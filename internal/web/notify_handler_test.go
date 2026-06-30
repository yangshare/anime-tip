package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/user/anime-tip/internal/store"
)

// fakeNotifier 注入 handler 的测试替身，按预设返回 Ping 结果。
type fakeNotifier struct{ err error }

func (f *fakeNotifier) Ping() error { return f.err }

func newTestHandlerDB(t *testing.T) *Handler {
	t.Helper()
	db, err := store.InitDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewHandler(db, nil, nil)
}

func callTestNotify(t *testing.T, h *Handler) (code int, body map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/notify/test", nil)

	h.TestNotify(c)

	code = w.Code
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return code, body
}

// 未配置 SendKey 时，应返回 ok:false（走真实 ServerChan 的空 key 校验）。
func TestTestNotify_missingKey(t *testing.T) {
	h := newTestHandlerDB(t)
	// 故意不预置 server_chan_key，使用默认 newNotifier。

	code, body := callTestNotify(t, h)
	if code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200", code)
	}
	if body["ok"] != false {
		t.Fatalf("ok = %v, 期望 false", body["ok"])
	}
	detail, _ := body["detail"].(string)
	if !strings.Contains(detail, "empty") {
		t.Fatalf("detail 期望包含 'empty', 实际: %q", detail)
	}
}

// Ping 成功时返回 ok:true。
func TestTestNotify_success(t *testing.T) {
	h := newTestHandlerDB(t)
	if err := store.SetSetting(h.db, "server_chan_key", "test-key"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	h.newNotifier = func(string) notifier { return &fakeNotifier{err: nil} }

	code, body := callTestNotify(t, h)
	if code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200", code)
	}
	if body["ok"] != true {
		t.Fatalf("ok = %v, 期望 true", body["ok"])
	}
}

// Ping 失败时返回 ok:false 且 detail 含错误信息。
func TestTestNotify_pingFail(t *testing.T) {
	h := newTestHandlerDB(t)
	if err := store.SetSetting(h.db, "server_chan_key", "test-key"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	h.newNotifier = func(string) notifier {
		return &fakeNotifier{err: errFake("server chan 失败(code=40001): 错误的Key")}
	}

	code, body := callTestNotify(t, h)
	if code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200", code)
	}
	if body["ok"] != false {
		t.Fatalf("ok = %v, 期望 false", body["ok"])
	}
	detail, _ := body["detail"].(string)
	if !strings.Contains(detail, "40001") {
		t.Fatalf("detail 期望包含 '40001', 实际: %q", detail)
	}
}

type errFake string

func (e errFake) Error() string { return string(e) }
