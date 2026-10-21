package app

import (
	"fmt"
	"time"

	"github.com/caichengle666/sbtun/config"
)

const (
	defaultSubscriptionUpdateHours = 24
	subscriptionCheckInterval      = 10 * time.Minute
	subscriptionRetryDelay         = time.Hour // 下载失败后隔一小时再试，不反复请求
)

// SubscriptionUpdateHours 返回订阅自动更新间隔（小时），0 表示关闭。
func SubscriptionUpdateHours(cfg config.Config) int {
	if cfg.SubscriptionAutoUpdateDisabled {
		return 0
	}
	if cfg.SubscriptionUpdateHours > 0 {
		return cfg.SubscriptionUpdateHours
	}
	return defaultSubscriptionUpdateHours
}

// SetSubscriptionAutoUpdate 设置订阅自动更新间隔（小时），0 关闭。
func (a *App) SetSubscriptionAutoUpdate(hours int) error {
	if hours < 0 || hours > 24*30 {
		return fmt.Errorf("自动更新间隔应在 1 到 720 小时之间，0 表示关闭")
	}
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	return a.mutateConfigAndSyncLocked(func(_ config.Config, next *config.Config) error {
		next.SubscriptionAutoUpdateDisabled = hours == 0
		next.SubscriptionUpdateHours = 0
		if hours > 0 && hours != defaultSubscriptionUpdateHours {
			next.SubscriptionUpdateHours = hours
		}
		return nil
	}, false)
}

// subscriptionUpdateLoop 定时检查订阅，到期的自动重新拉取节点。
func (a *App) subscriptionUpdateLoop() {
	if a.ctx == nil {
		return
	}
	failedAt := map[string]time.Time{}
	timer := time.NewTimer(time.Minute) // 启动后稍等网络就绪再查
	defer timer.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-timer.C:
			a.autoUpdateSubscriptions(failedAt, time.Now())
			timer.Reset(subscriptionCheckInterval)
		}
	}
}

func (a *App) autoUpdateSubscriptions(failedAt map[string]time.Time, now time.Time) {
	cfg := a.GetConfig()
	hours := SubscriptionUpdateHours(cfg)
	if hours == 0 {
		return
	}
	for _, sub := range dueSubscriptions(cfg.Subscriptions, time.Duration(hours)*time.Hour, failedAt, now) {
		if _, err := a.UpdateSubscription(sub.ID); err != nil {
			failedAt[sub.ID] = now
			continue
		}
		delete(failedAt, sub.ID)
	}
}

// dueSubscriptions 挑出超过间隔没更新、且不在失败等待期内的订阅。
func dueSubscriptions(subs []config.Subscription, interval time.Duration, failedAt map[string]time.Time, now time.Time) []config.Subscription {
	var due []config.Subscription
	for _, sub := range subs {
		if t, ok := failedAt[sub.ID]; ok && now.Sub(t) < subscriptionRetryDelay {
			continue
		}
		if updated, err := time.Parse(time.RFC3339, sub.UpdatedAt); err == nil && now.Sub(updated) < interval {
			continue
		}
		due = append(due, sub)
	}
	return due
}
