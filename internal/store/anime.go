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

// ImportItem 承载一条待导入记录及其在原导出数组中的下标，
// 便于 store 写入失败时回填与原文件一致的 error index。
type ImportItem struct {
	Index int
	Anime model.Anime
}

// ImportError 描述一条导入失败的条目。Index 为其在原导出数组中的下标。
type ImportError struct {
	Index int    `json:"index"`
	VodID int    `json:"vod_id"`
	Error string `json:"error"`
}

// ImportResult 汇总一次导入的计数与失败明细。
type ImportResult struct {
	Imported int           `json:"imported"`
	Skipped  int           `json:"skipped"`
	Errors   []ImportError `json:"errors"`
}

// ImportAnimes 在单个事务内逐条导入。
// 传入的 items 应已通过 handler 层逐条校验（vod_id 正整数、name 非空、play_url 合法）。
//   - 已存在 vod_id → Skipped++，保留本地记录与本地 play_url 不变。
//   - 未命中 → 插入，INSERT 含 play_url，进度基线留默认空值/0（「重新关注」语义）。
//   - 单条 SQL 失败 → 记 ImportError 继续下一条，不回滚整批；事务最终提交所有合法条。
//
// 与现有 CreateAnime 的 INSERT（不带 play_url）不同，故单独写语句，不改 CreateAnime。
func ImportAnimes(db *sql.DB, items []ImportItem) (ImportResult, error) {
	result := ImportResult{Errors: []ImportError{}}

	tx, err := db.Begin()
	if err != nil {
		return result, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // 提交成功后 Rollback 无副作用

	for _, item := range items {
		// 判重：命中则跳过，保留本地记录
		var existingID int64
		err := tx.QueryRow(`SELECT id FROM animes WHERE vod_id = ?`, item.Anime.VodID).Scan(&existingID)
		if err == nil {
			result.Skipped++
			continue
		}
		if err != sql.ErrNoRows {
			result.Errors = append(result.Errors, ImportError{
				Index: item.Index, VodID: item.Anime.VodID, Error: fmt.Sprintf("查重失败: %v", err),
			})
			continue
		}

		// 未命中 → 插入，含 play_url，基线留默认
		_, err = tx.Exec(
			`INSERT INTO animes (vod_id, name, cover, current_remarks, last_notified_remarks, last_notified_episode, play_url) VALUES (?, ?, ?, '', '', 0, ?)`,
			item.Anime.VodID, item.Anime.Name, item.Anime.Cover, item.Anime.PlayURL,
		)
		if err != nil {
			result.Errors = append(result.Errors, ImportError{
				Index: item.Index, VodID: item.Anime.VodID, Error: fmt.Sprintf("插入失败: %v", err),
			})
			continue
		}
		result.Imported++
	}

	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("commit tx: %w", err)
	}
	return result, nil
}
