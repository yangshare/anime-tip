package scheduler

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/user/anime-tip/internal/crawler"
	"github.com/user/anime-tip/internal/store"
)

// stubVodServer 返回一个模拟苹果 CMS vod 接口的 server，固定返回 vod_id=1 的 remarks。
func stubVodServer(t *testing.T, remarks string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := `{"code":1,"msg":"","list":[{"vod_id":1,"vod_name":"x","vod_remarks":"` + remarks + `"}]}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
}

func newTestScheduler(t *testing.T, baseURL string) (*Scheduler, *sql.DB) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "test.db")
	db, err := store.InitDB(p)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	cr := crawler.NewClient(baseURL, "")
	return New(db, cr), db
}

// 首次关注：基线空 → 不应推送（无更新），但应建基线（last_notified_remarks/episode 被推进）。
// 该路径不触发 Server 配推送，避免外网依赖。
func TestCheckUpdates_firstTrackBuildsBaselineNoNotify(t *testing.T) {
	srv := stubVodServer(t, "更新至第5集")
	defer srv.Close()

	sched, db := newTestScheduler(t, srv.URL)
	// seed 一条基线空的关注记录
	if _, err := db.Exec(`INSERT INTO animes (vod_id, name, current_remarks, last_notified_remarks, last_notified_episode) VALUES (1, 'x', '', '', 0)`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// 未配置 server_chan_key，若误推送会因 key 为空报错；此处期望无更新故不推送、无错
	if err := sched.CheckUpdates(); err != nil {
		t.Fatalf("首次检查不应报错: %v", err)
	}

	// 基线应被建立：last_notified_remarks 推进到最新，episode=5
	var remarks string
	var ep int
	if err := db.QueryRow(`SELECT last_notified_remarks, last_notified_episode FROM animes WHERE vod_id=1`).Scan(&remarks, &ep); err != nil {
		t.Fatalf("查基线: %v", err)
	}
	if remarks != "更新至第5集" {
		t.Errorf("基线 remarks 期望 推进到 更新至第5集，得到 %q", remarks)
	}
	if ep != 5 {
		t.Errorf("基线 episode 期望 5，得到 %d", ep)
	}
}

// 集数前进：第二轮 baseEpisode=5 → 6 应触发推送；因无 server_chan_key 推送会失败，
// 此时基线必须保持不变（不推进），验证「推送成功后才落库」。
func TestCheckUpdates_pushFailureKeepsBaseline(t *testing.T) {
	srv := stubVodServer(t, "更新至第6集")
	defer srv.Close()

	sched, db := newTestScheduler(t, srv.URL)
	if _, err := db.Exec(`INSERT INTO animes (vod_id, name, current_remarks, last_notified_remarks, last_notified_episode) VALUES (1, 'x', '更新至第5集', '更新至第5集', 5)`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	err := sched.CheckUpdates()
	if err == nil {
		t.Fatal("期望推送失败返回 error（无 server_chan_key），得到 nil")
	}

	// 推送失败 → 基线保持 5，未被推进到 6
	var ep int
	if err := db.QueryRow(`SELECT last_notified_episode FROM animes WHERE vod_id=1`).Scan(&ep); err != nil {
		t.Fatalf("查基线: %v", err)
	}
	if ep != 5 {
		t.Errorf("推送失败时基线应保持 5，得到 %d", ep)
	}
}
