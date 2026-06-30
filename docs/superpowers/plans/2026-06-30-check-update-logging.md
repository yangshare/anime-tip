# 检查更新日志增强 + 文件轮转 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** (1) 在 `CheckUpdates()` 各节点补日志使每次检查可还原；(2) 日志同时输出到 stderr 和按大小轮转的文件，保证事后回溯。

**架构：** 任务 1 已完成——`scheduler.go` 的日志埋点。任务 2-4 新增文件轮转：config 加 `LogFile` 字段、main.go 用 lumberjack + MultiWriter 配置全局 logger。仅用标准库 + lumberjack。

**技术栈：** Go 标准库 `log`/`io`/`os`/`path/filepath`/`time`/`strings`，`gopkg.in/natefinch/lumberjack.v2`。

**规格：** `docs/superpowers/specs/2026-06-30-check-update-logging-design.md`

---

## 关于 TDD 的说明

日志埋点为纯副作用、无新行为逻辑，不可单测（强测需引入接口抽象 + sqlmock，违反 YAGNI）。文件轮转同样依赖文件系统副作用与外部库行为，单测脆弱且收益低。验证采用 `go build` + `go vet` + 手动触发观察日志文件。这是经审慎判断的取舍，而非遗漏。

---

## 文件结构

- `internal/scheduler/scheduler.go` — **已完成**（任务 1），日志埋点。
- `internal/config/config.go` — 新增 `LogFile` 字段与 `LOG_FILE` 环境变量。
- `cmd/server/main.go` — 配置 lumberjack + MultiWriter。

---

## 任务 1：补齐 CheckUpdates 日志埋点  ✅ 已完成

import 加 `time`/`strings`；TryLock 后埋点 `start` + defer 耗时；ListAnimes 后补"共 N 部"；推送前补发现更新汇总；推送失败补失败日志。已 commit。

---

## 任务 2：config 新增 LogFile 字段

**文件：**
- 修改：`internal/config/config.go`（Config 结构体第 12-22 行；默认值第 32-36 行；overrideFromEnv 第 88-115 行）

- [ ] **步骤 1：Config 结构体加 LogFile 字段**

在 `internal/config/config.go` 的 `DBPath` 字段后（第 18 行之后、`Keke9BaseURL` 之前或之后均可）加入：

```go
	DBPath        string `yaml:"db_path"`
	LogFile       string `yaml:"log_file"`
```

- [ ] **步骤 2：默认值加 LogFile**

把默认值块（第 32-36 行）：

```go
	cfg := &Config{
		Port:      "8080",
		CheckCron: "0 * * * *",
		DBPath:    "anime-tip.db",
	}
```

改为：

```go
	cfg := &Config{
		Port:      "8080",
		CheckCron: "0 * * * *",
		DBPath:    "anime-tip.db",
		LogFile:   "logs/anime-tip.log",
	}
```

- [ ] **步骤 3：overrideFromEnv 加 LOG_FILE**

在 `overrideFromEnv` 末尾（`DB_PATH` 处理之后，第 114 行之后）加入：

```go
	if v := os.Getenv("DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("LOG_FILE"); v != "" {
		cfg.LogFile = v
	}
```

- [ ] **步骤 4：编译验证**

运行：`go build ./...`
预期：无输出，退出码 0。

- [ ] **步骤 5：Commit**

```bash
git add internal/config/config.go
git commit -m "feat(config): 新增 log_file 配置项支持日志路径"
```

---

## 任务 3：main.go 配置 lumberjack 文件轮转

**文件：**
- 修改：`cmd/server/main.go`（import 块第 3-12 行；Validate 之后、InitDB 之前 第 24-25 行附近）

- [ ] **步骤 1：go get 引入 lumberjack 依赖**

运行：`go get gopkg.in/natefinch/lumberjack.v2`
预期：`go: added gopkg.in/natefinch/lumberjack.v2 vX.X.X`，`go.mod` 与 `go.sum` 更新。

- [ ] **步骤 2：import 块加入 io、path/filepath、lumberjack**

把 `cmd/server/main.go` 的 import 块（第 3-12 行）：

```go
import (
	"flag"
	"log"

	"github.com/user/anime-tip/internal/config"
	"github.com/user/anime-tip/internal/crawler"
	"github.com/user/anime-tip/internal/scheduler"
	"github.com/user/anime-tip/internal/store"
	"github.com/user/anime-tip/internal/web"
)
```

改为：

```go
import (
	"flag"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/user/anime-tip/internal/config"
	"github.com/user/anime-tip/internal/crawler"
	"github.com/user/anime-tip/internal/scheduler"
	"github.com/user/anime-tip/internal/store"
	"github.com/user/anime-tip/internal/web"
	"gopkg.in/natefinch/lumberjack.v2"
)
```

- [ ] **步骤 3：在配置校验后配置全局 logger**

定位 `cmd/server/main.go` 中 Validate 之后、`log.Printf("anime-tip starting...")` 附近（第 24-25 行）：

```go
	if err := cfg.Validate(); err != nil {
		log.Fatalf("配置校验失败: %v", err)
	}
	log.Printf("anime-tip starting on :%s", cfg.Port)
```

在两行之间插入日志初始化：

```go
	if err := cfg.Validate(); err != nil {
		log.Fatalf("配置校验失败: %v", err)
	}

	// 配置全局日志输出：同时写 stderr（控制台）和按大小轮转的日志文件。
	// lumberjack 仅按大小切割，MaxAge/MaxBackups 是旧文件清理策略。
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
```

- [ ] **步骤 4：编译验证**

运行：`go build ./...`
预期：无输出，退出码 0。

- [ ] **步骤 5：静态检查**

运行：`go vet ./...`
预期：无输出，退出码 0。

- [ ] **步骤 6：Commit**

```bash
git add cmd/server/main.go go.mod go.sum
git commit -m "feat(log): 日志双写 stderr 与 lumberjack 轮转文件"
```

---

## 任务 4：手动触发验证（文件 + 日志内容）

**文件：** 无代码改动。

- [ ] **步骤 1：启动服务**

运行：`go run ./cmd/server`
预期：服务启动；项目根目录下 `logs/anime-tip.log` 被创建。

- [ ] **步骤 2：触发检查并核对日志文件**

调用 `POST http://localhost:<端口>/api/check`，然后查看 `logs/anime-tip.log`，确认包含：
```
[scheduler] 开始检查动漫更新，共 N 部
[scheduler] 检查结束，耗时 ...
```
且文件内容与控制台一致。

- [ ] **步骤 3：（可选）验证失败分支写入文件**

任选：
- 临时把 `vod_base_url` 改成不可达地址 → 日志文件应含 `获取动漫 ... 详情失败: ...` + 末尾 `检查结束，耗时 ...`。
- 把 `server_chan_key` 清空并在有更新时触发 → 日志文件应含 `推送通知失败: ...` + 末尾耗时。

- [ ] **步骤 4：确认日志目录已被 .gitignore 忽略（避免误提交日志）**

检查 `.gitignore` 是否忽略 `logs/`；若否，补一条 `logs/` 并单独提交（见任务 5）。

---

## 任务 5：忽略日志目录（如需）

**文件：** `.gitignore`

- [ ] **步骤 1：检查并补充**

查看 `.gitignore`，若未忽略 `logs/`，追加：

```
logs/
```

- [ ] **步骤 2：Commit（若有改动）**

```bash
git add .gitignore
git commit -m "chore: 忽略 logs/ 日志目录"
```
若无改动则跳过。

---

## 自检结果

**1. 规格覆盖度：**
- 埋点各节点 → 任务 1（已完成）✓
- 文件轮转：config 字段 → 任务 2 ✓；main 配置 MultiWriter + lumberjack → 任务 3 ✓
- 默认路径 `logs/anime-tip.log` → 任务 2 步骤 2 ✓
- 参数硬编码 MaxSize=10/Backups=7/Age=7 → 任务 3 步骤 3 ✓
- MkdirAll 确保目录 → 任务 3 步骤 3 ✓
- 配置阶段走 stderr 的时机取舍 → 任务 3 步骤 3 注释 + log.Fatalf 仍默认输出 ✓
- 验证（build/vet/手动/文件创建）→ 任务 2/3/4 ✓
- 非目标（不配参数化、不引 zap）→ 计划未引入 ✓

**2. 占位符扫描：** 无 TODO/待定；所有代码步骤含完整代码块；轮转参数为具体数值。✓

**3. 类型一致性：** `cfg.LogFile` 在任务 2 定义，任务 3 引用一致；`lumberjack.Logger` 字段名（Filename/MaxSize/MaxBackups/MaxAge/LocalTime）与库 v2 API 一致；`io.MultiWriter`、`filepath.Dir`、`os.MkdirAll` 签名正确。✓

无遗漏，无矛盾。
