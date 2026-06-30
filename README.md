# 🍥 anime-tip

动漫更新提醒器——自动追踪关注动漫的更新进度，通过 Server 酱推送微信通知。

## 特性

- 🔍 **动漫搜索**：从可可动漫（keke9）数据源搜索动漫
- 📌 **关注管理**：添加/移除关注，实时查看更新进度
- ⏰ **定时检查**：Cron 定时轮询，检测到更新自动推送
- 📬 **微信通知**：通过 Server 酱将更新推送到微信
- 🌐 **Web 管理界面**：基于 Alpine.js 的轻量单页应用

## 项目结构

```
anime-tip/
├── cmd/server/           # 程序入口
│   └── main.go
├── internal/
│   ├── config/            # 配置加载（YAML 文件 + 环境变量）
│   ├── crawler/           # keke9 API 爬虫客户端
│   ├── model/             # 数据模型（Anime、Keke9 API 响应）
│   ├── notify/            # Server 酱推送
│   ├── scheduler/         # Cron 定时调度器
│   ├── store/             # SQLite 数据库操作
│   └── web/               # Gin HTTP 路由 + Handler
├── web/                   # 前端静态文件（Alpine.js 单页应用）
│   ├── index.html
│   └── static/
│       └── style.css
├── Dockerfile             # 多阶段构建
├── docker-compose.yml
├── Makefile               # 本地构建入口（make build / run / clean）
├── config.example.yaml    # 配置文件示例
├── go.mod
└── go.sum
```

## 技术栈

| 层级 | 技术 |
|------|------|
| 后端 | Go 1.25 + Gin |
| 数据库 | SQLite（modernc.org/sqlite，纯 Go 实现，无需 CGO） |
| 定时任务 | robfig/cron/v3 |
| 前端 | Alpine.js 3（CDN 引入，无需构建） |
| 推送 | Server 酱（sctapi.ftqq.com） |
| 容器化 | Docker 多阶段构建 + docker-compose |

## 快速开始

### 环境要求

- Go >= 1.25
- 或者 Docker + Docker Compose

### 方式一：本地运行

```bash
# 克隆项目
git clone https://github.com/user/anime-tip.git
cd anime-tip

# 安装依赖
go mod download

# 方式一：使用配置文件
cp config.example.yaml config.yaml
# 编辑 config.yaml，填入 Server 酱 SendKey 等
go run ./cmd/server/

# 方式二：使用环境变量
export SERVER_CHAN_KEY=your_sendkey_here
go run ./cmd/server/
```

启动后访问 `http://localhost:8080` 即可打开 Web 管理界面。

> 想要编译后的可执行文件而不是 `go run`？使用 Makefile 统一构建，产物会输出到 `build/` 目录，不会污染项目根目录：
>
> ```bash
> make build      # 编译到 build/anime-tip（Windows 为 build/anime-tip.exe）
> make run        # 直接 go run，不产出二进制
> make clean      # 清理 build/ 与运行时日志
> ```
>
> 程序日志通过标准输出打印（标准库 `log` 包）。如需落盘，运行时重定向即可，建议统一放到 `logs/`（已被 `.gitignore` 忽略）：
>
> ```bash
> ./build/anime-tip > logs/server.log 2>&1
> ```

### 方式二：Docker Compose 部署

```bash
# 克隆项目
git clone https://github.com/user/anime-tip.git
cd anime-tip

# 方式一：使用环境变量
# 编辑 docker-compose.yml，填入 Server 酱 SendKey
# SERVER_CHAN_KEY=your_sendkey_here

# 方式二：使用配置文件
# 创建 config.yaml 后挂载到容器中
# 在 docker-compose.yml 的 volumes 中添加：
#   - ./config.yaml:/app/config.yaml

# 一键启动
docker-compose up -d
```

访问 `http://localhost:8080` 打开管理界面。

## 配置

支持 **YAML 配置文件** 和 **环境变量** 两种方式，优先级为：

**默认值 < 配置文件 < 环境变量**（环境变量优先级最高）

### 配置文件

复制示例文件后按需修改：

```bash
cp config.example.yaml config.yaml
```

也可通过命令行参数或环境变量指定配置文件路径：

```bash
# 命令行参数
go run ./cmd/server/ -config /path/to/config.yaml

# 环境变量
export CONFIG_PATH=/path/to/config.yaml
```

> 路径解析优先级：`-config` 参数 > `CONFIG_PATH` 环境变量 > 默认 `config.yaml`。
> 传空字符串（如 `-config ""`）等同未指定，会回退到下一优先级；显式指定的路径若不存在，会回退到默认值 + 环境变量（仅打印提示，不报错）。

> `config.yaml` 已加入 `.gitignore`，不会意外提交敏感信息（如 SendKey）。

### 环境变量

| 环境变量 | 默认值 | 说明 |
|----------|--------|------|
| `PORT` | `8080` | HTTP 服务监听端口 |
| `CHECK_CRON` | `0 * * * *` | Cron 表达式，控制定时检查频率（默认每小时整点）。`CHECK_INTERVAL` 为兼容别名，等价生效 |
| `SERVER_CHAN_KEY` | （空） | Server 酱 SendKey，用于推送微信通知 |
| `KEKE9_BASE_URL` | `https://www.keke9.com` | 可可动漫 API 地址 |
| `DB_PATH` | `anime-tip.db` | SQLite 数据库文件路径 |

### config.yaml 配置项

```yaml
port: "8080"
check_cron: "0 * * * *"
server_chan_key: ""               # Server 酱 SendKey
keke9_base_url: "https://www.keke9.com"
db_path: "anime-tip.db"
```

### Cron 表达式示例

| 表达式 | 含义 |
|--------|------|
| `0 * * * *` | 每小时整点检查 |
| `*/30 * * * *` | 每 30 分钟检查一次 |
| `0 9,21 * * *` | 每天 9:00 和 21:00 检查 |

## API 接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/animes` | 获取关注列表 |
| POST | `/api/animes` | 添加关注 |
| DELETE | `/api/animes/:id` | 取消关注 |
| GET | `/api/search?q=关键词` | 搜索动漫 |
| GET | `/api/settings` | 获取设置 |
| PUT | `/api/settings` | 更新设置 |
| POST | `/api/check` | 手动触发更新检查 |

### 添加关注示例

```bash
curl -X POST http://localhost:8080/api/animes \
  -H "Content-Type: application/json" \
  -d '{"vod_id": 12345, "name": "咒术回战", "cover": "https://...", "current_remarks": "更新至第24集"}'
```

## 工作原理

1. **搜索与关注**：在 Web 界面搜索动漫，点击「关注」添加到监控列表
2. **定时检查**：调度器按 Cron 表达式定时运行，逐个获取关注动漫的最新信息
3. **变化检测**：对比当前 `remarks` 与上次推送时的 `remarks`，不同则判定为有更新
4. **推送通知**：将所有更新汇总为一条消息，通过 Server 酱推送到微信
5. **状态同步**：推送成功后更新数据库中的 `current_remarks` 和 `last_notified_remarks`

> 调度器内置互斥锁，防止手动触发和定时触发并发执行导致重复推送。

## 数据存储

使用 SQLite（WAL 模式），数据文件默认为 `anime-tip.db`，包含两张表：

- **`animes`**：关注的动漫列表（vod_id、名称、封面、当前进度、上次推送进度）
- **`settings`**：键值对配置（如 server_chan_key）

Docker 部署时数据库文件持久化在命名卷 `anime-tip-data` 中。

## 许可证

[MIT](./LICENSE)
