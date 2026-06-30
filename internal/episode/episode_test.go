package episode

import "testing"

func TestParseEpisode(t *testing.T) {
	cases := []struct {
		remarks string
		wantEp  int
		wantOk  bool
	}{
		{"更新至第10集", 10, true},
		{"更新至10集", 10, true},
		{"第10集", 10, true},
		{"10集", 10, true},
		{"更新至第10话", 10, true},
		{"第10话", 10, true},
		{"更新到10", 10, true},
		{"10", 10, true},
		// 含年份时不能误取 2024，要取紧邻 集/话 后缀的数字
		{"2024年第10集", 10, true},
		{"2024年第10话", 10, true},
		// 无数字文本
		{"完结", 0, false},
		{"正片", 0, false},
		{"预告", 0, false},
		{"HD", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		gotEp, gotOk := ParseEpisode(c.remarks)
		if gotEp != c.wantEp || gotOk != c.wantOk {
			t.Errorf("ParseEpisode(%q) = (%d, %v), want (%d, %v)",
				c.remarks, gotEp, gotOk, c.wantEp, c.wantOk)
		}
	}
}
