// 安卓 WebView 里模拟 Wails 的 window.go / window.runtime，调用经 SbtunNative 转给 Go。
// 必须在页面主脚本之前加载（CI 把它插到 index.html 的 <head> 最前面）。
(() => {
  const pending = new Map()
  // 编号带本次页面加载的随机前缀：页面重载后，旧页面迟到的结果不会投给新页面的同号请求。
  const page = Math.random().toString(36).slice(2)
  let seq = 0
  const TIMEOUT = 5 * 60 * 1000 // 后端卡住时不让 Promise 永远挂着；扫码等用户操作不限时

  const native = (kind, name, args) => new Promise((resolve, reject) => {
    const id = `${page}:${++seq}`
    const timer = name === 'ScanQRCode' ? 0 : setTimeout(() => {
      pending.delete(id)
      reject(new Error(`调用超时：${name}`))
    }, TIMEOUT)
    pending.set(id, { resolve, reject, timer })
    SbtunNative.call(id, kind, name, JSON.stringify(args))
  })

  window.__sbtunResolve = (id, json) => {
    const p = pending.get(id)
    if (!p) return
    pending.delete(id)
    clearTimeout(p.timer)
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
