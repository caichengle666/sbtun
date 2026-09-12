
import './style.css'

const app = document.querySelector('#app')

const state = {
  running: false,
  statusState: 'stopped',
  statusMessage: '',
  config: null,
  selectedNode: '',
  manualProtocol: 'vless',
  manualForm: { url: '', name: '', server: '', port: '', password: '' },
  customRuleForm: { match_type: 'domain_suffix', value: '', action: 'proxy' },
  customRuleImport: '',
  rules: null,
  ruleUpdating: '',
  allRulesUpdating: false,
  nodeSwitching: false,
  nodeHealth: {},
  traffic: { up: 0, down: 0 },
}

const MODES = [
  { id: 'smart', name: '智能分流', desc: '国内直连，其余自动代理' },
  { id: 'global', name: '全局代理', desc: '所有非本机流量走代理' },
  { id: 'direct', name: '全局直连', desc: '所有流量直接连接' },
  { id: 'custom', name: '自定义', desc: '按用户规则精细控制' },
]

app.innerHTML = render()

function render() {
  return `
    <main class="shell">
      <section class="card hero">
        <div>
          <div class="eyebrow">sbtun</div>
          <h1>网络代理，一键开启</h1>
          <p class="muted">节点、DNS、路由和 TUN 由程序自动管理。</p>
        </div>
        <button id="power" class="switch ${state.running ? 'on' : ''}">${state.running ? '关闭 TUN' : '开启 TUN'}</button>
      </section>

      <section class="card status">
        <span class="dot ${dotClass(state.statusState)}"></span>
        <span id="statusText">${statusLabel(state.statusState, state.statusMessage)}</span>
        <span id="trafficText" class="muted">${formatTraffic(state.traffic)}</span>
      </section>

      <section class="grid">
        <div class="card">
          <h2>节点</h2>
          <div id="nodePanel">${renderNodes()}</div>
          <div style="margin-top:12px;border-top:1px solid #25284a;padding-top:12px">
            <div class="row">
              <input id="nodeUrl" class="input" value="${escapeHtml(state.manualForm.url)}" placeholder="节点链接 vmess:// vless:// ss:// 或订阅" />
              <button id="importBtn" class="btn btn-primary">导入</button>
            </div>
            <div class="row">
              <input id="nodeName" class="input" value="${escapeHtml(state.manualForm.name)}" placeholder="节点名称" style="flex:1" />
            </div>
            <div class="row">
              <input id="nodeServer" class="input" value="${escapeHtml(state.manualForm.server)}" placeholder="服务器地址" />
              <input id="nodePort" class="input" value="${escapeHtml(state.manualForm.port)}" placeholder="端口" style="max-width:80px" />
            </div>
            <div class="row">
              <select id="nodeProtocol" class="select">
                <option value="vless" ${state.manualProtocol === 'vless' ? 'selected' : ''}>VLESS</option>
                <option value="vmess" ${state.manualProtocol === 'vmess' ? 'selected' : ''}>VMess</option>
                <option value="trojan" ${state.manualProtocol === 'trojan' ? 'selected' : ''}>Trojan</option>
                <option value="shadowsocks" ${state.manualProtocol === 'shadowsocks' ? 'selected' : ''}>Shadowsocks</option>
                <option value="socks" ${state.manualProtocol === 'socks' ? 'selected' : ''}>SOCKS</option>
                <option value="http" ${state.manualProtocol === 'http' ? 'selected' : ''}>HTTP</option>
                <option value="hysteria2" ${state.manualProtocol === 'hysteria2' ? 'selected' : ''}>Hysteria2</option>
              </select>
              <input id="nodePassword" class="input" value="${escapeHtml(state.manualForm.password)}" placeholder="UUID/密码" />
            </div>
            <div class="row">
              <button id="addNodeBtn" class="btn btn-ghost">手动添加节点</button>
            </div>
          </div>
        </div>

        <div class="card">
          <h2>路由模式</h2>
          <div class="mode-grid">
            ${MODES.map(m => `
              <button class="mode-btn ${state.config?.routing_mode === m.id ? 'active' : ''}" data-mode="${m.id}">
                ${m.name}
                <span class="mode-desc">${m.desc}</span>
              </button>
            `).join('')}
          </div>
        </div>
      </section>

      ${state.config?.routing_mode === 'custom' ? `
      <section class="card">
        <h2>自定义分流规则</h2>
        <div id="customRulesPanel">${renderCustomRules()}</div>
        <div class="row">
          <select id="ruleMatchType" class="select">
            <option value="domain_suffix">域名后缀</option>
            <option value="domain_keyword">域名关键词</option>
            <option value="domain">完整域名</option>
            <option value="ip_cidr">IP 网段</option>
            <option value="port">端口</option>
          </select>
          <input id="ruleValue" class="input" value="${escapeHtml(state.customRuleForm.value)}" placeholder="例如 example.com 或 443" />
          <select id="ruleAction" class="select">
            <option value="proxy">代理</option>
            <option value="direct">直连</option>
            <option value="block">阻断</option>
          </select>
          <button id="addRuleBtn" class="btn btn-primary">添加规则</button>
        </div>
        <div class="row">
          <textarea id="ruleImport" class="input" rows="3" placeholder="粘贴规则链接、文件路径或文本；每行：proxy,domain_suffix,example.com"></textarea>
          <button id="importRuleBtn" class="btn btn-ghost">导入规则</button>
        </div>
      </section>
      ` : ''}

      <section class="card">
        <h2>智能分流规则集</h2>
        <div id="rulesPanel">${renderRules()}</div>
        <div style="margin-top:12px">
          <button id="updateAllRules" class="btn btn-primary">更新全部规则集</button>
        </div>
      </section>
    </main>
    <div id="toast" class="toast" style="display:none"></div>
  `
}

function dotClass(s) {
  if (s === 'running') return 'running'
  if (s === 'error') return 'error'
  if (s === 'starting' || s === 'stopping') return 'starting'
  return ''
}

function statusLabel(s, msg) {
  const labels = { stopped: '已停止', starting: '正在启动...', running: '运行中', stopping: '正在停止...', error: '错误' }
  if (s === 'error' && msg) return '错误: ' + msg
  return labels[s] || s
}

function formatTraffic(t) {
  const fmt = n => {
    n = Number(n) || 0
    if (n < 1024) return n + ' B'
    if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB'
    if (n < 1024 * 1024 * 1024) return (n / 1024 / 1024).toFixed(1) + ' MB'
    return (n / 1024 / 1024 / 1024).toFixed(2) + ' GB'
  }
  return '实时 ↑ ' + fmt(t?.up) + '/s  ↓ ' + fmt(t?.down) + '/s'
}

function renderRules() {
  if (!state.rules?.length) {
    return '<div class="empty">正在加载规则集信息...</div>'
  }
  const downloaded = state.rules.filter(r => r.exists).length
  return state.rules.map(r => `
    <div class="node-item rule-item">
      <span class="badge ${r.exists ? 'proxy' : ''}">${r.exists ? '已下载' : '未下载'}</span>
      <span style="flex:1">
        <span class="node-name">${r.name}</span>
        <span class="node-server">${r.exists ? formatRuleInfo(r) : '点击更新下载'}</span>
      </span>
      <span class="node-actions">
        <button class="btn btn-ghost update-rule" data-id="${r.id}" ${state.ruleUpdating || state.allRulesUpdating ? 'disabled' : ''}>${state.ruleUpdating === r.id ? '更新中...' : '更新'}</button>
      </span>
    </div>
  `).join('') + `<div class="rules-summary">已准备 ${downloaded}/${state.rules.length} 个规则集 · 智能分流会使用全部规则集</div>`
}

function formatRuleInfo(rule) {
  const size = Number(rule.size || 0).toLocaleString('zh-CN') + ' bytes'
  const updated = rule.updated_at ? new Date(rule.updated_at).toLocaleString('zh-CN') : '时间未知'
  const sha = rule.sha256 ? ' · SHA256 ' + rule.sha256 : ''
  return size + ' · ' + updated + sha
}

function renderNodes() {
  if (!state.config?.nodes?.length) {
    return '<div class="empty">尚未导入节点</div>'
  }
  return `<ul class="node-list">${state.config.nodes.map(n => `
    <li class="node-item ${state.config.current_node_id === n.id ? 'active' : ''}">
      <span class="badge ${n.id.startsWith('direct') ? 'direct' : 'proxy'}">${n.protocol}</span>
      <span style="flex:1">
        <span class="node-name">${escapeHtml(n.name)}</span>
        <span class="node-server">${escapeHtml(n.server)}:${n.port}</span>
        ${renderNodeHealth(n.id)}
      </span>
      <span class="node-actions">
        <button class="btn btn-ghost select-node" data-id="${n.id}" ${state.nodeSwitching ? 'disabled' : ''}>${state.config.current_node_id === n.id ? '当前' : '选择'}</button>
        <button class="btn btn-ghost test-node" data-id="${n.id}">测试</button>
        <button class="btn btn-danger rm-node" data-id="${n.id}">删除</button>
      </span>
    </li>
  `).join('')}</ul>`
}

function renderNodeHealth(id) {
  const health = state.nodeHealth[id]
  if (!health) return ''
  const check = (label, result) => {
    const suffix = result?.latency_ms ? ` · ${result.latency_ms} ms` : ''
    return `<span class="health-check ${result?.ok ? 'ok' : 'bad'}">${label}: ${escapeHtml(result?.message || '未测试')}${suffix}</span>`
  }
  return `<span class="node-health">
    ${check('Ping', health.ping)}
    ${check(health.transport === 'udp' ? 'UDP' : 'TCPing', health.port)}
    ${check('URL', health.url)}
  </span>`
}

function escapeHtml(s) {
  return String(s ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))
}

function showToast(msg, type = '') {
  const t = document.querySelector('#toast')
  t.textContent = msg
  t.className = 'toast ' + type
  t.style.display = 'block'
  clearTimeout(t._timer)
  t._timer = setTimeout(() => { t.style.display = 'none' }, 3000)
}

async function refreshStatus() {
  try {
    const s = await window.go.app.App.GetStatus()
    state.running = s.running
    state.statusState = s.state
    state.statusMessage = s.message
    state.traffic = { up: s.upload_bytes || 0, down: s.download_bytes || 0 }
    updateStatusView()
  } catch (e) {
    showToast('获取状态失败: ' + e.message, 'error')
  }
}

function renderCustomRules() {
  const rules = state.config?.custom_rules || []
  if (!rules.length) return '<div class="empty">暂无自定义规则</div>'
  const labels = { domain_suffix: '域名后缀', domain_keyword: '域名关键词', domain: '完整域名', ip_cidr: 'IP 网段', port: '端口' }
  const actions = { proxy: '代理', direct: '直连', block: '阻断' }
  return rules.map((rule, index) => `
    <div class="node-item rule-item">
      <span class="badge">${labels[rule.match_type] || rule.match_type}</span>
      <span style="flex:1"><span class="node-name">${escapeHtml(rule.value)}</span><span class="node-server">${actions[rule.action] || rule.action}</span></span>
      <button class="btn btn-danger remove-custom-rule" data-index="${index}">删除</button>
    </div>
  `).join('')
}

function updateStatusView() {
  const statusText = document.querySelector('#statusText')
  const trafficText = document.querySelector('#trafficText')
  const power = document.querySelector('#power')
  const dot = document.querySelector('.status .dot')
  if (!statusText || !trafficText || !power || !dot) {
    renderApp()
    return
  }
  statusText.textContent = statusLabel(state.statusState, state.statusMessage)
  trafficText.textContent = formatTraffic(state.traffic)
  power.textContent = state.running ? '关闭 TUN' : '开启 TUN'
  power.className = 'switch ' + (state.running ? 'on' : '')
  dot.className = 'dot ' + dotClass(state.statusState)
}

async function refreshConfig() {
  try {
    state.config = await window.go.app.App.GetConfig()
    renderApp()
  } catch (e) {
    showToast('获取配置失败: ' + e.message, 'error')
  }
}

async function refreshRules() {
  try {
    state.rules = await window.go.app.App.ListRules()
    renderApp()
  } catch (e) {
    console.error('list rules failed:', e)
  }
}

function renderApp() {
  app.innerHTML = render()
  bindEvents()
}

function bindEvents() {
  for (const id of ['nodeUrl', 'nodeName', 'nodeServer', 'nodePort', 'nodePassword']) {
    const input = document.querySelector('#' + id)
    if (input) {
      input.addEventListener('input', () => {
        const key = id === 'nodeUrl' ? 'url' : id === 'nodeName' ? 'name' : id === 'nodeServer' ? 'server' : id === 'nodePort' ? 'port' : 'password'
        state.manualForm[key] = input.value
      })
    }
  }
  const ruleValue = document.querySelector('#ruleValue')
  if (ruleValue) {
    ruleValue.addEventListener('input', () => { state.customRuleForm.value = ruleValue.value })
  }
  const ruleMatchType = document.querySelector('#ruleMatchType')
  if (ruleMatchType) {
    ruleMatchType.value = state.customRuleForm.match_type
    ruleMatchType.addEventListener('change', () => { state.customRuleForm.match_type = ruleMatchType.value })
  }
  const ruleAction = document.querySelector('#ruleAction')
  if (ruleAction) {
    ruleAction.value = state.customRuleForm.action
    ruleAction.addEventListener('change', () => { state.customRuleForm.action = ruleAction.value })
  }
  const addRuleBtn = document.querySelector('#addRuleBtn')
  if (addRuleBtn) {
    addRuleBtn.addEventListener('click', async () => {
      const value = state.customRuleForm.value.trim()
      if (!value) { showToast('请输入规则值', 'error'); return }
      const cfg = JSON.parse(JSON.stringify(state.config))
      cfg.custom_rules = cfg.custom_rules || []
      cfg.custom_rules.push({ match_type: state.customRuleForm.match_type, value, action: state.customRuleForm.action })
      try {
        await window.go.app.App.SaveConfig(cfg)
        state.config = cfg
        state.customRuleForm.value = ''
        renderApp()
        showToast('规则已添加', 'success')
      } catch (e) { showToast(e.message || String(e), 'error') }
    })
  }
  const ruleImport = document.querySelector('#ruleImport')
  if (ruleImport) {
    ruleImport.value = state.customRuleImport
    ruleImport.addEventListener('input', () => { state.customRuleImport = ruleImport.value })
  }
  const importRuleBtn = document.querySelector('#importRuleBtn')
  if (importRuleBtn) {
    importRuleBtn.addEventListener('click', async () => {
      const source = state.customRuleImport.trim()
      if (!source) { showToast('请输入规则链接、文件路径或文本', 'error'); return }
      importRuleBtn.disabled = true
      try {
        const count = await window.go.app.App.ImportCustomRules(source)
        state.customRuleImport = ''
        await refreshConfig()
        showToast('已导入 ' + count + ' 条规则', 'success')
      } catch (e) { showToast(e.message || String(e), 'error') }
      finally { importRuleBtn.disabled = false }
    })
  }
  document.querySelectorAll('.remove-custom-rule').forEach(btn => {
    btn.addEventListener('click', async () => {
      const cfg = JSON.parse(JSON.stringify(state.config))
      cfg.custom_rules.splice(Number(btn.dataset.index), 1)
      try {
        await window.go.app.App.SaveConfig(cfg)
        state.config = cfg
        renderApp()
        showToast('规则已删除', 'success')
      } catch (e) { showToast(e.message || String(e), 'error') }
    })
  })
  const power = document.querySelector('#power')
  if (power) {
    power.addEventListener('click', async () => {
      power.disabled = true
      try {
        if (state.running) {
          await window.go.app.App.Stop()
        } else {
          await window.go.app.App.Start()
        }
        await refreshStatus()
      } catch (e) {
        showToast(e.message || String(e), 'error')
        await refreshStatus()
      } finally {
        power.disabled = false
      }
    })
  }

  document.querySelectorAll('.mode-btn').forEach(btn => {
    btn.addEventListener('click', async () => {
      const mode = btn.dataset.mode
      try {
        await window.go.app.App.SetRoutingMode(mode)
        await refreshConfig()
    await refreshRules()
        showToast('路由模式已切换', 'success')
      } catch (e) {
        showToast(e.message, 'error')
      }
    })
  })

  document.querySelectorAll('.select-node').forEach(btn => {
    btn.addEventListener('click', async () => {
      if (state.nodeSwitching) return
      state.nodeSwitching = true
      document.querySelectorAll('.select-node').forEach(item => { item.disabled = true })
      try {
        await window.go.app.App.SelectNode(btn.dataset.id)
        await refreshConfig()
        await refreshRules()
        showToast('节点已选择', 'success')
      } catch (e) {
        showToast(e.message, 'error')
      } finally {
        state.nodeSwitching = false
        document.querySelectorAll('.select-node').forEach(item => { item.disabled = false })
      }
    })
  })

  document.querySelectorAll('.rm-node').forEach(btn => {
    btn.addEventListener('click', async () => {
      try {
        await window.go.app.App.RemoveNode(btn.dataset.id)
        await refreshConfig()
    await refreshRules()
        showToast('节点已删除', 'success')
      } catch (e) {
        showToast(e.message, 'error')
      }
    })
  })

  document.querySelectorAll('.test-node').forEach(btn => {
    btn.addEventListener('click', async () => {
      btn.disabled = true
      try {
        const result = await window.go.app.App.TestNode(btn.dataset.id)
        state.nodeHealth[btn.dataset.id] = result
        renderApp()
        showToast(result.message, result.healthy ? 'success' : 'error')
      } catch (e) {
        showToast(e.message || String(e), 'error')
      }
    })
  })

  document.querySelectorAll('.update-rule').forEach(btn => {
    btn.addEventListener('click', async () => {
      state.ruleUpdating = btn.dataset.id
      renderApp()
      try {
        await window.go.app.App.UpdateRule(btn.dataset.id)
        state.ruleUpdating = ''
        await refreshRules()
        showToast('规则集更新成功', 'success')
      } catch (e) {
        state.ruleUpdating = ''
        showToast(e.message || String(e), 'error')
        await refreshRules()
      } finally {
        state.ruleUpdating = ''
      }
    })
  })

  const updateAllRules = document.querySelector('#updateAllRules')
  if (updateAllRules) {
    updateAllRules.disabled = Boolean(state.ruleUpdating || state.allRulesUpdating)
    updateAllRules.textContent = state.allRulesUpdating ? '更新中...' : '更新全部规则集'
    updateAllRules.addEventListener('click', async () => {
      state.allRulesUpdating = true
      renderApp()
      try {
        const errors = await window.go.app.App.UpdateAllRules()
        state.allRulesUpdating = false
        await refreshRules()
        if (errors?.length) {
          showToast('部分规则集更新失败: ' + errors.join('; '), 'error')
        } else {
          showToast('全部规则集更新成功', 'success')
        }
      } catch (e) {
        state.allRulesUpdating = false
        showToast(e.message || String(e), 'error')
        await refreshRules()
      } finally {
        state.allRulesUpdating = false
      }
    })
  }

  const importBtn = document.querySelector('#importBtn')
  if (importBtn) {
    importBtn.addEventListener('click', async () => {
      const url = state.manualForm.url.trim()
      if (!url) { showToast('请输入节点或订阅链接', 'error'); return }
      importBtn.disabled = true
      try {
        const added = await window.go.app.App.ImportSubscription(url)
        await refreshConfig()
    await refreshRules()
        showToast('导入成功，新增 ' + added + ' 个节点', 'success')
        state.manualForm.url = ''
        const urlInput = document.querySelector('#nodeUrl')
        if (urlInput) urlInput.value = ''
      } catch (e) {
        showToast(e.message, 'error')
      } finally {
        importBtn.disabled = false
      }
    })
  }

  const addNodeBtn = document.querySelector('#addNodeBtn')
  const nodeProtocol = document.querySelector('#nodeProtocol')
  if (nodeProtocol) {
    nodeProtocol.addEventListener('change', () => {
      state.manualProtocol = nodeProtocol.value
    })
  }
  if (addNodeBtn) {
    addNodeBtn.addEventListener('click', async () => {
      const name = state.manualForm.name.trim()
      const server = state.manualForm.server.trim()
      const port = parseInt(state.manualForm.port.trim(), 10)
      const protocol = document.querySelector('#nodeProtocol').value
      const password = state.manualForm.password.trim()
      if (!name || !server || !port) {
        showToast('请填写名称、服务器和端口', 'error'); return
      }
      try {
        await window.go.app.App.AddNode({
          id: 'manual-' + Date.now(),
          name, server, port, protocol,
          settings: password ? { uuid: password, password } : {},
        })
        await refreshConfig()
    await refreshRules()
        showToast('节点已添加', 'success')
        state.manualForm.name = ''
        state.manualForm.server = ''
        state.manualForm.port = ''
        state.manualForm.password = ''
        for (const id of ['nodeName', 'nodeServer', 'nodePort', 'nodePassword']) {
          const input = document.querySelector('#' + id)
          if (input) input.value = ''
        }
      } catch (e) {
        showToast(e.message, 'error')
      }
    })
  }
}

// Wails runtime ready
window.addEventListener('DOMContentLoaded', async () => {
  if (window.go?.app?.App) {
    await refreshStatus()
    await refreshConfig()
    await refreshRules()
    setInterval(refreshStatus, 2000)
  } else {
    showToast('Wails 运行时未加载', 'error')
    renderApp()
  }
})
