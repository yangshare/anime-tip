package model

import (
	"strconv"
	"strings"
	"time"
)

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

// FlexInt 兼容苹果 CMS provide/vod 接口中数字字段既可能是字符串也可能是数字的情况
// （如 "vod_id":"28802"）。反序列化接受 string/number/float；
// 序列化统一输出为数字，保证前端按数字比较（===）与数据库 int 一致。
type FlexInt int

func (f FlexInt) Int() int { return int(f) }

func (f FlexInt) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Itoa(int(f))), nil
}

func (f *FlexInt) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(string(data))
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	if s == "" {
		*f = 0
		return nil
	}
	// 优先按整数解析；少数源给浮点（如 28802.0）时先转 float 再截断。
	if n, err := strconv.Atoi(s); err == nil {
		*f = FlexInt(n)
		return nil
	}
	if fv, err := strconv.ParseFloat(s, 64); err == nil {
		*f = FlexInt(int(fv))
		return nil
	}
	*f = 0
	return nil
}

// VodListResponse 苹果 CMS provide/vod 标准采集接口响应
type VodListResponse struct {
	Code int       `json:"code"`
	Msg  string    `json:"msg"`
	Total int      `json:"total"`
	List []VodItem `json:"list"`
}

// VodItem 视频项（标准 MacCMS vod 字段）
type VodItem struct {
	VodID      FlexInt `json:"vod_id"`
	VodName    string  `json:"vod_name"`
	VodPic     string  `json:"vod_pic"`
	VodRemarks string  `json:"vod_remarks"`
	VodArea    string  `json:"vod_area"`
	VodYear    string  `json:"vod_year"`
	VodClass   string  `json:"vod_class"`
	VodScore   string  `json:"vod_score"`
	TypeID     FlexInt `json:"type_id"`
}
