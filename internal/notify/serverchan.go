package notify

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// maxRespBytes 限制读取的响应体大小，避免异常响应耗尽内存。
const maxRespBytes = 8 << 10 // 8 KiB

type ServerChan struct {
	SendKey string
	Client  *http.Client
	apiBase string // 测试注入用，生产为空则走默认地址
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
	Name      string
	Remarks   string
	DetailURL string
}

// Ping 测试 Server 酱连通性，发送一条测试消息。
// 返回 nil 表示 key 有效且可达，否则返回具体错误。
func (sc *ServerChan) Ping() error {
	if sc.SendKey == "" {
		return fmt.Errorf("server chan sendkey is empty")
	}
	return sc.post("🍥 anime-tip 连通性测试",
		"如果你收到了这条消息，说明 Server 酱通知配置正确！\n\n—— anime-tip")
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

	return sc.post(title, body.String())
}

// post 向 Server 酱发送一条消息，并按真实 API 行为判定成功与否。
//
// Server 酱 Turbo 的约定：HTTP 状态码非 2xx 直接判失败；
// 即使返回 2xx，仍需解析响应体 JSON 的 code 字段，code!=0 视为失败
// （如限流、Key 失效等场景下接口可能仍返回 2xx）。
func (sc *ServerChan) post(title, desp string) error {
	apiURL := fmt.Sprintf("%s/%s.send", sc.apiBaseURL(), sc.SendKey)
	data := url.Values{}
	data.Set("title", title)
	data.Set("desp", desp)

	resp, err := sc.Client.PostForm(apiURL, data)
	if err != nil {
		return fmt.Errorf("server chan post: %w", err)
	}
	defer resp.Body.Close()

	// 限制读取大小，避免异常响应耗尽内存。
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxRespBytes))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("server chan status %d: %s", resp.StatusCode, string(respBody))
	}

	// 2xx 仍需校验业务码：code!=0 表示失败（如 Key 失效、限流等）。
	var r struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(respBody, &r); err != nil {
		return fmt.Errorf("server chan 解析响应失败: %w (body: %s)", err, string(respBody))
	}
	if r.Code != 0 {
		return fmt.Errorf("server chan 失败(code=%d): %s", r.Code, r.Message)
	}
	return nil
}

// apiBaseURL 返回 Server 酱 API 根地址，默认为 https://sctapi.ftqq.com。
func (sc *ServerChan) apiBaseURL() string {
	if sc.apiBase != "" {
		return sc.apiBase
	}
	return "https://sctapi.ftqq.com"
}
