package crawler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/user/anime-tip/internal/model"
)

// 默认详情页 URL 模板（苹果 CMS 通用 friendly URL）。
// {base} 替换为数据源根地址，{id} 替换为 vod_id。
const defaultDetailURLTmpl = "{base}/index.php/vod/detail/id/{id}.html"

// Client 苹果 CMS provide/vod 采集接口客户端。
// 数据源地址走配置，来源站不可用时换一个即可，不再硬绑死域名。
type Client struct {
	BaseURL       string
	DetailURLTmpl string
	HTTPClient    *http.Client
}

func NewClient(baseURL, detailURLTmpl string) *Client {
	if detailURLTmpl == "" {
		detailURLTmpl = defaultDetailURLTmpl
	}
	return &Client{
		BaseURL:       strings.TrimRight(baseURL, "/"),
		DetailURLTmpl: detailURLTmpl,
		HTTPClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// SearchAnime 按关键词搜索动漫（ac=detail 返回含 vod_remarks 的详细列表）。
func (c *Client) SearchAnime(keyword string) ([]model.VodItem, error) {
	u, err := url.Parse(c.BaseURL + "/api.php/provide/vod/")
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}
	q := u.Query()
	q.Set("ac", "detail")
	q.Set("wd", keyword)
	u.RawQuery = q.Encode()

	var resp model.VodListResponse
	if err := c.doGet(u.String(), &resp); err != nil {
		return nil, err
	}
	if resp.Code != 1 {
		return nil, fmt.Errorf("vod API error: %s", resp.Msg)
	}
	return resp.List, nil
}

// GetAnimeDetail 按 vod_id 获取单个动漫详情（检查更新用）。
func (c *Client) GetAnimeDetail(vodID int) (*model.VodItem, error) {
	u, err := url.Parse(c.BaseURL + "/api.php/provide/vod/")
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}
	q := u.Query()
	q.Set("ac", "detail")
	q.Set("ids", strconv.Itoa(vodID))
	u.RawQuery = q.Encode()

	var resp model.VodListResponse
	if err := c.doGet(u.String(), &resp); err != nil {
		return nil, err
	}
	if resp.Code != 1 {
		return nil, fmt.Errorf("vod API error: %s", resp.Msg)
	}
	if len(resp.List) == 0 {
		return nil, errors.New("vod detail not found")
	}
	return &resp.List[0], nil
}

// DetailURL 按 vod_id 构造详情页地址（用于通知中的可点击链接）。
func (c *Client) DetailURL(vodID int) string {
	out := strings.ReplaceAll(c.DetailURLTmpl, "{base}", c.BaseURL)
	out = strings.ReplaceAll(out, "{id}", strconv.Itoa(vodID))
	return out
}

func (c *Client) doGet(rawURL string, target interface{}) error {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json, text/plain, */*")

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
