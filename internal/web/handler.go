package web

import (
	"database/sql"

	"github.com/gin-gonic/gin"
	"github.com/user/anime-tip/internal/crawler"
	"github.com/user/anime-tip/internal/notify"
	"github.com/user/anime-tip/internal/scheduler"
)

// notifier 抽象通知渠道，便于在 handler 层注入测试替身。
type notifier interface {
	Ping() error
}

type Handler struct {
	db          *sql.DB
	crawler     *crawler.Client
	scheduler   *scheduler.Scheduler
	newNotifier func(sendKey string) notifier // 默认走 ServerChan，测试可注入
}

func NewHandler(db *sql.DB, crawler *crawler.Client, sched *scheduler.Scheduler) *Handler {
	return &Handler{
		db:        db,
		crawler:   crawler,
		scheduler: sched,
		newNotifier: func(sendKey string) notifier {
			return notify.NewServerChan(sendKey)
		},
	}
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	r.Static("/static", "./web/static")
	r.StaticFile("/", "./web/index.html")

	api := r.Group("/api")
	{
		api.GET("/animes", h.ListAnimes)
		api.POST("/animes", h.CreateAnime)
		api.DELETE("/animes/:id", h.DeleteAnime)

		api.GET("/search", h.Search)

		api.GET("/settings", h.ListSettings)
		api.PUT("/settings", h.UpdateSettings)

		api.POST("/check", h.TriggerCheck)
		api.POST("/notify/test", h.TestNotify)
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Next()
	}
}

func SetupRouter(h *Handler) *gin.Engine {
	r := gin.Default()
	r.Use(corsMiddleware())
	h.RegisterRoutes(r)
	return r
}
