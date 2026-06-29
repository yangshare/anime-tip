package web

import (
	"net/http"
	"strconv"

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
