package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/caichengle666/sbtun/config"
)

func (a *App) ImportCustomRules(source string) (int, error) {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	data, err := loadRuleSource(strings.TrimSpace(source))
	if err != nil {
		return 0, err
	}
	rules, err := parseCustomRules(data)
	if err != nil {
		return 0, err
	}
	err = a.mutateConfigAndSyncLocked(func(_ config.Config, next *config.Config) error {
		next.CustomRules = append(next.CustomRules, rules...)
		return nil
	}, true)
	if err != nil {
		return 0, err
	}
	return len(rules), nil
}

func loadRuleSource(source string) ([]byte, error) {
	if source == "" {
		return nil, fmt.Errorf("规则内容或链接不能为空")
	}
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		resp, err := (&http.Client{Timeout: 30 * time.Second}).Get(source)
		if err != nil {
			return nil, fmt.Errorf("下载规则失败: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("规则 HTTP %d", resp.StatusCode)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		return body, nil
	}
	if data, err := os.ReadFile(source); err == nil {
		return data, nil
	}
	return []byte(source), nil
}

func parseCustomRules(data []byte) ([]config.Rule, error) {
	text := strings.TrimSpace(string(data))
	var rules []config.Rule
	if strings.HasPrefix(text, "[") {
		if err := json.Unmarshal([]byte(text), &rules); err != nil {
			return nil, fmt.Errorf("JSON 规则格式错误: %w", err)
		}
	} else {
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
				continue
			}
			parts := strings.Split(line, ",")
			if len(parts) < 3 {
				parts = strings.Fields(line)
			}
			if len(parts) < 3 {
				continue
			}
			first, second, third := strings.ToLower(strings.TrimSpace(parts[0])), strings.ToLower(strings.TrimSpace(parts[1])), strings.TrimSpace(strings.Join(parts[2:], ","))
			if first == "proxy" || first == "direct" || first == "block" {
				rules = append(rules, config.Rule{Action: first, MatchType: second, Value: third})
			} else {
				rules = append(rules, config.Rule{MatchType: first, Value: strings.TrimSpace(parts[1]), Action: third})
			}
		}
	}
	for _, rule := range rules {
		if err := validateImportedRule(rule); err != nil {
			return nil, err
		}
	}
	if len(rules) == 0 {
		return nil, fmt.Errorf("未识别到有效规则")
	}
	return rules, nil
}

func validateImportedRule(rule config.Rule) error {
	validMatch := map[string]bool{"domain_suffix": true, "domain_keyword": true, "domain": true, "ip_cidr": true, "port": true}
	validAction := map[string]bool{"proxy": true, "direct": true, "block": true}
	if !validMatch[strings.ToLower(rule.MatchType)] || strings.TrimSpace(rule.Value) == "" || !validAction[strings.ToLower(rule.Action)] {
		return fmt.Errorf("无效规则: %s,%s,%s", rule.Action, rule.MatchType, rule.Value)
	}
	return nil
}
