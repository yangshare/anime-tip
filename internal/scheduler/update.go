package scheduler

import "github.com/user/anime-tip/internal/episode"

// DecideUpdate 依据本轮 remarks 与库中基线，判定是否推送并给出应写入的新基线。
//   latest       = 本轮抓到的 vod_remarks
//   baseRemarks  = 库中 last_notified_remarks（上次推送时的 remarks）
//   baseEpisode  = 库中 last_notified_episode（上次推送时解析出的集数）
// 返回：
//   shouldNotify = 是否触发推送
//   newRemarks   = 应写入的 last_notified_remarks
//   newEpisode   = 应写入的 last_notified_episode
//
// 判定规则见规格第 6 节：
//   - 能解析集数：仅当 newEp > baseEpisode（严格前进）才推送并推进基线；相等/回退不推、基线不变。
//   - 不能解析集数：退化为字符串比较，变化即推送；基线 remarks 推进、episode 保持。
//   - 首次关注（基线空）：不推送，只建基线。
func DecideUpdate(latest, baseRemarks string, baseEpisode int) (shouldNotify bool, newRemarks string, newEpisode int) {
	newEp, ok := episode.ParseEpisode(latest)
	firstTime := baseRemarks == "" && baseEpisode == 0

	// 首次关注：建基线、不推送
	if firstTime {
		newRemarks = latest
		if ok {
			newEpisode = newEp
		}
		return false, newRemarks, newEpisode
	}

	if ok {
		// 情况 A：能解析集数，严格单调递增才推送
		if baseEpisode > 0 && newEp > baseEpisode {
			return true, latest, newEp
		}
		// 相等 / 回退 / baseEpisode==0 但非首次：不推，基线保持
		return false, baseRemarks, baseEpisode
	}

	// 情况 B：解析不出集数，退化为字符串比较
	if latest != baseRemarks {
		return true, latest, baseEpisode
	}
	return false, baseRemarks, baseEpisode
}
