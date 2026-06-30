# 检查更新日志增强 + 文件轮转 — 设计规格

- **日期**：2026-06-30（初版）/ 2026-06-30 修订（新增文件轮转）
- **范围**：`internal/scheduler/scheduler.go`、`internal/config/config.go`、`cmd/server/main.go`
- **状态**：已批准（含文件轮转修订），待编写实现计划

## 背景与问题

当前 `CheckUpdates()` 只有零星几条 `log` 输出到 stderr，存在两个盲区：

1. **不知道何时做过检查**——开始日志没有规模，结束无汇总，无法还原一次完整检查。
2. **不知道哪个环节失败**——数据源接口不通、通知推送不通时，错误要么被 `continue` 吞、要么直接上抛而无本地留痕。
3. **日志不固化**——标准 `log` 默认写 stderr，进程退出/容器重启后无外部收集即丢失，无法事后回溯。（修订新增）

## 目标

1. 每次检查能从日志还原：什么时候查了、查了多少部、哪些环节出错、总耗时。
2. 日志同时输出到控制台（stderr）和**轮转日志文件**，保证事后可回溯。

## 非目标（YAGNI）

- 不引入结构化日志库（zap/logrus 等），保持标准 `log` 包。
- 不持久化到数据库、不加前端历史页面。
- 轮转参数（大小/份数/天数）**不做配置化**，硬编码合理默认；仅日志文件路径可配。如未来需要可再加。

## 设计

### 一、检查过程日志埋点（仅 `scheduler.go`）

保持现有 `[scheduler]` 前缀和 `log` 包风格，在以下节点补充/修改：

| 节点 | 现状 | 改进 |
|------|------|------|
| 开始检查 | 仅一句话 | 补关注列表数量：`开始检查动漫更新，共 N 部` |
| 单部抓取失败 | 已有 log | 保持 |
| 单部抓取成功但无变化 | 无 | **不记**（避免噪音） |
| 发现更新汇总 | 无 | `发现 X 部有更新：《..》《..》` |
| 推送 ServerChan | 仅成功后一行 | 推送失败时在 `return err` 前补一条明确失败 log |
| 检查结束 | 无 | 用 `defer` 保证所有退出路径打印耗时：`检查结束，耗时 X` |

**取舍**：
1. 不记录"无变化"的成功项，避免刷屏。
2. 推送失败在 `return err` 前补 log，确保本地留痕。
3. 耗时用 `defer + time.Since()`，覆盖所有退出路径（含失败）。`start` 埋点放在 `TryLock` 成功之后，避免"任务执行中"的提前返回也打印误导性耗时。

### 二、日志文件轮转（`config.go` + `main.go`）

**新增依赖**：`gopkg.in/natefinch/lumberjack.v2`（按大小切割，MaxAge/MaxBackups 清理旧文件）。

> 说明：lumberjack 只按**大小**触发切割，不按时间。MaxAge(天)、MaxBackups(份) 是旧文件清理策略。本设计据此采用"按大小切 + 按 7 天/7 份清理"，而非"按天切割"。

**config.go**：
- 新增字段 `LogFile string`，yaml 键 `log_file`，默认值 `logs/anime-tip.log`。
- 环境变量 `LOG_FILE` 覆盖（优先级最高）。

**main.go**：
- 在 `config.Load` 成功、`Validate` 通过后，立即配置全局 logger：
  - `os.MkdirAll(filepath.Dir(cfg.LogFile), 0o755)` 确保目录存在（lumberjack 不保证创建父目录）。
  - `log.SetOutput(io.MultiWriter(os.Stderr, &lumberjack.Logger{...}))`。
- lumberjack 参数（硬编码）：`Filename=cfg.LogFile`、`MaxSize=10`(MB)、`MaxBackups=7`、`MaxAge=7`(天)、`LocalTime=true`。
- **初始化时机取舍**：配置加载/校验阶段（`log.Fatalf`）仍走默认 stderr——此时日志路径尚未确定；加载成功后切换输出。这是合理的，致命的配置错误仍能在控制台看到。

## 涉及文件

- `internal/scheduler/scheduler.go` — 日志埋点（引入 `time`、`strings`）。
- `internal/config/config.go` — 新增 `LogFile` 字段与 `LOG_FILE` 环境变量解析。
- `cmd/server/main.go` — 配置 lumberjack + MultiWriter，引入 `io`、`log`(已引入)、`path/filepath`、lumberjack。

## 验证

- `go build ./...` 通过。
- `go vet ./...` 通过。
- 启动服务，确认 `logs/anime-tip.log` 被创建且内容与控制台一致。
- 手动触发检查，确认各节点日志（含耗时、失败分支）写入文件。
- （可选）人为制造日志量超 10MB 验证轮转产生 `.log.1` 等备份，旧文件超 7 天/7 份被清理。
