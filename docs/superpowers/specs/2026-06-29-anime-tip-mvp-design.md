# anime-tip MVP 设计规格

## 项目概览

**名称**：anime-tip（动漫更新提醒器）
**目标**：监控 keke9.com 上的动漫更新，当关注的动漫有新集时，通过 Server 酱推送微信通知。
**技术栈**：Go + Gin + SQLite + Alpine.js + Docker

## 架构

```
┌─────────────┐     ┌──────────────┐     ┌──────────┐
│  Web 管理页面  │────▶│   Go 后端服务   │────▶│ keke9 API │
│ (HTML+Alpine)│◀────│  (Gin + Cron) │     │ /api.php  │
└─────────────┘     └──────┬───────┘     └──────────┘
                           │
                           ▼
                    ┌──────────────┐
                    │   Server 酱   │
                    │   微信推送     │
                    └──────────────┘
```

MVP 不包含：Playwright Cookie 获取、HTML 解析兜底。仅通过 MacCMS API 获取数据。

## 核心流程

1. 用户在 Web 页面搜索动漫 → 后端调 keke9 `/api.php/vod/` 搜索接口
2. 用户点击"关注"→ 后端存入 SQLite 关注列表，记录当前集数
3. 每小时 cron 任务遍历关注列表 → 调 keke9 API 检查 `vod_remarks` 是否变化
4. 检测到新集数 → 调 Server 酱推送微信通知
5. 多个更新合并为一条消息推送（省 Server 酱额度）

## 数据存储（SQLite）

### animes 表

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | 自增主键 |
| vod_id | INTEGER UNIQUE | keke9 视频 ID |
| name | TEXT | 动漫名称 |
| cover | TEXT | 封面图 URL |
| current_remarks | TEXT | 当前集数描述（如"更新至12集"） |
| last_notified_remarks | TEXT | 上次推送时的集数描述 |
| created_at | DATETIME | 关注时间 |

### settings 表

| 字段 | 类型 | 说明 |
|------|------|------|
| key | TEXT PK | 配置键名 |
| value | TEXT | 配置值 |

需要的设置项：`server_chan_key`（Server 酱 SendKey）

## API 调用（keke9 MacCMS v10 标准接口）

- **搜索动漫**：`GET /api.php/vod/get_list?vod_name=关键词&type_id=4` → 搜索动漫（type_id=4 为动漫分类）
- **获取详情**：`GET /api.php/vod/get_detail?vod_id=xxx` → 获取单部动漫详情 + 集数
- **检查更新**：用 get_detail 取每个关注项的 `vod_remarks` 字段

### API 返回格式

```json
{
    "code": 1,
    "msg": "获取成功",
    "info": {
        "offset": 0,
        "limit": 20,
        "total": 1234,
        "rows": [
            {
                "vod_id": 1,
                "vod_name": "...",
                "vod_pic": "https://...",
                "vod_actor": "...",
                "vod_area": "日本",
                "vod_year": "2025",
                "vod_class": "热血",
                "vod_remarks": "更新至12集",
                "vod_score": "9.0",
                "type_id": 4,
                "vod_link": "/voddetail/1.html"
            }
        ]
    }
}
```

## Web 管理界面（3 个视图，单页应用 Alpine.js）

1. **首页/关注列表**：显示关注的所有动漫，当前集数，最后检查时间；支持取消关注
2. **搜索页**：输入关键词搜 keke9 动漫库，点击"关注"添加
3. **设置页**：配置 Server 酱 SendKey

## 后端 API 设计

### 关注列表

- `GET /api/animes` - 获取关注列表
- `POST /api/animes` - 添加关注（body: vod_id, name, cover, current_remarks）
- `DELETE /api/animes/:id` - 取消关注

### 搜索

- `GET /api/search?q=关键词` - 搜索 keke9 动漫（代理转发 keke9 API）

### 设置

- `GET /api/settings` - 获取所有设置
- `PUT /api/settings` - 更新设置

### 手动触发

- `POST /api/check` - 手动触发一次更新检查

## 定时任务

- 使用 robfig/cron 库
- 默认每小时执行一次更新检查
- 检查间隔可通过环境变量 `CHECK_INTERVAL` 配置

## 更新检测逻辑

1. 遍历 animes 表所有记录
2. 对每条记录调 keke9 API get_detail 获取最新 vod_remarks
3. 如果 vod_remarks != last_notified_remarks，判定为有更新
4. 收集本轮所有有更新的动漫
5. 合并为一条消息，调 Server 酱推送
6. 推送成功后，将 last_notified_remarks 更新为当前 vod_remarks
7. 如果推送失败，不更新 last_notified_remarks，下次检查会再次尝试

## 推送消息格式

```
🍥 动漫更新提醒

《某动漫名》更新了！
当前进度：更新至第6集
详情链接：https://www.keke9.com/voddetail/12345.html

—— anime-tip
```

多个更新聚合为一条消息，每部动漫分行展示。

## 项目结构

```
anime-tip/
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   ├── config/          # 配置（端口、检查间隔等）
│   ├── crawler/          # keke9 API 调用
│   ├── model/            # 数据结构定义
│   ├── notify/           # Server 酱推送
│   ├── scheduler/        # robfig/cron 定时任务
│   ├── store/            # SQLite CRUD
│   └── web/              # Gin 路由 + API handler
├── web/
│   ├── index.html        # 单页应用（Alpine.js）
│   └── static/
│       ├── style.css
│       └── app.js
├── Dockerfile
├── docker-compose.yml
├── go.mod
└── go.sum
```

## 部署方案

- Docker 镜像构建（多阶段构建，最终镜像仅包含 Go 二进制 + web 静态文件）
- docker-compose.yml 定义服务，挂载 SQLite 数据目录
- 环境变量：`PORT`（默认 8080）、`CHECK_INTERVAL`（默认 `0 * * * *`）、`SERVER_CHAN_KEY`（也可通过 Web 设置页配置）

## MVP 范围边界

**包含**：
- 搜索 keke9 动漫并添加关注
- 定时检查更新 + Server 酱微信推送
- Web 管理界面（关注列表、搜索、设置）
- Docker 部署

**不包含**：
- 多用户支持
- Playwright Cookie 自动获取
- HTML 解析兜底
- 多站点支持
- 邮件/其他通知渠道
- 用户认证
