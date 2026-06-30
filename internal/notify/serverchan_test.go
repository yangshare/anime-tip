package notify

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPing_emptyKey(t *testing.T) {
	sc := NewServerChan("")
	err := sc.Ping()
	if err == nil {
		t.Fatal("期望返回错误，但得到 nil")
	}
	want := "server chan sendkey is empty"
	if err.Error() != want {
		t.Fatalf("错误信息 = %q, 期望 %q", err.Error(), want)
	}
}

func TestPing_success(t *testing.T) {
	// 模拟 ServerChan API 返回 200 + code:0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":0,"message":"success"}`))
	}))
	defer srv.Close()

	sc := NewServerChan("test-key")
	sc.apiBase = srv.URL

	if err := sc.Ping(); err != nil {
		t.Fatalf("期望返回 nil，但得到错误: %v", err)
	}
}

func TestPing_invalidKey(t *testing.T) {
	// 贴近真实行为：无效 Key 时 Server 酱返回 HTTP 400 + code:40001
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"message":"[AUTH]错误的Key","code":40001,"info":"错误的Key","scode":461}`))
	}))
	defer srv.Close()

	sc := NewServerChan("bad-key")
	sc.apiBase = srv.URL

	err := sc.Ping()
	if err == nil {
		t.Fatal("期望返回错误，但得到 nil")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Fatalf("期望错误信息包含状态码 400，实际: %v", err)
	}
}

// 限流/业务失败场景：HTTP 200 但 code!=0，仍应判定失败。
func TestPing_businessError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":40002,"message":"触发频率限制"}`))
	}))
	defer srv.Close()

	sc := NewServerChan("some-key")
	sc.apiBase = srv.URL

	err := sc.Ping()
	if err == nil {
		t.Fatal("期望返回错误，但得到 nil")
	}
	if !strings.Contains(err.Error(), "40002") {
		t.Fatalf("期望错误信息包含业务码 40002，实际: %v", err)
	}
}

func TestPing_serverError(t *testing.T) {
	// 模拟 ServerChan API 返回 500
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	sc := NewServerChan("some-key")
	sc.apiBase = srv.URL

	err := sc.Ping()
	if err == nil {
		t.Fatal("期望返回错误，但得到 nil")
	}
}

// 空更新列表应直接返回 nil，且不发起网络请求。
func TestSend_emptyItems(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":0}`))
	}))
	defer srv.Close()

	sc := NewServerChan("some-key")
	sc.apiBase = srv.URL

	if err := sc.Send(nil); err != nil {
		t.Fatalf("期望返回 nil，但得到错误: %v", err)
	}
	if called {
		t.Fatal("空列表不应发起网络请求")
	}
}

func TestSend_success(t *testing.T) {
	var gotTitle, gotDesp string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotTitle = r.PostForm.Get("title")
		gotDesp = r.PostForm.Get("desp")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":0,"message":"success"}`))
	}))
	defer srv.Close()

	sc := NewServerChan("test-key")
	sc.apiBase = srv.URL

	items := []UpdateItem{
		{Name: "葬送的芙莉莲", Remarks: "第12集", DetailURL: "https://example.com/1"},
		{Name: "咒术回战", Remarks: "更新至第5集", DetailURL: "https://example.com/2"},
	}
	if err := sc.Send(items); err != nil {
		t.Fatalf("期望返回 nil，但得到错误: %v", err)
	}

	if gotTitle != "🍥 动漫更新提醒" {
		t.Fatalf("title = %q, 期望 %q", gotTitle, "🍥 动漫更新提醒")
	}
	// desp 应包含每个番剧名与进度。
	for _, want := range []string{"《葬送的芙莉莲》更新了！", "第12集", "《咒术回战》更新了！", "更新至第5集"} {
		if !strings.Contains(gotDesp, want) {
			t.Errorf("desp 缺少片段 %q\ndesp 实际内容:\n%s", want, gotDesp)
		}
	}
}
