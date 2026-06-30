# 动漫更新判定：remarks 归一化 + 单调性保护 设计规格

- 日期：2026-06-30
- 主题：增强「判断一部动漫是否有更新」的容错性与正确性
- 状态：已通过头脑风暴，待审查

## 1. 背景与问题

当前判定逻辑位于 `internal/scheduler/scheduler.go` 的 `CheckUpdates()`，依据是采集源返回的 `vod_remarks` 字符串与库中 `last_notified_remarks` 的**严格相等比较**：

```go
if a.LastNotifiedRemarks != "" && latestRemarks != a.LastNotifiedRemarks {
    // 有更新
}
```

存在的脆弱点：

1. **完全依赖原始字符串相等**：源站对同一进度可能换写法（「更新至10集」↔「更新至第10集」↔「第10集」），字符串一变就误报更新。
2. **无法区分前进 / 后退 / 抖动**：源站临时回退 remarks（如把「更新至12集」误填成「10集」再改回）时，会推送「倒退」通知，缺乏单调性保护。
3. **解析不出集数时无兜底语义**：remarks 可能是「完结」「预告」「HD」等非数字文本，无法比较大小。

苹果 CMS 的 `VodItem` 中除 `vod_remarks` 外未解析其他更可靠字段（如 `vod_play_url`，但解析复杂度高，本次不引入）。

## 2. 目标与非目标

### 目标

- 消除 remarks 写法差异导致的误报。
- 引入集数单调性保护：只有集数「严格前进」才视为更新，回退 / 相等 / 抖动均不推送。
- 解析不出集数时仍有合理兜底，不漏报真实状态变化。

### 非目标（YAGNI）

- 不解析 `vod_play_url` 作为集数兜底（方案 C，复杂度过高）。
- 不引入数据库迁移版本机制（schema_version 表 / 独立迁移脚本）。
- 不改变推送通道（Server 酱）与通知聚合方式。
- 不改变接口失败时的「跳过、不动基线」行为。

## 3. 数据模型变更

`animes` 表新增一列：

- `last_notified_episode INTEGER NOT NULL DEFAULT 0`：上次推送后解析出的集数基线，单调性比较的数值基准。

语义约定：

- `current_remarks`：最近一次抓到的 remarks，**仅供前端展示「当前进度」，不参与判定**。
- `last_notified_remarks`：上次推送时的 remarks 字符串，用于「解析不出集数」分支的字符串比较。
- `last_notified_episode`：上次推送时解析出的集数，用于「能解析出集数」分支的单调比较。

三者职责分离，避免展示数据污染判定基线。

## 4. 迁移方式

启动时在 `internal/store/db.go` 的 `migrate()` 中幂等补列。沿用项目现有「`InitDB` → `migrate`」单条路径，不引入脚本入口。

```go
func migrate(db *sql.DB) error {
    _, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS animes (
            ...,
            last_notified_episode INTEGER NOT NULL DEFAULT 0,
            ...
        );
        CREATE TABLE IF NOT EXISTS settings (...);
    `)
    if err != nil {
        return err
    }

    // 老库补列（幂等）：SQLite 无 ADD COLUMN IF NOT EXISTS，靠捕获错误实现
    _, err = db.Exec(`ALTER TABLE animes ADD COLUMN last_notified_episode INTEGER NOT NULL DEFAULT 0`)
    if err != nil && !strings.Contains(err.Error(), "duplicate column") {
        return err
    }
    return nil
}
```

存量记录 `last_notified_episode = 0`，下一轮只要解析出新集数 > 0 即触发一次推送，符合「首次建基线后正常推进」预期。

## 5. remarks 归一化解析

新增纯函数：

```go
// internal/episode/episode.go（新建包，单一职责）
// ParseEpisode 从 vod_remarks 文本提取当前集数。
// 返回 (episode, ok)：ok=false 表示无法解析出数值集数。
func ParseEpisode(remarks string) (int, bool)
```

### 解析规则

优先匹配「紧邻 `集/话` 后缀的数字」；匹配不到再退化为「首个连续数字」。

理由： remarks 形态多样，「串里第一个数字」有时是年份（如 `2024年第10集`），直接取首个数字会误解析为 2024。优先认 `集/话` 后缀可规避此问题，退化分支兜底纯数字写法（如 `10`）。

### 覆盖表

| remarks 文本 | 解析结果 |
|---|---|
| `更新至第10集` / `更新至10集` / `第10集` / `10集` | `(10, true)` |
| `更新至第10话` / `第10话` | `(10, true)` |
| `10` | `(10, true)` |
| `2024年第10集` | `(10, true)`（不误取 2024） |
| `完结` / `正片` / `预告` / `HD` | `(0, false)` |
| 空串 | `(0, false)` |

## 6. 单调性判定状态机

判定抽成无 IO 的纯函数，便于单测：

```go
// internal/scheduler/update.go（新建）
// DecideUpdate 依据本轮 remarks 与库中基线，判定是否推送并给出新基线。
//   baseRemarks  = last_notified_remarks
//   baseEpisode  = last_notified_episode
// 返回 (shouldNotify, newRemarks, newEpisode)，newRemarks/newEpisode 为应写入的新基线。
func DecideUpdate(latestRemarks, baseRemarks string, baseEpisode int) (shouldNotify bool, newRemarks string, newEpisode int)
```

流程：

```
对本轮 latestRemarks 调用 ParseEpisode → (newEp, newOk)

情况 A：newOk == true（本轮解析出集数）
    if baseEpisode > 0 且 newEp > baseEpisode:
        → shouldNotify = true（集数前进）
    else:
        → shouldNotify = false（集数相等 / 回退 / 抖动，均不推）
    newRemarks  = latestRemarks
    newEpisode  = newEp

情况 B：newOk == false（解析不出，如「完结」「HD」）
    if baseRemarks != "" 且 latestRemarks != baseRemarks:
        → shouldNotify = true（字符串变化，退化为字符串比较）
    else:
        → shouldNotify = false
    newRemarks  = latestRemarks
    newEpisode  = baseEpisode   // 保持原值，本轮无数值集数

情况 C：首次关注（baseRemarks == "" 且 baseEpisode == 0）
    → shouldNotify = false（只建基线，不推送）
    newRemarks  = latestRemarks
    newEpisode  = newEp（若 newOk）
```

边界确认：情况 B 之后 remarks 由 `完结` 变回 `更新至第13集` 时，`newEp=13 > baseEpisode(=12)` → 推送，行为正确。故「情况 B 不改 episode 基线」成立。

## 7. CheckUpdates 编排改造

`CheckUpdates()` 循环体改为编排：

1. 拉取 `latestRemarks`（抓取失败 → 日志 + `continue`，不动基线，与现状一致）。
2. 调用 `DecideUpdate(latestRemarks, a.LastNotifiedRemarks, a.LastNotifiedEpisode)`。
3. `shouldNotify` 为真则加入 `updates` 列表。
4. 收集 `(id, latestRemarks, newRemarks, newEpisode)` 到 `toUpdate`。
5. 有更新则推送 Server 酱；**推送成功后**才落库（顺序与现状一致）。
6. 落库写入 `current_remarks = latestRemarks`、`last_notified_remarks = newRemarks`、`last_notified_episode = newEpisode`。

`store.UpdateAnimeRemarks` 签名扩展为接收 `lastNotifiedEpisode int`。

## 8. 错误处理

- 单部抓取失败：日志 + `continue`，不动基线，下一轮重试（维持现状）。
- 推送失败：不落库基线，下一轮重试（维持现状）。
- 不引入额外重试 / 退避机制（YAGNI）。

## 9. 测试策略（TDD，先于实现）

### 9.1 `ParseEpisode` 表驱动测试（`internal/episode/episode_test.go`）

覆盖第 5 节覆盖表全部用例，含边界：空串、纯数字、`2024年第10集`、`完结`、`HD`。

### 9.2 `DecideUpdate` 状态机测试（`internal/scheduler/update_test.go`）

覆盖：

- 集数前进（12 → 13）→ 推送，基线推进到 13。
- 集数相等、写法变（`更新至12集` → `第12集`）→ 不推送。
- 集数回退（13 → 12）→ 不推送。
- 解析不出且字符串变化（`更新至12集` → `完结`）→ 推送。
- 解析不出且字符串不变 → 不推送。
- 首次关注（基线空）→ 不推送，建基线。
- 情况 B 后回到数字集数且前进 → 推送。

### 9.3 行为回归守卫

推送成功才落库的顺序，用集成测试或对 `CheckUpdates` 的现有点位守住，避免基线错乱。

## 10. 影响范围

- 新增：`internal/episode/{episode.go,episode_test.go}`、`internal/scheduler/update.go`（+测试）。
- 修改：`internal/store/db.go`（migrate 补列）、`internal/store/anime.go`（`UpdateAnimeRemarks` 签名 + `ListAnimes`/`GetAnimeByVodID`/`CreateAnime` 的 SELECT/INSERT 加列）、`internal/model/anime.go`（`Anime` 加 `LastNotifiedEpisode` 字段）、`internal/scheduler/scheduler.go`（`CheckUpdates` 改为编排）。
- 不变：推送通道、通知格式、crawler、web handler（除随 schema 加字段外无逻辑变更）。
