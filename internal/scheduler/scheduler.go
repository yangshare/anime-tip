package scheduler

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/user/anime-tip/internal/crawler"
	"github.com/user/anime-tip/internal/notify"
	"github.com/user/anime-tip/internal/store"
)

type Scheduler struct {
	db      *sql.DB
	crawler *crawler.Client
	cron    *cron.Cron
	mu      sync.Mutex
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
	// 防止并发执行（手动触发 + 定时触发叠加、多次点击）
	if !s.mu.TryLock() {
		return fmt.Errorf("检查任务正在执行中，请稍后再试")
	}
	defer s.mu.Unlock()

	start := time.Now()
	defer func() {
		log.Printf("[scheduler] 检查结束，耗时 %s", time.Since(start))
	}()

	animes, err := store.ListAnimes(s.db)
	if err != nil {
		return err
	}
	log.Printf("[scheduler] 开始检查动漫更新，共 %d 部", len(animes))
	if len(animes) == 0 {
		log.Println("[scheduler] 关注列表为空，跳过检查")
		return nil
	}

	var updates []notify.UpdateItem
	var toUpdate []struct {
		id              int64
		currentRemarks  string
		lastRemarks     string
		lastEpisode     int
	}

	for _, a := range animes {
		detail, err := s.crawler.GetAnimeDetail(a.VodID)
		if err != nil {
			log.Printf("[scheduler] 获取动漫 %s (vod_id=%d) 详情失败: %v", a.Name, a.VodID, err)
			continue
		}

		latestRemarks := detail.VodRemarks
		shouldNotify, newRemarks, newEpisode := DecideUpdate(latestRemarks, a.LastNotifiedRemarks, a.LastNotifiedEpisode)

		if shouldNotify {
			updates = append(updates, notify.UpdateItem{
				Name:      a.Name,
				Remarks:   latestRemarks,
				DetailURL: s.crawler.DetailURL(a.VodID),
			})
		}

		// 仅当基线有变化时才收集更新（避免无谓写库）
		if newRemarks != a.LastNotifiedRemarks || newEpisode != a.LastNotifiedEpisode {
			toUpdate = append(toUpdate, struct {
				id              int64
				currentRemarks  string
				lastRemarks     string
				lastEpisode     int
			}{a.ID, latestRemarks, newRemarks, newEpisode})
		}
	}

	// 如果有更新，推送通知
	if len(updates) > 0 {
		names := make([]string, 0, len(updates))
		for _, u := range updates {
			names = append(names, "《"+u.Name+"》")
		}
		log.Printf("[scheduler] 发现 %d 部有更新：%s", len(updates), strings.Join(names, " "))

		sendKey, _ := store.GetSetting(s.db, "server_chan_key")
		sc := notify.NewServerChan(sendKey)
		if err := sc.Send(updates); err != nil {
			log.Printf("[scheduler] 推送通知失败: %v", err)
			return err
		}
		log.Printf("[scheduler] 推送了 %d 条动漫更新", len(updates))
	}

	// 推送成功后更新数据库
	for _, u := range toUpdate {
		if err := store.UpdateAnimeRemarks(s.db, u.id, u.currentRemarks, u.lastRemarks, u.lastEpisode); err != nil {
			log.Printf("[scheduler] 更新动漫 remarks 失败 (id=%d): %v", u.id, err)
		}
	}

	return nil
}
