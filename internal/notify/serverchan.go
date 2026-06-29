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
	Name      string
	Remarks   string
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
