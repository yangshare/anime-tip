// Package episode 从苹果 CMS vod_remarks 文本中归一化解析集数。
//
// vod_remarks 形态多样（"更新至第10集"/"第10话"/"10"/"完结"…），
// 直接字符串比较会导致写法差异误报更新。本包把它解析成 (集数, 是否成功)。
package episode

import (
	"regexp"
	"strconv"
)

// epSuffixRe 优先匹配紧邻「集/话」后缀的数字，规避把年份（如 2024年第10集）
// 误当集数。匹配组 1 为集数。
var epSuffixRe = regexp.MustCompile(`(\d+)\s*[集话]`)

// firstNumRe 退化匹配首个连续数字，兜底 "10" 这种纯数字写法。
var firstNumRe = regexp.MustCompile(`\d+`)

// ParseEpisode 从 remarks 提取当前集数。
// 返回 (episode, ok)：ok=false 表示无法解析出数值集数（如 "完结"/"HD"/空串）。
func ParseEpisode(remarks string) (int, bool) {
	if m := epSuffixRe.FindStringSubmatch(remarks); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n, true
		}
	}
	if m := firstNumRe.FindString(remarks); m != "" {
		if n, err := strconv.Atoi(m); err == nil {
			return n, true
		}
	}
	return 0, false
}
