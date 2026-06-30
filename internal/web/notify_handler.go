package web

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/user/anime-tip/internal/store"
)

// TestNotify 测试 Server 酱通知连通性
func (h *Handler) TestNotify(c *gin.Context) {
	sendKey, err := store.GetSetting(h.db, "server_chan_key")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取配置失败: " + err.Error()})
		return
	}

	sc := h.newNotifier(sendKey)
	if err := sc.Ping(); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"ok":     false,
			"detail": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":     true,
		"detail": "测试消息已发送，请检查微信是否收到",
	})
}
