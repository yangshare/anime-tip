package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/goccy/go-yaml"
)

type Config struct {
	Port          string `yaml:"port"`
	CheckCron     string `yaml:"check_cron"`
	ServerChanKey string `yaml:"server_chan_key"`
	VodBaseURL    string `yaml:"vod_base_url"`
	VodDetailURL  string `yaml:"vod_detail_url"`
	DBPath        string `yaml:"db_path"`
	// Keke9BaseURL 旧版（keke9 数据源）配置键的兼容别名，
	// 仅用于平滑迁移：未显式设置 vod_base_url 时回退到该值。新配置请用 vod_base_url。
	Keke9BaseURL string `yaml:"keke9_base_url"`
}

// Load 从配置文件与环境变量加载配置。
//
// flag 解析由调用方（main）完成，路径通过 configPath 传入，避免 flag.Parse
// 与 testing 包冲突，同时使 Load 可被单元测试。
//
// 配置优先级：默认值 < 配置文件 < 环境变量（环境变量优先级最高）。
func Load(configPath string) (*Config, error) {
	// 1. 默认值（VodBaseURL 的默认值在别名解析后统一兜底，便于检测用户是否显式设置）
	cfg := &Config{
		Port:      "8080",
		CheckCron: "0 * * * *",
		DBPath:    "anime-tip.db",
	}

	// 2. 从 YAML 配置文件读取
	path := resolveConfigPath(configPath)
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		// 文件存在；解析失败视为致命错误，避免半截配置进入系统
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("解析配置文件 %s 失败: %w", path, err)
		}
	case errors.Is(err, os.ErrNotExist):
		// 默认 config.yaml 不存在属正常情况，静默跳过；
		// 若是显式指定的路径不存在，给出提示但不致命
		if configPath != "" || os.Getenv("CONFIG_PATH") != "" {
			fmt.Fprintf(os.Stderr, "提示: 配置文件 %s 不存在，使用默认值与环境变量\n", path)
		}
	default:
		// 其它读取错误（如权限不足）需上抛
		return nil, fmt.Errorf("读取配置文件 %s 失败: %w", path, err)
	}

	// 3. 环境变量覆盖（优先级最高）
	overrideFromEnv(cfg)

	// 4. 旧版数据源配置键的向后兼容：
	//    - KEKE9_BASE_URL 环境变量在未设 VOD_BASE_URL 时作为数据源地址（存量 Docker/CI 平滑迁移）；
	//    - keke9_base_url YAML 键同理。
	//    - VodBaseURL 仍未显式设置（空）时，回退到内置默认数据源。
	if cfg.VodBaseURL == "" {
		if cfg.Keke9BaseURL != "" {
			cfg.VodBaseURL = cfg.Keke9BaseURL
		} else {
			cfg.VodBaseURL = "https://cj.lziapi.com"
		}
	}

	return cfg, nil
}

// resolveConfigPath 解析配置文件路径：显式传入 > CONFIG_PATH 环境变量 > config.yaml。
func resolveConfigPath(configPath string) string {
	if configPath != "" {
		return configPath
	}
	if v := os.Getenv("CONFIG_PATH"); v != "" {
		return v
	}
	return "config.yaml"
}

// overrideFromEnv 用环境变量覆盖配置项，只有非空的环境变量才会覆盖。
func overrideFromEnv(cfg *Config) {
	if v := os.Getenv("PORT"); v != "" {
		cfg.Port = v
	}
	// 优先使用 CHECK_CRON（与 yaml 字段 check_cron 语义一致）；
	// CHECK_INTERVAL 作为兼容别名保留，便于存量 Docker/CI 用户平滑迁移。
	if v := os.Getenv("CHECK_CRON"); v != "" {
		cfg.CheckCron = v
	} else if v := os.Getenv("CHECK_INTERVAL"); v != "" {
		cfg.CheckCron = v
	}
	if v := os.Getenv("SERVER_CHAN_KEY"); v != "" {
		cfg.ServerChanKey = v
	}
	// 数据源地址：VOD_BASE_URL 为主键；KEKE9_BASE_URL 为旧版兼容别名（仅当主键未设置时由 Load 回填）。
	if v := os.Getenv("VOD_BASE_URL"); v != "" {
		cfg.VodBaseURL = v
	}
	if v := os.Getenv("KEKE9_BASE_URL"); v != "" {
		cfg.Keke9BaseURL = v
	}
	if v := os.Getenv("VOD_DETAIL_URL"); v != "" {
		cfg.VodDetailURL = v
	}
	if v := os.Getenv("DB_PATH"); v != "" {
		cfg.DBPath = v
	}
}

// Validate 校验配置项合法性，在启动早期暴露明显的配置错误。
func (c *Config) Validate() error {
	port, err := strconv.Atoi(c.Port)
	if err != nil {
		return fmt.Errorf("端口 %q 不是合法数字: %w", c.Port, err)
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("端口 %d 超出有效范围 1-65535", port)
	}
	return nil
}
