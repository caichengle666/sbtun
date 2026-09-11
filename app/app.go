package app

import "context"

// App 是 Wails 与核心控制层之间的最小桥接对象。
// 后续 TUN、节点、订阅和运行状态 API 都从这里暴露给前端。
type App struct {
	ctx context.Context
}

func New() *App { return &App{} }

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) Ping() string {
	return "sbtun 运行正常"
}
