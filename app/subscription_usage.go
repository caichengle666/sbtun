package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/caichengle666/sbtun/config"
)

// parseSubscriptionUsage 解析机场通用的 subscription-userinfo 响应头：
// "upload=123; download=456; total=789; expire=1700000000"。没有流量总量也没有到期时间时返回 nil。
func parseSubscriptionUsage(header string) *config.SubscriptionUsage {
	var u config.SubscriptionUsage
	for _, part := range strings.Split(header, ";") {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		// 个别面板输出小数或科学计数法，按浮点解析再取整。
		f, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil || f < 0 {
			continue
		}
		n := int64(f)
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "upload":
			u.Upload = n
		case "download":
			u.Download = n
		case "total":
			u.Total = n
		case "expire":
			u.Expire = n
		}
	}
	if u.Total == 0 && u.Expire == 0 {
		return nil
	}
	return &u
}

// SubscriptionUsageText 把流量信息写成一行，如“剩余 85.3 GB / 共 100 GB · 2026-12-31 到期”。
// warn 为 true 表示已过期、7 天内到期或流量剩余不足 10%，界面会醒目显示。
func SubscriptionUsageText(u *config.SubscriptionUsage, now time.Time) (text string, warn bool) {
	if u == nil {
		return "", false
	}
	var parts []string
	if u.Total > 0 {
		left := u.Total - u.Upload - u.Download
		if left <= 0 {
			parts = append(parts, "流量已用完")
			warn = true
		} else {
			parts = append(parts, fmt.Sprintf("剩余 %s / 共 %s", formatBytes(left), formatBytes(u.Total)))
			warn = left*10 < u.Total
		}
	}
	if u.Expire > 0 {
		expire := time.Unix(u.Expire, 0)
		switch left := expire.Sub(now); {
		case left <= 0:
			parts = append(parts, "已过期")
			warn = true
		case left < 7*24*time.Hour:
			parts = append(parts, fmt.Sprintf("%d 天后到期", int(left.Hours()/24)+1))
			warn = true
		default:
			parts = append(parts, expire.Local().Format("2006-01-02")+" 到期")
		}
	}
	return strings.Join(parts, " · "), warn
}

func formatBytes(n int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	f, i := float64(n), 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	if i == 0 || f >= 100 {
		return fmt.Sprintf("%.0f %s", f, units[i])
	}
	return fmt.Sprintf("%.1f %s", f, units[i])
}
