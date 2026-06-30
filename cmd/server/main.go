package main

import (
	"flag"
	"log"

	"github.com/user/anime-tip/internal/config"
	"github.com/user/anime-tip/internal/crawler"
	"github.com/user/anime-tip/internal/scheduler"
	"github.com/user/anime-tip/internal/store"
	"github.com/user/anime-tip/internal/web"
)

func main() {
	configPath := flag.String("config", "", "配置文件路径（默认 config.yaml，可用 CONFIG_PATH 环境变量覆盖）")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	log.Printf("anime-tip starting on :%s", cfg.Port)

	// 初始化数据库
	db, err := store.InitDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}
	defer db.Close()

	// 如果配置中有 Server Chan Key，写入 settings（支持配置文件和环境变量两种来源）
	if cfg.ServerChanKey != "" {
		if err := store.SetSetting(db, "server_chan_key", cfg.ServerChanKey); err != nil {
			log.Printf("警告: 写入 server_chan_key 失败: %v", err)
		}
	}

	// 初始化 keke9 爬虫客户端
	cr := crawler.NewClient(cfg.Keke9BaseURL)

	// 初始化定时调度器
	sched := scheduler.New(db, cr)
	sched.Start(cfg.CheckCron)

	// 初始化 Gin 路由
	h := web.NewHandler(db, cr, sched)
	r := web.SetupRouter(h)

	log.Printf("anime-tip 已启动，监听 :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("启动 HTTP 服务失败: %v", err)
	}
}
