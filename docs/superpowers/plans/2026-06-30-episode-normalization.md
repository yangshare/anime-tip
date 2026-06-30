# 动漫更新判定：归一化与单调性保护 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 把「判断动漫是否有更新」从 remarks 字符串相等比较，改为集数归一化解析 + 严格单调递增判定，消除写法差异误报与回退/抖动误报。

**架构：** 新增 `internal/episode` 包提供 `ParseEpisode` 纯函数（remarks → 集数）；新增 `internal/scheduler/update.go` 提供 `DecideUpdate` 纯函数（本轮 remarks + 基线 → 是否推送 + 新基线）；`CheckUpdates` 退化为编排（拉取→判定→聚合→推送→成功后落库）；`animes` 表新增 `last_notified_episode` 列，启动时幂等迁移。

**技术栈：** Go 1.25、`database/sql` + modernc.org/sqlite、标准 `testing`（表驱动）、robfig/cron、gin。

**测试命令：** `go test ./...`（项目根目录执行，PowerShell）。单包：`go test ./internal/episode -v`。

**规格：** `docs/superpowers/specs/2026-06-30-episode-normalization-design.md`

---

## 文件结构

| 文件 | 职责 | 动作 |
|---|---|---|
| `internal/episode/episode.go` | `ParseEpisode`：remarks → 集数纯函数 | 创建 |
| `internal/episode/episode_test.go` | `ParseEpisode` 表驱动测试 | 创建 |
| `internal/scheduler/update.go` | `DecideUpdate`：判定状态机纯函数 | 创建 |
| `internal/scheduler/update_test.go` | `DecideUpdate` 状态机测试 | 创建 |
| `internal/model/anime.go` | `Anime` 结构体加 `LastNotifiedEpisode` | 修改 |
| `internal/store/db.go` | `migrate` 幂等补列 | 修改 |
| `internal/store/anime.go` | SELECT/INSERT/UPDATE 加列、`UpdateAnimeRemarks` 签名扩展 | 修改 |
| `internal/scheduler/scheduler.go` | `CheckUpdates` 改为编排 | 修改 |
| `internal/scheduler/scheduler_test.go` | 落库顺序回归测试 | 创建 |

依赖顺序：T1（episode）→ T2（update，依赖 episode）→ T3（model）→ T4（store，依赖 model）→ T5（scheduler 编排，依赖 update+store）→ T6（回归测试，依赖 T5）。每个任务结束 commit。

---

### 任务 1：remarks 归一化解析 `ParseEpisode`

**文件：**
- 创建：`internal/episode/episode.go`
- 测试：`internal/episode/episode_test.go`

- [ ] **步骤 1：编写失败的测试**

创建 `internal/episode/episode_test.go`：

```go
package episode

import "testing"

func TestParseEpisode(t *testing.T) {
	cases := []struct {
		remarks string
		wantEp  int
		wantOk  bool
	}{
		{"更新至第10集", 10, true},
		{"更新至10集", 10, true},
		{"第10集", 10, true},
		{"10集", 10, true},
		{"更新至第10话", 10, true},
		{"第10话", 10, true},
		{"更新到10", 10, true},
		{"10", 10, true},
		// 含年份时不能误取 2024，要取紧邻 集/话 后缀的数字
		{"2024年第10集", 10, true},
		{"2024年第10话", 10, true},
		// 无数字文本
		{"完结", 0, false},
		{"正片", 0, false},
		{"预告", 0, false},
		{"HD", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		gotEp, gotOk := ParseEpisode(c.remarks)
		if gotEp != c.wantEp || gotOk != c.wantOk {
			t.Errorf("ParseEpisode(%q) = (%d, %v), want (%d, %v)",
				c.remarks, gotEp, gotOk, c.wantEp, c.wantOk)
		}
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/episode -v`
预期：FAIL，报错 `undefined: ParseEpisode`（包未实现）。

- [ ] **步骤 3：编写最少实现代码**

创建 `internal/episode/episode.go`：

```go
// Package episode 从苹果 CMS vod_remarks 文本中归一化解析集数。
//
// vod_remarks 形态多样（"更新至第10集"/"第10话"/"10"/"完结"…），
// 直接字符串比较会导致写法差异误报更新。本包把它解析成 (集数, 是否成功)。
package episode

import (
	"regexp"
	"strconv"
)

// epSuffixRe 优先匹配紧邻「集/话」后缀的数字，规避把年份（如 2024年第10集）
// 误当集数。匹配组 1 为集数。
var epSuffixRe = regexp.MustCompile(`(\d+)\s*[集话]`)

// firstNumRe 退化匹配首个连续数字，兜底 "10" 这种纯数字写法。
var firstNumRe = regexp.MustCompile(`\d+`)

// ParseEpisode 从 remarks 提取当前集数。
// 返回 (episode, ok)：ok=false 表示无法解析出数值集数（如 "完结"/"HD"/空串）。
func ParseEpisode(remarks string) (int, bool) {
	if m := epSuffixRe.FindStringSubmatch(remarks); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n, true
		}
	}
	if m := firstNumRe.FindString(remarks); m != "" {
		if n, err := strconv.Atoi(m); err == nil {
			return n, true
		}
	}
	return 0, false
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/episode -v`
预期：PASS（全部用例通过）。

- [ ] **步骤 5：Commit**

```bash
git add internal/episode/episode.go internal/episode/episode_test.go
git commit -m "feat(episode): 新增 remarks 集数归一化解析 ParseEpisode"
```

---

### 任务 2：单调性判定状态机 `DecideUpdate`

**文件：**
- 创建：`internal/scheduler/update.go`
- 测试：`internal/scheduler/update_test.go`

- [ ] **步骤 1：编写失败的测试**

创建 `internal/scheduler/update_test.go`：

```go
package scheduler

import "testing"

func TestDecideUpdate(t *testing.T) {
	cases := []struct {
		name         string
		latest       string
		baseRemarks  string
		baseEpisode  int
		wantNotify   bool
		wantRemarks  string
		wantEpisode  int
	}{
		// 集数前进 → 推送，基线推进
		{"前进", "更新至第13集", "更新至第12集", 12, true, "更新至第13集", 13},
		// 集数相等、写法变 → 不推，基线保持（不能推进到新写法，否则下一轮误报）
		{"相等写法变", "第12集", "更新至12集", 12, false, "更新至12集", 12},
		// 集数回退 → 不推，基线保持
		{"回退", "更新至第11集", "更新至第12集", 12, false, "更新至第12集", 12},
		// 解析不出、字符串变化 → 推送（退化为字符串比较）
		{"变完结", "完结", "更新至第12集", 12, true, "完结", 12},
		// 解析不出、字符串不变 → 不推
		{"完结不变", "完结", "完结", 12, false, "完结", 12},
		// 首次关注（基线空）→ 不推，建基线
		{"首次关注数字", "更新至第5集", "", 0, false, "更新至第5集", 5},
		{"首次关注完结", "完结", "", 0, false, "完结", 0},
		// 情况 B 后回到数字集数且前进 → 推送（baseEpisode 仍是完结前的 12）
		{"完结后更新", "更新至第13集", "完结", 12, true, "更新至第13集", 13},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotNotify, gotRemarks, gotEpisode := DecideUpdate(c.latest, c.baseRemarks, c.baseEpisode)
			if gotNotify != c.wantNotify {
				t.Errorf("notify = %v, want %v", gotNotify, c.wantNotify)
			}
			if gotRemarks != c.wantRemarks {
				t.Errorf("remarks = %q, want %q", gotRemarks, c.wantRemarks)
			}
			if gotEpisode != c.wantEpisode {
				t.Errorf("episode = %d, want %d", gotEpisode, c.wantEpisode)
			}
		})
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/scheduler -run TestDecideUpdate -v`
预期：FAIL，报错 `undefined: DecideUpdate`。

- [ ] **步骤 3：编写最少实现代码**

创建 `internal/scheduler/update.go`：

```go
package scheduler

import "github.com/user/anime-tip/internal/episode"

// DecideUpdate 依据本轮 remarks 与库中基线，判定是否推送并给出应写入的新基线。
//   latest       = 本轮抓到的 vod_remarks
//   baseRemarks  = 库中 last_notified_remarks（上次推送时的 remarks）
//   baseEpisode  = 库中 last_notified_episode（上次推送时解析出的集数）
// 返回：
//   shouldNotify = 是否触发推送
//   newRemarks   = 应写入的 last_notified_remarks
//   newEpisode   = 应写入的 last_notified_episode
//
// 判定规则见规格第 6 节：
//   - 能解析集数：仅当 newEp > baseEpisode（严格前进）才推送并推进基线；相等/回退不推、基线不变。
//   - 不能解析集数：退化为字符串比较，变化即推送；基线 remarks 推进、episode 保持。
//   - 首次关注（基线空）：不推送，只建基线。
func DecideUpdate(latest, baseRemarks string, baseEpisode int) (shouldNotify bool, newRemarks string, newEpisode int) {
	newEp, ok := episode.ParseEpisode(latest)
	firstTime := baseRemarks == "" && baseEpisode == 0

	// 首次关注：建基线、不推送
	if firstTime {
		newRemarks = latest
		if ok {
			newEpisode = newEp
		}
		return false, newRemarks, newEpisode
	}

	if ok {
		// 情况 A：能解析集数，严格单调递增才推送
		if baseEpisode > 0 && newEp > baseEpisode {
			return true, latest, newEp
		}
		// 相等 / 回退 / baseEpisode==0 但非首次：不推，基线保持
		return false, baseRemarks, baseEpisode
	}

	// 情况 B：解析不出集数，退化为字符串比较
	if latest != baseRemarks {
		return true, latest, baseEpisode
	}
	return false, baseRemarks, baseEpisode
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/scheduler -run TestDecideUpdate -v`
预期：PASS（全部子用例通过）。

- [ ] **步骤 5：Commit**

```bash
git add internal/scheduler/update.go internal/scheduler/update_test.go
git commit -m "feat(scheduler): 新增 DecideUpdate 单调性判定状态机"
```

---

### 任务 3：`Anime` 模型加 `LastNotifiedEpisode` 字段

**文件：**
- 修改：`internal/model/anime.go:10-18`

- [ ] **步骤 1：修改结构体**

在 `internal/model/anime.go` 的 `Anime` 结构体中，`LastNotifiedRemarks` 之后加字段：

```go
// Anime 关注的动漫记录
type Anime struct {
	ID                  int64     `json:"id"`
	VodID               int       `json:"vod_id"`
	Name                string    `json:"name"`
	Cover               string    `json:"cover"`
	CurrentRemarks      string    `json:"current_remarks"`
	LastNotifiedRemarks string    `json:"last_notified_remarks"`
	LastNotifiedEpisode int       `json:"last_notified_episode"`
	CreatedAt           time.Time `json:"created_at"`
}
```

- [ ] **步骤 2：验证编译**

运行：`go build ./...`
预期：编译失败，报错指向 `store/anime.go` 的 `Scan`/`Exec` 列数不匹配（此时尚未改 store，属预期，下一任务修复）。记录报错位置供任务 4 对照。

- [ ] **步骤 3：Commit**

```bash
git add internal/model/anime.go
git commit -m "feat(model): Anime 新增 LastNotifiedEpisode 字段"
```

---

### 任务 4：store 迁移与 SQL 加列

**文件：**
- 修改：`internal/store/db.go:21-39`（migrate）
- 修改：`internal/store/anime.go`（ListAnimes / GetAnimeByVodID / CreateAnime / UpdateAnimeRemarks）
- 测试：`internal/store/anime_test.go`（新建，迁移幂等 + 读写列）

- [ ] **步骤 1：编写迁移与读写的失败测试**

创建 `internal/store/anime_test.go`：

```go
package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	p := filepath.Join(t.TempDir(), "test.db")
	db, err := InitDB(p)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// migrate 应建出 last_notified_episode 列。
func TestMigrate_addsLastNotifiedEpisodeColumn(t *testing.T) {
	db := newTestDB(t)

	var hasCol bool
	err := db.QueryRow(`SELECT COUNT(*) > 0 FROM pragma_table_info('animes') WHERE name='last_notified_episode'`).Scan(&hasCol)
	if err != nil {
		t.Fatalf("查询列: %v", err)
	}
	if !hasCol {
		t.Fatal("期望 animes 表存在 last_notified_episode 列")
	}
}

// 重复打开同一库应幂等无错（CREATE TABLE IF NOT EXISTS 跳过 + ALTER 补列遇 duplicate column 被忽略）。
func TestMigrate_idempotentOnExistingDB(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test.db")
	if _, err := InitDB(p); err != nil {
		t.Fatalf("首次 InitDB: %v", err)
	}
	// 第二次打开同一库：CREATE TABLE IF NOT EXISTS 跳过，ALTER 补列遇「duplicate column」被忽略
	if _, err := InitDB(p); err != nil {
		t.Fatalf("二次 InitDB 应幂等无错: %v", err)
	}
}

// UpdateAnimeRemarks 应同时写入 last_notified_episode。
func TestUpdateAnimeRemarks_writesEpisode(t *testing.T) {
	db := newTestDB(t)
	a := &struct{ VodID int; Name string }{1, "测试番"}
	if _, err := db.Exec(`INSERT INTO animes (vod_id, name, current_remarks, last_notified_remarks) VALUES (?, ?, '', '')`, a.VodID, a.Name); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var id int64
	if err := db.QueryRow(`SELECT id FROM animes WHERE vod_id=1`).Scan(&id); err != nil {
		t.Fatalf("查 id: %v", err)
	}

	if err := UpdateAnimeRemarks(db, id, "更新至第13集", "更新至第13集", 13); err != nil {
		t.Fatalf("UpdateAnimeRemarks: %v", err)
	}

	var ep int
	if err := db.QueryRow(`SELECT last_notified_episode FROM animes WHERE id=?`, id).Scan(&ep); err != nil {
		t.Fatalf("查 episode: %v", err)
	}
	if ep != 13 {
		t.Errorf("last_notified_episode 期望 13，得到 %d", ep)
	}
}

// ListAnimes 应回填 LastNotifiedEpisode。
func TestListAnimes_returnsEpisode(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.Exec(`INSERT INTO animes (vod_id, name, current_remarks, last_notified_remarks, last_notified_episode) VALUES (1, 'x', '', '', 7)`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	got, err := ListAnimes(db)
	if err != nil {
		t.Fatalf("ListAnimes: %v", err)
	}
	if len(got) != 1 || got[0].LastNotifiedEpisode != 7 {
		t.Fatalf("期望 1 条且 episode=7，得到 %+v", got)
	}
}
```

注：`TestMigrate_idempotentOnExistingDB` 验证 `ALTER` 补列的幂等性（列已存在时捕获 `duplicate column` 错误并忽略）。老库（无该列的旧 schema）的补列路径由 `migrate` 中 `ALTER` 语句本身覆盖；若需显式验证老库补列，可新建一个仅含旧列的临时表后调用 `migrate`，但属可选增强，本计划不强制。

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/store -v`
预期：FAIL（`UpdateAnimeRemarks` 签名不匹配编译错，或列缺失）。

- [ ] **步骤 3：编写实现代码**

3a. 修改 `internal/store/db.go` 的 `migrate`（在文件 import 加 `"strings"`）：

```go
package store

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

func InitDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS animes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			vod_id INTEGER NOT NULL UNIQUE,
			name TEXT NOT NULL,
			cover TEXT NOT NULL DEFAULT '',
			current_remarks TEXT NOT NULL DEFAULT '',
			last_notified_remarks TEXT NOT NULL DEFAULT '',
			last_notified_episode INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		);
	`)
	if err != nil {
		return err
	}

	// 老库补列（幂等）：SQLite 无 ADD COLUMN IF NOT EXISTS，靠捕获 duplicate column 错误忽略。
	_, err = db.Exec(`ALTER TABLE animes ADD COLUMN last_notified_episode INTEGER NOT NULL DEFAULT 0`)
	if err != nil && !strings.Contains(err.Error(), "duplicate column") {
		return err
	}
	return nil
}
```

3b. 修改 `internal/store/anime.go`，所有 SELECT/INSERT/UPDATE 加列，`UpdateAnimeRemarks` 签名加 `lastNotifiedEpisode int`：

```go
package store

import (
	"database/sql"
	"fmt"

	"github.com/user/anime-tip/internal/model"
)

func ListAnimes(db *sql.DB) ([]model.Anime, error) {
	rows, err := db.Query(`SELECT id, vod_id, name, cover, current_remarks, last_notified_remarks, last_notified_episode, created_at FROM animes ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("query animes: %w", err)
	}
	defer rows.Close()

	var animes []model.Anime
	for rows.Next() {
		var a model.Anime
		if err := rows.Scan(&a.ID, &a.VodID, &a.Name, &a.Cover, &a.CurrentRemarks, &a.LastNotifiedRemarks, &a.LastNotifiedEpisode, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan anime: %w", err)
		}
		animes = append(animes, a)
	}
	return animes, rows.Err()
}

func GetAnimeByVodID(db *sql.DB, vodID int) (*model.Anime, error) {
	var a model.Anime
	err := db.QueryRow(`SELECT id, vod_id, name, cover, current_remarks, last_notified_remarks, last_notified_episode, created_at FROM animes WHERE vod_id = ?`, vodID).Scan(
		&a.ID, &a.VodID, &a.Name, &a.Cover, &a.CurrentRemarks, &a.LastNotifiedRemarks, &a.LastNotifiedEpisode, &a.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get anime by vod_id: %w", err)
	}
	return &a, nil
}

func CreateAnime(db *sql.DB, a *model.Anime) error {
	_, err := db.Exec(`INSERT INTO animes (vod_id, name, cover, current_remarks, last_notified_remarks, last_notified_episode) VALUES (?, ?, ?, ?, '', 0)`,
		a.VodID, a.Name, a.Cover, a.CurrentRemarks,
	)
	return err
}

func DeleteAnime(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM animes WHERE id = ?`, id)
	return err
}

// UpdateAnimeRemarks 推送成功后写入基线：current_remarks 同步最新抓取值（展示用），
// last_notified_remarks / last_notified_episode 为判定基线。
func UpdateAnimeRemarks(db *sql.DB, id int64, currentRemarks, lastNotifiedRemarks string, lastNotifiedEpisode int) error {
	_, err := db.Exec(`UPDATE animes SET current_remarks = ?, last_notified_remarks = ?, last_notified_episode = ? WHERE id = ?`,
		currentRemarks, lastNotifiedRemarks, lastNotifiedEpisode, id,
	)
	return err
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/store -v`
预期：PASS（全部用例通过）。

- [ ] **步骤 5：Commit**

```bash
git add internal/store/db.go internal/store/anime.go internal/store/anime_test.go
git commit -m "feat(store): animes 新增 last_notified_episode 列与幂等迁移"
```

---

### 任务 5：`CheckUpdates` 编排改造

**文件：**
- 修改：`internal/scheduler/scheduler.go:48-131`（CheckUpdates 循环体）

- [ ] **步骤 1：修改 CheckUpdates 为编排**

把 `internal/scheduler/scheduler.go` 的 `CheckUpdates` 中 `for _, a := range animes` 循环体及 `toUpdate` 结构替换如下（其余并发互斥、计时、推送聚合、落库顺序不变）：

```go
	var updates []notify.UpdateItem
	var toUpdate []struct {
		id              int64
		currentRemarks string
		lastRemarks    string
		lastEpisode    int
	}

	for _, a := range animes {
		detail, err := s.crawler.GetAnimeDetail(a.VodID)
		if err != nil {
			log.Printf("[scheduler] 获取动漫 %s (vod_id=%d) 详情失败: %v", a.Name, a.VodID, err)
			continue
		}

		latestRemarks := detail.VodRemarks
		shouldNotify, newRemarks, newEpisode := DecideUpdate(latestRemarks, a.LastNotifiedRemarks, a.LastNotifiedEpisode)

		if shouldNotify {
			updates = append(updates, notify.UpdateItem{
				Name:      a.Name,
				Remarks:   latestRemarks,
				DetailURL: s.crawler.DetailURL(a.VodID),
			})
		}

		// 仅当基线有变化时才收集更新（避免无谓写库）
		if newRemarks != a.LastNotifiedRemarks || newEpisode != a.LastNotifiedEpisode {
			toUpdate = append(toUpdate, struct {
				id              int64
				currentRemarks string
				lastRemarks    string
				lastEpisode    int
			}{a.ID, latestRemarks, newRemarks, newEpisode})
		}
	}
```

并把落库循环改为传新签名：

```go
	// 推送成功后更新数据库
	for _, u := range toUpdate {
		if err := store.UpdateAnimeRemarks(s.db, u.id, u.currentRemarks, u.lastRemarks, u.lastEpisode); err != nil {
			log.Printf("[scheduler] 更新动漫 remarks 失败 (id=%d): %v", u.id, err)
		}
	}
```

- [ ] **步骤 2：验证全量编译与测试**

运行：`go build ./... && go test ./...`
预期：编译通过，`episode`/`scheduler`/`store` 全部测试 PASS。

- [ ] **步骤 3：Commit**

```bash
git add internal/scheduler/scheduler.go
git commit -m "feat(scheduler): CheckUpdates 改用 DecideUpdate 编排，单调性判定"
```

---

### 任务 6：落库顺序回归测试

**文件：**
- 测试：`internal/scheduler/scheduler_test.go`（新建）

- [ ] **步骤 1：编写失败测试**

`CheckUpdates` 依赖 `*crawler.Client` 与 Server 酨推送（外网）。为可测，测试用真实 SQLite 库 + 一个 stub HTTP server 模拟苹果 CMS vod 接口；Server 配推送必然失败（无 key），用于验证「推送失败时基线不被推进」。若 stub 过重，退化方案：仅验证「首次关注建基线不推送」路径——该路径不触发推送，可用任意可达 crawler baseURL。

创建 `internal/scheduler/scheduler_test.go`：

```go
package scheduler

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/user/anime-tip/internal/crawler"
	"github.com/user/anime-tip/internal/store"
)

// stubVodServer 返回一个模拟苹果 CMS vod 接口的 server，按 ids 返回对应 remarks。
// detailJSON 模板：{code,list:[{vod_id, vod_name, vod_remarks}]}
func stubVodServer(t *testing.T, remarksByID map[int]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ids := r.URL.Query().Get("ids")
		remarks := remarksByID
		_ = ids
		body := `{"code":1,"msg":"","list":[{"vod_id":1,"vod_name":"x","vod_remarks":"` + remarks[1] + `"}]}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
}

func newTestScheduler(t *testing.T, baseURL string) (*Scheduler, *sql.DB) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "test.db")
	db, err := store.InitDB(p)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	cr := crawler.NewClient(baseURL, "")
	return New(db, cr), db
}

// 首次关注：基线空 → 不应推送（无更新），但应建基线（current_remarks 被同步）。
// 该路径不触发 Server 配推送，避免外网依赖。
func TestCheckUpdates_firstTrackBuildsBaselineNoNotify(t *testing.T) {
	srv := stubVodServer(t, map[int]string{1: "更新至第5集"})
	defer srv.Close()

	sched, db := newTestScheduler(t, srv.URL)
	// seed 一条基线空的关注记录
	if _, err := db.Exec(`INSERT INTO animes (vod_id, name, current_remarks, last_notified_remarks, last_notified_episode) VALUES (1, 'x', '', '', 0)`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// 未配置 server_chan_key，若误推送会因 key 为空报错；此处期望无更新故不推送、无错
	if err := sched.CheckUpdates(); err != nil {
		t.Fatalf("首次检查不应报错: %v", err)
	}

	// 基线应被建立：last_notified_remarks 推进到最新，episode=5
	var remarks string
	var ep int
	if err := db.QueryRow(`SELECT last_notified_remarks, last_notified_episode FROM animes WHERE vod_id=1`).Scan(&remarks, &ep); err != nil {
		t.Fatalf("查基线: %v", err)
	}
	if remarks != "更新至第5集" {
		t.Errorf("基线 remarks 期望 推进到 更新至第5集，得到 %q", remarks)
	}
	if ep != 5 {
		t.Errorf("基线 episode 期望 5，得到 %d", ep)
	}
}

// 集数前进：第二轮 baseEpisode=5 → 6 应触发推送；因无 server_chan_key 推送会失败，
// 此时基线必须保持不变（不推进），验证「推送成功后才落库」。
func TestCheckUpdates_pushFailureKeepsBaseline(t *testing.T) {
	srv := stubVodServer(t, map[int]string{1: "更新至第6集"})
	defer srv.Close()

	sched, db := newTestScheduler(t, srv.URL)
	if _, err := db.Exec(`INSERT INTO animes (vod_id, name, current_remarks, last_notified_remarks, last_notified_episode) VALUES (1, 'x', '更新至第5集', '更新至第5集', 5)`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	err := sched.CheckUpdates()
	if err == nil {
		t.Fatal("期望推送失败返回 error（无 server_chan_key），得到 nil")
	}

	// 推送失败 → 基线保持 5，未被推进到 6
	var ep int
	if err := db.QueryRow(`SELECT last_notified_episode FROM animes WHERE vod_id=1`).Scan(&ep); err != nil {
		t.Fatalf("查基线: %v", err)
	}
	if ep != 5 {
		t.Errorf("推送失败时基线应保持 5，得到 %d", ep)
	}
}
```

- [ ] **步骤 2：运行测试验证失败/通过**

运行：`go test ./internal/scheduler -v`
预期：两个用例 PASS。
- 若 `TestCheckUpdates_firstTrackBuildsBaselineNoNotify` 因 stub 返回固定 remarks 而通过，符合预期。
- 若 `TestCheckUpdates_pushFailureKeepsBaseline` 因 `CheckUpdates` 在推送失败时 `return err` 早退、未落库 → PASS（基线保持）。若失败，说明编排顺序被破坏，回到任务 5 检查。

- [ ] **步骤 3：Commit**

```bash
git add internal/scheduler/scheduler_test.go
git commit -m "test(scheduler): 首次建基线与推送失败保持基线的回归测试"
```

---

## 自检

**1. 规格覆盖度**
- 第 3 节数据模型 `last_notified_episode` → 任务 3（model）+ 任务 4（迁移）。✓
- 第 4 节迁移方式（幂等 ALTER）→ 任务 4 步骤 3a + 测试 `TestMigrate_idempotentOnExistingDB`。✓
- 第 5 节 `ParseEpisode` 解析规则（优先 集/话 后缀，退化首个数字，覆盖表）→ 任务 1，测试覆盖表全部用例含 `2024年第10集`。✓
- 第 6 节 `DecideUpdate` 状态机（情况 A/B/C、回退基线保持）→ 任务 2，测试覆盖前进/相等/回退/变完结/完结不变/首次/完结后更新。✓
- 第 7 节编排改造 → 任务 5。✓
- 第 8 节错误处理（抓取失败 continue、推送失败不落库）→ 任务 5 编排 + 任务 6 回归测试。✓
- 第 9 节测试策略 → 任务 1/2/6。✓
- 无遗漏章节。

**2. 占位符扫描**：无 TODO/待定；任务 4 步骤 1 测试中 `p2` 段已在同步骤明确要求删除（非占位，是显式清理指令）。✓

**3. 类型一致性**
- `ParseEpisode(remarks string) (int, bool)`：任务 1 定义，任务 2 调用。✓
- `DecideUpdate(latest, baseRemarks string, baseEpisode int) (bool, string, int)`：任务 2 定义，任务 5 调用，返回值顺序 (shouldNotify, newRemarks, newEpisode) 一致。✓
- `UpdateAnimeRemarks(db, id int64, currentRemarks, lastNotifiedRemarks string, lastNotifiedEpisode int)`：任务 4 定义，任务 5 调用参数顺序 (u.id, u.currentRemarks, u.lastRemarks, u.lastEpisode) 一致。✓
- `Anime.LastNotifiedEpisode int`：任务 3 定义，任务 4 SELECT/Scan、任务 5 读取 `a.LastNotifiedRemarks`/`a.LastNotifiedEpisode` 一致。✓

类型一致，无矛盾。
