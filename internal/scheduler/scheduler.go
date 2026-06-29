package scheduler

import (
	"database/sql"
	"log"

	"github.com/robfig/cron/v3"
	"github.com/user/anime-tip/internal/crawler"
	"github.com/user/anime-tip/internal/notify"
	"github.com/user/anime-tip/internal/store"
)

type Scheduler struct {
	db      *sql.DB
	crawler *crawler.Client
	cron    *cron.Cron
}

func New(db *sql.DB, crawler *crawler.Client) *Scheduler {
	return &Scheduler{
		db:      db,
		crawler: crawler,
		cron:    cron.New(),
	}
}

func (s *Scheduler) Start(cronExpr string) {
	s.cron.AddFunc(cronExpr, func() {
		log.Println("[scheduler] 开始检查动漫更新...")
		if err := s.CheckUpdates(); err != nil {
			log.Printf("[scheduler] 检查更新失败: %v", err)
		}
	})
	s.cron.Start()
	log.Printf("[scheduler] 定时任务已启动: %s", cronExpr)
}

func (s *Scheduler) Stop() {
	s.cron.Stop()
}

// CheckUpdates 检查所有关注动漫的更新，推送变化
func (s *Scheduler) CheckUpdates() error {
	animes, err := store.ListAnimes(s.db)
	if err != nil {
		return err
	}
	if len(animes) == 0 {
		log.Println("[scheduler] 关注列表为空，跳过检查")
		return nil
	}

	var updates []notify.UpdateItem
	var toUpdate []struct {
		id                  int64
		currentRemarks      string
		lastNotifiedRemarks string
	}

	for _, a := range animes {
		detail, err := s.crawler.GetAnimeDetail(a.VodID)
		if err != nil {
			log.Printf("[scheduler] 获取动漫 %s (vod_id=%d) 详情失败: %v", a.Name, a.VodID, err)
			continue
		}

		latestRemarks := detail.Info.VodRemarks

		// 当前记录的 remarks 与上次推送的 remarks 不同，说明有更新
		if a.LastNotifiedRemarks != "" && latestRemarks != a.LastNotifiedRemarks {
			updates = append(updates, notify.UpdateItem{
				Name:      a.Name,
				Remarks:   latestRemarks,
				DetailURL: s.crawler.BaseURL + detail.Info.VodLink,
			})
		}

		// 无论是否有变化，都更新 current_remarks；如果上次推送的 remarks 为空（首次关注），不触发通知
		if latestRemarks != a.CurrentRemarks || a.LastNotifiedRemarks != a.CurrentRemarks {
			toUpdate = append(toUpdate, struct {
				id                  int64
				currentRemarks      string
				lastNotifiedRemarks string
			}{a.ID, latestRemarks, a.CurrentRemarks})
		}
	}

	// 如果有更新，推送通知
	if len(updates) > 0 {
		sendKey, _ := store.GetSetting(s.db, "server_chan_key")
		sc := notify.NewServerChan(sendKey)
		if err := sc.Send(updates); err != nil {
			return err
		}
		log.Printf("[scheduler] 推送了 %d 条动漫更新", len(updates))
	}

	// 推送成功后更新数据库
	for _, u := range toUpdate {
		if err := store.UpdateAnimeRemarks(s.db, u.id, u.currentRemarks, u.lastNotifiedRemarks); err != nil {
			log.Printf("[scheduler] 更新动漫 remarks 失败 (id=%d): %v", u.id, err)
		}
	}

	return nil
}
