# 设计：关注列表导出/导入（备份）

日期：2026-07-01

## 背景

anime-tip 的关注列表持久化在 SQLite `animes` 表。当前没有备份/迁移手段：用户换设备、重装、误删都意味着关注清单丢失、只能一条条重新搜索关注。用户希望增加关注列表的导出/导入功能，目的是方便备份与跨实例迁移。

## 目标

- 一键导出当前关注列表为 JSON 文件，便于离线保存。
- 一键从 JSON 文件导入，恢复关注清单。
- 导出/导入入口集中在「设置」tab 的「数据备份」分组。

## 非目标（YAGNI）

- **不备份进度基线**：导出仅含关注清单字段（`vod_id/name/cover/play_url`），不含 `current_remarks / last_notified_remarks / last_notified_episode / id / created_at`。导入后相当于「重新关注」，首检由 scheduler 重新建立基线。
- **不备份设置项**：Server 酱 SendKey 等设置不进入导出文件（含敏感信息，且与关注清单是两件事）。
- 不做增量/差异备份、不做定时自动备份（用户手动触发即可）。
- 不做 CSV 等多格式，仅 JSON（与现有 API 一致、能干净承载中文名与 URL）。
- 不做导出文件的加密、签名校验。

## 导出文件格式

```json
{
  "version": 1,
  "exported_at": "2026-07-01T12:00:00Z",
  "animes": [
    { "vod_id": 28802, "name": "葬送的芙莉莲", "cover": "https://...", "play_url": "https://..." }
  ]
}
```

- **字段白名单**：仅 `vod_id / name / cover / play_url`，明确不含进度基线与内部主键/时间戳。
- `version: 1` 留作后续格式演进兼容；导入时仅接受 `1`（缺省也视为 `1`，宽松兼容早期手写文件），其它值 → 400。
- `exported_at` 为 ISO8601（RFC3339）时间戳，仅供人阅读，导入时忽略。

## 后端接口

### `GET /api/animes/export`

- 调 `store.ListAnimes` 取全字段记录，在 handler 层映射为精简结构（字段白名单）。
- 设响应头 `Content-Disposition: attachment; filename="anime-tip-follows-<YYYY-MM-DD>.json"`，日期取服务器当日，触发浏览器下载。
- `model.Anime` 与 store 层**不动**：字段裁剪只在 handler 做，保持 model 作为库内完整表征。

### `POST /api/animes/import`

- 请求体即导出文件的原始结构：`{ "version": 1, "exported_at": "...", "animes": [...] }`。前端 `JSON.parse` 后原样 POST 整个对象，handler 取 `animes` 字段（`version` 用于校验，`exported_at` 忽略）。不要求前端重组字段。
- handler 整体校验：
  - `version` 缺省或为 `1` 通过；其它值 → 400「不支持的导出版本」。
  - `animes` 数组长度上限 **2000**，超过 → 400（防滥用）。
- handler 逐条校验（非法条记入 `errors`，不中断整体）：
  - `vod_id` 为正整数。
  - `name` 非空字符串。
  - `play_url` 复用现有 `validatePlayURL`：空串合法（留空）；非空须 `http(s)://` 且 ≤2048 字符。
- 调 `store.ImportAnimes`（见下）。
- 成功返回 `200`：
  ```json
  { "imported": 12, "skipped": 3, "errors": [{"index":5,"vod_id":0,"error":"name 不能为空"}] }
  ```
  - `imported` = 新插入条数。
  - `skipped` = `vod_id` 已存在于当前库的条数（不动本地记录，含本地 play_url）。
  - `errors` = handler 校验阶段即非法、或 store 写入阶段失败的条目，含其在原数组中的 `index`、`vod_id`（可能为 0）、`error` 描述。

## store 层

新增 `ImportAnimes` 及结果类型，职责单一、事务原子：

```go
// ImportItem 承载一条待导入记录及其在原导出数组中的下标，
// 便于 store 写入失败时回填与原文件一致的 error index。
type ImportItem struct {
    Index int
    Anime model.Anime
}

type ImportError struct {
    Index int    `json:"index"`
    VodID int    `json:"vod_id"`
    Error string `json:"error"`
}

type ImportResult struct {
    Imported int           `json:"imported"`
    Skipped  int           `json:"skipped"`
    Errors   []ImportError `json:"errors"`
}

func ImportAnimes(db *sql.DB, items []ImportItem) (ImportResult, error)
```

- handler 先对原始数组逐条校验：非法条直接产出 `ImportError{Index: 原下标}` 加入结果，**不传入 store**；通过校验的条目包装成 `ImportItem{Index: 原下标, Anime: ...}` 传入 store。
- store 开事务，逐条处理（传入的 `items` 已是合法子集）：
  - `GetAnimeByVodID` 命中 → `Skipped++`（保留本地记录与本地 `play_url` 不变）。
  - 未命中 → 插入，INSERT **含 `play_url`**：
    ```sql
    INSERT INTO animes (vod_id, name, cover, current_remarks, last_notified_remarks, last_notified_episode, play_url)
    VALUES (?, ?, ?, '', '', 0, ?)
    ```
    进度基线留默认空值/0，符合「重新关注」语义。这与现有 `CreateAnime` 的 INSERT（不带 `play_url`）不同，故单独写语句，不改 `CreateAnime`。
  - 单条 SQL 失败 → 记 `ImportError{Index: item.Index, VodID: ..., Error: ...}` 继续下一条，**不回滚整批**；事务最终提交所有合法条目。
- handler 把自身校验阶段的 `errors` 与 store 返回的 `errors` 合并后返回前端。
- `CreateAnime`、`UpdateAnimeRemarks`、`UpdateAnimePlayURL` 等现有方法保持不变。

## 前端交互

「设置」tab 新增「数据备份」分组，与现有 Server 酱分组并列：

1. **导出按钮**：
   - `fetch('/api/animes/export')` → `res.blob()` → 构造 `URL.createObjectURL` + 隐藏 `<a download>` 点击触发下载，文件名沿用响应头 `Content-Disposition`。
2. **导入**：`<input type="file" accept=".json,application/json">` + 「导入」按钮。
   - 选文件 → `FileReader.readAsText` → `JSON.parse`：解析失败直接 `alert('文件格式不正确')`，不发请求。
   - 解析成功 → POST `/api/animes/import`，请求体为解析得到的整个对象（`{version, exported_at, animes}`），不重组字段。
   - 响应展示：`已导入 {imported} 条，跳过 {skipped} 条`；`errors` 非空时额外列出前若干条（如「第 N 条：原因」）。
   - 成功后调 `loadAnimes()` 刷新关注列表。
3. **CSS**：`web/static/style.css` 增加备份分组样式，复用现有 `.setting-group` 与按钮风格，保持视觉一致。

## 错误处理与边界

| 情况 | 处理 |
|---|---|
| 文件非合法 JSON | 前端 `alert` 拦截；后端兜底 400 |
| `version` 缺省或 `1` | 正常导入；其它值 → 400「不支持的导出版本」 |
| 单条 `name` 空 / `vod_id`≤0 / `play_url` 非法 | 计入 `errors` 跳过，不中断整体 |
| `vod_id` 已存在 | `skipped++`，不动本地记录 |
| 数组长度 > 2000 | 400 |
| 导入中途单条 SQL 失败 | 该条入 `errors`，事务仍提交其余合法条 |
| 导出时关注列表为空 | 正常返回 `{version,exported_at,animes:[]}`，下载空清单文件 |

## 测试（TDD）

1. **store 层**（`internal/store/anime_test.go`，已存在，追加）：
   - `TestImportAnimes`：传入 `[]ImportItem`（含已通过 handler 校验的合法子集 + 原下标）→ 全新条目 `imported` 计数正确、`play_url` 落库；含已存在 `vod_id` → `skipped` 计数正确且本地 `play_url` 不被覆盖；模拟单条 SQL 失败（如构造重复 `vod_id` 触发 UNIQUE 冲突，或注入坏值）→ 该条 `ImportError.Index` 与传入下标一致、其余条目正常提交、事务不回滚整批。

2. **handler 层**（新增 `internal/web/import_export_handler_test.go`，参考 `internal/web/notify_handler_test.go` 的真实 sqlite + gin 测试模式）：
   - `TestExportAnimes`：返回结构含 `version=1 / exported_at / animes`；字段白名单正确（无 `last_notified_*`、无 `id`）；`Content-Disposition` 含日期文件名。
   - `TestImportAnimes`：
     - 合法批（含一条已存在）→ 200，`imported/skipped` 正确。
     - `version` 非 1 → 400。
     - 数组超上限 → 400。
     - 含 `name` 空 / `vod_id`≤0 / `play_url` 非法条 → 该条进 `errors`、其余正常导入。
   - `TestImportExportRoundTrip`：导出 → 清库 → 导入 → 关注清单还原（vod_id/name/cover/play_url 一致，基线重置为默认）。

3. **前端**：项目无前端测试框架，不引入。手动验证清单：
   - 导出下载得到 JSON，字段与白名单一致。
   - 修改/删除若干条后再导入，已存在条目被跳过、新增条目进入列表。
   - 导入非法文件（非 JSON）有提示且不发请求。
   - 导入后关注列表自动刷新。
   - 空关注列表导出仍可下载空清单，再导入不报错。

## 影响面

- 新增/改动文件：
  - `internal/web/handler.go`（`RegisterRoutes` 注册 `GET /api/animes/export`、`POST /api/animes/import`）
  - `internal/web/import_export_handler.go`（**新文件**：`ExportAnimes` + `ImportAnimes` + 字段裁剪 + 逐条校验）
  - `internal/store/anime.go`（新增 `ImportAnimes` + `ImportItem` / `ImportResult` / `ImportError`）
  - `internal/store/anime_test.go`（追加 `TestImportAnimes`）
  - `internal/web/import_export_handler_test.go`（**新文件**）
  - `web/index.html`（设置 tab 增「数据备份」分组 + 导出/导入 JS 方法）
  - `web/static/style.css`（备份分组样式）
- **不动**：`internal/model/anime.go`、`scheduler`、`crawler`、`notify`、`CreateAnime`/`UpdateAnime`/`DeleteAnime`、关注/搜索流程、settings 读写。
- 不涉及数据库 schema 变更（无新增列、无迁移）。
