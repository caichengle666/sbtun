package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/caichengle666/sbtun/core/rules"
)

// 规则集的下载都放在 operationMu 外（每个最长 30 秒），锁内只做安装、保存和重载，
// 不卡住切换节点、自动切换和退出。直连下载失败时和订阅一样经已有节点下载（安卓上 App 自身不走 VPN）。

var errNoRulesManager = errors.New("规则管理器未初始化")

// baseContext 是 App 的生命周期上下文，退出时取消；未初始化（测试）时用 Background。
func (a *App) baseContext() context.Context {
	if a != nil && a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

type fetchedRuleSet struct {
	data []byte
	err  error
}

func (f *proxiedFetcher) fetchRule(rawURL string) ([]byte, error) {
	var data []byte
	err := f.do(f.app.baseContext(), func(client *http.Client) error {
		var err error
		data, err = rules.Fetch(f.app.baseContext(), client, rawURL)
		return err
	})
	return data, err
}

// fetchRuleURLs 并发下载（最多 4 个），结果顺序与 urls 一致。
func (a *App) fetchRuleURLs(urls []string) []fetchedRuleSet {
	fetcher := &proxiedFetcher{app: a, timeout: rules.FetchTimeout}
	defer fetcher.close()
	out := make([]fetchedRuleSet, len(urls))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, rawURL := range urls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			out[i].data, out[i].err = fetcher.fetchRule(rawURL)
		}()
	}
	wg.Wait()
	return out
}

func (a *App) fetchRule(rawURL string) ([]byte, error) {
	got := a.fetchRuleURLs([]string{strings.TrimSpace(rawURL)})[0]
	return got.data, got.err
}

// ListRules 返回规则集列表；规则集清单读不出（权限、损坏）时返回错误，不把默认列表当成用户的列表。
func (a *App) ListRules() ([]rules.RuleInfo, error) {
	if a.rulesManager == nil {
		return nil, errNoRulesManager
	}
	return a.rulesManager.List()
}

func (a *App) AddRuleSet(name, url, action string) error {
	if a.rulesManager == nil {
		return errNoRulesManager
	}
	if err := a.rulesManager.ValidateInput(name, url, ""); err != nil {
		return err
	}
	data, err := a.fetchRule(url)
	if err != nil {
		return fmt.Errorf("添加规则集失败: %w", err)
	}
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	if _, err := a.rulesManager.Add(name, url, action, data, a.validateRuleSet); err != nil {
		return err
	}
	return a.reloadIfRunningLocked()
}

func (a *App) EditRuleSet(id, name, url, action string) error {
	if a.rulesManager == nil {
		return errNoRulesManager
	}
	set, err := a.rulesManager.Get(id)
	if err != nil {
		return err
	}
	if err := a.rulesManager.ValidateInput(name, url, id); err != nil {
		return err
	}
	var data []byte
	if set.NeedsDownload(url) {
		if data, err = a.fetchRule(url); err != nil {
			return fmt.Errorf("更新规则集失败: %w", err)
		}
	}
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	if err := a.rulesManager.Edit(id, name, url, action, data, a.validateRuleSet); err != nil {
		return err
	}
	return a.reloadIfRunningLocked()
}

// SetRuleSetAction 修改用户规则集命中后的去向（屏蔽、代理、直连或节点/节点组等）。
func (a *App) SetRuleSetAction(id, action string) error {
	return a.changeRuleSets(func(m *rules.Manager) error { return m.SetAction(id, action) })
}

func (a *App) SetRuleEnabled(id string, enabled bool) error {
	return a.changeRuleSets(func(m *rules.Manager) error { return m.SetEnabled(id, enabled) })
}

func (a *App) DeleteRuleSet(id string) error {
	return a.changeRuleSets(func(m *rules.Manager) error { return m.Delete(id) })
}

func (a *App) changeRuleSets(change func(*rules.Manager) error) error {
	if a.rulesManager == nil {
		return errNoRulesManager
	}
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	if err := change(a.rulesManager); err != nil {
		return err
	}
	return a.reloadIfRunningLocked()
}

func (a *App) RestoreDefaultRuleSets() error {
	if a.rulesManager == nil {
		return errNoRulesManager
	}
	failures, err := a.refreshRuleSets(a.rulesManager.Defaults(), a.rulesManager.RestoreDefaults)
	if err != nil {
		return err
	}
	if len(failures) > 0 {
		return fmt.Errorf("默认规则已恢复，但部分规则下载失败: %s", strings.Join(failures, "; "))
	}
	return nil
}

func (a *App) UpdateRule(id string) error {
	if a.rulesManager == nil {
		return errNoRulesManager
	}
	set, err := a.rulesManager.Get(id)
	if err != nil {
		return err
	}
	data, err := a.fetchRule(set.URL)
	if err != nil {
		return err
	}
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	if err := a.rulesManager.Install(id, data, a.validateRuleSet); err != nil {
		return err
	}
	return a.reloadIfRunningLocked()
}

func (a *App) UpdateAllRules() []error {
	if a.rulesManager == nil {
		return []error{errNoRulesManager}
	}
	sets, err := a.rulesManager.Sets()
	if err != nil {
		return []error{err}
	}
	enabled := sets[:0]
	for _, set := range sets {
		if set.Enabled {
			enabled = append(enabled, set)
		}
	}
	failures, _ := a.refreshRuleSets(enabled, nil)
	var errs []error
	for _, failure := range failures {
		errs = append(errs, errors.New(failure))
	}
	return errs
}

// refreshRuleSets 在锁外下载 sets；锁内先执行 before（可为 nil，失败则什么都不装），再逐个安装并重载一次。
// 返回 “名称: 原因” 形式的失败列表（重载失败也算一条）。
func (a *App) refreshRuleSets(sets []rules.RuleSet, before func() error) ([]string, error) {
	urls := make([]string, len(sets))
	for i, set := range sets {
		urls[i] = set.URL
	}
	fetched := a.fetchRuleURLs(urls)
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	if before != nil {
		if err := before(); err != nil {
			return nil, err
		}
	}
	var failures []string
	for i, set := range sets {
		err := fetched[i].err
		if err == nil {
			err = a.rulesManager.Install(set.ID, fetched[i].data, a.validateRuleSet)
		}
		if err != nil {
			failures = append(failures, set.Name+": "+err.Error())
		}
	}
	if err := a.reloadIfRunningLocked(); err != nil {
		failures = append(failures, err.Error())
	}
	return failures, nil
}
