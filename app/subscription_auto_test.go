package app

import (
	"testing"
	"time"

	"github.com/caichengle666/sbtun/config"
)

func TestSubscriptionUpdateHours(t *testing.T) {
	if got := SubscriptionUpdateHours(config.Config{}); got != 24 {
		t.Fatalf("默认应为 24 小时，得到 %d", got)
	}
	if got := SubscriptionUpdateHours(config.Config{SubscriptionUpdateHours: 6}); got != 6 {
		t.Fatalf("得到 %d", got)
	}
	if got := SubscriptionUpdateHours(config.Config{SubscriptionAutoUpdateDisabled: true, SubscriptionUpdateHours: 6}); got != 0 {
		t.Fatalf("关闭时应为 0，得到 %d", got)
	}
}

func TestDueSubscriptions(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) string { return now.Add(-d).Format(time.RFC3339) }
	subs := []config.Subscription{
		{ID: "fresh", UpdatedAt: at(time.Hour)},
		{ID: "old", UpdatedAt: at(25 * time.Hour)},
		{ID: "never"},
		{ID: "failed", UpdatedAt: at(48 * time.Hour)},
		{ID: "retry", UpdatedAt: at(48 * time.Hour)},
	}
	failed := map[string]time.Time{"failed": now.Add(-10 * time.Minute), "retry": now.Add(-2 * time.Hour)}
	var ids []string
	for _, s := range dueSubscriptions(subs, 24*time.Hour, failed, now) {
		ids = append(ids, s.ID)
	}
	want := []string{"old", "never", "retry"}
	if len(ids) != len(want) {
		t.Fatalf("得到 %v，期望 %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("得到 %v，期望 %v", ids, want)
		}
	}
}
