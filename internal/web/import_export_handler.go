package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/user/anime-tip/internal/model"
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

// importRequest 为导入请求体，即导出文件的原始结构。前端 JSON.parse 后原样 POST 整个对象。
type importRequest struct {
	Version int             `json:"version"`
	Animes  []importItemRaw `json:"animes"`
}

// importItemRaw 对应导出文件中的单条，字段为白名单。
type importItemRaw struct {
	VodID   int    `json:"vod_id"`
	Name    string `json:"name"`
	Cover   string `json:"cover"`
	PlayURL string `json:"play_url"`
}

const maxImportAnimes = 2000

// ImportAnimes 从导出文件 JSON 导入关注列表。
// 整体校验：version 缺省（0）或 1 通过（其它→400）；animes 长度上限 2000（超→400）。
// 逐条校验：vod_id 正整数、name 非空、play_url 复用 validatePlayURL。
// 非法条计入 errors 不传入 store；通过校验的条目交 store 事务写入。
func (h *Handler) ImportAnimes(c *gin.Context) {
	var req importRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件格式不正确：" + err.Error()})
		return
	}

	// version 校验：缺省（0）视为 1
	if req.Version != 0 && req.Version != 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的导出版本：" + strconv.Itoa(req.Version)})
		return
	}

	if len(req.Animes) > maxImportAnimes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "导入条目过多（最多 2000 条）"})
		return
	}

	// 逐条校验，合法条包装成 ImportItem 传入 store
	items := make([]store.ImportItem, 0, len(req.Animes))
	var errs []store.ImportError

	for i, a := range req.Animes {
		if err := validateImportItem(a); err != nil {
			errs = append(errs, store.ImportError{
				Index: i, VodID: a.VodID, Error: err.Error(),
			})
			continue
		}
		items = append(items, store.ImportItem{
			Index: i,
			Anime: model.Anime{
				VodID:   a.VodID,
				Name:    a.Name,
				Cover:   a.Cover,
				PlayURL: a.PlayURL,
			},
		})
	}

	res, err := store.ImportAnimes(h.db, items)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 合并 handler 校验阶段与 store 写入阶段的 errors
	allErrors := errs
	if res.Errors != nil {
		allErrors = append(allErrors, res.Errors...)
	}
	if allErrors == nil {
		allErrors = []store.ImportError{}
	}
	res.Errors = allErrors

	c.JSON(http.StatusOK, res)
}

// validateImportItem 校验单条导入项。vod_id 正整数；name 非空；play_url 复用 validatePlayURL。
func validateImportItem(a importItemRaw) error {
	if a.VodID <= 0 {
		return fmt.Errorf("vod_id 必须为正整数")
	}
	if strings.TrimSpace(a.Name) == "" {
		return fmt.Errorf("name 不能为空")
	}
	if err := validatePlayURL(a.PlayURL); err != nil {
		return err
	}
	return nil
}
