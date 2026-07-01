package main

import (
	"flag"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"

	"github.com/user/anime-tip/internal/config"
	"github.com/user/anime-tip/internal/crawler"
	"github.com/user/anime-tip/internal/scheduler"
	"github.com/user/anime-tip/internal/store"
	"github.com/user/anime-tip/internal/web"
	"gopkg.in/natefinch/lumberjack.v2"
)

func main() {
	configPath := flag.String("config", "", "配置文件路径（默认 config.yaml，可用 CONFIG_PATH 环境变量覆盖）")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("配置校验失败: %v", err)
	}

	// 配置全局日志输出：同时写 stderr（控制台）和按大小轮转的日志文件。
	// lumberjack 仅按大小触发切割，MaxAge/MaxBackups 是旧文件清理策略。
	if dir := filepath.Dir(cfg.LogFile); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatalf("创建日志目录 %s 失败: %v", dir, err)
		}
	}
	log.SetOutput(io.MultiWriter(os.Stderr, &lumberjack.Logger{
		Filename:   cfg.LogFile,
		MaxSize:    10, // MB
		MaxBackups: 7,
		MaxAge:     7, // 天
		LocalTime:  true,
	}))

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

	// 初始化数据源爬虫客户端（苹果 CMS provide/vod 协议）
	cr := crawler.NewClient(cfg.VodBaseURL, cfg.VodDetailURL)

	// 初始化定时调度器
	sched := scheduler.New(db, cr)
	sched.Start(cfg.CheckCron)

	// 初始化 Gin 路由
	h := web.NewHandler(db, cr, sched)
	r := web.SetupRouter(h)

	log.Printf("anime-tip 已启动（监听 :%s），可通过以下地址访问：", cfg.Port)
	log.Printf("  本机：   http://localhost:%s", cfg.Port)
	for _, ip := range localIPv4s() {
		log.Printf("  局域网： http://%s:%s", ip, cfg.Port)
	}
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("启动 HTTP 服务失败: %v", err)
	}
}

// localIPv4s 返回本机所有非回环的 IPv4 地址，用于在启动日志中输出局域网访问地址。
func localIPv4s() []string {
	var ips []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return ips
	}
	for _, iface := range ifaces {
		// 跳过未启用或回环接口
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			if v4 := ip.To4(); v4 != nil {
				ips = append(ips, v4.String())
			}
		}
	}
	return ips
}
