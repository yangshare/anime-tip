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

func (c *Client) doGet(rawURL string, target interface{}) error {
	req, err := http.NewRequest("GET", rawURL, nil)
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
