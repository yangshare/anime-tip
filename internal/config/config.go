package config

import "os"

type Config struct {
	Port           string
	CheckCron      string
	ServerChanKey  string
	Keke9BaseURL   string
	DBPath         string
}

func Load() *Config {
	return &Config{
		Port:          getEnv("PORT", "8080"),
		CheckCron:     getEnv("CHECK_INTERVAL", "0 * * * *"),
		ServerChanKey: getEnv("SERVER_CHAN_KEY", ""),
		Keke9BaseURL:  getEnv("KEKE9_BASE_URL", "https://www.keke9.com"),
		DBPath:        getEnv("DB_PATH", "anime-tip.db"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
