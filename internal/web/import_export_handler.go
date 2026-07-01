package web

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/user/anime-tip/internal/store"
)

// exportAnime 为导出文件的字段白名单结构：仅 vod_id/name/cover/play_url，
// 明确不含 id/current_remarks/last_notified_*/created_at 等内部与基线字段。
type exportAnime struct {
	VodID   int    `json:"vod_id"`
	Name    string `json:"name"`
	Cover   string `json:"cover"`
	PlayURL string `json:"play_url"`
}

// exportFile 为导出 JSON 顶层结构。
type exportFile struct {
	Version    int           `json:"version"`
	ExportedAt string        `json:"exported_at"`
	Animes     []exportAnime `json:"animes"`
}

// ExportAnimes 导出当前关注列表为 JSON 文件下载。
// 字段裁剪只在 handler 做，model.Anime 与 store 层保持完整表征不动。
func (h *Handler) ExportAnimes(c *gin.Context) {
	animes, err := store.ListAnimes(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	out := exportFile{
		Version:    1,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Animes:     make([]exportAnime, 0, len(animes)),
	}
	for _, a := range animes {
		out.Animes = append(out.Animes, exportAnime{
			VodID:   a.VodID,
			Name:    a.Name,
			Cover:   a.Cover,
			PlayURL: a.PlayURL,
		})
	}

	filename := "anime-tip-follows-" + time.Now().UTC().Format("2006-01-02") + ".json"
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.JSON(http.StatusOK, out)
}
