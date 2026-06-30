package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("写入临时配置失败: %v", err)
	}
	return p
}

// 默认 config.yaml 不存在时，回退到默认值 + 环境变量，不报错。
func TestLoad_defaultFileMissing(t *testing.T) {
	// 切到临时目录，确保没有 config.yaml 干扰
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("CONFIG_PATH", "")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("期望无错，得到: %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("Port 期望 8080，得到 %s", cfg.Port)
	}
	if cfg.CheckCron != "0 * * * *" {
		t.Errorf("CheckCron 期望默认值，得到 %s", cfg.CheckCron)
	}
	if cfg.Keke9BaseURL != "https://www.keke9.com" {
		t.Errorf("Keke9BaseURL 期望默认值，得到 %s", cfg.Keke9BaseURL)
	}
	if cfg.DBPath != "anime-tip.db" {
		t.Errorf("DBPath 期望默认值，得到 %s", cfg.DBPath)
	}
	if cfg.ServerChanKey != "" {
		t.Errorf("ServerChanKey 期望空，得到 %s", cfg.ServerChanKey)
	}
}

// YAML 覆盖默认值。
func TestLoad_yamlOverridesDefaults(t *testing.T) {
	p := writeTempConfig(t, "port: \"9090\"\ncheck_cron: \"*/30 * * * *\"\nserver_chan_key: \"yaml-key\"\nkeke9_base_url: \"https://example.com\"\ndb_path: \"/data/anime-tip.db\"\n")

	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("期望无错，得到: %v", err)
	}
	if cfg.Port != "9090" {
		t.Errorf("Port 期望 9090，得到 %s", cfg.Port)
	}
	if cfg.CheckCron != "*/30 * * * *" {
		t.Errorf("CheckCron 期望 */30 * * * *，得到 %s", cfg.CheckCron)
	}
	if cfg.ServerChanKey != "yaml-key" {
		t.Errorf("ServerChanKey 期望 yaml-key，得到 %s", cfg.ServerChanKey)
	}
	if cfg.Keke9BaseURL != "https://example.com" {
		t.Errorf("Keke9BaseURL 期望 https://example.com，得到 %s", cfg.Keke9BaseURL)
	}
	if cfg.DBPath != "/data/anime-tip.db" {
		t.Errorf("DBPath 期望 /data/anime-tip.db，得到 %s", cfg.DBPath)
	}
}

// YAML 中未设置的字段保留默认值（不被清零）。
func TestLoad_yamlKeepsDefaultsForMissingFields(t *testing.T) {
	p := writeTempConfig(t, "port: \"7070\"\n")

	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("期望无错，得到 %v", err)
	}
	if cfg.Port != "7070" {
		t.Errorf("Port 期望 7070，得到 %s", cfg.Port)
	}
	if cfg.CheckCron != "0 * * * *" {
		t.Errorf("CheckCron 应保留默认值，得到 %s", cfg.CheckCron)
	}
	if cfg.DBPath != "anime-tip.db" {
		t.Errorf("DBPath 应保留默认值，得到 %s", cfg.DBPath)
	}
}

// 环境变量优先级高于 YAML。
func TestLoad_envOverridesYaml(t *testing.T) {
	p := writeTempConfig(t, "port: \"9090\"\nserver_chan_key: \"yaml-key\"\n")

	t.Setenv("PORT", "3000")
	t.Setenv("SERVER_CHAN_KEY", "env-key")

	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("期望无错，得到 %v", err)
	}
	if cfg.Port != "3000" {
		t.Errorf("Port 期望被环境变量覆盖为 3000，得到 %s", cfg.Port)
	}
	if cfg.ServerChanKey != "env-key" {
		t.Errorf("ServerChanKey 期望被环境变量覆盖为 env-key，得到 %s", cfg.ServerChanKey)
	}
}

// 空环境变量不应覆盖 YAML/默认值。
func TestLoad_emptyEnvDoesNotOverride(t *testing.T) {
	p := writeTempConfig(t, "port: \"9090\"\n")
	t.Setenv("PORT", "")

	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("期望无错，得到 %v", err)
	}
	if cfg.Port != "9090" {
		t.Errorf("空环境变量不应覆盖，Port 期望 9090，得到 %s", cfg.Port)
	}
}

// CONFIG_PATH 环境变量指定路径。
func TestLoad_configPathEnv(t *testing.T) {
	p := writeTempConfig(t, "port: \"8181\"\n")
	t.Setenv("CONFIG_PATH", p)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("期望无错，得到: %v", err)
	}
	if cfg.Port != "8181" {
		t.Errorf("Port 期望 8181，得到 %s", cfg.Port)
	}
}

// 显式指定的路径不存在时，不报错（降级到默认值+环境变量）。
func TestLoad_explicitPathMissing(t *testing.T) {
	t.Setenv("PORT", "5555")
	cfg, err := Load(filepath.Join(t.TempDir(), "no-such-file.yaml"))
	if err != nil {
		t.Fatalf("显式路径不存在应降级而非报错，得到: %v", err)
	}
	if cfg.Port != "5555" {
		t.Errorf("Port 期望由环境变量 5555 提供，得到 %s", cfg.Port)
	}
}

// YAML 解析失败应返回错误（fail-fast）。
func TestLoad_invalidYamlReturnsError(t *testing.T) {
	p := writeTempConfig(t, "port: [unterminated\n  bad yaml\n")
	_, err := Load(p)
	if err == nil {
		t.Fatal("期望解析失败返回错误，得到 nil")
	}
}

// CHECK_CRON 环境变量覆盖 YAML 的 check_cron。
func TestLoad_checkCronEnvPreferred(t *testing.T) {
	p := writeTempConfig(t, "check_cron: \"0 0 * * *\"\n")
	t.Setenv("CHECK_CRON", "*/15 * * * *")
	t.Setenv("CHECK_INTERVAL", "")

	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("期望无错，得到: %v", err)
	}
	if cfg.CheckCron != "*/15 * * * *" {
		t.Errorf("期望 CHECK_CRON 生效，得到 %s", cfg.CheckCron)
	}
}

// CHECK_INTERVAL 作为兼容别名，在未设 CHECK_CRON 时生效。
func TestLoad_checkIntervalAlias(t *testing.T) {
	t.Setenv("CHECK_CRON", "")
	t.Setenv("CHECK_INTERVAL", "0 9,21 * * *")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("期望无错，得到: %v", err)
	}
	if cfg.CheckCron != "0 9,21 * * *" {
		t.Errorf("期望 CHECK_INTERVAL 别名生效，得到 %s", cfg.CheckCron)
	}
}

// CHECK_CRON 优先级高于 CHECK_INTERVAL。
func TestLoad_checkCronOverridesAlias(t *testing.T) {
	t.Setenv("CHECK_CRON", "*/10 * * * *")
	t.Setenv("CHECK_INTERVAL", "*/20 * * * *")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("期望无错，得到: %v", err)
	}
	if cfg.CheckCron != "*/10 * * * *" {
		t.Errorf("期望 CHECK_CRON 优先于 CHECK_INTERVAL，得到 %s", cfg.CheckCron)
	}
}

func TestValidate_portValid(t *testing.T) {
	cases := []string{"1", "8080", "65535"}
	for _, p := range cases {
		if err := (&Config{Port: p}).Validate(); err != nil {
			t.Errorf("端口 %s 应合法，得到: %v", p, err)
		}
	}
}

func TestValidate_portInvalid(t *testing.T) {
	cases := []string{"0", "65536", "999999", "abc", "-1", ""}
	for _, p := range cases {
		if err := (&Config{Port: p}).Validate(); err == nil {
			t.Errorf("端口 %q 应非法，但 Validate 返回 nil", p)
		}
	}
}
