package web

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *Handler) TriggerCheck(c *gin.Context) {
	go func() {
		_ = h.scheduler.CheckUpdates()
	}()
	c.JSON(http.StatusOK, gin.H{"message": "检查任务已在后台启动"})
}
