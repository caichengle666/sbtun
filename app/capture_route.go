package app

import (
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/caichengle666/sbtun/config"
	"github.com/caichengle666/sbtun/core/capture"
)

func annotateCaptureRoutes(flows []capture.Flow, cfg config.Config) {
	for i := range flows {
		flows[i].Route = captureRoute(flows[i], cfg)
	}
}

func captureRoute(flow capture.Flow, cfg config.Config) string {
	if flow.Error != "" {
		return "failed"
	}
	host := strings.ToLower(strings.TrimSpace(flow.Host))
	if ip := net.ParseIP(host); ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()) {
		return "direct"
	}
	switch cfg.RoutingMode {
	case config.RoutingDirect:
		return "direct"
	case config.RoutingGlobal:
		return "proxy"
	case config.RoutingCustom:
		for _, rule := range cfg.CustomRules {
			if captureRuleMatches(rule, flow, host) {
				return strings.ToLower(strings.TrimSpace(rule.Action))
			}
		}
		return "proxy"
	default:
		return "smart"
	}
}

func captureRuleMatches(rule config.Rule, flow capture.Flow, host string) bool {
	value := strings.ToLower(strings.TrimSpace(rule.Value))
	if value == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(rule.MatchType)) {
	case "domain":
		return host == value
	case "domain_suffix":
		return host == value || strings.HasSuffix(host, "."+value)
	case "domain_keyword":
		return strings.Contains(host, value)
	case "port":
		parsed, err := url.Parse(flow.URL)
		if err != nil {
			return false
		}
		port := parsed.Port()
		if port == "" {
			if parsed.Scheme == "https" {
				port = "443"
			} else if parsed.Scheme == "http" {
				port = "80"
			}
		}
		_, err = strconv.Atoi(value)
		return err == nil && port == value
	case "ip_cidr":
		ip := net.ParseIP(host)
		_, network, err := net.ParseCIDR(value)
		return err == nil && ip != nil && network.Contains(ip)
	default:
		return false
	}
}
