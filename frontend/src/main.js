
import './style.css'

const app = document.querySelector('#app')

const state = {
  theme: localStorage.getItem('sbtun-theme') || 'dark',
  running: false,
  statusState: 'stopped',
  statusMessage: '',
  config: null,
  selectedNode: '',
  manualProtocol: 'vless',
  manualForm: { url: '', name: '', server: '', port: '', password: '', username: '', method: '', network: '', server_name: '', flow: '', alter_id: '', security: '', packet_encoding: '', transport_type: '', transport_path: '', transport_host: '', transport_service_name: '', plugin: '', plugin_opts: '', version: '5', up_mbps: '', down_mbps: '', obfs_type: '', obfs_password: '', insecure: false, tls: false },
  customRuleForm: { match_type: 'domain_suffix', value: '', action: 'proxy' },
  customRuleImport: '',
  rules: null,
  ruleUpdating: '',
  allRulesUpdating: false,
  nodeSwitching: false,
  nodeHealth: {},
  traffic: { up: 0, down: 0 },
  view: 'overview',
}

const MODES = [
  { id: 'smart', name: '智能分流', desc: '国内直连，其余自动代理' },
  { id: 'global', name: '全局代理', desc: '所有非本机流量走代理' },
  { id: 'direct', name: '全局直连', desc: '所有流量直接连接' },
  { id: 'custom', name: '自定义', desc: '按用户规则精细控制' },
]

const NODE_FIELDS = {
  vless: [
    { key: 'password', label: 'UUID', type: 'password', placeholder: 'VLESS 用户 UUID', required: true },
    { key: 'flow', label: 'Flow', type: 'select', options: [['', '无'], ['xtls-rprx-vision', 'xtls-rprx-vision']] },
    { key: 'network', label: '网络', type: 'select', options: [['', 'TCP + UDP'], ['tcp', 'TCP'], ['udp', 'UDP']] },
    { key: 'tls', label: 'TLS', type: 'checkbox' },
    { key: 'server_name', label: 'TLS SNI', placeholder: '可选，默认服务器地址' },
    { key: 'transport_type', label: '传输', type: 'select', options: [['', '无'], ['ws', 'WebSocket'], ['http', 'HTTP'], ['grpc', 'gRPC'], ['httpupgrade', 'HTTPUpgrade']] },
  ],
  vmess: [
    { key: 'password', label: 'UUID', type: 'password', placeholder: 'VMess 用户 UUID', required: true },
    { key: 'security', label: '加密', type: 'select', options: [['auto', 'auto'], ['none', 'none'], ['zero', 'zero'], ['aes-128-gcm', 'aes-128-gcm'], ['chacha20-poly1305', 'chacha20-poly1305']] },
    { key: 'alter_id', label: 'Alter ID', type: 'number', placeholder: '默认 0' },
    { key: 'network', label: '网络', type: 'select', options: [['', 'TCP + UDP'], ['tcp', 'TCP'], ['udp', 'UDP']] },
    { key: 'tls', label: 'TLS', type: 'checkbox' },
    { key: 'server_name', label: 'TLS SNI', placeholder: '可选，默认服务器地址' },
    { key: 'transport_type', label: '传输', type: 'select', options: [['', '无'], ['ws', 'WebSocket'], ['http', 'HTTP'], ['grpc', 'gRPC'], ['httpupgrade', 'HTTPUpgrade']] },
  ],
  trojan: [
    { key: 'password', label: '密码', type: 'password', placeholder: 'Trojan 密码', required: true },
    { key: 'network', label: '网络', type: 'select', options: [['', 'TCP + UDP'], ['tcp', 'TCP'], ['udp', 'UDP']] },
    { key: 'tls', label: 'TLS', type: 'checkbox' },
    { key: 'server_name', label: 'TLS SNI', placeholder: '可选，默认服务器地址' },
    { key: 'transport_type', label: '传输', type: 'select', options: [['', '无'], ['ws', 'WebSocket'], ['http', 'HTTP'], ['grpc', 'gRPC'], ['httpupgrade', 'HTTPUpgrade']] },
  ],
  shadowsocks: [
    { key: 'method', label: '加密方式', placeholder: '例如 aes-128-gcm', required: true },
    { key: 'password', label: '密码', type: 'password', placeholder: 'Shadowsocks 密码', required: true },
    { key: 'network', label: '网络', type: 'select', options: [['', 'TCP + UDP'], ['tcp', 'TCP'], ['udp', 'UDP']] },
    { key: 'plugin', label: '插件', placeholder: '可选：obfs-local / v2ray-plugin' },
    { key: 'plugin_opts', label: '插件参数', placeholder: '可选' },
  ],
  socks: [
    { key: 'version', label: '版本', type: 'select', options: [['5', 'SOCKS5'], ['4', 'SOCKS4'], ['4a', 'SOCKS4a']] },
    { key: 'username', label: '用户名', placeholder: '可选' },
    { key: 'password', label: '密码', type: 'password', placeholder: '可选' },
    { key: 'network', label: '网络', type: 'select', options: [['', 'TCP + UDP'], ['tcp', 'TCP'], ['udp', 'UDP']] },
  ],
  http: [
    { key: 'username', label: '用户名', placeholder: '可选' },
    { key: 'password', label: '密码', type: 'password', placeholder: '可选' },
    { key: 'tls', label: 'TLS', type: 'checkbox' },
  ],
  hysteria2: [
    { key: 'password', label: '密码', type: 'password', placeholder: 'Hysteria2 密码', required: true },
    { key: 'server_name', label: 'TLS SNI', placeholder: '可选，默认服务器地址' },
    { key: 'up_mbps', label: '上行 Mbps', type: 'number', placeholder: '可选' },
    { key: 'down_mbps', label: '下行 Mbps', type: 'number', placeholder: '可选' },
    { key: 'obfs_type', label: 'Obfs 类型', type: 'select', options: [['', '关闭'], ['salamander', 'salamander']] },
    { key: 'obfs_password', label: 'Obfs 密码', type: 'password', placeholder: '启用 obfs 时填写' },
    { key: 'insecure', label: '跳过证书校验', type: 'checkbox' },
  ],
}

document.documentElement.dataset.theme = state.theme
app.innerHTML = render()

function render() {
  return `
    <main class="shell">
      <aside class="sidebar">
        <div class="brand"><span class="brand-mark">s</span><span>sbtun</span></div>
        <nav class="nav-list">
          ${[
            ['overview', '概览'],
            ['nodes', '节点'],
            ['routing', '路由模式'],
            ['rules', '规则集'],
          ].map(([id, label]) => `<button class="nav-item ${state.view === id ? 'active' : ''}" data-view="${id}">${label}</button>`).join('')}
        </nav>
        <div class="sidebar-status">
          <span class="dot ${dotClass(state.statusState)}"></span>
          <span>${statusLabel(state.statusState, state.statusMessage)}</span>
        </div>
      </aside>

      <section class="workspace">
        <header class="topbar">
          <div>
            <div class="eyebrow">sbtun / ${viewLabel(state.view)}</div>
            <h1>${viewTitle(state.view)}</h1>
          </div>
          <div class="topbar-actions">
            <span class="traffic-chip" id="trafficText">${formatTraffic(state.traffic)}</span>
            <button id="themeToggle" class="theme-toggle" type="button" aria-label="切换主题" title="切换主题">${state.theme === 'dark' ? '☀' : '☾'}</button>
            <button id="power" class="switch ${state.running ? 'on' : ''}">${state.running ? '关闭 TUN' : '开启 TUN'}</button>
          </div>
        </header>
        <section class="status-bar">
          <span class="dot ${dotClass(state.statusState)}"></span>
          <strong id="statusText">${statusLabel(state.statusState, state.statusMessage)}</strong>
          <span class="muted">节点、DNS、路由和 TUN 由程序自动管理。</span>
        </section>
        ${renderView()}
      </section>
    </main>
    <div id="toast" class="toast" style="display:none"></div>
  `
}

function viewLabel(view) {
  return { overview: '概览', nodes: '节点', routing: '路由模式', rules: '规则集' }[view] || '概览'
}

function viewTitle(view) {
  return { overview: '运行概览', nodes: '节点管理', routing: '路由模式', rules: '智能分流规则集' }[view] || '运行概览'
}

function renderView() {
  if (state.view === 'nodes') return renderNodesPage()
  if (state.view === 'routing') return renderRoutingPage()
  if (state.view === 'rules') return renderRulesPage()
  return renderOverview()
}

function renderOverview() {
  const current = state.config?.nodes?.find(n => n.id === state.config.current_node_id)
  return `<div class="overview-grid">
    <section class="card focus-card">
      <div class="section-kicker">当前节点</div>
      <h2>${escapeHtml(current?.name || '未选择节点')}</h2>
      <p class="muted">${current ? escapeHtml(current.server) + ':' + current.port : '请先在节点页导入或添加节点'}</p>
      ${current ? renderNodeHealth(current.id) : '<div class="empty">暂无健康检测结果</div>'}
      <button class="btn btn-primary" data-view="nodes">管理节点</button>
    </section>
    <section class="card metric-card">
      <div class="section-kicker">连接状态</div>
      <div class="metric-value"><span class="dot ${dotClass(state.statusState)}"></span>${statusLabel(state.statusState, state.statusMessage)}</div>
      <span class="muted">${formatTraffic(state.traffic)}</span>
    </section>
    <section class="card overview-wide">
      <div class="section-heading"><h2>路由模式</h2><button class="btn btn-ghost" data-view="routing">调整</button></div>
      <div class="mode-summary"><strong>${MODES.find(m => m.id === state.config?.routing_mode)?.name || '未设置'}</strong><span class="muted">${MODES.find(m => m.id === state.config?.routing_mode)?.desc || ''}</span></div>
    </section>
  </div>`
}

function renderNodesPage() {
  return `<section class="page-stack">
      <div class="section-heading"><div><h2>节点列表</h2><p class="muted">选择可用节点，或运行三项健康测试。</p></div><button class="btn btn-primary" data-scroll="node-import">导入节点</button></div>
    <div id="nodePanel">${renderNodes()}</div>
    <div class="card node-import" id="node-import"><h2>添加节点</h2>
      <div class="row"><input id="nodeUrl" class="input" value="${escapeHtml(state.manualForm.url)}" placeholder="节点链接 vmess:// vless:// ss:// 或订阅" /><button id="importBtn" class="btn btn-primary">导入</button></div>
      <div class="row"><select id="nodeProtocol" class="select"><option value="vless" ${state.manualProtocol === 'vless' ? 'selected' : ''}>VLESS</option><option value="vmess" ${state.manualProtocol === 'vmess' ? 'selected' : ''}>VMess</option><option value="trojan" ${state.manualProtocol === 'trojan' ? 'selected' : ''}>Trojan</option><option value="shadowsocks" ${state.manualProtocol === 'shadowsocks' ? 'selected' : ''}>Shadowsocks</option><option value="socks" ${state.manualProtocol === 'socks' ? 'selected' : ''}>SOCKS</option><option value="http" ${state.manualProtocol === 'http' ? 'selected' : ''}>HTTP</option><option value="hysteria2" ${state.manualProtocol === 'hysteria2' ? 'selected' : ''}>Hysteria2</option></select></div>
      <div class="row"><input id="nodeName" class="input" value="${escapeHtml(state.manualForm.name)}" placeholder="节点名称" /></div>
      <div class="row"><input id="nodeServer" class="input" value="${escapeHtml(state.manualForm.server)}" placeholder="服务器地址" /><input id="nodePort" class="input port-input" value="${escapeHtml(state.manualForm.port)}" placeholder="端口" /></div>
      ${renderManualAdvancedFields()}
      <button id="addNodeBtn" class="btn btn-ghost">手动添加节点</button>
    </div>
  </section>`
}

function renderManualAdvancedFields() {
  const fields = NODE_FIELDS[state.manualProtocol] || []
  return `<div class="advanced-fields"><div class="field-hint">${state.manualProtocol.toUpperCase()} 专用参数</div>${fields.map(renderNodeField).join('')}${renderTransportFields()}</div>`
}

function renderTransportFields() {
  const type = state.manualForm.transport_type
  if (!type || state.manualProtocol === 'hysteria2') return ''
  if (type === 'grpc') return `<div class="transport-fields"><div class="field-hint">gRPC 参数</div>${renderSettingInput('transport_service_name', 'Service Name', '可选')}</div>`
  if (type === 'ws' || type === 'http' || type === 'httpupgrade') return `<div class="transport-fields"><div class="field-hint">${type === 'ws' ? 'WebSocket' : type === 'httpupgrade' ? 'HTTPUpgrade' : 'HTTP'} 参数</div>${renderSettingInput('transport_host', 'Host', '可选')}${renderSettingInput('transport_path', 'Path', '例如 /ws')}</div>`
  return ''
}

function renderSettingInput(key, label, placeholder) {
  return `<label class="setting-field"><span>${label}</span><input data-setting="${key}" class="input" value="${escapeHtml(state.manualForm[key] || '')}" placeholder="${placeholder}" /></label>`
}

function renderNodeField(field) {
  const value = state.manualForm[field.key] ?? ''
  if (field.type === 'checkbox') {
    return `<label class="check-field"><input data-setting="${field.key}" type="checkbox" ${value ? 'checked' : ''} /> ${field.label}</label>`
  }
  if (field.type === 'select') {
    return `<label class="setting-field"><span>${field.label}</span><select data-setting="${field.key}" class="select">${field.options.map(([key, label]) => `<option value="${key}" ${value === key ? 'selected' : ''}>${label}</option>`).join('')}</select></label>`
  }
  return `<label class="setting-field"><span>${field.label}</span><input data-setting="${field.key}" class="input" type="${field.type || 'text'}" value="${escapeHtml(value)}" placeholder="${field.placeholder || ''}" ${field.required ? 'required' : ''} /></label>`
}

function buildManualSettings(protocol, password) {
  const form = state.manualForm
  const settings = {}
  for (const field of NODE_FIELDS[protocol] || []) {
    const value = form[field.key]
    if (field.type === 'checkbox') {
      if (value) settings[field.key] = 'true'
    } else if (String(value || '').trim()) {
      settings[field.key] = String(value).trim()
    }
  }
  if (protocol === 'vless' || protocol === 'vmess') {
    settings.uuid = settings.password || password
    delete settings.password
  }
  if (protocol === 'hysteria2' && settings.server_name) {
    settings.sni = settings.server_name
    delete settings.server_name
  }
  return settings
}

function renderRoutingPage() {
  return `<div class="page-stack"><section class="card"><h2>选择路由模式</h2><div class="mode-grid">${MODES.map(m => `<button class="mode-btn ${state.config?.routing_mode === m.id ? 'active' : ''}" data-mode="${m.id}">${m.name}<span class="mode-desc">${m.desc}</span></button>`).join('')}</div></section>
    ${state.config?.routing_mode === 'custom' ? `<section class="card"><h2>自定义分流规则</h2><div id="customRulesPanel">${renderCustomRules()}</div><div class="row"><select id="ruleMatchType" class="select"><option value="domain_suffix">域名后缀</option><option value="domain_keyword">域名关键词</option><option value="domain">完整域名</option><option value="ip_cidr">IP 网段</option><option value="port">端口</option></select><input id="ruleValue" class="input" value="${escapeHtml(state.customRuleForm.value)}" placeholder="例如 example.com 或 443" /><select id="ruleAction" class="select"><option value="proxy">代理</option><option value="direct">直连</option><option value="block">阻断</option></select><button id="addRuleBtn" class="btn btn-primary">添加规则</button></div><div class="row"><textarea id="ruleImport" class="input" rows="3" placeholder="粘贴规则链接、文件路径或文本；每行：proxy,domain_suffix,example.com"></textarea><button id="importRuleBtn" class="btn btn-ghost">导入规则</button></div></section>` : ''}
  </div>`
}

function renderRulesPage() {
  return `<div class="page-stack"><section class="card"><div id="rulesPanel">${renderRules()}</div><button id="updateAllRules" class="btn btn-primary">${state.allRulesUpdating ? '更新中...' : '更新全部规则集'}</button></section></div>`
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
    const next = await window.go.app.App.GetConfig()
    const changed = JSON.stringify(state.config) !== JSON.stringify(next)
    state.config = next
    if (changed) renderApp()
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
  document.documentElement.dataset.theme = state.theme
  app.innerHTML = render()
  bindEvents()
}

function bindEvents() {
  const themeToggle = document.querySelector('#themeToggle')
  if (themeToggle) {
    themeToggle.addEventListener('click', () => {
      state.theme = state.theme === 'dark' ? 'light' : 'dark'
      localStorage.setItem('sbtun-theme', state.theme)
      renderApp()
    })
  }
  document.querySelectorAll('[data-view]').forEach(item => {
    item.addEventListener('click', () => {
      state.view = item.dataset.view
      renderApp()
    })
  })
  document.querySelectorAll('[data-scroll]').forEach(item => {
    item.addEventListener('click', () => document.querySelector('#' + item.dataset.scroll)?.scrollIntoView({ behavior: 'smooth' }))
  })
  for (const id of ['nodeUrl', 'nodeName', 'nodeServer', 'nodePort']) {
    const input = document.querySelector('#' + id)
    if (input) {
      input.addEventListener('input', () => {
        const key = id === 'nodeUrl' ? 'url' : id === 'nodeName' ? 'name' : id === 'nodeServer' ? 'server' : 'port'
        state.manualForm[key] = input.value
      })
      if (input.type === 'checkbox') {
        input.addEventListener('change', () => { state.manualForm[id === 'nodeTLS' ? 'tls' : 'insecure'] = input.checked })
      }
    }
  }
  for (const id of ['nodeTLS', 'nodeInsecure']) {
    const input = document.querySelector('#' + id)
    if (input) input.addEventListener('change', () => { state.manualForm[id === 'nodeTLS' ? 'tls' : 'insecure'] = input.checked })
  }
  document.querySelectorAll('[data-setting]').forEach(input => {
    const update = () => {
      state.manualForm[input.dataset.setting] = input.type === 'checkbox' ? input.checked : input.value
      if (input.dataset.setting === 'transport_type' && input.type === 'select-one') renderApp()
    }
    input.addEventListener('input', update)
    input.addEventListener('change', update)
  })
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
      renderApp()
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
          settings: buildManualSettings(protocol, password),
        })
        await refreshConfig()
    await refreshRules()
        showToast('节点已添加', 'success')
        state.manualForm.name = ''
        state.manualForm.server = ''
        state.manualForm.port = ''
        for (const key of Object.keys(state.manualForm)) state.manualForm[key] = typeof state.manualForm[key] === 'boolean' ? false : ''
        for (const id of ['nodeName', 'nodeServer', 'nodePort']) {
          const input = document.querySelector('#' + id)
          if (input) input.value = ''
        }
        renderApp()
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
    setInterval(refreshConfig, 500)
  } else {
    showToast('Wails 运行时未加载', 'error')
    renderApp()
  }
})
