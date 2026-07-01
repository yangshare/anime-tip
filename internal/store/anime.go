package store

import (
	"database/sql"
	"fmt"

	"github.com/user/anime-tip/internal/model"
)

func ListAnimes(db *sql.DB) ([]model.Anime, error) {
	rows, err := db.Query(`SELECT id, vod_id, name, cover, current_remarks, last_notified_remarks, last_notified_episode, play_url, created_at FROM animes ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("query animes: %w", err)
	}
	defer rows.Close()

	var animes []model.Anime
	for rows.Next() {
		var a model.Anime
		if err := rows.Scan(&a.ID, &a.VodID, &a.Name, &a.Cover, &a.CurrentRemarks, &a.LastNotifiedRemarks, &a.LastNotifiedEpisode, &a.PlayURL, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan anime: %w", err)
		}
		animes = append(animes, a)
	}
	return animes, rows.Err()
}

func GetAnimeByVodID(db *sql.DB, vodID int) (*model.Anime, error) {
	var a model.Anime
	err := db.QueryRow(`SELECT id, vod_id, name, cover, current_remarks, last_notified_remarks, last_notified_episode, play_url, created_at FROM animes WHERE vod_id = ?`, vodID).Scan(
		&a.ID, &a.VodID, &a.Name, &a.Cover, &a.CurrentRemarks, &a.LastNotifiedRemarks, &a.LastNotifiedEpisode, &a.PlayURL, &a.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get anime by vod_id: %w", err)
	}
	return &a, nil
}

func CreateAnime(db *sql.DB, a *model.Anime) error {
	_, err := db.Exec(`INSERT INTO animes (vod_id, name, cover, current_remarks, last_notified_remarks, last_notified_episode) VALUES (?, ?, ?, ?, '', 0)`,
		a.VodID, a.Name, a.Cover, a.CurrentRemarks,
	)
	return err
}

func DeleteAnime(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM animes WHERE id = ?`, id)
	return err
}

// UpdateAnimeRemarks 推送成功后写入基线：current_remarks 同步最新抓取值（展示用），
// last_notified_remarks / last_notified_episode 为判定基线。
func UpdateAnimeRemarks(db *sql.DB, id int64, currentRemarks, lastNotifiedRemarks string, lastNotifiedEpisode int) error {
	_, err := db.Exec(`UPDATE animes SET current_remarks = ?, last_notified_remarks = ?, last_notified_episode = ? WHERE id = ?`,
		currentRemarks, lastNotifiedRemarks, lastNotifiedEpisode, id,
	)
	return err
}

// UpdateAnimePlayURL 更新某部动漫的播放地址；空串表示清除。
func UpdateAnimePlayURL(db *sql.DB, id int64, playURL string) error {
	_, err := db.Exec(`UPDATE animes SET play_url = ? WHERE id = ?`, playURL, id)
	return err
}

// GetAnimeByID 按主键取单条，用于更新后回读最新记录。
func GetAnimeByID(db *sql.DB, id int64) (*model.Anime, error) {
	var a model.Anime
	err := db.QueryRow(`SELECT id, vod_id, name, cover, current_remarks, last_notified_remarks, last_notified_episode, play_url, created_at FROM animes WHERE id = ?`, id).Scan(
		&a.ID, &a.VodID, &a.Name, &a.Cover, &a.CurrentRemarks, &a.LastNotifiedRemarks, &a.LastNotifiedEpisode, &a.PlayURL, &a.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get anime by id: %w", err)
	}
	return &a, nil
}
