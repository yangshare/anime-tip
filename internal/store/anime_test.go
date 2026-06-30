package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	p := filepath.Join(t.TempDir(), "test.db")
	db, err := InitDB(p)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// migrate 应建出 last_notified_episode 列。
func TestMigrate_addsLastNotifiedEpisodeColumn(t *testing.T) {
	db := newTestDB(t)

	var hasCol bool
	err := db.QueryRow(`SELECT COUNT(*) > 0 FROM pragma_table_info('animes') WHERE name='last_notified_episode'`).Scan(&hasCol)
	if err != nil {
		t.Fatalf("查询列: %v", err)
	}
	if !hasCol {
		t.Fatal("期望 animes 表存在 last_notified_episode 列")
	}
}

// 重复打开同一库应幂等无错（CREATE TABLE IF NOT EXISTS 跳过 + ALTER 补列遇 duplicate column 被忽略）。
func TestMigrate_idempotentOnExistingDB(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test.db")
	db1, err := InitDB(p)
	if err != nil {
		t.Fatalf("首次 InitDB: %v", err)
	}
	db1.Close() // 释放文件句柄，避免 Windows 下二次打开与清理冲突
	// 第二次打开同一库：CREATE TABLE IF NOT EXISTS 跳过，ALTER 补列遇「duplicate column」被忽略
	db2, err := InitDB(p)
	if err != nil {
		t.Fatalf("二次 InitDB 应幂等无错: %v", err)
	}
	db2.Close()
}

// UpdateAnimeRemarks 应同时写入 last_notified_episode。
func TestUpdateAnimeRemarks_writesEpisode(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.Exec(`INSERT INTO animes (vod_id, name, current_remarks, last_notified_remarks) VALUES (1, '测试番', '', '')`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var id int64
	if err := db.QueryRow(`SELECT id FROM animes WHERE vod_id=1`).Scan(&id); err != nil {
		t.Fatalf("查 id: %v", err)
	}

	if err := UpdateAnimeRemarks(db, id, "更新至第13集", "更新至第13集", 13); err != nil {
		t.Fatalf("UpdateAnimeRemarks: %v", err)
	}

	var ep int
	if err := db.QueryRow(`SELECT last_notified_episode FROM animes WHERE id=?`, id).Scan(&ep); err != nil {
		t.Fatalf("查 episode: %v", err)
	}
	if ep != 13 {
		t.Errorf("last_notified_episode 期望 13，得到 %d", ep)
	}
}

// ListAnimes 应回填 LastNotifiedEpisode。
func TestListAnimes_returnsEpisode(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.Exec(`INSERT INTO animes (vod_id, name, current_remarks, last_notified_remarks, last_notified_episode) VALUES (1, 'x', '', '', 7)`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	got, err := ListAnimes(db)
	if err != nil {
		t.Fatalf("ListAnimes: %v", err)
	}
	if len(got) != 1 || got[0].LastNotifiedEpisode != 7 {
		t.Fatalf("期望 1 条且 episode=7，得到 %+v", got)
	}
}
