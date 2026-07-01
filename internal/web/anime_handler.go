package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/user/anime-tip/internal/model"
	"github.com/user/anime-tip/internal/store"
)

func (h *Handler) ListAnimes(c *gin.Context) {
	animes, err := store.ListAnimes(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if animes == nil {
		animes = []model.Anime{}
	}
	c.JSON(http.StatusOK, gin.H{"data": animes})
}

func (h *Handler) CreateAnime(c *gin.Context) {
	var req struct {
		VodID          int    `json:"vod_id" binding:"required"`
		Name           string `json:"name" binding:"required"`
		Cover          string `json:"cover"`
		CurrentRemarks string `json:"current_remarks"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 检查是否已关注
	existing, err := store.GetAnimeByVodID(h.db, req.VodID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "该动漫已在关注列表中"})
		return
	}

	a := &model.Anime{
		VodID:          req.VodID,
		Name:           req.Name,
		Cover:          req.Cover,
		CurrentRemarks: req.CurrentRemarks,
	}
	if err := store.CreateAnime(h.db, a); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": a})
}

func (h *Handler) DeleteAnime(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := store.DeleteAnime(h.db, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已取消关注"})
}

// UpdateAnime 编辑关注动漫的字段（当前仅 play_url）。
// 空串表示清除播放地址；非空必须以 http:// 或 https:// 开头。
func (h *Handler) UpdateAnime(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req struct {
		PlayURL string `json:"play_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := validatePlayURL(req.PlayURL); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := store.UpdateAnimePlayURL(h.db, id, req.PlayURL); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	updated, err := store.GetAnimeByID(h.db, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if updated == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "动漫不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": updated})
}

// validatePlayURL 校验播放地址：空串合法（清除）；非空须 http(s) 且 ≤2048 字符。
func validatePlayURL(u string) error {
	if u == "" {
		return nil
	}
	if len(u) > 2048 {
		return errors.New("播放地址过长（最多 2048 字符）")
	}
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return errors.New("播放地址必须以 http:// 或 https:// 开头")
	}
	return nil
}
