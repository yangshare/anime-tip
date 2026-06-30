package scheduler

import "testing"

func TestDecideUpdate(t *testing.T) {
	cases := []struct {
		name        string
		latest      string
		baseRemarks string
		baseEpisode int
		wantNotify  bool
		wantRemarks string
		wantEpisode int
	}{
		// 集数前进 → 推送，基线推进
		{"前进", "更新至第13集", "更新至第12集", 12, true, "更新至第13集", 13},
		// 集数相等、写法变 → 不推，基线保持（不能推进到新写法，否则下一轮误报）
		{"相等写法变", "第12集", "更新至12集", 12, false, "更新至12集", 12},
		// 集数回退 → 不推，基线保持
		{"回退", "更新至第11集", "更新至第12集", 12, false, "更新至第12集", 12},
		// 解析不出、字符串变化 → 推送（退化为字符串比较）
		{"变完结", "完结", "更新至第12集", 12, true, "完结", 12},
		// 解析不出、字符串不变 → 不推
		{"完结不变", "完结", "完结", 12, false, "完结", 12},
		// 首次关注（基线空）→ 不推，建基线
		{"首次关注数字", "更新至第5集", "", 0, false, "更新至第5集", 5},
		{"首次关注完结", "完结", "", 0, false, "完结", 0},
		// 情况 B 后回到数字集数且前进 → 推送（baseEpisode 仍是完结前的 12）
		{"完结后更新", "更新至第13集", "完结", 12, true, "更新至第13集", 13},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotNotify, gotRemarks, gotEpisode := DecideUpdate(c.latest, c.baseRemarks, c.baseEpisode)
			if gotNotify != c.wantNotify {
				t.Errorf("notify = %v, want %v", gotNotify, c.wantNotify)
			}
			if gotRemarks != c.wantRemarks {
				t.Errorf("remarks = %q, want %q", gotRemarks, c.wantRemarks)
			}
			if gotEpisode != c.wantEpisode {
				t.Errorf("episode = %d, want %d", gotEpisode, c.wantEpisode)
			}
		})
	}
}
