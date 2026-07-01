# 设计：关注动漫的播放地址（点击名称新 tab 打开）

日期：2026-07-01

## 背景

anime-tip 的关注列表（`web/index.html` 的「关注列表」tab）每张卡片当前展示封面、名称（`<h3 x-text="a.name">`，纯文本）、当前进度和「取消关注」按钮。用户希望：点击关注的动漫名称，能在浏览器新 tab 打开该动漫提前配置好的播放地址。

播放地址由用户手动维护，不来自采集源。关注时不强制填写，可后续编辑补上。

## 目标

- 关注列表的每部动漫可单独配置一个播放地址。
- 名称配置了地址时渲染为可点击链接，`target="_blank"` 新 tab 打开；未配置时保持普通文本并提示「未配置播放地址」，不可点击。
- 每张卡片提供「编辑」入口，可随时填入或修改播放地址。

## 非目标（YAGNI）

- 不从苹果 CMS 采集源自动抓取播放地址。
- 不做多播放源、多地址列表，每部动漫仅一个播放地址。
- 不做 URL 深度合法性校验或预览抓取。
- 关注（POST）流程不变，不在关注时填地址。

## 数据模型

在 `animes` 表新增一列：

```sql
ALTER TABLE animes ADD COLUMN play_url TEXT NOT NULL DEFAULT '';
```

迁移走现有幂等补列模式：`internal/store/db.go` 的 `migrate()` 已有一处 `ALTER TABLE ... ADD COLUMN` 捕获 `duplicate column` 错误忽略的先例，照搬同款逻辑，保证老库升级幂等、新库（CREATE TABLE 时）不重复加列。具体做法：在 `CREATE TABLE IF NOT EXISTS animes (...)` 的列定义中直接加入 `play_url TEXT NOT NULL DEFAULT ''`（新库一次建好），并保留一条幂等 `ALTER TABLE ... ADD COLUMN play_url`（老库补列），与现有 `last_notified_episode` 的处理完全一致。

`internal/model/anime.go` 的 `Anime` 结构体新增字段：

```go
PlayURL string `json:"play_url"`
```

所有读写该表的 SQL 同步补列：

- `internal/store/anime.go`
  - `ListAnimes`：SELECT 列表与 `Scan` 增加 `play_url`。
  - `GetAnimeByVodID`：同上。
  - `CreateAnime`：**不改**——关注时 play_url 为空，沿用 `INSERT ... VALUES (?, ?, ?, ?, '', 0)`，DEFAULT '' 即可。
  - 新增 `UpdateAnimePlayURL(db, id, playURL)` 方法。

## 后端接口

新增字段级编辑接口，与现有 RESTful 风格一致：

```
PATCH /api/animes/:id
Content-Type: application/json
{ "play_url": "https://..." }
```

- 路由在 `internal/web/handler.go` 的 `RegisterRoutes` 中注册：`api.PATCH("/animes/:id", h.UpdateAnime)`。
- handler 实现放 `internal/web/anime_handler.go`，新增 `UpdateAnime(c *gin.Context)`：
  - 解析 `:id`（沿用 `DeleteAnime` 的 `strconv.ParseInt`）。
  - 绑定 body `{ play_url string }`。
  - 校验 `play_url`（见下）。
  - 调用 `store.UpdateAnimePlayURL`。
  - 成功返回 `200 {data: <更新后的 anime>}` 或简洁 `{message: "已更新"}`，与 `DeleteAnime` 的简洁风格保持一致；这里选择返回更新后的记录，便于前端直接替换本地状态、少一次刷新。
- 选用 `PATCH` 而非 `PUT`：当前 `PUT /settings` 用于整体替换，字段级局部更新用 PATCH 更贴切。

`store.UpdateAnimePlayURL`：

```go
func UpdateAnimePlayURL(db *sql.DB, id int64, playURL string) error {
    _, err := db.Exec(`UPDATE animes SET play_url = ? WHERE id = ?`, playURL, id)
    return err
}
```

## 前端交互

`web/index.html` 关注列表卡片改造：

1. **名称区域条件渲染**（替换现有 `<h3 x-text="a.name">`）：
   - `a.play_url` 非空 → 渲染为 `<a :href="a.play_url" target="_blank" rel="noopener noreferrer" x-text="a.name">`，CSS 给出可点击外观（颜色、下划线、`cursor: pointer`）。
   - 为空 → 渲染 `<h3 x-text="a.name">` + 灰色小字 `<span>未配置播放地址</span>`。

2. **编辑入口**：卡片操作区在「取消关注」旁加「编辑」按钮。点击后该卡片切到编辑态：显示一个 `play_url` 文本输入框（预填当前值）+「保存」「取消」。
   - 编辑态用每条动漫的局部状态管理：在 `animes` 数组项里挂 `_editing`（bool）、`_draftUrl`（string）两个非持久化字段（下划线前缀，序列化时不会进库——它们只在前端使用）。或用独立的 `editingId` 控制当前编辑哪一条，单卡片编辑。采用单卡片编辑（`editingId === a.id`）更简单。
   - 「保存」校验协议头 → 调 `PATCH /api/animes/:id` → 成功后用响应中的 `play_url` 更新 `a.play_url`，清空 `editingId`。
   - 「取消」直接清空 `editingId`，不发请求。

3. **CSS**：在 `web/static/style.css` 增加链接态 `.card-info a` 与编辑态输入框样式，使其与现有卡片视觉一致。

## 错误处理与校验

- **后端**：`play_url` 校验——长度上限 2048；非空时必须以 `http://` 或 `https://` 开头，否则返回 400。空串允许（表示清除地址）。不做 URL 深度合法性校验。
- **前端**：保存前同样校验协议头，失败直接 `alert` 提示，不发请求。
- PATCH 接口对不存在的 id：`UpdateAnimePlayURL` 的 `Exec` 对 0 行影响不报错，本次暂不强制返回 404（与 `DeleteAnime` 现有行为一致，保持简洁）。如需更严谨可作为可选改进，不在本次范围。

## 测试（TDD）

1. **store 层**（`internal/store/anime_test.go`，已存在）：
   - `TestUpdateAnimePlayURL`：创建动漫 → 调用 `UpdateAnimePlayURL` → `ListAnimes`/`GetAnimeByVodID` 验证 `play_url` 已落库。
   - 迁移幂等：新库 migrate 后 `play_url` 列存在；老库（无该列）补列后再次 migrate 不报 `duplicate column` 之外的错。（参照现有 `last_notified_episode` 测试的思路。）

2. **handler 层**（新增 `internal/web/anime_handler_test.go`）：
   - 参考 `internal/web/notify_handler_test.go` 的测试模式（真实 sqlite + gin）。
   - `TestUpdateAnime`：合法 `http(s)://` 地址 → 200 且返回更新后的记录；非法协议头 → 400；空串 → 200（清除地址）。

3. **前端**：项目无前端测试框架，本次不引入。手动验证清单：配了地址点击名称新 tab 打开；未配时名称不可点且显示提示；编辑保存/取消正确；空地址→填地址→名称变链接。

## 影响面

- 改动文件：
  - `internal/store/db.go`（迁移）
  - `internal/model/anime.go`（结构体字段）
  - `internal/store/anime.go`（SQL、新方法）
  - `internal/store/anime_test.go`（测试）
  - `internal/web/handler.go`（路由）
  - `internal/web/anime_handler.go`（新 handler）
  - `internal/web/anime_handler_test.go`（新文件，测试）
  - `web/index.html`（卡片渲染 + 编辑态）
  - `web/static/style.css`（链接、编辑态样式）
- 不影响 scheduler / crawler / notify 现有逻辑：`CreateAnime` 不变，`UpdateAnimeRemarks`（推送基线）不变，`play_url` 与通知判定无关。
- 不涉及关注时的字段，POST `/api/animes` 行为不变。
