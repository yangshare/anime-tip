package store

import (
	"database/sql"
	"fmt"

	"github.com/user/anime-tip/internal/model"
)

func ListAnimes(db *sql.DB) ([]model.Anime, error) {
	rows, err := db.Query(`SELECT id, vod_id, name, cover, current_remarks, last_notified_remarks, created_at FROM animes ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("query animes: %w", err)
	}
	defer rows.Close()

	var animes []model.Anime
	for rows.Next() {
		var a model.Anime
		if err := rows.Scan(&a.ID, &a.VodID, &a.Name, &a.Cover, &a.CurrentRemarks, &a.LastNotifiedRemarks, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan anime: %w", err)
		}
		animes = append(animes, a)
	}
	return animes, rows.Err()
}

func GetAnimeByVodID(db *sql.DB, vodID int) (*model.Anime, error) {
	var a model.Anime
	err := db.QueryRow(`SELECT id, vod_id, name, cover, current_remarks, last_notified_remarks, created_at FROM animes WHERE vod_id = ?`, vodID).Scan(
		&a.ID, &a.VodID, &a.Name, &a.Cover, &a.CurrentRemarks, &a.LastNotifiedRemarks, &a.CreatedAt,
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
	_, err := db.Exec(`INSERT INTO animes (vod_id, name, cover, current_remarks, last_notified_remarks) VALUES (?, ?, ?, ?, '')`,
		a.VodID, a.Name, a.Cover, a.CurrentRemarks,
	)
	return err
}

func DeleteAnime(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM animes WHERE id = ?`, id)
	return err
}

func UpdateAnimeRemarks(db *sql.DB, id int64, currentRemarks, lastNotifiedRemarks string) error {
	_, err := db.Exec(`UPDATE animes SET current_remarks = ?, last_notified_remarks = ? WHERE id = ?`,
		currentRemarks, lastNotifiedRemarks, id,
	)
	return err
}
