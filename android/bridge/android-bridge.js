// 安卓 WebView 里模拟 Wails 的 window.go / window.runtime，调用经 SbtunNative 转给 Go。
// 必须在页面主脚本之前加载（CI 把它插到 index.html 的 <head> 最前面）。
(() => {
  const pending = new Map()
  let seq = 0

  const native = (kind, name, args) => new Promise((resolve, reject) => {
    const id = ++seq
    pending.set(id, { resolve, reject })
    SbtunNative.call(id, kind, name, JSON.stringify(args))
  })

  window.__sbtunResolve = (id, json) => {
    const p = pending.get(id)
    if (!p) return
    pending.delete(id)
    let out
    try {
      out = JSON.parse(json)
    } catch {
      return p.reject(new Error('返回数据无法解析'))
    }
    if (out.error) p.reject(new Error(out.error))
    else p.resolve(out.result)
  }

  const App = new Proxy({}, {
    get: (_, name) => typeof name === 'string' && name !== 'then'
      ? (...args) => native('app', name, args)
      : undefined,
  })
  const noop = () => {}

  window.go = { app: { App } }
  window.runtime = {
    Environment: () => Promise.resolve({ platform: 'android' }),
    ClipboardSetText: text => native('runtime', 'ClipboardSetText', [String(text)]).then(() => true),
    ScanQRCode: () => native('runtime', 'ScanQRCode', []),
    BrowserOpenURL: url => { native('runtime', 'BrowserOpenURL', [String(url)]).catch(noop) },
    WindowIsMaximised: () => Promise.resolve(false),
    WindowMinimise: noop,
    WindowToggleMaximise: noop,
    WindowHide: noop,
    WindowShow: noop,
    Quit: noop,
    EventsOn: () => noop,
    EventsOff: noop,
  }
  document.documentElement.dataset.platform = 'android'
})()
