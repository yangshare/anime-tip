package web

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// seedAnimeFull 直接插一条带基线/主键/play_url 的全字段记录，
// 便于断言导出裁剪后不含 id/基线/created_at 字段。
func seedAnimeFull(t *testing.T, db *sql.DB, vodID int, name, playURL string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO animes (vod_id, name, cover, current_remarks, last_notified_remarks, last_notified_episode, play_url) VALUES (?, ?, 'https://c/`+name+`', '当前第3集', '已通知第2集', 2, ?)`,
		vodID, name, playURL,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

// callExport 构造一个挂载 ExportAnimes 的 gin 上下文并调用，返回状态码、body、Content-Disposition。
func callExport(t *testing.T, h *Handler) (code int, body []byte, disposition string) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/animes/export", nil)

	h.ExportAnimes(c)

	code = w.Code
	body = w.Body.Bytes()
	disposition = w.Header().Get("Content-Disposition")
	return
}

// ExportAnimes 应返回 {version:1, exported_at, animes}，animes 字段为白名单（无 id/基线/created_at），
// 且 Content-Disposition 含日期文件名。
func TestExportAnimes(t *testing.T) {
	h := newTestHandlerDB(t)
	seedAnimeFull(t, h.db, 28802, "葬送的芙莉莲", "https://play/28802")
	seedAnimeFull(t, h.db, 28803, "鬼灭之刃", "") // 无 play_url

	code, body, disposition := callExport(t, h)
	if code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200", code)
	}

	var got struct {
		Version    int              `json:"version"`
		ExportedAt string           `json:"exported_at"`
		Animes     []map[string]any `json:"animes"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("解析响应: %v; body=%s", err, body)
	}
	if got.Version != 1 {
		t.Errorf("version = %d, 期望 1", got.Version)
	}
	if got.ExportedAt == "" {
		t.Error("exported_at 不应为空")
	}
	if _, err := time.Parse(time.RFC3339, got.ExportedAt); err != nil {
		t.Errorf("exported_at 不是合法 RFC3339: %q (%v)", got.ExportedAt, err)
	}
	if len(got.Animes) != 2 {
		t.Fatalf("animes 长度 = %d, 期望 2", len(got.Animes))
	}

	// 字段白名单：每条只应有 vod_id/name/cover/play_url 四个键
	for i, a := range got.Animes {
		for k := range a {
			switch k {
			case "vod_id", "name", "cover", "play_url":
			default:
				t.Errorf("animes[%d] 含非法字段 %q（应被裁剪）", i, k)
			}
		}
	}

	// 按 vod_id 找到芙莉莲，校验 play_url
	var frieren map[string]any
	for _, a := range got.Animes {
		if a["vod_id"].(float64) == 28802 {
			frieren = a
		}
	}
	if frieren == nil {
		t.Fatal("未找到 vod_id=28802")
	}
	if frieren["play_url"] != "https://play/28802" {
		t.Errorf("play_url = %v, 期望 https://play/28802", frieren["play_url"])
	}
	if frieren["name"] != "葬送的芙莉莲" {
		t.Errorf("name = %v, 期望 葬送的芙莉莲", frieren["name"])
	}

	if disposition == "" {
		t.Fatal("Content-Disposition 不应为空")
	}
	if !strings.Contains(disposition, "attachment;") {
		t.Errorf("Content-Disposition 期望含 attachment;, 实际: %q", disposition)
	}
	if !strings.Contains(disposition, "anime-tip-follows-") {
		t.Errorf("Content-Disposition 期望含 anime-tip-follows- 前缀, 实际: %q", disposition)
	}
	if !strings.Contains(disposition, ".json") {
		t.Errorf("Content-Disposition 期望含 .json 后缀, 实际: %q", disposition)
	}
}

// ExportAnimes 在空关注列表时应返回 animes:[]（非 null）。
func TestExportAnimes_emptyList(t *testing.T) {
	h := newTestHandlerDB(t)

	code, body, _ := callExport(t, h)
	if code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200", code)
	}

	var got struct {
		Version int              `json:"version"`
		Animes  []map[string]any `json:"animes"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("解析响应: %v; body=%s", err, body)
	}
	if got.Version != 1 {
		t.Errorf("version = %d, 期望 1", got.Version)
	}
	if got.Animes == nil || len(got.Animes) != 0 {
		t.Errorf("animes = %v, 期望非 null 空数组", got.Animes)
	}
}
