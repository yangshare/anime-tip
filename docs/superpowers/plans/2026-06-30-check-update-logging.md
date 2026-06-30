# 检查更新日志增强 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 在 `CheckUpdates()` 各关键节点补 stdout 文本日志，使每次检查的时间、规模、失败环节、耗时均可从日志还原。

**架构：** 仅修改 `internal/scheduler/scheduler.go` 的 `CheckUpdates()` 方法：在 `TryLock` 成功后埋点 `time.Now()` 并用 `defer` 保证所有退出路径都打印耗时；为关注列表规模、发现更新汇总、推送失败补 log。不引入新依赖（仅标准库 `time`、`strings`）。

**技术栈：** Go 标准库 `log` / `time` / `strings`，`github.com/robfig/cron/v3`（不动）。

**规格：** `docs/superpowers/specs/2026-06-30-check-update-logging-design.md`

---

## 关于 TDD 的说明（重要）

本次变更是**纯日志副作用**：没有新增行为逻辑、没有新分支、没有新返回值、没有新状态。被修改的 `CheckUpdates()` 依赖包级函数 `store.ListAnimes(s.db)`、`store.GetSetting`、`store.UpdateAnimeRemarks` 与具体类型 `*crawler.Client`，无可注入的接口。要单测 log 输出必须：
1. 把 `crawler.Client` 抽象成接口（扩大规格范围），
2. 引入 `sqlmock` 或真实内存 sqlite 来 mock 包级 `store.*`（引入新测试基础设施），
3. 用 `log.SetOutput(&bytes.Buffer{})` 捕获并断言字符串（脆弱测试）。

以上违反规格"非目标"中的 YAGNI 精神与"仅修改 scheduler.go"的范围限定。因此本计划**不走 TDD**，验证采用规格"验证"章节定义的方式：`go build ./...` + `go vet ./...` + 手动触发观察日志输出。这是经审慎判断的取舍，而非遗漏。

---

## 文件结构

- 修改：`internal/scheduler/scheduler.go`
  - 职责：定时/手动检查动漫更新的核心逻辑。本次仅增强其日志可观测性，不改任何控制流或数据库交互。
  - 唯一改动文件，无新建文件。

---

## 任务 1：补齐检查过程的日志埋点

**文件：**
- 修改：`internal/scheduler/scheduler.go`（import 块第 3-13 行；`CheckUpdates` 方法第 46-116 行）

- [ ] **步骤 1：在 import 块加入 `time` 和 `strings`**

把 `internal/scheduler/scheduler.go` 第 3-13 行的 import 块：

```go
import (
	"database/sql"
	"fmt"
	"log"
	"sync"

	"github.com/robfig/cron/v3"
	"github.com/user/anime-tip/internal/crawler"
	"github.com/user/anime-tip/internal/notify"
	"github.com/user/anime-tip/internal/store"
)
```

改为：

```go
import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/user/anime-tip/internal/crawler"
	"github.com/user/anime-tip/internal/notify"
	"github.com/user/anime-tip/internal/store"
)
```

- [ ] **步骤 2：在 TryLock 成功后埋点开始时间并用 defer 打印耗时**

定位 `CheckUpdates` 方法开头（约第 46-51 行）：

```go
func (s *Scheduler) CheckUpdates() error {
	// 防止并发执行（手动触发 + 定时触发叠加、多次点击）
	if !s.mu.TryLock() {
		return fmt.Errorf("检查任务正在执行中，请稍后再试")
	}
	defer s.mu.Unlock()
```

紧接 `defer s.mu.Unlock()` 之后插入埋点（注意：必须放在 TryLock 成功之后，这样"任务正在执行中"的提前返回不会打印误导性的耗时日志）：

```go
	defer s.mu.Unlock()

	start := time.Now()
	defer func() {
		log.Printf("[scheduler] 检查结束，耗时 %s", time.Since(start))
	}()
```

- [ ] **步骤 3：在获取关注列表后补"共 N 部"开始日志**

定位约第 53-60 行：

```go
	animes, err := store.ListAnimes(s.db)
	if err != nil {
		return err
	}
	if len(animes) == 0 {
		log.Println("[scheduler] 关注列表为空，跳过检查")
		return nil
	}
```

在 `if err != nil` 块与 `if len(animes) == 0` 块之间插入一行开始日志：

```go
	animes, err := store.ListAnimes(s.db)
	if err != nil {
		return err
	}
	log.Printf("[scheduler] 开始检查动漫更新，共 %d 部", len(animes))
	if len(animes) == 0 {
		log.Println("[scheduler] 关注列表为空，跳过检查")
		return nil
	}
```

- [ ] **步骤 4：在推送前补"发现 N 部更新"汇总，并在推送失败时补失败日志**

定位约第 98-106 行：

```go
	// 如果有更新，推送通知
	if len(updates) > 0 {
		sendKey, _ := store.GetSetting(s.db, "server_chan_key")
		sc := notify.NewServerChan(sendKey)
		if err := sc.Send(updates); err != nil {
			return err
		}
		log.Printf("[scheduler] 推送了 %d 条动漫更新", len(updates))
	}
```

改为（增加汇总日志、增加推送失败的本地日志留痕）：

```go
	// 如果有更新，推送通知
	if len(updates) > 0 {
		names := make([]string, 0, len(updates))
		for _, u := range updates {
			names = append(names, "《"+u.Name+"》")
		}
		log.Printf("[scheduler] 发现 %d 部有更新：%s", len(updates), strings.Join(names, " "))

		sendKey, _ := store.GetSetting(s.db, "server_chan_key")
		sc := notify.NewServerChan(sendKey)
		if err := sc.Send(updates); err != nil {
			log.Printf("[scheduler] 推送通知失败: %v", err)
			return err
		}
		log.Printf("[scheduler] 推送了 %d 条动漫更新", len(updates))
	}
```

- [ ] **步骤 5：编译验证**

运行：`go build ./...`
预期：无输出，退出码 0（编译通过，`time`/`strings` 已被使用，无未使用导入错误）。

- [ ] **步骤 6：静态检查**

运行：`go vet ./...`
预期：无输出，退出码 0。

- [ ] **步骤 7：手动触发验证日志输出**

启动服务（需提供有效 `config.yaml` 或环境变量）后，手动触发检查接口：

```
POST http://localhost:<port>/api/check
```

（若 `web.SetupRouter` 注册的路径不同，执行者需先在 `internal/web` 下确认 `TriggerCheck` 的实际路由再调用。）

关注日志中应能看到类似序列（成功路径）：
```
[scheduler] 开始检查动漫更新，共 N 部
[scheduler] 发现 X 部有更新：《..》《..》   # 仅当有更新时
[scheduler] 推送了 X 条动漫更新             # 仅当有更新时
[scheduler] 检查结束，耗时 1.2s
```

并人为制造一次失败路径验证留痕（任选其一）：
- 数据源接口不通：临时把 `vod_base_url` 改成不可达地址触发检查 → 应看到 `[scheduler] 获取动漫 <名> (vod_id=..) 详情失败: ...` 且末尾仍有 `检查结束，耗时 ...`。
- 推送不通：临时清空 `server_chan_key`（或填错值）并在有更新时触发 → 应看到 `[scheduler] 推送通知失败: ...` 且末尾仍有耗时日志。

确认：失败分支同样打印耗时日志（因步骤 2 用了 defer），且失败原因清晰可见。

- [ ] **步骤 8：Commit**

```bash
git add internal/scheduler/scheduler.go
git commit -m "feat(scheduler): 增强检查更新的过程日志与耗时统计"
```

---

## 自检结果

**1. 规格覆盖度：**
- "开始检查 补关注列表数量" → 任务 1 步骤 3 ✓
- "单部抓取失败 已有 log 保持" → 不需改动，已在现状中 ✓
- "单部抓取成功但无变化 不记" → 不需改动（本就不记）✓
- "发现更新汇总" → 任务 1 步骤 4（汇总日志）✓
- "推送失败 补明确失败 log" → 任务 1 步骤 4（`推送通知失败` log）✓
- "检查结束 补耗时" → 任务 1 步骤 2（defer 耗时）✓
- 取舍 1/2/3 → 步骤 4（不记无变化）、步骤 4（推送失败留痕）、步骤 2（time.Since 耗时）✓
- 非目标（不引入日志库/不落库/不写文件）→ 计划仅用标准库，无新建文件 ✓

**2. 占位符扫描：** 无 TODO/待定；所有代码步骤均含完整代码块；路由路径的提示已说明由执行者确认而非留空。✓

**3. 类型一致性：** `updates []notify.UpdateItem`、`u.Name`、`names []string`、`strings.Join`、`time.Since(start)` 均与现有代码及标准库签名一致；`start := time.Now()` 仅在步骤 2 定义，步骤 2 的 defer 引用一致。✓

无遗漏，无矛盾。
