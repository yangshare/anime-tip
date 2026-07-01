package web

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/user/anime-tip/internal/store"
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

// callImport 构造一个挂载 ImportAnimes 的 gin 上下文，POST jsonBody，返回状态码与解析后的 body。
func callImport(t *testing.T, h *Handler, jsonBody string) (code int, body map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/animes/import", strings.NewReader(jsonBody))
	c.Request.Header.Set("Content-Type", "application/json")

	h.ImportAnimes(c)

	code = w.Code
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return
}

// ImportAnimes 合法批（含一条已存在）→ 200，imported/skipped 正确，且不覆盖已存在条本地 play_url。
func TestImportAnimes_validBatch(t *testing.T) {
	h := newTestHandlerDB(t)
	seedAnimeFull(t, h.db, 100, "已存在番", "https://keep/100")

	body := `{"version":1,"exported_at":"2026-07-01T12:00:00Z","animes":[
		{"vod_id":100,"name":"已存在-导入侧","cover":"","play_url":"https://overwrite/100"},
		{"vod_id":200,"name":"全新番","cover":"https://c/200","play_url":"https://play/200"}
	]}`
	code, resp := callImport(t, h, body)
	if code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200, body=%v", code, resp)
	}
	if got := int(resp["imported"].(float64)); got != 1 {
		t.Errorf("imported = %d, 期望 1", got)
	}
	if got := int(resp["skipped"].(float64)); got != 1 {
		t.Errorf("skipped = %d, 期望 1", got)
	}
	if errs, ok := resp["errors"].([]any); ok && len(errs) != 0 {
		t.Errorf("errors = %v, 期望空", errs)
	}

	// 已存在条本地 play_url 不被覆盖
	existing, _ := store.GetAnimeByVodID(h.db, 100)
	if existing.PlayURL != "https://keep/100" {
		t.Errorf("已存在条 play_url = %q, 期望保持 https://keep/100", existing.PlayURL)
	}
}

// ImportAnimes version 非 1 → 400。
func TestImportAnimes_unsupportedVersion(t *testing.T) {
	h := newTestHandlerDB(t)
	body := `{"version":2,"exported_at":"","animes":[]}`
	code, resp := callImport(t, h, body)
	if code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, 期望 400", code)
	}
	if !strings.Contains(resp["error"].(string), "版本") {
		t.Errorf("error = %v, 期望含 '版本'", resp["error"])
	}
}

// version 缺省应视为 1，正常导入。
func TestImportAnimes_missingVersionTreatedAs1(t *testing.T) {
	h := newTestHandlerDB(t)
	body := `{"animes":[{"vod_id":999,"name":"无版本字段番","cover":"","play_url":""}]}`
	code, resp := callImport(t, h, body)
	if code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200, body=%v", code, resp)
	}
	if got := int(resp["imported"].(float64)); got != 1 {
		t.Errorf("imported = %d, 期望 1", got)
	}
}

// 数组超上限 2000 → 400。
func TestImportAnimes_overLimit(t *testing.T) {
	h := newTestHandlerDB(t)
	var sb strings.Builder
	sb.WriteString(`{"version":1,"animes":[`)
	for i := 0; i < 2001; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(`{"vod_id":`)
		sb.WriteString(strconv.Itoa(i + 1))
		sb.WriteString(`,"name":"n","cover":"","play_url":""}`)
	}
	sb.WriteString(`]}`)

	code, resp := callImport(t, h, sb.String())
	if code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, 期望 400", code)
	}
	if !strings.Contains(resp["error"].(string), "2000") {
		t.Errorf("error = %v, 期望含 '2000'", resp["error"])
	}
}

// ImportAnimes 含逐条非法条（name 空 / vod_id≤0 / play_url 非法）→ 该条进 errors，其余正常导入。
func TestImportAnimes_perItemValidation(t *testing.T) {
	h := newTestHandlerDB(t)
	body := `{"version":1,"animes":[
		{"vod_id":1,"name":"","cover":"","play_url":""},
		{"vod_id":0,"name":"零id","cover":"","play_url":""},
		{"vod_id":-5,"name":"负id","cover":"","play_url":""},
		{"vod_id":2,"name":"非法url","cover":"","play_url":"ftp://x"},
		{"vod_id":3,"name":"合法番","cover":"","play_url":"https://play/3"}
	]}`
	code, resp := callImport(t, h, body)
	if code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200, body=%v", code, resp)
	}
	if got := int(resp["imported"].(float64)); got != 1 {
		t.Errorf("imported = %d, 期望 1（仅 vod_id=3 合法）", got)
	}
	errs, _ := resp["errors"].([]any)
	if len(errs) != 4 {
		t.Fatalf("errors 长度 = %d, 期望 4, 实际 %v", len(errs), errs)
	}
	// 校验每条 error 的 index 与 vod_id
	wantIndices := map[int]int{0: 1, 1: 0, 2: -5, 3: 2}
	gotIndices := map[int]int{}
	for _, e := range errs {
		em, _ := e.(map[string]any)
		idx := int(em["index"].(float64))
		vid := int(em["vod_id"].(float64))
		gotIndices[idx] = vid
	}
	for idx, vid := range wantIndices {
		if gotIndices[idx] != vid {
			t.Errorf("index=%d 的 vod_id = %d, 期望 %d", idx, gotIndices[idx], vid)
		}
	}
	// 非法条不应入库
	if a, _ := store.GetAnimeByVodID(h.db, 2); a != nil {
		t.Error("vod_id=2（非法url）不应入库")
	}
	if a, _ := store.GetAnimeByVodID(h.db, 3); a == nil {
		t.Error("vod_id=3（合法）应入库")
	}
}

// ImportAnimes 请求体非合法 JSON → 400。
func TestImportAnimes_badJSON(t *testing.T) {
	h := newTestHandlerDB(t)
	code, resp := callImport(t, h, `{not json`)
	if code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, 期望 400", code)
	}
	if !strings.Contains(resp["error"].(string), "格式") {
		t.Errorf("error = %v, 期望含 '格式'", resp["error"])
	}
}

// TestImportExportRoundTrip：导出 → 清库 → 导入 → 关注清单还原（vod_id/name/cover/play_url 一致，基线重置为默认）。
func TestImportExportRoundTrip(t *testing.T) {
	h := newTestHandlerDB(t)
	seedAnimeFull(t, h.db, 111, "番A", "https://play/a")
	seedAnimeFull(t, h.db, 222, "番B", "")

	// 导出
	_, exportBody, _ := callExport(t, h)

	// 清库
	if _, err := h.db.Exec(`DELETE FROM animes`); err != nil {
		t.Fatalf("清库: %v", err)
	}

	// 用导出内容原样导入
	code, resp := callImport(t, h, string(exportBody))
	if code != http.StatusOK {
		t.Fatalf("导入状态码 = %d, 期望 200, body=%v", code, resp)
	}
	if got := int(resp["imported"].(float64)); got != 2 {
		t.Errorf("imported = %d, 期望 2", got)
	}

	// 还原校验
	a111, _ := store.GetAnimeByVodID(h.db, 111)
	if a111 == nil {
		t.Fatal("vod_id=111 未还原")
	}
	if a111.Name != "番A" || a111.Cover != "https://c/番A" || a111.PlayURL != "https://play/a" {
		t.Errorf("111 还原不一致: %+v", a111)
	}
	// 基线应被重置为默认（导出文件不含基线，导入后重新关注）
	if a111.CurrentRemarks != "" || a111.LastNotifiedRemarks != "" || a111.LastNotifiedEpisode != 0 {
		t.Errorf("111 基线未重置: %+v", a111)
	}

	a222, _ := store.GetAnimeByVodID(h.db, 222)
	if a222 == nil {
		t.Fatal("vod_id=222 未还原")
	}
	if a222.Name != "番B" || a222.PlayURL != "" {
		t.Errorf("222 还原不一致: %+v", a222)
	}
}

// 路由注册：SetupRouter 后 GET /api/animes/export 与 POST /api/animes/import 应可达。
func TestRoutes_exportAndImportRegistered(t *testing.T) {
	h := newTestHandlerDB(t)
	r := SetupRouter(h)

	// 导出
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/animes/export", nil))
	if w.Code != http.StatusOK {
		t.Errorf("GET /api/animes/export 状态码 = %d, 期望 200", w.Code)
	}

	// 导入空批
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodPost, "/api/animes/import", strings.NewReader(`{"version":1,"animes":[]}`)))
	if w2.Code != http.StatusOK {
		t.Errorf("POST /api/animes/import 状态码 = %d, 期望 200", w2.Code)
	}
}
