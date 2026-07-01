package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/user/anime-tip/internal/store"
)

// seedAnime 插入一条动漫并返回其 id。
func seedAnime(t *testing.T, h *Handler, vodID int, name string) int64 {
	t.Helper()
	a := struct {
		VodID          int    `json:"vod_id"`
		Name           string `json:"name"`
		Cover          string `json:"cover"`
		CurrentRemarks string `json:"current_remarks"`
	}{vodID, name, "", ""}
	body, _ := json.Marshal(a)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/animes", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.CreateAnime(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("seed CreateAnime: status=%d body=%s", w.Code, w.Body.String())
	}
	// CreateAnime handler 不回填 ID，按 vod_id 反查真实 id。
	got, err := store.GetAnimeByVodID(h.db, vodID)
	if err != nil || got == nil {
		t.Fatalf("seed 后反查 id 失败: %v", err)
	}
	return got.ID
}

func callUpdateAnime(t *testing.T, h *Handler, id int64, payload string) (int, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(id, 10)}}
	c.Request = httptest.NewRequest(http.MethodPatch, "/api/animes/"+strconv.FormatInt(id, 10), bytes.NewReader([]byte(payload)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.UpdateAnime(c)
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w.Code, body
}

// 合法 http(s) 地址应 200 并回填 play_url。
func TestUpdateAnime_validURL(t *testing.T) {
	h := newTestHandlerDB(t)
	id := seedAnime(t, h, 100, "番A")

	code, body := callUpdateAnime(t, h, id, `{"play_url":"https://example.com/play/100"}`)
	if code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200, body=%v", code, body)
	}
	data, _ := body["data"].(map[string]any)
	if data["play_url"] != "https://example.com/play/100" {
		t.Errorf("play_url = %v, 期望 https://example.com/play/100", data["play_url"])
	}

	// 落库校验
	got, err := store.GetAnimeByID(h.db, id)
	if err != nil {
		t.Fatalf("GetAnimeByID: %v", err)
	}
	if got.PlayURL != "https://example.com/play/100" {
		t.Errorf("落库 play_url = %q, 期望同上", got.PlayURL)
	}
}

// 空串应 200（表示清除地址）。
func TestUpdateAnime_emptyURL(t *testing.T) {
	h := newTestHandlerDB(t)
	id := seedAnime(t, h, 101, "番B")
	// 先设非空
	if err := store.UpdateAnimePlayURL(h.db, id, "https://x.test"); err != nil {
		t.Fatalf("preset: %v", err)
	}

	code, _ := callUpdateAnime(t, h, id, `{"play_url":""}`)
	if code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200", code)
	}
	got, _ := store.GetAnimeByID(h.db, id)
	if got.PlayURL != "" {
		t.Errorf("清除后 play_url = %q, 期望空", got.PlayURL)
	}
}

// 非法协议头（非 http/https）应 400。
func TestUpdateAnime_invalidScheme(t *testing.T) {
	h := newTestHandlerDB(t)
	id := seedAnime(t, h, 102, "番C")

	code, body := callUpdateAnime(t, h, id, `{"play_url":"javascript:alert(1)"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, 期望 400, body=%v", code, body)
	}
}

// 超长地址（>2048）应 400。
func TestUpdateAnime_tooLong(t *testing.T) {
	h := newTestHandlerDB(t)
	id := seedAnime(t, h, 103, "番D")

	long := `{"play_url":"https://example.com/` + strings.Repeat("a", 2100) + `"}`
	code, _ := callUpdateAnime(t, h, id, long)
	if code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, 期望 400", code)
	}
}
