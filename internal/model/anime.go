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
