# 关注列表导出/导入（备份）实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 在「设置」tab 增加「数据备份」分组，支持一键导出当前关注列表为 JSON 文件、一键从 JSON 文件导入恢复关注清单。

**架构：** 后端在 store 层新增事务原子的 `ImportAnimes`（含 play_url 的独立 INSERT，不复用 `CreateAnime`），handler 层新增 `import_export_handler.go` 承载导出/导入两个端点：导出在 handler 层做字段白名单裁剪（不动 `model.Anime`），导入在 handler 层做整体+逐条校验后调 store。前端用 Alpine.js 在设置 tab 加导出按钮（fetch→blob→`<a download>`）和导入（file input→`FileReader`→`JSON.parse`→POST 整个对象）。

**技术栈：** Go 1.25 + gin + modernc.org/sqlite（纯 Go 驱动，无 cgo）；前端 Alpine.js 3 + 原生 fetch/FileReader；测试用标准 `testing` + `httptest` + 真实 sqlite 临时库（参考 `notify_handler_test.go` 模式）。

---

## 文件结构

| 文件 | 动作 | 职责 |
|---|---|---|
| `internal/store/anime.go` | 修改（追加） | 新增 `ImportItem`/`ImportResult`/`ImportError` 类型 + `ImportAnimes` 函数：开事务，逐条 `GetAnimeByVodID` 判重（命中→skipped），未命中→含 play_url 的独立 INSERT；单条失败记 error 不回滚整批 |
| `internal/store/anime_test.go` | 修改（追加） | `TestImportAnimes`：覆盖 imported 计数+play_url 落库、skipped 不覆盖本地 play_url、单条失败 error.Index 一致且其余提交 |
| `internal/web/import_export_handler.go` | **新建** | `ExportAnimes`（`store.ListAnimes`→字段裁剪→JSON 下载头）+ `ImportAnimes`（version/上限整体校验→逐条校验→调 store→合并 errors）+ 裁剪结构 `exportAnime` |
| `internal/web/import_export_handler_test.go` | **新建** | 导出字段白名单+下载头、导入合法批/version/上限/逐条非法、往返还原 |
| `internal/web/handler.go` | 修改 | `RegisterRoutes` 注册 `GET /api/animes/export`、`POST /api/animes/import` |
| `web/index.html` | 修改 | 设置 tab 增「数据备份」分组（导出按钮 + file input + 导入按钮）+ `exportFollows`/`importFollows`/`onImportFilePicked` JS 方法 |
| `web/static/style.css` | 修改（追加） | `.backup-group`、`.backup-row`、`.import-result` 样式，复用现有按钮风格 |

**不动：** `internal/model/anime.go`、`scheduler`、`crawler`、`notify`、`CreateAnime`/`UpdateAnime`/`DeleteAnime`、关注/搜索流程、settings 读写、数据库 schema。

---

## 关键约定（贯穿全计划，命名必须一致）

- store 类型名：`ImportItem`、`ImportResult`、`ImportError`，函数 `ImportAnimes(db *sql.DB, items []ImportItem) (ImportResult, error)`。
- `ImportItem.Index` 是该条在**原始导出数组**中的下标，store 写入失败时回填到 `ImportError.Index`，保证与前端展示的「第 N 条」一致。
- handler 裁剪结构 `exportAnime` 字段 json tag：`vod_id`/`name`/`cover`/`play_url`，**不含** `id`/`current_remarks`/`last_notified_*`/`created_at`。
- 导出文件顶层结构：`{ "version": 1, "exported_at": "<RFC3339>", "animes": [...] }`。
- 导入请求体即导出文件原样对象，handler 取 `animes` 字段，`version` 用于校验（缺省或 1 通过，其它→400），`exported_at` 忽略。
- 数组上限 `2000`，超过→400。
- `play_url` 校验复用 `validatePlayURL`（`anime_handler.go` 已有，空串合法）。
- 响应 json：`{ "imported": N, "skipped": N, "errors": [{"index":N,"vod_id":N,"error":"..."}] }`。
- 导出下载头：`Content-Disposition: attachment; filename="anime-tip-follows-<YYYY-MM-DD>.json"`。
- 前端方法名：`exportFollows()`、`importFollows()`、`onImportFilePicked(event)`；状态字段 `exporting`、`importing`、`importResult`。

---

### 任务 1：store 层 — `ImportAnimes` 及结果类型

**文件：**
- 修改：`internal/store/anime.go`（在文件末尾追加）
- 测试：`internal/store/anime_test.go`（在文件末尾追加）

- [ ] **步骤 1：编写失败的测试**

在 `internal/store/anime_test.go` 末尾追加：

```go
// ImportAnimes 应：全新条目插入并落 play_url（imported++）；已存在 vod_id 跳过且不覆盖本地 play_url（skipped++）；
// 单条 SQL 失败记入 errors 且 Index 与传入下标一致，事务不回滚其余合法条。
func TestImportAnimes(t *testing.T) {
	db := newTestDB(t)

	// 预置一条已存在记录，本地 play_url 为 "https://keep-local/1"
	if _, err := db.Exec(`INSERT INTO animes (vod_id, name, cover, current_remarks, last_notified_remarks, last_notified_episode, play_url) VALUES (100, '已存在番', '', '', '', 0, 'https://keep-local/1'`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_ = db.Close()
	t.Setenv("", "") // no-op
	// 重新打开以保证 seed 生效（newTestDB 已 close，下面用新库重做更清晰）
```

> 注意：上面 seed 块有闭合问题。**用下面这个干净的完整版本替换上面追加内容**——直接追加这段，不要追加有问题的版本：

```go
// ImportAnimes 应：全新条目插入并落 play_url（imported++）；已存在 vod_id 跳过且不覆盖本地 play_url（skipped++）；
// 单条 SQL 失败记入 errors 且 Index 与传入下标一致，事务不回滚其余合法条。
func TestImportAnimes(t *testing.T) {
	db := newTestDB(t)

	// 预置一条已存在记录，本地 play_url 为 "https://keep-local/1"
	if _, err := db.Exec(`INSERT INTO animes (vod_id, name, cover, current_remarks, last_notified_remarks, last_notified_episode, play_url) VALUES (100, '已存在番', '', '', '', 0, 'https://keep-local/1')`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	items := []ImportItem{
		{Index: 0, Anime: model.Anime{VodID: 100, Name: "已存在番-导入侧", Cover: "", PlayURL: "https://overwrite/100"}}, // skipped，本地 play_url 不变
		{Index: 1, Anime: model.Anime{VodID: 200, Name: "全新番A", Cover: "https://c/200", PlayURL: "https://play/200"}},  // imported
		{Index: 2, Anime: model.Anime{VodID: 300, Name: "全新番B", Cover: "", PlayURL: ""}},                              // imported，空 play_url
	}

	got, err := ImportAnimes(db, items)
	if err != nil {
		t.Fatalf("ImportAnimes: %v", err)
	}

	if got.Imported != 2 {
		t.Errorf("Imported = %d, 期望 2", got.Imported)
	}
	if got.Skipped != 1 {
		t.Errorf("Skipped = %d, 期望 1", got.Skipped)
	}
	if len(got.Errors) != 0 {
		t.Errorf("Errors = %+v, 期望空", got.Errors)
	}

	// 已存在记录本地 play_url 不被覆盖
	existing, err := GetAnimeByVodID(db, 100)
	if err != nil {
		t.Fatalf("GetAnimeByVodID(100): %v", err)
	}
	if existing.PlayURL != "https://keep-local/1" {
		t.Errorf("已存在条 play_url = %q, 期望保持 https://keep-local/1", existing.PlayURL)
	}
	if existing.Name != "已存在番" {
		t.Errorf("已存在条 name = %q, 期望保持 '已存在番'", existing.Name)
	}

	// 新插入条 play_url 落库
	a200, err := GetAnimeByVodID(db, 200)
	if err != nil {
		t.Fatalf("GetAnimeByVodID(200): %v", err)
	}
	if a200.PlayURL != "https://play/200" {
		t.Errorf("vod_id=200 play_url = %q, 期望 https://play/200", a200.PlayURL)
	}
	// 基线重置为默认
	if a200.CurrentRemarks != "" || a200.LastNotifiedRemarks != "" || a200.LastNotifiedEpisode != 0 {
		t.Errorf("vod_id=200 基线未重置: %+v", a200)
	}

	a300, err := GetAnimeByVodID(db, 300)
	if err != nil {
		t.Fatalf("GetAnimeByVodID(300): %v", err)
	}
	if a300.PlayURL != "" {
		t.Errorf("vod_id=300 play_url = %q, 期望空串", a300.PlayURL)
	}
}

// ImportAnimes 在单条 SQL 失败时应记录 error（Index 与传入一致），且不回滚其余合法条。
func TestImportAnimes_singleFailureDoesNotRollbackBatch(t *testing.T) {
	db := newTestDB(t)

	// 构造一条合法 + 一条会触发 UNIQUE 冲突的（两条 vod_id 相同，store 内 GetAnimeByVodID 都查不到，
	// 第一条插入成功后第二条 INSERT 撞 UNIQUE → 该条入 errors，第一条保留）
	items := []ImportItem{
		{Index: 5, Anime: model.Anime{VodID: 500, Name: "番-X", Cover: "", PlayURL: ""}},
		{Index: 6, Anime: model.Anime{VodID: 500, Name: "番-X-重复", Cover: "", PlayURL: ""}}, // INSERT 撞 UNIQUE
		{Index: 7, Anime: model.Anime{VodID: 700, Name: "番-Y", Cover: "", PlayURL: "https://play/700"}},
	}

	got, err := ImportAnimes(db, items)
	if err != nil {
		t.Fatalf("ImportAnimes 返回 error: %v（期望返回 nil，单条失败进 errors）", err)
	}

	if got.Imported != 2 {
		t.Errorf("Imported = %d, 期望 2（500 与 700 插入成功）", got.Imported)
	}
	if len(got.Errors) != 1 {
		t.Fatalf("Errors 长度 = %d, 期望 1，实际 %+v", len(got.Errors), got.Errors)
	}
	if got.Errors[0].Index != 6 {
		t.Errorf("Errors[0].Index = %d, 期望 6", got.Errors[0].Index)
	}
	if got.Errors[0].VodID != 500 {
		t.Errorf("Errors[0].VodID = %d, 期望 500", got.Errors[0].VodID)
	}

	// 700 仍应存在，证明未回滚
	a700, err := GetAnimeByVodID(db, 700)
	if err != nil {
		t.Fatalf("GetAnimeByVodID(700): %v", err)
	}
	if a700 == nil {
		t.Fatal("vod_id=700 应存在（未回滚）")
	}
	if a700.PlayURL != "https://play/700" {
		t.Errorf("vod_id=700 play_url = %q, 期望 https://play/700", a700.PlayURL)
	}
}
```

> 同时在 `anime_test.go` 顶部 import 块补 `model` 包（若尚无）。当前 import 为 `database/sql`、`path/filepath`、`testing`。改为：

```go
import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/user/anime-tip/internal/model"
)
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/store/ -run TestImportAnimes -v`
预期：FAIL（编译错误），报错 `undefined: ImportItem` / `undefined: ImportAnimes`（store 层尚未定义）。

- [ ] **步骤 3：编写最少实现代码**

在 `internal/store/anime.go` 末尾追加（注意：本任务不引入新 import，`database/sql`/`fmt`/`model` 已在文件顶部 import）：

```go
// ImportItem 承载一条待导入记录及其在原导出数组中的下标，
// 便于 store 写入失败时回填与原文件一致的 error index。
type ImportItem struct {
	Index int
	Anime model.Anime
}

// ImportError 描述一条导入失败的条目。Index 为其在原导出数组中的下标。
type ImportError struct {
	Index int    `json:"index"`
	VodID int    `json:"vod_id"`
	Error string `json:"error"`
}

// ImportResult 汇总一次导入的计数与失败明细。
type ImportResult struct {
	Imported int           `json:"imported"`
	Skipped  int           `json:"skipped"`
	Errors   []ImportError `json:"errors"`
}

// ImportAnimes 在单个事务内逐条导入。
// 传入的 items 应已通过 handler 层逐条校验（vod_id 正整数、name 非空、play_url 合法）。
// - 已存在 vod_id → Skipped++，保留本地记录与本地 play_url 不变。
// - 未命中 → 插入，INSERT 含 play_url，进度基线留默认空值/0（「重新关注」语义）。
// - 单条 SQL 失败 → 记 ImportError 继续下一条，不回滚整批；事务最终提交所有合法条。
// 与现有 CreateAnime 的 INSERT（不带 play_url）不同，故单独写语句，不改 CreateAnime。
func ImportAnimes(db *sql.DB, items []ImportItem) (ImportResult, error) {
	result := ImportResult{Errors: []ImportError{}}

	tx, err := db.Begin()
	if err != nil {
		return result, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // 提交成功后 Rollback 无副作用

	for _, item := range items {
		// 判重：命中则跳过，保留本地记录
		var existingID int64
		err := tx.QueryRow(`SELECT id FROM animes WHERE vod_id = ?`, item.Anime.VodID).Scan(&existingID)
		if err == nil {
			result.Skipped++
			continue
		}
		if err != sql.ErrNoRows {
			result.Errors = append(result.Errors, ImportError{
				Index: item.Index, VodID: item.Anime.VodID, Error: fmt.Sprintf("查重失败: %v", err),
			})
			continue
		}

		// 未命中 → 插入，含 play_url，基线留默认
		_, err = tx.Exec(
			`INSERT INTO animes (vod_id, name, cover, current_remarks, last_notified_remarks, last_notified_episode, play_url) VALUES (?, ?, ?, '', '', 0, ?)`,
			item.Anime.VodID, item.Anime.Name, item.Anime.Cover, item.Anime.PlayURL,
		)
		if err != nil {
			result.Errors = append(result.Errors, ImportError{
				Index: item.Index, VodID: item.Anime.VodID, Error: fmt.Sprintf("插入失败: %v", err),
			})
			continue
		}
		result.Imported++
	}

	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("commit tx: %w", err)
	}
	return result, nil
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/store/ -run TestImportAnimes -v`
预期：PASS（两个测试用例均通过）。

随后运行全量 store 测试确认无回归：`go test ./internal/store/ -v`
预期：PASS。

- [ ] **步骤 5：Commit**

```bash
git add internal/store/anime.go internal/store/anime_test.go
git commit -m "feat(store): 新增 ImportAnimes 事务原子导入关注列表"
```

---

### 任务 2：handler 层 — 导出端点 `ExportAnimes`

**文件：**
- 创建：`internal/web/import_export_handler.go`
- 测试：`internal/web/import_export_handler_test.go`（本任务先建文件 + 导出测试，导入测试留到任务 3）

- [ ] **步骤 1：编写失败的测试**

创建 `internal/web/import_export_handler_test.go`：

```go
package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/user/anime-tip/internal/model"
	"github.com/user/anime-tip/internal/store"
)

// seedAnime 直接插一条全字段记录，便于断言导出裁剪后不含基线/主键字段。
func seedAnime(t *testing.T, db storeDB, vodID int, name, playURL string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO animes (vod_id, name, cover, current_remarks, last_notified_remarks, last_notified_episode, play_url) VALUES (?, ?, 'https://c/'+?, '更新至第3集', '更新至第2集', 2, ?)`,
		vodID, name, name, playURL,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

// storeDB 是 seedAnime 接受的最小抽象，便于传 *sql.DB（Handler.db 同类型）。
type storeDB interface {
	Exec(query string, args ...any) (any, error)
}
```

> 上面 `storeDB`/`seedAnime` 用 `Exec` 返回 `(any, error)` 与 `*sql.DB.Exec` 的 `(_, error)` 不匹配。**改用直接接收 `*sql.DB` 的干净版本**——`internal/web` 包测试里 `h.db` 就是 `*sql.DB`。用这个版本（替换上面 `seedAnime`+`storeDB` 整块）：

```go
// seedAnime 直接插一条带基线/主键的记录，便于断言导出裁剪后不含这些字段。
func seedAnime(t *testing.T, db *sql.DB, vodID int, name, playURL string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO animes (vod_id, name, cover, current_remarks, last_notified_remarks, last_notified_episode, play_url) VALUES (?, ?, 'https://c/`+name+`', '当前第3集', '已通知第2集', 2, ?)`,
		vodID, name, playURL,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

// callExport 构造一个挂载 ExportAnimes 的 gin 上下文并调用，返回状态码、body、Content-Disposition。
func callExport(t *testing.T, h *Handler) (code int, body []byte, disposition string) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/animes/export", nil)

	h.ExportAnimes(c)

	code = w.Code
	body = w.Body.Bytes()
	disposition = w.Header().Get("Content-Disposition")
	return
}
```

> `import_export_handler_test.go` 顶部 import 块最终应为：

```go
import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/user/anime-tip/internal/store"
)
```

（`time` 不要 import；上面草稿里误写了，删掉。`model` 也不需要在测试文件顶部 import，除非下面用到——本任务测试体里不直接用 `model.Anime`，只通过 `seedAnime` 插 SQL，故不需要。最终 import 块即上面这版。）

继续在 `import_export_handler_test.go` 追加导出测试体：

```go
// ExportAnimes 应返回 {version:1, exported_at, animes}，animes 字段为白名单（无 id/基线/created_at），
// 且 Content-Disposition 含日期文件名。
func TestExportAnimes(t *testing.T) {
	h := newTestHandlerDB(t)
	seedAnime(t, h.db, 28802, "葬送的芙莉莲", "https://play/28802")
	seedAnime(t, h.db, 28803, "鬼灭之刃", "") // 无 play_url

	code, body, disposition := callExport(t, h)
	if code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200", code)
	}

	var got struct {
		Version    int `json:"version"`
		ExportedAt string `json:"exported_at"`
		Animes     []map[string]any `json:"animes"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("解析响应: %v; body=%s", err, body)
	}
	if got.Version != 1 {
		t.Errorf("version = %d, 期望 1", got.Version)
	}
	if got.ExportedAt == "" {
		t.Error("exported_at 不应为空")
	}
	if _, err := time.Parse(time.RFC3339, got.ExportedAt); err != nil {
		t.Errorf("exported_at 不是合法 RFC3339: %q (%v)", got.ExportedAt, err)
	}
	if len(got.Animes) != 2 {
		t.Fatalf("animes 长度 = %d, 期望 2", len(got.Animes))
	}

	// 字段白名单：每条只应有 vod_id/name/cover/play_url 四个键
	for i, a := range got.Animes {
		for k := range a {
			switch k {
			case "vod_id", "name", "cover", "play_url":
			default:
				t.Errorf("animes[%d] 含非法字段 %q（应被裁剪）", i, k)
			}
		}
	}

	// 按 vod_id 找到芙莉莲，校验 play_url
	var frieren map[string]any
	for _, a := range got.Animes {
		if a["vod_id"].(float64) == 28802 {
			frieren = a
		}
	}
	if frieren == nil {
		t.Fatal("未找到 vod_id=28802")
	}
	if frieren["play_url"] != "https://play/28802" {
		t.Errorf("play_url = %v, 期望 https://play/28802", frieren["play_url"])
	}
	if frieren["name"] != "葬送的芙莉莲" {
		t.Errorf("name = %v, 期望 葬送的芙莉莲", frieren["name"])
	}

	if disposition == "" {
		t.Fatal("Content-Disposition 不应为空")
	}
	if !strings.Contains(disposition, "attachment;") {
		t.Errorf("Content-Disposition 期望含 attachment;, 实际: %q", disposition)
	}
	if !strings.Contains(disposition, "anime-tip-follows-") {
		t.Errorf("Content-Disposition 期望含 anime-tip-follows- 前缀, 实际: %q", disposition)
	}
	if !strings.Contains(disposition, ".json") {
		t.Errorf("Content-Disposition 期望含 .json 后缀, 实际: %q", disposition)
	}
}

// ExportAnimes 在空关注列表时应返回 animes:[]（非 null）。
func TestExportAnimes_emptyList(t *testing.T) {
	h := newTestHandlerDB(t)

	code, body, _ := callExport(t, h)
	if code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200", code)
	}

	var got struct {
		Version int              `json:"version"`
		Animes  []map[string]any `json:"animes"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("解析响应: %v; body=%s", err, body)
	}
	if got.Version != 1 {
		t.Errorf("version = %d, 期望 1", got.Version)
	}
	if got.Animes == nil || len(got.Animes) != 0 {
		t.Errorf("animes = %v, 期望非 null 空数组", got.Animes)
	}
}
```

> 因上面用到了 `time`、`strings`、`*sql.DB`，更新测试文件 import 块为最终版：

```go
import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/user/anime-tip/internal/store"
)
```

（`gin`、`store` 在 `callExport`/`newTestHandlerDB` 链中用到——`newTestHandlerDB` 在 `notify_handler_test.go` 已定义，同包复用。`store` 在 `newTestHandlerDB` 内部用到，本文件不直接调 `store.` 但保留以避免删错；若编译报 unused，删除 `store` import 即可。）

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/web/ -run TestExportAnimes -v`
预期：FAIL（编译错误），报错 `h.ExportAnimes undefined`（handler 尚未实现）。

- [ ] **步骤 3：编写最少实现代码**

创建 `internal/web/import_export_handler.go`：

```go
package web

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/user/anime-tip/internal/store"
)

// exportAnime 为导出文件的字段白名单结构：仅 vod_id/name/cover/play_url，
// 明确不含 id/current_remarks/last_notified_*/created_at 等内部与基线字段。
type exportAnime struct {
	VodID   int    `json:"vod_id"`
	Name    string `json:"name"`
	Cover   string `json:"cover"`
	PlayURL string `json:"play_url"`
}

// exportFile 为导出 JSON 顶层结构。
type exportFile struct {
	Version    int           `json:"version"`
	ExportedAt string        `json:"exported_at"`
	Animes     []exportAnime `json:"animes"`
}

// ExportAnimes 导出当前关注列表为 JSON 文件下载。
// 字段裁剪只在 handler 做，model.Anime 与 store 层保持完整表征不动。
func (h *Handler) ExportAnimes(c *gin.Context) {
	animes, err := store.ListAnimes(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	out := exportFile{
		Version:    1,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Animes:     make([]exportAnime, 0, len(animes)),
	}
	for _, a := range animes {
		out.Animes = append(out.Animes, exportAnime{
			VodID:   a.VodID,
			Name:    a.Name,
			Cover:   a.Cover,
			PlayURL: a.PlayURL,
		})
	}

	filename := "anime-tip-follows-" + time.Now().UTC().Format("2006-01-02") + ".json"
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.JSON(http.StatusOK, out)
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/web/ -run TestExportAnimes -v`
预期：PASS（两个测试用例均通过）。

- [ ] **步骤 5：Commit**

```bash
git add internal/web/import_export_handler.go internal/web/import_export_handler_test.go
git commit -m "feat(web): 新增关注列表导出端点 GET /api/animes/export"
```

---

### 任务 3：handler 层 — 导入端点 `ImportAnimes`

**文件：**
- 修改：`internal/web/import_export_handler.go`（追加 `ImportAnimes` handler）
- 测试：`internal/web/import_export_handler_test.go`（追加导入测试）

- [ ] **步骤 1：编写失败的测试**

在 `internal/web/import_export_handler_test.go` 末尾追加：

```go
// callImport 构造一个挂载 ImportAnimes 的 gin 上下文，POST jsonBody，返回状态码与解析后的 body。
func callImport(t *testing.T, h *Handler, jsonBody string) (code int, body map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/animes/import", strings.NewReader(jsonBody))
	c.Request.Header.Set("Content-Type", "application/json")

	h.ImportAnimes(c)

	code = w.Code
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return
}

// ImportAnimes 合法批（含一条已存在）→ 200，imported/skipped 正确。
func TestImportAnimes_validBatch(t *testing.T) {
	h := newTestHandlerDB(t)
	seedAnime(t, h.db, 100, "已存在番", "https://keep/100")

	body := `{"version":1,"exported_at":"2026-07-01T12:00:00Z","animes":[
		{"vod_id":100,"name":"已存在-导入侧","cover":"","play_url":"https://overwrite/100"},
		{"vod_id":200,"name":"全新番","cover":"https://c/200","play_url":"https://play/200"}
	]}`
	code, resp := callImport(t, h, body)
	if code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200, body=%v", code, resp)
	}
	if got := int(resp["imported"].(float64)); got != 1 {
		t.Errorf("imported = %d, 期望 1", got)
	}
	if got := int(resp["skipped"].(float64)); got != 1 {
		t.Errorf("skipped = %d, 期望 1", got)
	}
	if errs, ok := resp["errors"].([]any); ok && len(errs) != 0 {
		t.Errorf("errors = %v, 期望空", errs)
	}

	// 已存在条本地 play_url 不被覆盖
	existing, _ := store.GetAnimeByVodID(h.db, 100)
	if existing.PlayURL != "https://keep/100" {
		t.Errorf("已存在条 play_url = %q, 期望保持 https://keep/100", existing.PlayURL)
	}
}

// ImportAnimes version 非 1 → 400。
func TestImportAnimes_unsupportedVersion(t *testing.T) {
	h := newTestHandlerDB(t)
	body := `{"version":2,"exported_at":"","animes":[]}`
	code, resp := callImport(t, h, body)
	if code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, 期望 400", code)
	}
	if !strings.Contains(resp["error"].(string), "版本") {
		t.Errorf("error = %v, 期望含 '版本'", resp["error"])
	}
}

// version 缺省应视为 1，正常导入。
func TestImportAnimes_missingVersionTreatedAs1(t *testing.T) {
	h := newTestHandlerDB(t)
	body := `{"animes":[{"vod_id":999,"name":"无版本字段番","cover":"","play_url":""}]}`
	code, resp := callImport(t, h, body)
	if code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200, body=%v", code, resp)
	}
	if got := int(resp["imported"].(float64)); got != 1 {
		t.Errorf("imported = %d, 期望 1", got)
	}
}

// 数组超上限 2000 → 400。
func TestImportAnimes_overLimit(t *testing.T) {
	h := newTestHandlerDB(t)
	// 构造 2001 条
	var sb strings.Builder
	sb.WriteString(`{"version":1,"animes":[`)
	for i := 0; i < 2001; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(`{"vod_id":`)
		// 用 strconv 不必引入，直接拼数字字面量
		// 简单做法：写死 2001 个占位，但太长。改用 fmt.Sprintf 生成。
		_ = i
		break
	}
	// 上面 break 是占位错误写法，下面用干净版本替换整个函数体
```

> 上面 `TestImportAnimes_overLimit` 的构造写得有问题（用了 break 占位）。**用下面干净版本替换整个 `TestImportAnimes_overLimit` 函数**（需在 import 块加 `"strconv"`，见步骤末）：

```go
// 数组超上限 2000 → 400。
func TestImportAnimes_overLimit(t *testing.T) {
	h := newTestHandlerDB(t)
	var sb strings.Builder
	sb.WriteString(`{"version":1,"animes":[`)
	for i := 0; i < 2001; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(`{"vod_id":`)
		sb.WriteString(strconv.Itoa(i + 1))
		sb.WriteString(`,"name":"n","cover":"","play_url":""}`)
	}
	sb.WriteString(`]}`)

	code, resp := callImport(t, h, sb.String())
	if code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, 期望 400", code)
	}
	if !strings.Contains(resp["error"].(string), "2000") {
		t.Errorf("error = %v, 期望含 '2000'", resp["error"])
	}
}

// ImportAnimes 含逐条非法条（name 空 / vod_id≤0 / play_url 非法）→ 该条进 errors，其余正常导入。
func TestImportAnimes_perItemValidation(t *testing.T) {
	h := newTestHandlerDB(t)
	body := `{"version":1,"animes":[
		{"vod_id":1,"name":"","cover":"","play_url":""},
		{"vod_id":0,"name":"零id","cover":"","play_url":""},
		{"vod_id":-5,"name":"负id","cover":"","play_url":""},
		{"vod_id":2,"name":"非法url","cover":"","play_url":"ftp://x"},
		{"vod_id":3,"name":"合法番","cover":"","play_url":"https://play/3"}
	]}`
	code, resp := callImport(t, h, body)
	if code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200, body=%v", code, resp)
	}
	if got := int(resp["imported"].(float64)); got != 1 {
		t.Errorf("imported = %d, 期望 1（仅 vod_id=3 合法）", got)
	}
	errs, _ := resp["errors"].([]any)
	if len(errs) != 4 {
		t.Fatalf("errors 长度 = %d, 期望 4, 实际 %v", len(errs), errs)
	}
	// 校验每条 error 的 index 与 vod_id
	wantIndices := map[int]int{0: 1, 1: 0, 2: -5, 3: 2}
	gotIndices := map[int]int{}
	for _, e := range errs {
		em, _ := e.(map[string]any)
		idx := int(em["index"].(float64))
		vid := int(em["vod_id"].(float64))
		gotIndices[idx] = vid
	}
	for idx, vid := range wantIndices {
		if gotIndices[idx] != vid {
			t.Errorf("index=%d 的 vod_id = %d, 期望 %d", idx, gotIndices[idx], vid)
		}
	}
	// 非法条不应入库
	if a, _ := store.GetAnimeByVodID(h.db, 2); a != nil {
		t.Error("vod_id=2（非法url）不应入库")
	}
	if a, _ := store.GetAnimeByVodID(h.db, 3); a == nil {
		t.Error("vod_id=3（合法）应入库")
	}
}

// ImportAnimes 请求体非合法 JSON → 400。
func TestImportAnimes_badJSON(t *testing.T) {
	h := newTestHandlerDB(t)
	code, resp := callImport(t, h, `{not json`)
	if code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, 期望 400", code)
	}
	if !strings.Contains(resp["error"].(string), "格式") {
		t.Errorf("error = %v, 期望含 '格式'", resp["error"])
	}
}
```

> 因新增 `strconv` 用法，更新 `import_export_handler_test.go` import 块为最终版：

```go
import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/user/anime-tip/internal/store"
)
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/web/ -run TestImportAnimes -v`
预期：FAIL（编译错误），报错 `h.ImportAnimes undefined`（handler 尚未实现）。

- [ ] **步骤 3：编写最少实现代码**

在 `internal/web/import_export_handler.go` 末尾追加。先更新该文件 import 块，把 `"net/http"`、`"time"`、`gin`、`store` 扩展为含 `"encoding/json"`、`"errors"`、`"fmt"`、`"strconv"`、`"strings"`、`"github.com/user/anime-tip/internal/model"`。最终 import 块：

```go
import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/user/anime-tip/internal/model"
	"github.com/user/anime-tip/internal/store"
)
```

> 说明：`validatePlayURL` 在 `anime_handler.go`（同包）已定义，直接调用即可，无需新 import。逐条校验用 `errors`/`fmt`/`strconv`/`strings`/`model`。`json` 用于解析请求体。下面追加的代码会用到 `model.Anime`、`store.ImportItem`、`store.ImportAnimes`、`validatePlayURL`。

在文件末尾追加 handler：

```go
// importRequest 为导入请求体，即导出文件的原始结构。前端 JSON.parse 后原样 POST 整个对象。
type importRequest struct {
	Version   int             `json:"version"`
	Animes    []importItemRaw `json:"animes"`
}

// importItemRaw 对应导出文件中的单条，字段为白名单。
type importItemRaw struct {
	VodID   int    `json:"vod_id"`
	Name    string `json:"name"`
	Cover   string `json:"cover"`
	PlayURL string `json:"play_url"`
}

const maxImportAnimes = 2000

// ImportAnimes 从导出文件 JSON 导入关注列表。
// 整体校验：version 缺省或 1 通过（其它→400）；animes 长度上限 2000（超→400）。
// 逐条校验：vod_id 正整数、name 非空、play_url 复用 validatePlayURL。
// 非法条计入 errors 不传入 store；通过校验的条目交 store 事务写入。
func (h *Handler) ImportAnimes(c *gin.Context) {
	var req importRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件格式不正确：" + err.Error()})
		return
	}

	// version 校验：缺省（0）视为 1
	if req.Version != 0 && req.Version != 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的导出版本：" + strconv.Itoa(req.Version)})
		return
	}

	if len(req.Animes) > maxImportAnimes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "导入条目过多（最多 2000 条）"})
		return
	}

	// 逐条校验，合法条包装成 ImportItem 传入 store
	items := make([]store.ImportItem, 0, len(req.Animes))
	var errors []store.ImportError

	for i, a := range req.Animes {
		if err := validateImportItem(a); err != nil {
			errors = append(errors, store.ImportError{
				Index: i, VodID: a.VodID, Error: err.Error(),
			})
			continue
		}
		items = append(items, store.ImportItem{
			Index: i,
			Anime: model.Anime{
				VodID:   a.VodID,
				Name:    a.Name,
				Cover:   a.Cover,
				PlayURL: a.PlayURL,
			},
		})
	}

	res, err := store.ImportAnimes(h.db, items)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 合并 handler 校验阶段与 store 写入阶段的 errors
	allErrors := errors
	if res.Errors != nil {
		allErrors = append(allErrors, res.Errors...)
	}
	if allErrors == nil {
		allErrors = []store.ImportError{}
	}
	res.Errors = allErrors

	c.JSON(http.StatusOK, res)
}

// validateImportItem 校验单条导入项。vod_id 正整数；name 非空；play_url 复用 validatePlayURL。
func validateImportItem(a importItemRaw) error {
	if a.VodID <= 0 {
		return fmt.Errorf("vod_id 必须为正整数")
	}
	if strings.TrimSpace(a.Name) == "" {
		return fmt.Errorf("name 不能为空")
	}
	if err := validatePlayURL(a.PlayURL); err != nil {
		return err
	}
	return nil
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/web/ -run TestImportAnimes -v`
预期：PASS（全部用例通过）。

- [ ] **步骤 5：Commit**

```bash
git add internal/web/import_export_handler.go internal/web/import_export_handler_test.go
git commit -m "feat(web): 新增关注列表导入端点 POST /api/animes/import"
```

---

### 任务 4：handler 层 — 往返测试 + 路由注册

**文件：**
- 测试：`internal/web/import_export_handler_test.go`（追加往返测试）
- 修改：`internal/web/handler.go`（`RegisterRoutes` 注册两条路由）

- [ ] **步骤 1：编写失败的测试**

在 `import_export_handler_test.go` 末尾追加往返测试：

```go
// TestImportExportRoundTrip：导出 → 清库 → 导入 → 关注清单还原（vod_id/name/cover/play_url 一致，基线重置为默认）。
func TestImportExportRoundTrip(t *testing.T) {
	h := newTestHandlerDB(t)
	seedAnime(t, h.db, 111, "番A", "https://play/a")
	seedAnime(t, h.db, 222, "番B", "")

	// 导出
	_, exportBody, _ := callExport(t, h)

	// 清库
	if _, err := h.db.Exec(`DELETE FROM animes`); err != nil {
		t.Fatalf("清库: %v", err)
	}

	// 用导出内容原样导入
	code, resp := callImport(t, h, string(exportBody))
	if code != http.StatusOK {
		t.Fatalf("导入状态码 = %d, 期望 200, body=%v", code, resp)
	}
	if got := int(resp["imported"].(float64)); got != 2 {
		t.Errorf("imported = %d, 期望 2", got)
	}

	// 还原校验
	a111, _ := store.GetAnimeByVodID(h.db, 111)
	if a111 == nil {
		t.Fatal("vod_id=111 未还原")
	}
	if a111.Name != "番A" || a111.Cover != "https://c/番A" || a111.PlayURL != "https://play/a" {
		t.Errorf("111 还原不一致: %+v", a111)
	}
	// 基线应被重置为默认（导出文件不含基线，导入后重新关注）
	if a111.CurrentRemarks != "" || a111.LastNotifiedRemarks != "" || a111.LastNotifiedEpisode != 0 {
		t.Errorf("111 基线未重置: %+v", a111)
	}

	a222, _ := store.GetAnimeByVodID(h.db, 222)
	if a222 == nil {
		t.Fatal("vod_id=222 未还原")
	}
	if a222.Name != "番B" || a222.PlayURL != "" {
		t.Errorf("222 还原不一致: %+v", a222)
	}
}

// 路由注册：SetupRouter 后 GET /api/animes/export 与 POST /api/animes/import 应可达。
func TestRoutes_exportAndImportRegistered(t *testing.T) {
	h := newTestHandlerDB(t)
	r := SetupRouter(h)

	// 导出
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/animes/export", nil))
	if w.Code != http.StatusOK {
		t.Errorf("GET /api/animes/export 状态码 = %d, 期望 200", w.Code)
	}

	// 导入空批
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodPost, "/api/animes/import", strings.NewReader(`{"version":1,"animes":[]}`)))
	if w2.Code != http.StatusOK {
		t.Errorf("POST /api/animes/import 状态码 = %d, 期望 200", w2.Code)
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/web/ -run TestRoutes_exportAndImportRegistered -v`
预期：FAIL，`GET /api/animes/export` 返回 404（路由未注册）。`TestImportExportRoundTrip` 可能通过（直接调 handler，不经路由），但路由测试必失败。

- [ ] **步骤 3：编写最少实现代码**

修改 `internal/web/handler.go` 的 `RegisterRoutes`，在 `api.DELETE("/animes/:id", h.DeleteAnime)` 之后、`api.GET("/search", h.Search)` 之前插入两行：

```go
		api.GET("/animes/export", h.ExportAnimes)
		api.POST("/animes/import", h.ImportAnimes)
```

修改后该代码块为：

```go
	api := r.Group("/api")
	{
		api.GET("/animes", h.ListAnimes)
		api.POST("/animes", h.CreateAnime)
		api.PATCH("/animes/:id", h.UpdateAnime)
		api.DELETE("/animes/:id", h.DeleteAnime)

		api.GET("/animes/export", h.ExportAnimes)
		api.POST("/animes/import", h.ImportAnimes)

		api.GET("/search", h.Search)

		api.GET("/settings", h.ListSettings)
		api.PUT("/settings", h.UpdateSettings)

		api.POST("/check", h.TriggerCheck)
		api.POST("/notify/test", h.TestNotify)
	}
```

> gin 路由注意：`/animes/export` 与 `/animes/:id` 是不同路径段，gin 静态段优先于参数段，不冲突。`/animes/import` 同理。

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/web/ -run "TestImportExportRoundTrip|TestRoutes_exportAndImportRegistered" -v`
预期：PASS。

随后运行全量 web 测试确认无回归：`go test ./internal/web/ -v`
预期：PASS。

- [ ] **步骤 5：Commit**

```bash
git add internal/web/import_export_handler_test.go internal/web/handler.go
git commit -m "feat(web): 注册导出/导入路由并补往返测试"
```

---

### 任务 5：前端 — 「数据备份」分组 HTML 结构

**文件：**
- 修改：`web/index.html`（设置 tab 增分组 + 状态字段 + JS 方法）

- [ ] **步骤 1：编写（前端无自动测试，本任务直接实现 HTML 结构与状态字段，下一步加 JS 方法）**

修改 `web/index.html`。在「设置」tab 的 Server 酱分组之后、「保存设置」按钮之前，插入「数据备份」分组。

定位现有结构（`web/index.html:82-92`）：

```html
  <div x-show="tab==='settings'">
    <div class="setting-group">
      <label>Server 酱 SendKey</label>
      ...（Server 酱分组）...
      <p class="test-result" x-show="notifyResult" :class="notifyOk ? 'success' : 'fail'" x-text="notifyResult"></p>
    </div>
    <button class="btn-primary" @click="saveSettings">保存设置</button>
  </div>
```

在 `</div>`（Server 酱分组结束）与 `<button class="btn-primary" @click="saveSettings">` 之间插入：

```html
    <div class="setting-group backup-group">
      <label>数据备份</label>
      <div class="backup-row">
        <button class="btn-test" @click="exportFollows()" :disabled="exporting" x-text="exporting ? '导出中...' : '导出关注列表'"></button>
      </div>
      <div class="backup-row">
        <input type="file" accept=".json,application/json" @change="onImportFilePicked($event)" :disabled="importing">
        <button class="btn-test" @click="importFollows()" :disabled="importing || !importPayload" x-text="importing ? '导入中...' : '导入关注列表'"></button>
      </div>
      <p class="import-result" x-show="importResult" x-text="importResult"></p>
    </div>
```

- [ ] **步骤 2：在 Alpine `app()` 状态对象中新增字段**

修改 `web/index.html` 的 `app()` 返回对象（约 `web/index.html:98-113`）。在 `savingUrl: false,` 之后追加三行：

```js
    exporting: false,
    importing: false,
    importResult: '',
    importPayload: null,
```

- [ ] **步骤 3：手动验证（编译/启动）**

运行：`go build ./... && go run .`
预期：服务启动无错。浏览器打开设置 tab，应看到「数据备份」分组含导出按钮、file input、导入按钮（导入按钮此时因 `importPayload` 为 null 而禁用）。此时按钮尚无方法实现（任务 6 补 JS），点击会报 Alpine `exportFollows is not a function`——这是预期的，任务 6 修复。

- [ ] **步骤 4：Commit**

```bash
git add web/index.html
git commit -m "feat(web): 设置页增加数据备份分组 HTML 结构"
```

---

### 任务 6：前端 — 导出/导入 JS 方法

**文件：**
- 修改：`web/index.html`（在 `app()` 内 `testNotify` 方法之后追加三个方法）

- [ ] **步骤 1：实现 JS 方法**

修改 `web/index.html`。在 `testNotify()` 方法结束的 `},` 之后（约 `web/index.html:255` 附近，`testNotify` 的闭合 `},` 之后、对象结尾 `};` 之前）追加三个方法：

```js
    async exportFollows() {
      this.exporting = true;
      try {
        const res = await fetch('/api/animes/export');
        if (!res.ok) {
          const err = await res.json().catch(() => ({}));
          alert(err.error || '导出失败');
          return;
        }
        const blob = await res.blob();
        // 文件名优先取响应头 Content-Disposition，回退到默认
        let filename = 'anime-tip-follows.json';
        const disp = res.headers.get('Content-Disposition') || '';
        const m = disp.match(/filename="?([^"]+)"?/);
        if (m) filename = m[1];
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = filename;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);
      } catch (e) {
        alert('导出请求失败: ' + e.message);
      } finally {
        this.exporting = false;
      }
    },

    onImportFilePicked(event) {
      const file = event.target.files && event.target.files[0];
      if (!file) {
        this.importPayload = null;
        return;
      }
      const reader = new FileReader();
      reader.onload = () => {
        try {
          const obj = JSON.parse(reader.result);
          this.importPayload = obj;
          this.importResult = '';
        } catch (e) {
          this.importPayload = null;
          this.importResult = '❌ 文件格式不正确，请选择导出的 JSON 文件';
          alert('文件格式不正确');
        }
      };
      reader.onerror = () => {
        this.importPayload = null;
        this.importResult = '❌ 读取文件失败';
      };
      reader.readAsText(file);
    },

    async importFollows() {
      if (!this.importPayload) return;
      this.importing = true;
      try {
        const res = await fetch('/api/animes/import', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(this.importPayload),
        });
        const data = await res.json();
        if (!res.ok) {
          this.importResult = '❌ ' + (data.error || '导入失败');
          return;
        }
        let msg = '✅ 已导入 ' + data.imported + ' 条，跳过 ' + data.skipped + ' 条';
        if (data.errors && data.errors.length > 0) {
          const shown = data.errors.slice(0, 5).map(e => '第 ' + (e.index + 1) + ' 条：' + e.error).join('；');
          const more = data.errors.length > 5 ? '（等共 ' + data.errors.length + ' 条错误）' : '';
          msg += '；失败：' + shown + more;
        }
        this.importResult = msg;
        await this.loadAnimes();
      } catch (e) {
        this.importResult = '❌ 请求失败: ' + e.message;
      } finally {
        this.importing = false;
      }
    },
```

- [ ] **步骤 2：手动验证导出**

运行：`go build ./... && go run .`
预期：
- 浏览器打开，先在搜索 tab 关注 1-2 部动漫。
- 设置 tab 点「导出关注列表」→ 浏览器下载 `anime-tip-follows-<当天日期>.json`，内容含 `version:1`、`exported_at`、`animes` 数组，每条仅 `vod_id/name/cover/play_url`。
- 空关注列表时导出仍下载文件，`animes:[]`。

- [ ] **步骤 3：手动验证导入**

预期（同一运行实例）：
- 取消关注全部，清空列表。
- 「选择文件」选刚下载的 JSON → 「导入关注列表」按钮变为可点 → 点击 → 提示「✅ 已导入 N 条，跳过 0 条」→ 关注列表自动刷新还原。
- 再次导入同一文件 → 提示「✅ 已导入 0 条，跳过 N 条」（全部已存在）。
- 选一个非 JSON 文件（如 `.txt`）→ 弹「文件格式不正确」alert，不发请求，`importResult` 显示红字。
- 手工编辑 JSON 把一条 `name` 改空、一条 `play_url` 改 `ftp://x` → 导入 → 提示含「失败：第 X 条：name 不能为空；第 Y 条：...」，合法条仍进入列表。
- 导出空清单（空列表）→ 再导入 → 不报错，`animes:[]`。

- [ ] **步骤 4：Commit**

```bash
git add web/index.html
git commit -m "feat(web): 实现导出/导入关注列表 JS 方法"
```

---

### 任务 7：前端 — 数据备份分组 CSS

**文件：**
- 修改：`web/static/style.css`（末尾追加）

- [ ] **步骤 1：追加 CSS**

在 `web/static/style.css` 末尾追加：

```css
.backup-group .backup-row {
  display: flex;
  gap: 10px;
  align-items: center;
  margin-bottom: 10px;
}

.backup-group input[type="file"] {
  flex: 1;
  font-size: 13px;
  padding: 6px;
  background: #1a1a2e;
  border: 1px solid #2a2a3e;
  border-radius: 8px;
  color: #e0e0e0;
}

.import-result {
  font-size: 13px;
  margin-top: 8px;
  padding: 8px 12px;
  border-radius: 6px;
  background: #1a1a2e;
  color: #e0e0e0;
  word-break: break-all;
}
```

- [ ] **步骤 2：手动验证视觉一致性**

运行：`go build ./... && go run .`
预期：设置 tab「数据备份」分组与 Server 酱分组视觉风格一致（同色系、同圆角、同按钮样式），file input 与按钮横向排列，导入结果文字折行正常。

- [ ] **步骤 3：Commit**

```bash
git add web/static/style.css
git commit -m "style(web): 数据备份分组样式"
```

---

### 任务 8：全量验证

- [ ] **步骤 1：运行全量测试**

运行：`go test ./... -v`
预期：全部 PASS（store + web 所有测试，含新增的 `TestImportAnimes`、`TestExportAnimes`、`TestImportAnimes_*`、`TestImportExportRoundTrip`、`TestRoutes_exportAndImportRegistered`）。

- [ ] **步骤 2：构建检查**

运行：`go build ./...`
预期：无错。

- [ ] **步骤 3：vet 检查**

运行：`go vet ./...`
预期：无问题。

- [ ] **步骤 4：手动端到端验证**

运行：`go run .`，浏览器操作完整流程：
1. 搜索并关注 2 部动漫，为其中一部配置播放地址。
2. 设置 tab → 导出 → 得到 JSON，检查字段白名单（无基线/主键）。
3. 取消关注全部。
4. 导入刚导出的 JSON → 列表还原，播放地址保留，进度基线为「未知/重新关注」状态。
5. 再次导入同一文件 → 全部 skipped。
6. 导入非法文件（非 JSON）→ 有 alert 提示且不发请求。
7. 空列表导出 → 空清单文件 → 再导入不报错。
8. 导入含非法条（name 空、play_url 非 http）的 JSON → 非法条进 errors 提示，合法条入库。

预期：全部行为符合规格「错误处理与边界」表。

- [ ] **步骤 5：Commit（如有 vet/构建修复）**

若前面步骤发现需小修，修复后：

```bash
git add -A
git commit -m "test: 关注列表导出/导入全量验证通过"
```

若无修改则跳过此步。

---

## 自检结果

**1. 规格覆盖度：**
- 导出文件格式（version/exported_at/白名单）→ 任务 2（`exportFile`/`exportAnime` 结构）✅
- `GET /api/animes/export`（ListAnimes→裁剪→下载头）→ 任务 2 ✅
- `POST /api/animes/import`（整体+逐条校验→store→合并 errors）→ 任务 3 ✅
- `store.ImportAnimes` 及类型（事务原子、含 play_url INSERT、单条失败不回滚）→ 任务 1 ✅
- 前端导出（fetch→blob→`<a download>`）→ 任务 6 ✅
- 前端导入（file input→FileReader→JSON.parse→POST 整对象→展示→loadAnimes）→ 任务 5+6 ✅
- CSS 复用 `.setting-group` 与按钮风格 → 任务 7 ✅
- 错误处理边界表全部条目 → 任务 1/3/6 覆盖（非 JSON、version、单条非法、已存在、超上限、单条 SQL 失败、空列表）✅
- 测试 TDD（store/handler/前端手动清单）→ 任务 1/2/3/4 + 任务 6/8 手动 ✅
- 影响面文件清单 → 文件结构表一一对应 ✅
- 无遗漏。

**2. 占位符扫描：** 计划中无 "TODO/待定/后续实现/类似任务 N" 等占位符。每步含完整代码或精确命令。任务 5/6/7 为前端无自动测试，已用精确手动验证清单替代（符合规格「前端：项目无前端测试框架，不引入」）。任务 1/3 步骤内的「草稿→干净版替换」说明已内联给出最终可用代码，无残留占位。

**3. 类型一致性：**
- store：`ImportItem{Index int; Anime model.Anime}`、`ImportError{Index,VodID int; Error string}`、`ImportResult{Imported,Skipped int; Errors []ImportError}`、`ImportAnimes(db *sql.DB, items []ImportItem) (ImportResult, error)` — 任务 1 定义，任务 3 handler 引用 `store.ImportItem`/`store.ImportError`/`store.ImportAnimes` 一致 ✅
- handler：`exportAnime`/`exportFile`（任务 2）、`importRequest`/`importItemRaw`/`validateImportItem`/`maxImportAnimes`（任务 3）— 命名不冲突 ✅
- `validatePlayURL` — `anime_handler.go` 已有，任务 3 直接复用 ✅
- 前端：状态字段 `exporting/importing/importResult/importPayload`（任务 5 定义）与方法 `exportFollows/onImportFilePicked/importFollows`（任务 6 定义）在 HTML `@click`/`@change`/`x-text`/`x-show` 引用一致 ✅
- `newTestHandlerDB`/`seedAnime`/`callExport`/`callImport` 测试辅助函数命名前后一致 ✅

无类型/命名不一致。
