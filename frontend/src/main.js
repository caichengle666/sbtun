import './style.css'

const app = document.querySelector('#app')
app.innerHTML = `
  <main class="shell">
    <section class="card hero">
      <div>
        <div class="eyebrow">sbtun · 轻量级 TUN</div>
        <h1>网络代理，一键开启</h1>
        <p>节点、DNS、路由和 TUN 将由程序自动管理。</p>
      </div>
      <button id="tun" class="switch">开启 TUN</button>
    </section>
    <section class="grid">
      <div class="card">
        <h2>当前节点</h2>
        <div class="empty">尚未导入节点</div>
      </div>
      <div class="card">
        <h2>分流模式</h2>
        <div class="mode">智能分流</div>
        <small>国内直连，其余流量自动代理</small>
      </div>
    </section>
    <section class="card status">
      <span class="dot"></span>
      <span id="status">TUN 未运行</span>
    </section>
  </main>
`

document.querySelector('#tun').addEventListener('click', (event) => {
  const enabled = event.currentTarget.classList.toggle('active')
  event.currentTarget.textContent = enabled ? '关闭 TUN' : '开启 TUN'
  document.querySelector('#status').textContent = enabled ? '正在准备 TUN…' : 'TUN 未运行'
})
