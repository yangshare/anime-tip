package main

import (
	"fmt"
	"log"

	"github.com/user/anime-tip/internal/config"
)

func main() {
	cfg := config.Load()
	log.Printf("anime-tip starting on :%s", cfg.Port)
	log.Printf("check cron: %s", cfg.CheckCron)
	fmt.Printf("keke9 base url: %s\n", cfg.Keke9BaseURL)
}
