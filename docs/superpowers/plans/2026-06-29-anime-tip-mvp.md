# anime-tip MVP 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 构建 anime-tip MVP——一个监控 keke9.com 动漫更新并通过 Server 酱推送微信通知的 Web 应用

**架构：** Go 单体服务（Gin HTTP + robfig/cron 定时任务 + SQLite 存储），前端为 Alpine.js 单页应用。后端代理 keke9 MacCMS API 获取动漫数据，定时检查关注列表的集数变化，聚合更新后调 Server 酱推送。

**技术栈：** Go 1.22 / Gin / robfig/cron v3 / mattn/go-sqlite3 / Alpine.js 3 / Docker

---

## 文件结构

| 文件 | 职责 |
|------|------|
| `cmd/server/main.go` | 入口：加载配置、初始化 DB、启动 Gin + Cron |
| `internal/config/config.go` | 配置结构体 + 环境变量加载 |
| `internal/model/anime.go` | Anime 数据模型 + keke9 API 响应结构体 |
| `internal/model/setting.go` | Setting 数据模型 |
| `internal/store/db.go` | SQLite 初始化 + 迁移 |
| `internal/store/anime.go` | Anime 表 CRUD |
| `internal/store/setting.go` | Setting 表 CRUD |
| `internal/crawler/keke9.go` | keke9 API 客户端：搜索 + 详情 |
| `internal/notify/serverchan.go` | Server 酱推送：聚合消息 + HTTP 调用 |
| `internal/scheduler/scheduler.go` | Cron 定时任务：遍历关注→检查更新→推送 |
| `internal/web/handler.go` | Gin 路由注册 + 静态文件服务 |
| `internal/web/anime_handler.go` | 关注列表 CRUD API handler |
| `internal/web/search_handler.go` | 搜索代理 API handler |
| `internal/web/setting_handler.go` | 设置 CRUD API handler |
| `internal/web/check_handler.go` | 手动触发检查 API handler |
| `web/index.html` | 前端单页应用（Alpine.js 3） |
| `web/static/style.css` | 样式 |
| `Dockerfile` | 多阶段构建 |
| `docker-compose.yml` | 部署编排 |
| `go.mod` | Go 模块定义 |

---

### 任务 1：项目骨架 + 配置

**文件：**
- 创建：`go.mod`
- 创建：`internal/config/config.go`
- 创建：`cmd/server/main.go`

- [ ] **步骤 1：初始化 Go 模块**

运行：
```bash
cd E:\开源项目\电视影音\anime-tip
go mod init github.com/user/anime-tip
```

- [ ] **步骤 2：编写配置结构体**

```go
// internal/config/config.go
package config

import "os"

type Config struct {
	Port           string
	CheckCron      string
	ServerChanKey  string
	Keke9BaseURL   string
	DBPath         string
}

func Load() *Config {
	return &Config{
		Port:          getEnv("PORT", "8080"),
		CheckCron:     getEnv("CHECK_INTERVAL", "0 * * * *"),
		ServerChanKey: getEnv("SERVER_CHAN_KEY", ""),
		Keke9BaseURL:  getEnv("KEKE9_BASE_URL", "https://www.keke9.com"),
		DBPath:        getEnv("DB_PATH", "anime-tip.db"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
```

- [ ] **步骤 3：编写 main.go 骨架**

```go
// cmd/server/main.go
package main

import (
	"fmt"
	"log"

	"github.com/user/anime-tip/internal/config"
)

func main() {
	cfg := config.Load()
	log.Printf("anime-tip starting on :%s", cfg.Port)
	log.Printf("check cron: %s", cfg.CheckCron)
	fmt.Printf("keke9 base url: %s\n", cfg.Keke9BaseURL)
}
```

- [ ] **步骤 4：运行验证**

运行：
```bash
go run cmd/server/main.go
```
预期：输出 `anime-tip starting on :8080` 等日志

- [ ] **步骤 5：Commit**

```bash
git add go.mod go.sum internal/config/config.go cmd/server/main.go
git commit -m "feat: 项目骨架 + 配置加载"
```

---

### 任务 2：数据模型

**文件：**
- 创建：`internal/model/anime.go`
- 创建：`internal/model/setting.go`

- [ ] **步骤 1：编写 Anime 模型和 keke9 API 响应结构体**

```go
// internal/model/anime.go
package model

import "time"

// Anime 关注的动漫记录
type Anime struct {
	ID                  int64     `json:"id"`
	VodID               int       `json:"vod_id"`
	Name                string    `json:"name"`
	Cover               string    `json:"cover"`
	CurrentRemarks      string    `json:"current_remarks"`
	LastNotifiedRemarks string    `json:"last_notified_remarks"`
	CreatedAt           time.Time `json:"created_at"`
}

// Keke9ListResponse keke9 get_list API 响应
type Keke9ListResponse struct {
	Code int `json:"code"`
	Msg  string `json:"msg"`
	Info struct {
		Offset int           `json:"offset"`
		Limit  int           `json:"limit"`
		Total  int           `json:"total"`
		Rows   []Keke9VodItem `json:"rows"`
	} `json:"info"`
}

// Keke9DetailResponse keke9 get_detail API 响应
type Keke9DetailResponse struct {
	Code int `json:"code"`
	Msg  string `json:"msg"`
	Info struct {
		VodID      int    `json:"vod_id"`
		VodName    string `json:"vod_name"`
		VodPic     string `json:"vod_pic"`
		VodRemarks string `json:"vod_remarks"`
		VodArea    string `json:"vod_area"`
		VodYear    string `json:"vod_year"`
		VodClass   string `json:"vod_class"`
		VodActor   string `json:"vod_actor"`
		VodScore   string `json:"vod_score"`
		TypeID     int    `json:"type_id"`
		VodLink    string `json:"vod_link"`
	} `json:"info"`
}

// Keke9VodItem 列表接口中的单个视频项
type Keke9VodItem struct {
	VodID      int    `json:"vod_id"`
	VodName    string `json:"vod_name"`
	VodPic     string `json:"vod_pic"`
	VodRemarks string `json:"vod_remarks"`
	VodArea    string `json:"vod_area"`
	VodYear    string `json:"vod_year"`
	VodClass   string `json:"vod_class"`
	VodScore   string `json:"vod_score"`
	TypeID     int    `json:"type_id"`
	VodLink    string `json:"vod_link"`
}
```

- [ ] **步骤 2：编写 Setting 模型**

```go
// internal/model/setting.go
package model

// Setting 配置项
type Setting struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}
```

- [ ] **步骤 3：Commit**

```bash
git add internal/model/
git commit -m "feat: 数据模型定义"
```

---

### 任务 3：SQLite 存储层

**文件：**
- 创建：`internal/store/db.go`
- 创建：`internal/store/anime.go`
- 创建：`internal/store/setting.go`

- [ ] **步骤 1：安装 SQLite 依赖**

运行：
```bash
go get github.com/mattn/go-sqlite3
```

- [ ] **步骤 2：编写数据库初始化 + 迁移**

```go
// internal/store/db.go
package store

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

func InitDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
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
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		);
	`)
	return err
}
```

- [ ] **步骤 3：编写 Anime CRUD**

```go
// internal/store/anime.go
package store

import (
	"database/sql"
	"fmt"

	"github.com/user/anime-tip/internal/model"
)

func ListAnimes(db *sql.DB) ([]model.Anime, error) {
	rows, err := db.Query(`SELECT id, vod_id, name, cover, current_remarks, last_notified_remarks, created_at FROM animes ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("query animes: %w", err)
	}
	defer rows.Close()

	var animes []model.Anime
	for rows.Next() {
		var a model.Anime
		if err := rows.Scan(&a.ID, &a.VodID, &a.Name, &a.Cover, &a.CurrentRemarks, &a.LastNotifiedRemarks, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan anime: %w", err)
		}
		animes = append(animes, a)
	}
	return animes, rows.Err()
}

func GetAnimeByVodID(db *sql.DB, vodID int) (*model.Anime, error) {
	var a model.Anime
	err := db.QueryRow(`SELECT id, vod_id, name, cover, current_remarks, last_notified_remarks, created_at FROM animes WHERE vod_id = ?`, vodID).Scan(
		&a.ID, &a.VodID, &a.Name, &a.Cover, &a.CurrentRemarks, &a.LastNotifiedRemarks, &a.CreatedAt,
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
	_, err := db.Exec(`INSERT INTO animes (vod_id, name, cover, current_remarks, last_notified_remarks) VALUES (?, ?, ?, ?, '')`,
		a.VodID, a.Name, a.Cover, a.CurrentRemarks,
	)
	return err
}

func DeleteAnime(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM animes WHERE id = ?`, id)
	return err
}

func UpdateAnimeRemarks(db *sql.DB, id int64, currentRemarks, lastNotifiedRemarks string) error {
	_, err := db.Exec(`UPDATE animes SET current_remarks = ?, last_notified_remarks = ? WHERE id = ?`,
		currentRemarks, lastNotifiedRemarks, id,
	)
	return err
}
```

- [ ] **步骤 4：编写 Setting CRUD**

```go
// internal/store/setting.go
package store

import (
	"database/sql"
	"fmt"

	"github.com/user/anime-tip/internal/model"
)

func GetSetting(db *sql.DB, key string) (string, error) {
	var value string
	err := db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get setting %s: %w", key, err)
	}
	return value, nil
}

func SetSetting(db *sql.DB, key, value string) error {
	_, err := db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func ListSettings(db *sql.DB) ([]model.Setting, error) {
	rows, err := db.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return nil, fmt.Errorf("list settings: %w", err)
	}
	defer rows.Close()

	var settings []model.Setting
	for rows.Next() {
		var s model.Setting
		if err := rows.Scan(&s.Key, &s.Value); err != nil {
			return nil, fmt.Errorf("scan setting: %w", err)
		}
		settings = append(settings, s)
	}
	return settings, rows.Err()
}
```

- [ ] **步骤 5：编译验证**

运行：
```bash
go build ./...
```
预期：编译通过，无错误

- [ ] **步骤 6：Commit**

```bash
git add internal/store/ go.mod go.sum
git commit -m "feat: SQLite 存储层（数据库初始化 + Anime/Setting CRUD）"
```

---

### 任务 4：keke9 API 客户端

**文件：**
- 创建：`internal/crawler/keke9.go`

- [ ] **步骤 1：编写 keke9 API 客户端**

```go
// internal/crawler/keke9.go
package crawler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/user/anime-tip/internal/model"
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// SearchAnime 搜索动漫（type_id=4 表示动漫分类）
func (c *Client) SearchAnime(keyword string) ([]model.Keke9VodItem, error) {
	u, _ := url.Parse(c.BaseURL + "/api.php/vod/get_list")
	q := u.Query()
	q.Set("vod_name", keyword)
	q.Set("type_id", "4")
	u.RawQuery = q.Encode()

	var resp model.Keke9ListResponse
	if err := c.doGet(u.String(), &resp); err != nil {
		return nil, err
	}
	if resp.Code != 1 {
		return nil, fmt.Errorf("keke9 API error: %s", resp.Msg)
	}
	return resp.Info.Rows, nil
}

// GetAnimeDetail 获取单个动漫详情
func (c *Client) GetAnimeDetail(vodID int) (*model.Keke9DetailResponse, error) {
	u, _ := url.Parse(c.BaseURL + "/api.php/vod/get_detail")
	q := u.Query()
	q.Set("vod_id", fmt.Sprintf("%d", vodID))
	u.RawQuery = q.Encode()

	var resp model.Keke9DetailResponse
	if err := c.doGet(u.String(), &resp); err != nil {
		return nil, err
	}
	if resp.Code != 1 {
		return nil, fmt.Errorf("keke9 API error: %s", resp.Msg)
	}
	return &resp, nil
}

func (c *Client) doGet(url string, target interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", c.BaseURL+"/")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("json unmarshal: %w", err)
	}
	return nil
}
```

- [ ] **步骤 2：编译验证**

运行：
```bash
go build ./...
```
预期：编译通过

- [ ] **步骤 3：Commit**

```bash
git add internal/crawler/
git commit -m "feat: keke9 API 客户端（搜索 + 详情）"
```

---

### 任务 5：Server 酱推送

**文件：**
- 创建：`internal/notify/serverchan.go`

- [ ] **步骤 1：编写 Server 酱推送**

```go
// internal/notify/serverchan.go
package notify

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ServerChan struct {
	SendKey string
	Client  *http.Client
}

func NewServerChan(sendKey string) *ServerChan {
	return &ServerChan{
		SendKey: sendKey,
		Client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// UpdateItem 单个动漫更新项
type UpdateItem struct {
	Name     string
	Remarks  string
	DetailURL string
}

// Send 聚合推送多个动漫更新
func (sc *ServerChan) Send(items []UpdateItem) error {
	if sc.SendKey == "" {
		return fmt.Errorf("server chan sendkey is empty")
	}
	if len(items) == 0 {
		return nil
	}

	title := "🍥 动漫更新提醒"
	var body strings.Builder
	body.WriteString(title + "\n\n")

	for _, item := range items {
		body.WriteString(fmt.Sprintf("《%s》更新了！\n", item.Name))
		body.WriteString(fmt.Sprintf("当前进度：%s\n", item.Remarks))
		body.WriteString(fmt.Sprintf("详情链接：%s\n\n", item.DetailURL))
	}
	body.WriteString("—— anime-tip")

	apiURL := fmt.Sprintf("https://sctapi.ftqq.com/%s.send", sc.SendKey)
	data := url.Values{}
	data.Set("title", title)
	data.Set("desp", body.String())

	resp, err := sc.Client.PostForm(apiURL, data)
	if err != nil {
		return fmt.Errorf("server chan post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server chan status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}
```

- [ ] **步骤 2：编译验证**

运行：
```bash
go build ./...
```
预期：编译通过

- [ ] **步骤 3：Commit**

```bash
git add internal/notify/
git commit -m "feat: Server 酱微信推送（聚合消息）"
```

---

### 任务 6：定时调度 + 更新检查逻辑

**文件：**
- 创建：`internal/scheduler/scheduler.go`

- [ ] **步骤 1：安装 cron 依赖**

运行：
```bash
go get github.com/robfig/cron/v3
go get github.com/gin-gonic/gin
```

- [ ] **步骤 2：编写定时调度器**

```go
// internal/scheduler/scheduler.go
package scheduler

import (
	"database/sql"
	"log"

	"github.com/robfig/cron/v3"
	"github.com/user/anime-tip/internal/crawler"
	"github.com/user/anime-tip/internal/notify"
	"github.com/user/anime-tip/internal/store"
)

type Scheduler struct {
	db      *sql.DB
	crawler *crawler.Client
	cron    *cron.Cron
}

func New(db *sql.DB, crawler *crawler.Client) *Scheduler {
	return &Scheduler{
		db:      db,
		crawler: crawler,
		cron:    cron.New(),
	}
}

func (s *Scheduler) Start(cronExpr string) {
	s.cron.AddFunc(cronExpr, func() {
		log.Println("[scheduler] 开始检查动漫更新...")
		if err := s.CheckUpdates(); err != nil {
			log.Printf("[scheduler] 检查更新失败: %v", err)
		}
	})
	s.cron.Start()
	log.Printf("[scheduler] 定时任务已启动: %s", cronExpr)
}

func (s *Scheduler) Stop() {
	s.cron.Stop()
}

// CheckUpdates 检查所有关注动漫的更新，推送变化
func (s *Scheduler) CheckUpdates() error {
	animes, err := store.ListAnimes(s.db)
	if err != nil {
		return err
	}
	if len(animes) == 0 {
		log.Println("[scheduler] 关注列表为空，跳过检查")
		return nil
	}

	var updates []notify.UpdateItem
	var toUpdate []struct {
		id                  int64
		currentRemarks      string
		lastNotifiedRemarks string
	}

	for _, a := range animes {
		detail, err := s.crawler.GetAnimeDetail(a.VodID)
		if err != nil {
			log.Printf("[scheduler] 获取动漫 %s (vod_id=%d) 详情失败: %v", a.Name, a.VodID, err)
			continue
		}

		latestRemarks := detail.Info.VodRemarks

		// 当前记录的 remarks 与上次推送的 remarks 不同，说明有更新
		if a.LastNotifiedRemarks != "" && latestRemarks != a.LastNotifiedRemarks {
			updates = append(updates, notify.UpdateItem{
				Name:      a.Name,
				Remarks:   latestRemarks,
				DetailURL: s.crawler.BaseURL + detail.Info.VodLink,
			})
		}

		// 无论是否有变化，都更新 current_remarks；如果上次推送的 remarks 为空（首次关注），不触发通知
		if latestRemarks != a.CurrentRemarks || a.LastNotifiedRemarks != a.CurrentRemarks {
			toUpdate = append(toUpdate, struct {
				id                  int64
				currentRemarks      string
				lastNotifiedRemarks string
			}{a.ID, latestRemarks, a.CurrentRemarks})
		}
	}

	// 如果有更新，推送通知
	if len(updates) > 0 {
		sendKey, _ := store.GetSetting(s.db, "server_chan_key")
		sc := notify.NewServerChan(sendKey)
		if err := sc.Send(updates); err != nil {
			return err
		}
		log.Printf("[scheduler] 推送了 %d 条动漫更新", len(updates))
	}

	// 推送成功后更新数据库
	for _, u := range toUpdate {
		if err := store.UpdateAnimeRemarks(s.db, u.id, u.currentRemarks, u.lastNotifiedRemarks); err != nil {
			log.Printf("[scheduler] 更新动漫 remarks 失败 (id=%d): %v", u.id, err)
		}
	}

	return nil
}
```

- [ ] **步骤 3：编译验证**

运行：
```bash
go build ./...
```
预期：编译通过

- [ ] **步骤 4：Commit**

```bash
git add internal/scheduler/ go.mod go.sum
git commit -m "feat: 定时调度器 + 更新检查逻辑"
```

---

### 任务 7：Gin 路由 + API Handler

**文件：**
- 创建：`internal/web/handler.go`
- 创建：`internal/web/anime_handler.go`
- 创建：`internal/web/search_handler.go`
- 创建：`internal/web/setting_handler.go`
- 创建：`internal/web/check_handler.go`

- [ ] **步骤 1：编写路由注册 + 静态文件服务**

```go
// internal/web/handler.go
package web

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/user/anime-tip/internal/crawler"
	"github.com/user/anime-tip/internal/scheduler"
)

type Handler struct {
	db        *sql.DB
	crawler   *crawler.Client
	scheduler *scheduler.Scheduler
}

func NewHandler(db *sql.DB, cralwer *crawler.Client, sched *scheduler.Scheduler) *Handler {
	return &Handler{
		db:        db,
		crawler:   cralwer,
		scheduler: sched,
	}
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	r.Static("/static", "./web/static")
	r.StaticFile("/", "./web/index.html")

	api := r.Group("/api")
	{
		api.GET("/animes", h.ListAnimes)
		api.POST("/animes", h.CreateAnime)
		api.DELETE("/animes/:id", h.DeleteAnime)

		api.GET("/search", h.Search)

		api.GET("/settings", h.ListSettings)
		api.PUT("/settings", h.UpdateSettings)

		api.POST("/check", h.TriggerCheck)
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Next()
	}
}

func SetupRouter(h *Handler) *gin.Engine {
	r := gin.Default()
	r.Use(corsMiddleware())
	h.RegisterRoutes(r)
	return r
}
```

- [ ] **步骤 2：编写动漫关注 Handler**

```go
// internal/web/anime_handler.go
package web

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/user/anime-tip/internal/model"
	"github.com/user/anime-tip/internal/store"
)

func (h *Handler) ListAnimes(c *gin.Context) {
	animes, err := store.ListAnimes(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if animes == nil {
		animes = []model.Anime{}
	}
	c.JSON(http.StatusOK, gin.H{"data": animes})
}

func (h *Handler) CreateAnime(c *gin.Context) {
	var req struct {
		VodID          int    `json:"vod_id" binding:"required"`
		Name           string `json:"name" binding:"required"`
		Cover          string `json:"cover"`
		CurrentRemarks string `json:"current_remarks"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 检查是否已关注
	existing, err := store.GetAnimeByVodID(h.db, req.VodID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "该动漫已在关注列表中"})
		return
	}

	a := &model.Anime{
		VodID:          req.VodID,
		Name:           req.Name,
		Cover:          req.Cover,
		CurrentRemarks: req.CurrentRemarks,
	}
	if err := store.CreateAnime(h.db, a); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": a})
}

func (h *Handler) DeleteAnime(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := store.DeleteAnime(h.db, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已取消关注"})
}
```

- [ ] **步骤 3：编写搜索 Handler**

```go
// internal/web/search_handler.go
package web

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *Handler) Search(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "搜索关键词不能为空"})
		return
	}

	items, err := h.crawler.SearchAnime(q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}
```

- [ ] **步骤 4：编写设置 Handler**

```go
// internal/web/setting_handler.go
package web

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/user/anime-tip/internal/store"
)

func (h *Handler) ListSettings(c *gin.Context) {
	settings, err := store.ListSettings(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": settings})
}

func (h *Handler) UpdateSettings(c *gin.Context) {
	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	for key, value := range req {
		if err := store.SetSetting(h.db, key, value); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"message": "设置已更新"})
}
```

- [ ] **步骤 5：编写手动触发检查 Handler**

```go
// internal/web/check_handler.go
package web

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *Handler) TriggerCheck(c *gin.Context) {
	go func() {
		if err := h.scheduler.CheckUpdates(); err != nil {
			_ = c.Error(err)
		}
	}()
	c.JSON(http.StatusOK, gin.H{"message": "检查任务已在后台启动"})
}
```

- [ ] **步骤 6：编译验证**

运行：
```bash
go build ./...
```
预期：编译通过

- [ ] **步骤 7：Commit**

```bash
git add internal/web/
git commit -m "feat: Gin 路由 + 全部 API Handler"
```

---

### 任务 8：组装 main.go

**文件：**
- 修改：`cmd/server/main.go`

- [ ] **步骤 1：更新 main.go 完整串联所有组件**

```go
// cmd/server/main.go
package main

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/user/anime-tip/internal/config"
	"github.com/user/anime-tip/internal/crawler"
	"github.com/user/anime-tip/internal/scheduler"
	"github.com/user/anime-tip/internal/store"
	"github.com/user/anime-tip/internal/web"
)

func main() {
	cfg := config.Load()
	log.Printf("anime-tip starting on :%s", cfg.Port)

	// 初始化数据库
	db, err := store.InitDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}
	defer db.Close()

	// 如果环境变量有 Server Chan Key，写入 settings
	if cfg.ServerChanKey != "" {
		if err := store.SetSetting(db, "server_chan_key", cfg.ServerChanKey); err != nil {
			log.Printf("警告: 写入 server_chan_key 失败: %v", err)
		}
	}

	// 初始化 keke9 爬虫客户端
	cr := crawler.NewClient(cfg.Keke9BaseURL)

	// 初始化定时调度器
	sched := scheduler.New(db, cr)
	sched.Start(cfg.CheckCron)

	// 初始化 Gin 路由
	h := web.NewHandler(db, cr, sched)
	r := web.SetupRouter(h)

	log.Printf("anime-tip 已启动，监听 :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("启动 HTTP 服务失败: %v", err)
	}
}
```

- [ ] **步骤 2：编译验证**

运行：
```bash
go build ./cmd/server/
```
预期：编译通过

- [ ] **步骤 3：Commit**

```bash
git add cmd/server/main.go
git commit -m "feat: 组装 main.go 串联所有组件"
```

---

### 任务 9：前端 Web 界面

**文件：**
- 创建：`web/index.html`
- 创建：`web/static/style.css`

- [ ] **步骤 1：创建 CSS 样式**

```css
/* web/static/style.css */
* { margin: 0; padding: 0; box-sizing: border-box; }

body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  background: #0f0f1a;
  color: #e0e0e0;
  min-height: 100vh;
}

.app { max-width: 800px; margin: 0 auto; padding: 20px; }

header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 0;
  border-bottom: 1px solid #2a2a3e;
  margin-bottom: 24px;
}

header h1 { font-size: 20px; color: #8b5cf6; }

nav a {
  color: #aaa;
  text-decoration: none;
  margin-left: 16px;
  font-size: 14px;
  padding: 6px 12px;
  border-radius: 6px;
  transition: all 0.2s;
}
nav a:hover, nav a.active { color: #fff; background: #2a2a3e; }

.card {
  background: #1a1a2e;
  border-radius: 12px;
  padding: 16px;
  margin-bottom: 12px;
  display: flex;
  align-items: center;
  gap: 14px;
  transition: transform 0.2s;
}
.card:hover { transform: translateY(-2px); }

.card img {
  width: 60px;
  height: 80px;
  object-fit: cover;
  border-radius: 8px;
}

.card-info { flex: 1; }
.card-info h3 { font-size: 15px; margin-bottom: 4px; }
.card-info p { font-size: 13px; color: #888; }

.card .btn-remove {
  background: #dc2626;
  color: #fff;
  border: none;
  padding: 6px 14px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 12px;
}
.card .btn-remove:hover { background: #b91c1c; }

.btn-primary {
  background: #8b5cf6;
  color: #fff;
  border: none;
  padding: 8px 18px;
  border-radius: 8px;
  cursor: pointer;
  font-size: 14px;
  transition: background 0.2s;
}
.btn-primary:hover { background: #7c3aed; }

.btn-follow {
  background: #10b981;
  color: #fff;
  border: none;
  padding: 5px 12px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 12px;
}
.btn-follow:hover { background: #059669; }
.btn-follow:disabled { background: #555; cursor: not-allowed; }

input[type="text"], input[type="password"] {
  width: 100%;
  padding: 10px 14px;
  background: #1a1a2e;
  border: 1px solid #2a2a3e;
  border-radius: 8px;
  color: #e0e0e0;
  font-size: 14px;
  outline: none;
  transition: border-color 0.2s;
}
input:focus { border-color: #8b5cf6; }

.search-bar {
  display: flex;
  gap: 10px;
  margin-bottom: 20px;
}
.search-bar input { flex: 1; }

.setting-group {
  margin-bottom: 20px;
}
.setting-group label {
  display: block;
  font-size: 13px;
  color: #888;
  margin-bottom: 6px;
}

.empty-state {
  text-align: center;
  color: #666;
  padding: 60px 0;
  font-size: 14px;
}

.check-btn {
  background: transparent;
  border: 1px solid #8b5cf6;
  color: #8b5cf6;
  padding: 6px 14px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 12px;
  margin-left: 12px;
}
.check-btn:hover { background: #8b5cf6; color: #fff; }
```

- [ ] **步骤 2：创建前端单页应用**

```html
<!-- web/index.html -->
<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>anime-tip 动漫更新提醒</title>
  <link rel="stylesheet" href="/static/style.css">
  <script defer src="https://cdn.jsdelivr.net/npm/alpinejs@3.x.x/dist/cdn.min.js"></script>
</head>
<body>
<div class="app" x-data="app()" x-init="init()">

  <header>
    <h1>🍥 anime-tip</h1>
    <nav>
      <a href="#" :class="{ active: tab==='list' }" @click.prevent="tab='list'">关注列表</a>
      <a href="#" :class="{ active: tab==='search' }" @click.prevent="tab='search'">搜索</a>
      <a href="#" :class="{ active: tab==='settings' }" @click.prevent="tab='settings'">设置</a>
      <button class="check-btn" @click="checkNow" :disabled="checking">
        {{ checking ? '检查中...' : '立即检查' }}
      </button>
    </nav>
  </header>

  <!-- 关注列表 -->
  <div x-show="tab==='list'">
    <div class="empty-state" x-show="animes.length===0 && !loading">
      还没有关注任何动漫，去「搜索」添加吧！
    </div>
    <div class="card" x-for="a in animes" :key="a.id">
      <img :src="a.cover || '/static/placeholder.png'" :alt="a.name">
      <div class="card-info">
        <h3 x-text="a.name"></h3>
        <p x-text="'当前：' + (a.current_remarks || '未知')"></p>
      </div>
      <button class="btn-remove" @click="removeAnime(a.id)">取消关注</button>
    </div>
  </div>

  <!-- 搜索 -->
  <div x-show="tab==='search'">
    <div class="search-bar">
      <input type="text" x-model="query" placeholder="输入动漫名称搜索..."
             @keydown.enter="search">
      <button class="btn-primary" @click="search" :disabled="searching">
        {{ searching ? '搜索中...' : '搜索' }}
      </button>
    </div>
    <div class="card" x-for="item in searchResults" :key="item.vod_id">
      <img :src="item.vod_pic" :alt="item.vod_name">
      <div class="card-info">
        <h3 x-text="item.vod_name"></h3>
        <p x-text="(item.vod_remarks || '未知') + ' · ' + (item.vod_area || '')"></p>
      </div>
      <button class="btn-follow"
              @click="followAnime(item)"
              :disabled="isFollowed(item.vod_id)">
        {{ isFollowed(item.vod_id) ? '已关注' : '关注' }}
      </button>
    </div>
    <div class="empty-state" x-show="searchResults.length===0 && searched">
      没有找到相关动漫
    </div>
  </div>

  <!-- 设置 -->
  <div x-show="tab==='settings'">
    <div class="setting-group">
      <label>Server 酱 SendKey</label>
      <input type="password" x-model="settings.server_chan_key" placeholder="输入 Server 酱 SendKey">
    </div>
    <button class="btn-primary" @click="saveSettings">保存设置</button>
  </div>

</div>

<script>
function app() {
  return {
    tab: 'list',
    animes: [],
    query: '',
    searchResults: [],
    settings: { server_chan_key: '' },
    loading: false,
    searching: false,
    searched: false,
    checking: false,

    async init() {
      await this.loadAnimes();
      await this.loadSettings();
    },

    async loadAnimes() {
      this.loading = true;
      const res = await fetch('/api/animes');
      const data = await res.json();
      this.animes = data.data || [];
      this.loading = false;
    },

    async search() {
      if (!this.query.trim()) return;
      this.searching = true;
      this.searched = true;
      const res = await fetch('/api/search?q=' + encodeURIComponent(this.query));
      const data = await res.json();
      this.searchResults = data.data || [];
      this.searching = false;
    },

    isFollowed(vodId) {
      return this.animes.some(a => a.vod_id === vodId);
    },

    async followAnime(item) {
      const res = await fetch('/api/animes', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          vod_id: item.vod_id,
          name: item.vod_name,
          cover: item.vod_pic,
          current_remarks: item.vod_remarks || '',
        }),
      });
      if (res.ok) {
        await this.loadAnimes();
      } else {
        const err = await res.json();
        alert(err.error || '关注失败');
      }
    },

    async removeAnime(id) {
      if (!confirm('确定取消关注？')) return;
      await fetch('/api/animes/' + id, { method: 'DELETE' });
      await this.loadAnimes();
    },

    async loadSettings() {
      const res = await fetch('/api/settings');
      const data = await res.json();
      const list = data.data || [];
      const map = {};
      list.forEach(s => { map[s.key] = s.value; });
      this.settings = { server_chan_key: map.server_chan_key || '' };
    },

    async saveSettings() {
      await fetch('/api/settings', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(this.settings),
      });
      alert('设置已保存');
    },

    async checkNow() {
      this.checking = true;
      await fetch('/api/check', { method: 'POST' });
      this.checking = false;
      alert('检查已触发，请稍候查看微信通知');
    },
  };
}
</script>
</body>
</html>
```

- [ ] **步骤 3：验证前端页面可加载**

运行后端后，浏览器访问 `http://localhost:8080/` 应能看到页面

- [ ] **步骤 4：Commit**

```bash
git add web/
git commit -m "feat: 前端 Web 管理界面（Alpine.js 单页应用）"
```

---

### 任务 10：Docker 部署

**文件：**
- 创建：`Dockerfile`
- 创建：`docker-compose.yml`

- [ ] **步骤 1：编写 Dockerfile（多阶段构建）**

```dockerfile
# Dockerfile
# 阶段1: 构建
FROM golang:1.22-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -o /anime-tip ./cmd/server/

# 阶段2: 运行
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /anime-tip .
COPY web/ ./web/
ENV PORT=8080
ENV CHECK_INTERVAL="0 * * * *"
ENV DB_PATH=/data/anime-tip.db
EXPOSE 8080
VOLUME /data
CMD ["./anime-tip"]
```

- [ ] **步骤 2：编写 docker-compose.yml**

```yaml
# docker-compose.yml
version: "3.8"
services:
  anime-tip:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - anime-tip-data:/data
    environment:
      - PORT=8080
      - CHECK_INTERVAL=0 * * * *
      - SERVER_CHAN_KEY=
      - KEKE9_BASE_URL=https://www.keke9.com
    restart: unless-stopped

volumes:
  anime-tip-data:
```

- [ ] **步骤 3：Commit**

```bash
git add Dockerfile docker-compose.yml
git commit -m "feat: Docker 多阶段构建 + docker-compose 部署"
```

---

### 任务 11：端到端集成验证

**文件：** 无新增

- [ ] **步骤 1：本地启动完整服务**

运行：
```bash
go run ./cmd/server/
```
预期：输出 `anime-tip 已启动，监听 :8080`

- [ ] **步骤 2：验证前端页面**

浏览器访问 `http://localhost:8080/`，确认三个 tab（关注列表、搜索、设置）可切换

- [ ] **步骤 3：验证搜索功能**

在搜索页输入关键词，确认可返回搜索结果

- [ ] **步骤 4：验证关注功能**

点击搜索结果的"关注"按钮，确认出现在关注列表

- [ ] **步骤 5：验证设置保存**

在设置页输入 Server 酱 Key 并保存，刷新页面确认值保留

- [ ] **步骤 6：验证手动检查**

点击"立即检查"按钮，确认后台日志输出检查过程

- [ ] **步骤 7：Commit 验证结果**

如有修复，commit 修复代码

---

## 自检结果

### 1. 规格覆盖度

| 规格需求 | 对应任务 |
|----------|---------|
| 搜索 keke9 动漫 | 任务 4 (crawler) + 任务 7 (search handler) |
| 添加/取消关注 | 任务 3 (store) + 任务 7 (anime handler) |
| 定时检查更新 | 任务 6 (scheduler) |
| Server 酱推送 | 任务 5 (notify) |
| 聚合推送 | 任务 5 (Send 方法) + 任务 6 (CheckUpdates) |
| Web 管理界面 | 任务 9 (前端) |
| Docker 部署 | 任务 10 |
| 环境变量配置 | 任务 1 (config) |

**遗漏：** 无

### 2. 占位符扫描

无 TODO/TBD/待定/后续实现等占位符。所有步骤包含完整代码。

### 3. 类型一致性

- `model.Anime` 字段名在 store/web/scheduler 中使用一致
- `model.Keke9VodItem` 字段名与 API 响应 JSON tag 匹配
- `notify.UpdateItem` 在 scheduler 和 notify 之间一致
- `store` 函数签名在 handler 中调用匹配
