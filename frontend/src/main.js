
import './style.css'

const app = document.querySelector('#app')
const initialTheme = localStorage.getItem('sbtun-theme') === 'light' ? 'light' : 'dark'

const state = {
  theme: initialTheme,
  running: false,
  version: '',
  singBoxVersion: '',
  singBoxUpdating: false,
  singBoxUpdateMessage: '',
  singBoxUpdateState: 'idle',
  statusState: 'stopped',
  statusMessage: '',
  runtimeNodeID: '',
  selectorSyncState: 'idle',
  selectorSyncMessage: '',
  config: null,
  selectedNode: '',
  manualProtocol: 'vless',
  editingNodeId: '',
  manualForm: { url: '', name: '', server: '', port: '', password: '', username: '', method: '', network: '', server_name: '', alpn: '', flow: '', alter_id: '', security: '', packet_encoding: '', reality_public_key: '', reality_short_id: '', utls_fingerprint: '', transport_type: '', transport_path: '', transport_host: '', transport_service_name: '', plugin: '', plugin_opts: '', version: '5', up_mbps: '', down_mbps: '', server_ports: '', hop_interval: '', path: '', obfs_type: '', obfs_password: '', disable_path_mtu_discovery: false, udp_over_tcp: false, multiplex: false, insecure: false, tls: false },
  customRuleForm: { match_type: 'domain_suffix', value: '', action: 'proxy' },
  customRuleImport: '',
  rules: null,
  ruleSetForm: { name: '', url: '' },
  editingRuleSetID: '',
  ruleFilter: 'all',
  ruleUpdating: '',
  allRulesUpdating: false,
  ruleMenuDocumentBound: false,
  dnsFilterFormOpen: false,
  nodeSwitching: false,
  nodeHealth: {},
  selectedNodes: new Set(),
  batchTesting: false,
  nodeFilter: 'all',
  traffic: { up: 0, down: 0 },
  diagnostics: null,
  ipv6Status: null,
  captureStatus: null,
  captureFlows: [],
  captureDraft: '',
  captureDraftDirty: false,
  captureEnabledDraft: false,
  captureEnabledDirty: false,
  selectedCaptureID: 0,
  selectedCaptureFlow: null,
  captureDetailTab: 'overview',
  captureSearch: '',
  captureFilter: 'all',
  captureListScroll: 0,
  captureDetailScroll: 0,
  captureBodyScroll: {},
  view: 'overview',
  statusRefreshing: false,
  configRefreshing: false,
}

const MODES = [
  { id: 'smart', name: '智能分流', desc: '国内直连，其余自动代理' },
  { id: 'global', name: '全局代理', desc: '所有非本机流量走代理' },
  { id: 'direct', name: '全局直连', desc: '所有流量直接连接' },
  { id: 'custom', name: '自定义', desc: '按用户规则精细控制' },
]

const RULE_MATCH_TYPES = [
  { id: 'domain_suffix', label: '域名后缀', hint: '匹配以该后缀结尾的域名', placeholder: '例如 example.com' },
  { id: 'domain_keyword', label: '域名关键词', hint: '匹配域名中包含的关键词', placeholder: '例如 google' },
  { id: 'domain', label: '完整域名', hint: '只匹配一个完整域名', placeholder: '例如 www.example.com' },
  { id: 'ip_cidr', label: 'IP 网段', hint: '填写 IPv4 或 IPv6 网段', placeholder: '例如 192.168.1.0/24' },
  { id: 'port', label: '端口', hint: '只填写一个端口号', placeholder: '例如 443' },
]

const RULE_ACTIONS = [
  { id: 'proxy', label: '代理', hint: '匹配流量通过当前节点' },
  { id: 'direct', label: '直连', hint: '匹配流量直接连接' },
  { id: 'block', label: '阻断', hint: '匹配流量直接拒绝' },
]

const NODE_FIELDS = {
  vless: [
    { key: 'password', label: 'UUID', type: 'password', placeholder: 'VLESS 用户 UUID', required: true },
    { key: 'flow', label: 'Flow', type: 'select', options: [['', '无'], ['xtls-rprx-vision', 'xtls-rprx-vision']] },
    { key: 'packet_encoding', label: 'Packet Encoding', placeholder: '可选：xudp' },
    { key: 'network', label: '网络', type: 'select', options: [['', 'TCP + UDP'], ['tcp', 'TCP'], ['udp', 'UDP']] },
    { key: 'tls', label: 'TLS', type: 'checkbox' },
    { key: 'server_name', label: 'TLS SNI', placeholder: '可选，默认服务器地址' },
    { key: 'alpn', label: 'TLS ALPN', placeholder: '逗号分隔，例如 h2,http/1.1' },
    { key: 'insecure', label: '跳过证书校验', type: 'checkbox' },
    { key: 'reality_public_key', label: 'Reality Public Key', placeholder: '可选' },
    { key: 'reality_short_id', label: 'Reality Short ID', placeholder: '可选' },
    { key: 'utls_fingerprint', label: 'uTLS 指纹', placeholder: '可选，例如 chrome' },
    { key: 'transport_type', label: '传输', type: 'select', options: [['', '无'], ['ws', 'WebSocket'], ['http', 'HTTP'], ['grpc', 'gRPC'], ['httpupgrade', 'HTTPUpgrade']] },
    { key: 'multiplex', label: '启用 Multiplex', type: 'checkbox' },
  ],
  vmess: [
    { key: 'password', label: 'UUID', type: 'password', placeholder: 'VMess 用户 UUID', required: true },
    { key: 'security', label: '加密', type: 'select', options: [['auto', 'auto'], ['none', 'none'], ['zero', 'zero'], ['aes-128-gcm', 'aes-128-gcm'], ['chacha20-poly1305', 'chacha20-poly1305']] },
    { key: 'alter_id', label: 'Alter ID', type: 'number', placeholder: '默认 0' },
    { key: 'packet_encoding', label: 'Packet Encoding', placeholder: '可选：xudp' },
    { key: 'network', label: '网络', type: 'select', options: [['', 'TCP + UDP'], ['tcp', 'TCP'], ['udp', 'UDP']] },
    { key: 'tls', label: 'TLS', type: 'checkbox' },
    { key: 'server_name', label: 'TLS SNI', placeholder: '可选，默认服务器地址' },
    { key: 'alpn', label: 'TLS ALPN', placeholder: '逗号分隔，例如 h2,http/1.1' },
    { key: 'insecure', label: '跳过证书校验', type: 'checkbox' },
    { key: 'utls_fingerprint', label: 'uTLS 指纹', placeholder: '可选，例如 chrome' },
    { key: 'transport_type', label: '传输', type: 'select', options: [['', '无'], ['ws', 'WebSocket'], ['http', 'HTTP'], ['grpc', 'gRPC'], ['httpupgrade', 'HTTPUpgrade']] },
    { key: 'multiplex', label: '启用 Multiplex', type: 'checkbox' },
  ],
  trojan: [
    { key: 'password', label: '密码', type: 'password', placeholder: 'Trojan 密码', required: true },
    { key: 'network', label: '网络', type: 'select', options: [['', 'TCP + UDP'], ['tcp', 'TCP'], ['udp', 'UDP']] },
    { key: 'tls', label: 'TLS', type: 'checkbox' },
    { key: 'server_name', label: 'TLS SNI', placeholder: '可选，默认服务器地址' },
    { key: 'alpn', label: 'TLS ALPN', placeholder: '逗号分隔，例如 h2,http/1.1' },
    { key: 'insecure', label: '跳过证书校验', type: 'checkbox' },
    { key: 'transport_type', label: '传输', type: 'select', options: [['', '无'], ['ws', 'WebSocket'], ['http', 'HTTP'], ['grpc', 'gRPC'], ['httpupgrade', 'HTTPUpgrade']] },
    { key: 'multiplex', label: '启用 Multiplex', type: 'checkbox' },
  ],
  shadowsocks: [
    { key: 'method', label: '加密方式', placeholder: '例如 aes-128-gcm', required: true },
    { key: 'password', label: '密码', type: 'password', placeholder: 'Shadowsocks 密码', required: true },
    { key: 'network', label: '网络', type: 'select', options: [['', 'TCP + UDP'], ['tcp', 'TCP'], ['udp', 'UDP']] },
    { key: 'plugin', label: '插件', placeholder: '可选：obfs-local / v2ray-plugin' },
    { key: 'plugin_opts', label: '插件参数', placeholder: '可选' },
    { key: 'udp_over_tcp', label: 'UDP over TCP', type: 'checkbox' },
    { key: 'multiplex', label: '启用 Multiplex', type: 'checkbox' },
  ],
  socks: [
    { key: 'version', label: '版本', type: 'select', options: [['5', 'SOCKS5'], ['4', 'SOCKS4'], ['4a', 'SOCKS4a']] },
    { key: 'username', label: '用户名', placeholder: '可选' },
    { key: 'password', label: '密码', type: 'password', placeholder: '可选' },
    { key: 'network', label: '网络', type: 'select', options: [['', 'TCP + UDP'], ['tcp', 'TCP'], ['udp', 'UDP']] },
    { key: 'udp_over_tcp', label: 'UDP over TCP', type: 'checkbox' },
  ],
  http: [
    { key: 'username', label: '用户名', placeholder: '可选' },
    { key: 'password', label: '密码', type: 'password', placeholder: '可选' },
    { key: 'tls', label: 'TLS', type: 'checkbox' },
    { key: 'server_name', label: 'TLS SNI', placeholder: '可选' },
    { key: 'alpn', label: 'TLS ALPN', placeholder: '逗号分隔，例如 h2,http/1.1' },
    { key: 'insecure', label: '跳过证书校验', type: 'checkbox' },
    { key: 'path', label: 'HTTP Path', placeholder: '可选' },
  ],
  hysteria2: [
    { key: 'password', label: '密码', type: 'password', placeholder: 'Hysteria2 密码', required: true },
    { key: 'server_name', label: 'TLS SNI', placeholder: '可选，默认服务器地址' },
    { key: 'alpn', label: 'TLS ALPN', placeholder: '逗号分隔，例如 h3' },
    { key: 'up_mbps', label: '上行 Mbps', type: 'number', placeholder: '可选' },
    { key: 'down_mbps', label: '下行 Mbps', type: 'number', placeholder: '可选' },
    { key: 'server_ports', label: '跳跃端口', placeholder: '例如 2000-3000 或 443,8443' },
    { key: 'hop_interval', label: '跳跃间隔', placeholder: '例如 30s' },
    { key: 'network', label: '网络', type: 'select', options: [['', 'TCP + UDP'], ['tcp', 'TCP'], ['udp', 'UDP']] },
    { key: 'obfs_type', label: 'Obfs 类型', type: 'select', options: [['', '关闭'], ['salamander', 'salamander']] },
    { key: 'obfs_password', label: 'Obfs 密码', type: 'password', placeholder: '启用 obfs 时填写' },
    { key: 'disable_path_mtu_discovery', label: '禁用 Path MTU Discovery', type: 'checkbox' },
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
            ['overview', '运行概览'],
            ['nodes', '节点管理'],
            ['routing', '路由模式'],
            ['rules', '分流规则'],
            ['capture', '流量分析'],
          ].map(([id, label]) => `<button class="nav-item ${state.view === id ? 'active' : ''}" data-view="${id}">${label}</button>`).join('')}
        </nav>
        <div class="sidebar-status">
          <div class="sidebar-status-line"><span class="dot ${dotClass(state.statusState)}"></span><span>${statusLabel(state.statusState, state.statusMessage)}</span></div>
          <div class="sidebar-versions">
            ${state.version ? `<span class="app-version" title="管理器版本 v${escapeHtml(state.version)}">程序 v${escapeHtml(state.version)}</span>` : ''}
            <span class="app-version" title="sing-box 内核版本 ${state.singBoxVersion ? 'v' + escapeHtml(state.singBoxVersion) : '读取中'}">内核 ${state.singBoxVersion ? 'v' + escapeHtml(state.singBoxVersion) : '--'}</span>
          </div>
        </div>
      </aside>

      <section class="workspace">
        <header class="topbar">
          <div>
            <div class="eyebrow">sbtun / ${viewLabel(state.view)}</div>
            <h1>${viewTitle(state.view)}</h1>
          </div>
          <div class="topbar-actions">
            <button id="themeToggle" class="theme-toggle" type="button" aria-label="切换主题" title="切换主题">${state.theme === 'dark' ? '☀' : '<span class="moon-icon" aria-hidden="true"></span>'}</button>
            <button id="power" class="switch ${state.running ? 'on' : ''}">${state.running ? '关闭 TUN' : '开启 TUN'}</button>
          </div>
        </header>
        <section class="status-bar">
          <span class="dot ${dotClass(state.statusState)}"></span>
          <strong id="statusText">${statusLabel(state.statusState, state.statusMessage)}</strong>
          ${state.running && state.selectorSyncState !== 'idle' ? `<span class="badge selector-sync ${state.selectorSyncState}" title="${escapeHtml(state.selectorSyncMessage)}">${selectorSyncLabel(state.selectorSyncState)}</span>` : ''}
	  <span class="muted" id="statusTraffic">${formatTraffic(state.traffic)}</span>
        </section>
        ${renderView()}
      </section>
    </main>
    <div id="toast" class="toast" style="display:none"></div>
  `
}

function viewLabel(view) {
  return { overview: '运行概览', nodes: '节点管理', routing: '路由模式', rules: '分流规则', capture: '流量分析' }[view] || '运行概览'
}

function viewTitle(view) {
  return { overview: '运行概览', nodes: '节点管理', routing: '路由模式', rules: '智能分流规则集', capture: 'HTTP 请求分析' }[view] || '运行概览'
}

function activeNodeID() {
  if (state.running && state.runtimeNodeID) return state.runtimeNodeID
  return state.config?.current_node_id || ''
}

function renderView() {
  if (state.view === 'nodes') return renderNodesPage()
  if (state.view === 'routing') return renderRoutingPage()
  if (state.view === 'rules') return renderRulesPage()
  if (state.view === 'capture') return renderCapturePage()
  return renderOverview()
}

function renderOverview() {
  const current = state.config?.nodes?.find(n => n.id === activeNodeID())
  return `<div class="overview-grid">
    <section class="card focus-card overview-wide">
      <div class="focus-card-main">
        <div class="section-kicker">当前节点</div>
        <h2>${escapeHtml(current?.name || '未选择节点')}</h2>
        <p class="muted">${current ? escapeHtml(current.server) + ':' + current.port : '请先在节点页导入或添加节点'}</p>
        ${current ? renderNodeHealth(current.id) : '<div class="empty">暂无健康检测结果</div>'}
      </div>
      <button class="btn btn-primary" data-view="nodes">管理节点</button>
    </section>
    <section class="card overview-wide">
      <div class="section-heading"><h2>路由模式</h2><button class="btn btn-ghost" data-view="routing">调整</button></div>
      <div class="mode-summary"><strong>${MODES.find(m => m.id === state.config?.routing_mode)?.name || '未设置'}</strong><span class="muted">${MODES.find(m => m.id === state.config?.routing_mode)?.desc || ''}</span></div>
    </section>
    <section class="card overview-wide">
      <div class="kernel-update-row"><div><h2>sing-box 内核</h2><p class="muted">当前版本：${state.singBoxVersion ? 'v' + escapeHtml(state.singBoxVersion) : '读取中'}</p>${state.singBoxUpdateMessage ? `<p class="kernel-update-status ${state.singBoxUpdateState}">${escapeHtml(state.singBoxUpdateMessage)}</p>` : ''}</div><button id="updateSingBox" class="btn btn-ghost" ${state.singBoxUpdating ? 'disabled' : ''}>${state.singBoxUpdating ? '正在检查更新…' : '更新内核'}</button></div>
    </section>
  </div>`
}

function renderNodesPage() {
  const nodes = state.config?.nodes || []
  const allSelected = nodes.length > 0 && nodes.every(node => state.selectedNodes.has(node.id))
  return `<section class="page-stack">
      <div class="section-heading node-page-heading"><div><h2>节点列表</h2><p class="muted">选择节点后可批量测试、导出或删除。</p></div><span class="node-batch-actions"><button class="btn btn-ghost" id="toggleSelectNodes" ${nodes.length ? '' : 'disabled'}>${allSelected ? '反选' : '全选'}</button><button class="btn btn-ghost" id="batchTestNodes" ${state.batchTesting ? 'disabled' : ''}>${state.batchTesting ? '测试中' : '批量测试'}</button><button class="btn btn-ghost" id="batchExportNodes">批量导出</button><button class="btn btn-danger" id="batchDeleteNodes">批量删除</button><button class="btn btn-primary" data-scroll="node-import">导入节点</button></span></div>
    <div id="nodePanel">${renderNodes()}</div>
    <div class="card node-import" id="node-import"><h2>${state.editingNodeId ? '编辑节点全部参数' : '添加节点'}</h2>
      <div class="row"><input id="nodeUrl" class="input" value="${escapeHtml(state.manualForm.url)}" placeholder="节点链接 vmess:// vless:// ss:// 或订阅" /><button id="importBtn" class="btn btn-primary">导入</button></div>
      <div class="row"><select id="nodeProtocol" class="select"><option value="vless" ${state.manualProtocol === 'vless' ? 'selected' : ''}>VLESS</option><option value="vmess" ${state.manualProtocol === 'vmess' ? 'selected' : ''}>VMess</option><option value="trojan" ${state.manualProtocol === 'trojan' ? 'selected' : ''}>Trojan</option><option value="shadowsocks" ${state.manualProtocol === 'shadowsocks' ? 'selected' : ''}>Shadowsocks</option><option value="socks" ${state.manualProtocol === 'socks' ? 'selected' : ''}>SOCKS</option><option value="http" ${state.manualProtocol === 'http' ? 'selected' : ''}>HTTP</option><option value="hysteria2" ${state.manualProtocol === 'hysteria2' ? 'selected' : ''}>Hysteria2</option></select></div>
      <div class="row"><input id="nodeName" class="input" value="${escapeHtml(state.manualForm.name)}" placeholder="节点名称" /></div>
      <div class="row"><input id="nodeServer" class="input" value="${escapeHtml(state.manualForm.server)}" placeholder="服务器地址" /><input id="nodePort" class="input port-input" value="${escapeHtml(state.manualForm.port)}" placeholder="端口" /></div>
      ${renderManualAdvancedFields()}
      <button id="addNodeBtn" class="btn btn-ghost">${state.editingNodeId ? '保存节点修改' : '手动添加节点'}</button>
      ${state.editingNodeId ? '<button id="cancelEditNode" class="btn btn-ghost">取消编辑</button>' : ''}
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
  const matchType = RULE_MATCH_TYPES.find(item => item.id === state.customRuleForm.match_type) || RULE_MATCH_TYPES[0]
  const action = RULE_ACTIONS.find(item => item.id === state.customRuleForm.action) || RULE_ACTIONS[0]
  const diagnostics = state.diagnostics
  return `<div class="page-stack"><section class="card"><h2>选择路由模式</h2><div class="mode-grid">${MODES.map(m => `<button class="mode-btn ${state.config?.routing_mode === m.id ? 'active' : ''}" data-mode="${m.id}">${m.name}<span class="mode-desc">${m.desc}</span></button>`).join('')}</div><label class="toggle-field"><input id="ipv6Toggle" type="checkbox" ${state.config?.ipv6_enabled ? 'checked' : ''}><span>启用 IPv6</span><small>${escapeHtml(state.ipv6Status?.message || '正在检测设备 IPv6')}</small></label><label class="toggle-field"><input id="diagnosticsToggle" type="checkbox" ${state.config?.diagnostics_enabled ? 'checked' : ''}><span>启用诊断信息</span><small>显示当前 selector 和运行节点，仅用于排查问题</small></label></section>${state.config?.diagnostics_enabled ? `<section class="card"><h2>运行诊断</h2><div class="diagnostics-grid"><span>当前配置节点</span><strong>${escapeHtml(diagnostics?.current_node_id || '未选择')}</strong><span>实际 selector</span><strong>${escapeHtml(diagnostics?.selector || '未读取')}</strong><span>状态</span><strong>${escapeHtml(diagnostics?.message || '读取中')}</strong></div></section>` : ''}
    ${state.config?.routing_mode === 'custom' ? `<section class="card"><div class="section-heading"><div><h2>自定义分流规则</h2><p class="muted">先选择匹配对象，再填写内容和处理方式。</p></div></div><div id="customRulesPanel">${renderCustomRules()}</div><div class="rule-editor"><label class="rule-field"><span>匹配对象</span><select id="ruleMatchType" class="select">${RULE_MATCH_TYPES.map(item => `<option value="${item.id}" ${item.id === matchType.id ? 'selected' : ''}>${item.label}</option>`).join('')}</select></label><label class="rule-field"><span>匹配内容</span><input id="ruleValue" class="input" value="${escapeHtml(state.customRuleForm.value)}" placeholder="${matchType.placeholder}" /></label><label class="rule-field"><span>处理方式</span><select id="ruleAction" class="select">${RULE_ACTIONS.map(item => `<option value="${item.id}" ${item.id === action.id ? 'selected' : ''}>${item.label}</option>`).join('')}</select></label><button id="addRuleBtn" class="btn btn-primary">添加规则</button><small class="rule-hint value-hint" id="ruleValueHint">${matchType.hint}</small><small class="rule-hint action-hint" id="ruleActionHint">${action.hint}</small></div><details class="rule-import" open><summary>批量导入规则</summary><div class="rule-import-body"><textarea id="ruleImport" class="input" rows="3" placeholder="每行一条，例如：proxy,domain_suffix,example.com"></textarea><button id="importRuleBtn" class="btn btn-ghost">导入</button></div></details></section>` : ''}
  </div>`
}

function renderRulesPage() {
  const filter = state.ruleFilter || 'all'
  return `<div class="page-stack rules-page">
    <section class="card rules-card">
      <div class="section-heading rules-page-heading">
        <div><h2>规则集</h2><p class="muted">管理默认规则集和用户添加的 DNS 规则集。</p></div>
        <select id="ruleSetFilter" class="select rule-filter-select" aria-label="筛选规则集">
          <option value="all" ${filter === 'all' ? 'selected' : ''}>全部规则</option>
          <option value="default" ${filter === 'default' ? 'selected' : ''}>默认规则</option>
          <option value="custom" ${filter === 'custom' ? 'selected' : ''}>用户规则</option>
        </select>
      </div>
      ${renderRuleSummary()}
      <div class="rule-layout">
        <div class="rule-list-column">
          <div id="rulesPanel">${renderRules()}</div>
          <div class="rules-toolbar"><span class="rules-summary-note">启用的规则集会参与当前路由模式，更新失败时保留旧文件。</span><button id="updateAllRules" class="btn btn-primary">${state.allRulesUpdating ? '更新中...' : '更新全部规则集'}</button></div>
        </div>
        <aside class="rule-set-form card">
          <h3>${state.editingRuleSetID ? '编辑规则集' : '添加规则集'}</h3>
          <p class="muted">添加 sing-box SRS 或 DNS 域名规则集 URL。</p>
          <label class="rule-form-field"><span>规则集名称</span><input id="ruleSetName" class="input" value="${escapeHtml(state.ruleSetForm.name)}" placeholder="例如 广告过滤规则" /></label>
          <label class="rule-form-field"><span>规则集 URL</span><input id="ruleSetURL" class="input" value="${escapeHtml(state.ruleSetForm.url)}" placeholder="https://example.com/rules.srs" /></label>
          <div class="rule-form-actions"><button id="addRuleSetBtn" class="btn btn-primary">${state.editingRuleSetID ? '保存修改' : '添加规则集'}</button>${state.editingRuleSetID ? '<button id="cancelEditRuleSet" class="btn btn-ghost">取消</button>' : '<a class="btn btn-ghost rule-source-link" href="https://github.com/razaxq/dns-blocklists-sing-box/blob/main/README_zh.md" target="_blank" rel="noopener noreferrer">查找规则集 ↗</a><button id="restoreDefaultRules" class="btn btn-ghost">恢复默认规则</button>'}</div>
        </aside>
      </div>
    </section>
    <section class="card dns-filter-card">
      <div class="section-heading"><div><h2>手动 DNS 过滤规则</h2><p class="muted">单独添加域名或 IP，命中后直接阻断。优先级高于规则集。</p></div><button id="openFilterRuleBtn" class="btn btn-primary">添加过滤规则</button></div>
      ${state.dnsFilterFormOpen ? `<div class="dns-filter-editor"><label class="rule-form-field"><span>匹配对象</span><select id="filterRuleMatchType" class="select">${RULE_MATCH_TYPES.filter(item => item.id !== 'port').map(item => `<option value="${item.id}">${item.label}</option>`).join('')}</select></label><label class="rule-form-field"><span>匹配内容</span><input id="filterRuleValue" class="input" placeholder="例如 ads.example.com" /></label><div class="rule-form-actions"><button id="addFilterRuleBtn" class="btn btn-primary">添加规则</button><button id="cancelFilterRuleBtn" class="btn btn-ghost">取消</button></div></div>` : ''}
      ${renderDNSFilterRules()}
      <div class="rules-toolbar dns-filter-toolbar"><span class="rules-summary-note">共 ${(state.config?.dns_filter_rules || []).length} 条手动过滤规则</span></div>
    </section>
  </div>`
}

function renderCapturePage() {
  const status = state.captureStatus || {}
  const enabled = state.captureEnabledDirty ? state.captureEnabledDraft : Boolean(state.config?.capture_enabled)
  const domains = state.captureDraftDirty ? state.captureDraft : (state.config?.capture_domains || []).join('\n')
  const visibleFlows = filterCaptureFlows(state.captureFlows)
  const selected = state.selectedCaptureFlow?.id === state.selectedCaptureID ? state.selectedCaptureFlow : state.captureFlows.find(flow => flow.id === state.selectedCaptureID)
  const mode = MODES.find(item => item.id === state.config?.routing_mode)?.name || '未设置'
  return `<div class="page-stack capture-page">
    <section class="card capture-status-panel">
      <div><span class="section-kicker">抓包状态</span><strong class="capture-status-title">${escapeHtml(status.message || (status.running ? '分析器运行中' : '分析器未运行'))}</strong></div>
      <div class="capture-status-metrics"><span><i class="dot ${status.running ? 'running' : ''}"></i>分析器 ${status.running ? '运行中' : '未运行'}</span><span><i class="dot ${state.running ? 'running' : dotClass(state.statusState)}"></i>TUN ${state.running ? '运行中' : '未运行'}</span><span>记录 <strong>${state.captureFlows.length}</strong>/${status.max_flows || '—'}</span><span>路由 <strong>${escapeHtml(mode)}</strong></span><span class="${status.unsaved ? 'capture-unsaved' : ''}">${status.unsaved ? '有未保存内容' : '已写入磁盘'}</span></div>
      <label class="toggle-field"><input id="captureEnabled" type="checkbox" ${enabled ? 'checked' : ''}><span>启用分析</span></label>
    </section>
    <section class="card capture-settings">
      <div class="section-heading"><div><h2>抓包规则</h2><p class="muted">每行一个域名或关键词；输入 * 抓取全部 HTTP/HTTPS 请求。</p></div><button id="saveCapture" class="btn btn-primary">保存并应用</button></div>
      <textarea id="captureDomains" class="input" rows="4" placeholder="例如 example.com、google；输入 * 抓取全部 HTTP/HTTPS">${escapeHtml(domains)}</textarea>
      <div class="capture-settings-footer"><div class="capture-cert"><span class="badge ${status.certificate_installed ? 'direct' : ''}">${status.certificate_installed ? `证书已安装（${escapeHtml(status.certificate_level || '未知级别')}）` : '证书未安装'}</span>${status.user_certificate_supported ? '<select id="captureCertLevel" class="input capture-cert-level"><option value="system">系统级</option><option value="user">当前用户</option></select>' : '<span class="muted">Linux 使用系统级证书</span>'}<button id="installCaptureCA" class="btn btn-ghost">安装系统证书</button><button id="uninstallCaptureCA" class="btn btn-ghost">卸载系统证书</button></div><span class="capture-storage" title="${escapeHtml(status.storage_path || '')}">${escapeHtml(status.storage_path || 'capture.json')}</span></div>
    </section>
    <section class="card capture-records">
      <div class="section-heading capture-records-heading"><div><h2>请求记录</h2><p class="muted">显示 ${visibleFlows.length} 条，共 ${state.captureFlows.length} 条</p></div><div class="capture-record-actions"><button id="saveCaptureFile" class="btn btn-ghost">保存记录</button><button id="clearCapture" class="btn btn-danger">删除全部</button></div></div>
      <div class="capture-filters"><input id="captureSearch" class="input" value="${escapeHtml(state.captureSearch)}" placeholder="搜索 URL、Host 或方法"><div class="capture-filter-group">${[['all', '全部'], ['error', '错误'], ['2xx', '2xx'], ['4xx5xx', '4xx/5xx']].map(([id, label]) => `<button class="filter-btn ${state.captureFilter === id ? 'active' : ''}" data-capture-filter="${id}">${label}</button>`).join('')}</div></div>
      <div class="capture-list" id="captureList">
        <div class="capture-list-head"><span>方法</span><span>Host / Path</span><span>状态</span><span>出口</span><span>耗时</span></div>
        ${visibleFlows.length ? visibleFlows.map(flow => renderCaptureRow(flow, selected)).join('') : '<div class="empty">没有符合条件的请求</div>'}
      </div>
    </section>
    <section class="card capture-detail-card">
      <div class="section-heading"><div><h2>请求详情</h2><p class="muted">${selected ? escapeHtml(selected.host) : '选择一条请求查看完整内容'}</p></div></div>
      <div class="capture-detail" id="captureDetail">${selected ? renderCaptureDetail(selected) : '<div class="empty">请选择一条请求</div>'}</div>
    </section>
  </div>`
}

function renderCaptureDetail(flow) {
  const tabs = [['overview', '概览'], ['request-headers', '请求头'], ['response-headers', '响应头'], ['request-body', '请求体'], ['response-body', '响应体']]
  let content = ''
  switch (state.captureDetailTab) {
    case 'request-headers': content = `<div class="capture-tab-content">${renderHeaders(flow.request_headers)}</div>`; break
    case 'response-headers': content = `<div class="capture-tab-content">${renderHeaders(flow.response_headers)}</div>`; break
    case 'request-body': content = `<div class="capture-tab-content"><h2>请求正文${flow.request_truncated ? '（已截断）' : ''}</h2>${renderCaptureBody(flow.id, 'request-body', flow.request_body, flow.request_encoding)}</div>`; break
    case 'response-body': content = `<div class="capture-tab-content"><h2>响应正文${flow.response_truncated ? '（已截断）' : ''}</h2>${renderCaptureBody(flow.id, 'response-body', flow.response_body, flow.response_encoding)}</div>`; break
    default: content = `<div class="capture-overview"><span>协议<strong>${escapeHtml(flow.protocol || '—')}</strong></span><span>状态<strong>${flow.status_code || '—'}</strong></span><span>出口<strong class="capture-route ${captureRouteClass(flow.route)}">${captureRouteLabel(flow.route)}</strong></span><span>耗时<strong>${flow.duration_ms || 0} ms</strong></span><span>请求<strong>${formatBytes(flow.request_bytes)}</strong></span><span>响应<strong>${formatBytes(flow.response_bytes)}</strong></span></div>${flow.error ? `<div class="capture-error">${escapeHtml(flow.error)}</div>` : ''}`
  }
  return `<div class="capture-detail-head"><span class="capture-method">${escapeHtml(flow.method)}</span><strong>${escapeHtml(flow.url)}</strong></div>
    <div class="capture-tabs">${tabs.map(([id, label]) => `<button class="capture-tab ${state.captureDetailTab === id ? 'active' : ''}" data-capture-tab="${id}">${label}</button>`).join('')}</div>${content}`
}

function renderCaptureRow(flow, selected) {
  const failed = Boolean(flow.error) || flow.route === 'failed'
  return `<button class="capture-row ${selected?.id === flow.id ? 'active' : ''} ${failed ? 'capture-row-error' : ''}" data-capture-id="${flow.id}"><span class="capture-method">${escapeHtml(flow.method)}</span><span class="capture-url" title="${escapeHtml(flow.url)}">${escapeHtml(flow.host)}${escapeHtml(capturePath(flow.url))}</span><span class="capture-status">${failed ? 'ERR' : (flow.status_code || '—')}</span><span class="capture-route ${captureRouteClass(flow.route)}">${captureRouteLabel(flow.route)}</span><span class="capture-time">${flow.duration_ms || 0} ms</span></button>`
}

function filterCaptureFlows(flows) {
  const query = state.captureSearch.trim().toLowerCase()
  return flows.filter(flow => {
    if (query && !`${flow.method} ${flow.host} ${flow.url}`.toLowerCase().includes(query)) return false
    if (state.captureFilter === 'error') return Boolean(flow.error) || flow.route === 'failed'
    if (state.captureFilter === '2xx') return flow.status_code >= 200 && flow.status_code < 300
    if (state.captureFilter === '4xx5xx') return flow.status_code >= 400 && flow.status_code < 600
    return true
  })
}

function captureRouteLabel(route) {
  return { direct: '直连', proxy: '代理', block: '阻断', failed: '失败', smart: '智能', custom: '规则' }[route] || '未知'
}

function captureRouteClass(route) {
  return ['direct', 'proxy', 'block', 'failed', 'smart'].includes(route) ? route : 'smart'
}

function renderCaptureBody(flowID, tab, body, encoding) {
  if (!body) return '<div class="empty">无</div>'
  return `<pre data-capture-body-scroll="${flowID}:${tab}">${encoding === 'base64' ? 'Base64\n' : ''}${escapeHtml(body)}</pre>`
}

function renderHeaders(headers) {
  const entries = Object.entries(headers || {}).sort(([a], [b]) => a.localeCompare(b))
  if (!entries.length) return '<div class="empty">无</div>'
  return `<dl>${entries.map(([name, values]) => `<dt>${escapeHtml(name)}</dt><dd>${escapeHtml((values || []).join('\n'))}</dd>`).join('')}</dl>`
}

function capturePath(rawURL) {
  try {
    const url = new URL(rawURL)
    return url.pathname + url.search
  } catch { return '' }
}

function formatBytes(value) {
  const n = Number(value)
  if (!Number.isFinite(n) || n < 0) return '—'
  if (n < 1024) return n + ' B'
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB'
  return (n / 1024 / 1024).toFixed(1) + ' MB'
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

function selectorSyncLabel(state) {
  return { syncing: '节点同步中', synced: '节点已同步', failed: '节点同步异常，正在重试' }[state] || ''
}

function formatTraffic(t) {
  const fmt = n => {
    n = Number(n) || 0
    if (n < 1024) return n + ' B'
    if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB'
    if (n < 1024 * 1024 * 1024) return (n / 1024 / 1024).toFixed(1) + ' MB'
    return (n / 1024 / 1024 / 1024).toFixed(2) + ' GB'
  }
  return '实时 ↑ ' + fmt(t?.up) + '/s  ↓ ' + fmt(t?.down) + '/s  ·  累计 ↑ ' + fmt(t?.upTotal) + '  ↓ ' + fmt(t?.downTotal)
}

function renderRuleSummary() {
  const rules = state.rules || []
  const downloaded = rules.filter(rule => rule.exists).length
  const enabled = rules.filter(rule => rule.enabled).length
  const latest = rules.filter(rule => rule.updated_at).sort((a, b) => new Date(b.updated_at) - new Date(a.updated_at))[0]
  const latestText = latest ? new Date(latest.updated_at).toLocaleDateString('zh-CN') : '暂无'
  return `<div class="rules-summary-grid">
    <div class="rules-summary-item"><span>规则集</span><strong>${rules.length}</strong><small>默认与用户规则</small></div>
    <div class="rules-summary-item"><span>已启用</span><strong>${enabled}</strong><small>当前参与分流</small></div>
    <div class="rules-summary-item"><span>已下载</span><strong>${downloaded}/${rules.length}</strong><small>本地文件状态</small></div>
    <div class="rules-summary-item"><span>最近更新</span><strong>${latestText}</strong><small>最后一次成功更新</small></div>
  </div>`
}

function renderRules() {
  if (!state.rules) {
    return '<div class="empty">正在加载规则集信息...</div>'
  }
  const filter = state.ruleFilter || 'all'
  const rules = state.rules.filter(rule => filter === 'all' || (filter === 'default' ? rule.source === 'default' : rule.source !== 'default'))
  if (!rules.length) return '<div class="empty">没有符合条件的规则集</div>'
  return `<div class="rule-set-list">${rules.map(r => `
    <div class="rule-set-row">
      <button class="rule-switch ${r.enabled ? 'on' : ''} toggle-rule" data-id="${r.id}" aria-label="${r.enabled ? '停用' : '启用'} ${escapeHtml(r.name)}"><span></span></button>
      <div class="rule-set-main"><div class="rule-set-title"><strong>${escapeHtml(r.name)}</strong><span class="rule-tag ${r.source === 'default' ? 'default' : 'custom'}">${r.source === 'default' ? '默认' : '用户添加'}</span></div><div class="rule-set-file">${r.exists ? escapeHtml(formatRuleInfo(r)) : '未下载 · 点击更新下载'}</div></div>
      <span class="rule-tag ${r.exists ? 'ready' : 'missing'}">${r.exists ? '已下载' : '未下载'}</span>
      <span class="rule-set-date">${r.updated_at ? escapeHtml(new Date(r.updated_at).toLocaleDateString('zh-CN')) : '尚未更新'}</span>
      <details class="rule-menu" data-rule-menu-id="${r.id}"><summary aria-label="规则集操作">⋯</summary><div class="rule-menu-list"><button class="rule-menu-action update-rule" data-id="${r.id}" ${state.ruleUpdating || state.allRulesUpdating ? 'disabled' : ''}>${state.ruleUpdating === r.id ? '更新中...' : '更新'}</button><button class="rule-menu-action edit-rule" data-id="${r.id}">编辑</button><button class="rule-menu-action rule-copy-url" data-url="${escapeHtml(r.url || '')}">查看 URL</button><button class="rule-menu-action danger delete-rule" data-id="${r.id}">删除</button></div></details>
    </div>`).join('')}</div>`
}

function renderDNSFilterRules() {
  const rules = state.config?.dns_filter_rules || []
  if (!rules.length) return '<div class="dns-empty empty">暂无手动 DNS 过滤规则</div>'
  return `<div class="dns-filter-table"><div class="dns-filter-head"><span>状态</span><span>匹配对象</span><span>匹配内容</span><span>处理</span><span></span></div>${rules.map((rule, index) => `<div class="dns-filter-row"><span class="rule-tag ready">已启用</span><span>${escapeHtml(ruleMatchLabel(rule.match_type))}</span><strong>${escapeHtml(rule.value)}</strong><span class="rule-tag block">阻断</span><button class="btn btn-ghost remove-filter-rule" data-index="${index}">删除</button></div>`).join('')}</div>`
}

function formatRuleInfo(rule) {
  const size = Number(rule.size || 0).toLocaleString('zh-CN') + ' bytes'
  const updated = rule.updated_at ? new Date(rule.updated_at).toLocaleString('zh-CN') : '时间未知'
  const sha = rule.sha256 ? ' · SHA256 ' + rule.sha256 : ''
  return size + ' · ' + updated + sha
}

function renderNodes() {
  const allNodes = state.config?.nodes || []
  const currentNodeID = activeNodeID()
  const filterBtns = [['all', '全部'], ['hysteria2', 'Hysteria2'], ['vless', 'VLESS'], ['vmess', 'VMess'], ['trojan', 'Trojan'], ['shadowsocks', 'SS'], ['socks', 'SOCKS'], ['http', 'HTTP']]
  const filterHtml = `<div class="node-filters">${filterBtns.map(([id, label]) => {
    const count = id === 'all' ? allNodes.length : allNodes.filter(n => n.protocol === id).length
    return `<button class="filter-btn ${state.nodeFilter === id ? 'active' : ''}" data-filter="${id}">${label} (${count})</button>`
  }).join('')}</div>`
  const filtered = allNodes.filter(n => state.nodeFilter === 'all' || n.protocol === state.nodeFilter)
  if (!filtered.length) return filterHtml + '<div class="empty">没有匹配的节点</div>'
  const groups = {}
  for (const n of filtered) {
    const key = n.protocol || 'unknown'
    if (!groups[key]) groups[key] = []
    groups[key].push(n)
  }
  const protocolOrder = ['hysteria2', 'vless', 'vmess', 'trojan', 'shadowsocks', 'socks', 'http', 'unknown']
  const sortedKeys = Object.keys(groups).sort((a, b) => {
    const ai = protocolOrder.indexOf(a)
    const bi = protocolOrder.indexOf(b)
    return (ai === -1 ? 99 : ai) - (bi === -1 ? 99 : bi)
  })
  const groupsHtml = sortedKeys.map(protocol => {
    const nodes = groups[protocol]
    const items = nodes.map(n => `
      <li class="node-item ${currentNodeID === n.id ? 'active' : ''}">
        <input class="node-select" type="checkbox" data-id="${n.id}" ${state.selectedNodes.has(n.id) ? 'checked' : ''} aria-label="选择 ${escapeHtml(n.name)}" />
        <div class="node-info">
          <div class="node-row">
            <span class="node-name" title="${escapeHtml(n.name)}">${escapeHtml(n.name)}</span>
            <span class="node-server" title="${escapeHtml(n.server)}:${n.port}">${escapeHtml(n.server)}:${n.port}</span>
          </div>
          <div class="node-health-row">${renderNodeHealth(n.id)}</div>
        </div>
        <div class="node-actions">
          <button class="btn btn-ghost select-node" data-id="${n.id}" ${state.nodeSwitching ? 'disabled' : ''}>${currentNodeID === n.id ? '当前' : '选择'}</button>
          <button class="btn btn-ghost test-node" data-id="${n.id}">测试</button>
          <button class="btn btn-ghost export-node" data-id="${n.id}">导出</button>
          <button class="btn btn-danger rm-node" data-id="${n.id}">删除</button>
          <button class="btn btn-ghost edit-node" data-id="${n.id}">编辑</button>
        </div>
      </li>`).join('')
    return `<div class="node-group"><div class="node-group-header"><span class="badge ${protocol === 'direct' ? 'direct' : 'proxy'}">${protocol.toUpperCase()}</span><span class="node-group-count">${nodes.length} 个</span></div><ul class="node-list">${items}</ul></div>`
  }).join('')
  return filterHtml + groupsHtml
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
  if (state.statusRefreshing) return
  state.statusRefreshing = true
  try {
    const previousNodeID = activeNodeID()
    const previousSelectorSync = `${state.selectorSyncState}:${state.selectorSyncMessage}`
    const s = await window.go.app.App.GetStatus()
    state.version = s.version || state.version
    state.running = s.running
    state.statusState = s.state
    state.statusMessage = s.message
    state.traffic = { up: s.upload_bytes || 0, down: s.download_bytes || 0, upTotal: s.upload_total_bytes || 0, downTotal: s.download_total_bytes || 0 }
    state.runtimeNodeID = s.running ? (s.current_node_id || '') : ''
    state.selectorSyncState = s.selector_sync_state || 'idle'
    state.selectorSyncMessage = s.selector_sync_message || ''
    const selectorSyncChanged = previousSelectorSync !== `${state.selectorSyncState}:${state.selectorSyncMessage}`
    if (previousNodeID !== activeNodeID() || selectorSyncChanged) renderApp()
    updateStatusView()
  } catch (e) {
    showToast('获取状态失败: ' + e.message, 'error')
  } finally {
    state.statusRefreshing = false
  }
}

async function refreshSingBoxVersion() {
  try {
    state.singBoxVersion = await window.go.app.App.SingBoxVersion()
    renderApp()
  } catch (e) {
    console.error('read sing-box version failed:', e)
  }
}

function renderCustomRules() {
  const rules = state.config?.custom_rules || []
  if (!rules.length) return '<div class="empty">暂无自定义规则</div>'
  return rules.map((rule, index) => `
    <div class="node-item rule-item">
      <span class="badge">${escapeHtml(ruleMatchLabel(rule.match_type))}</span>
      <span class="rule-content"><span class="node-name">${escapeHtml(rule.value)}</span><span class="node-server">${escapeHtml(ruleActionLabel(rule.action))}</span></span>
      <button class="btn btn-danger remove-custom-rule" data-index="${index}">删除</button>
    </div>
  `).join('')
}

function updateStatusView() {
  const statusText = document.querySelector('#statusText')
  const statusTraffic = document.querySelector('#statusTraffic')
  const power = document.querySelector('#power')
  const dot = document.querySelector('.status .dot')
  if (!statusText || !power || !dot) {
    renderApp()
    return
  }
  statusText.textContent = statusLabel(state.statusState, state.statusMessage)
  if (statusTraffic) statusTraffic.textContent = formatTraffic(state.traffic)
  power.textContent = state.running ? '关闭 TUN' : '开启 TUN'
  power.className = 'switch ' + (state.running ? 'on' : '')
  dot.className = 'dot ' + dotClass(state.statusState)
}

async function refreshConfig() {
  if (state.configRefreshing) return
  state.configRefreshing = true
  try {
    const next = await window.go.app.App.GetConfig()
    const changed = JSON.stringify(state.config) !== JSON.stringify(next)
    state.config = next
    if (changed) renderApp()
  } catch (e) {
    showToast('获取配置失败: ' + e.message, 'error')
  } finally {
    state.configRefreshing = false
  }
}

async function refreshIPv6Status() {
  try {
    state.ipv6Status = await window.go.app.App.GetIPv6Status()
  } catch (e) {
    state.ipv6Status = { available: false, message: 'IPv6 状态检测失败，运行时将使用 IPv4' }
  }
}

function ruleMatchLabel(id) { return RULE_MATCH_TYPES.find(item => item.id === id)?.label || id }
function ruleActionLabel(id) { return RULE_ACTIONS.find(item => item.id === id)?.label || id }

function validateCustomRule(matchType, value) {
  if (!value) return '请输入匹配内容'
  if (matchType === 'port' && (!/^\d+$/.test(value) || Number(value) < 1 || Number(value) > 65535)) return '端口必须是 1-65535 的整数'
  if (matchType === 'ip_cidr') {
    const cidr = value.includes('/') ? value : `${value}/32`
    if (!/^([0-9a-f:.]+)\/\d+$/i.test(cidr)) return '请输入合法的 IP 网段，例如 192.168.1.0/24'
  }
  if (matchType !== 'port' && /[\s,]/.test(value)) return '规则内容不能包含空格或逗号'
  return ''
}

function updateRuleEditorHints() {
  const matchType = RULE_MATCH_TYPES.find(item => item.id === state.customRuleForm.match_type) || RULE_MATCH_TYPES[0]
  const ruleValue = document.querySelector('#ruleValue')
  const ruleValueHint = document.querySelector('#ruleValueHint')
  if (ruleValue) ruleValue.placeholder = matchType.placeholder
  if (ruleValueHint) ruleValueHint.textContent = matchType.hint
}

async function refreshDiagnostics() {
  try {
    state.diagnostics = await window.go.app.App.GetDiagnostics()
  } catch (e) {
    state.diagnostics = { message: e.message || String(e) }
  }
}

async function refreshCapture() {
  try {
    const [status, flows] = await Promise.all([
      window.go.app.App.GetCaptureStatus(),
      window.go.app.App.GetCaptureFlows(),
    ])
    state.captureStatus = status
    state.captureFlows = flows || []
    if (state.selectedCaptureID && !state.captureFlows.some(flow => flow.id === state.selectedCaptureID)) {
      state.selectedCaptureID = 0
      state.selectedCaptureFlow = null
    }
    if (!state.selectedCaptureID && state.captureFlows.length) state.selectedCaptureID = state.captureFlows[0].id
    if (state.selectedCaptureID && state.selectedCaptureFlow?.id !== state.selectedCaptureID) {
      state.selectedCaptureFlow = await window.go.app.App.GetCaptureFlow(state.selectedCaptureID)
    } else if (state.selectedCaptureFlow) {
      const summary = state.captureFlows.find(flow => flow.id === state.selectedCaptureID)
      if (summary) state.selectedCaptureFlow = { ...state.selectedCaptureFlow, route: summary.route, error: summary.error, status_code: summary.status_code, duration_ms: summary.duration_ms }
    }
    if (state.view === 'capture' && document.activeElement?.id !== 'captureDomains') renderApp()
  } catch (e) {
    console.error('capture refresh failed:', e)
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
  const focused = document.activeElement
  const focusSnapshot = focused?.id ? {
    id: focused.id,
    start: typeof focused.selectionStart === 'number' ? focused.selectionStart : null,
    end: typeof focused.selectionEnd === 'number' ? focused.selectionEnd : null,
  } : null
  const captureList = document.querySelector('#captureList')
  const captureDetail = document.querySelector('#captureDetail')
  const captureBody = document.querySelector('[data-capture-body-scroll]')
  if (captureList) state.captureListScroll = captureList.scrollTop
  if (captureDetail) state.captureDetailScroll = captureDetail.scrollTop
  if (captureBody) state.captureBodyScroll[captureBody.dataset.captureBodyScroll] = { top: captureBody.scrollTop, left: captureBody.scrollLeft }
  const openRuleMenus = [...document.querySelectorAll('.rule-menu[open]')].map(menu => menu.dataset.ruleMenuId)
  document.documentElement.dataset.theme = state.theme
  app.innerHTML = render()
  bindEvents()
  openRuleMenus.forEach(id => {
    const menu = document.querySelector(`.rule-menu[data-rule-menu-id="${CSS.escape(id)}"]`)
    if (menu) menu.open = true
  })
  const restoredList = document.querySelector('#captureList')
  const restoredDetail = document.querySelector('#captureDetail')
  const restoredBody = document.querySelector('[data-capture-body-scroll]')
  if (restoredList) restoredList.scrollTop = state.captureListScroll
  if (restoredDetail) restoredDetail.scrollTop = state.captureDetailScroll
  if (restoredBody) {
    const scroll = state.captureBodyScroll[restoredBody.dataset.captureBodyScroll]
    if (scroll) {
      restoredBody.scrollTop = scroll.top
      restoredBody.scrollLeft = scroll.left
    }
  }
  if (focusSnapshot) {
    const restored = document.getElementById(focusSnapshot.id)
    if (restored) {
      restored.focus({ preventScroll: true })
      if (focusSnapshot.start !== null && typeof restored.setSelectionRange === 'function') {
        restored.setSelectionRange(focusSnapshot.start, focusSnapshot.end)
      }
    }
  }
}

function bindEvents() {
  if (!state.ruleMenuDocumentBound) {
    document.addEventListener('click', event => {
      const clickedMenu = event.target.closest?.('.rule-menu')
      document.querySelectorAll('.rule-menu[open]').forEach(menu => {
        if (!clickedMenu || menu !== clickedMenu) menu.open = false
      })
    })
    state.ruleMenuDocumentBound = true
  }
  const themeToggle = document.querySelector('#themeToggle')
  if (themeToggle) {
    themeToggle.addEventListener('click', () => {
      state.theme = state.theme === 'dark' ? 'light' : 'dark'
      localStorage.setItem('sbtun-theme', state.theme)
      renderApp()
    })
  }
  document.querySelectorAll('[data-view]').forEach(item => {
    item.addEventListener('click', async () => {
      state.view = item.dataset.view
      if (state.view === 'routing') await refreshIPv6Status()
      if (state.view === 'capture') await refreshCapture()
      renderApp()
    })
  })
  const updateSingBox = document.querySelector('#updateSingBox')
  if (updateSingBox) {
    updateSingBox.addEventListener('click', async () => {
      state.singBoxUpdating = true
      state.singBoxUpdateState = 'loading'
      state.singBoxUpdateMessage = '正在检查官方稳定版并校验资源…'
      renderApp()
      try {
        const result = await window.go.app.App.UpdateSingBox()
        state.singBoxVersion = result.current_version || state.singBoxVersion
        state.singBoxUpdateState = 'success'
        state.singBoxUpdateMessage = result.message || 'sing-box 更新完成'
        showToast(result.message || 'sing-box 更新完成', 'success')
      } catch (e) {
        state.singBoxUpdateState = 'error'
        state.singBoxUpdateMessage = e.message || String(e)
        showToast(e.message || String(e), 'error')
      } finally {
        state.singBoxUpdating = false
        renderApp()
      }
    })
  }
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
    ruleMatchType.addEventListener('change', () => {
      state.customRuleForm.match_type = ruleMatchType.value
      updateRuleEditorHints()
    })
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
      const error = validateCustomRule(state.customRuleForm.match_type, value)
      if (error) { showToast(error, 'error'); return }
      const cfg = JSON.parse(JSON.stringify(state.config))
      cfg.custom_rules = cfg.custom_rules || []
      const duplicate = cfg.custom_rules.some(rule => rule.match_type === state.customRuleForm.match_type && rule.value.toLowerCase() === value.toLowerCase() && rule.action === state.customRuleForm.action)
      if (duplicate) { showToast('这条规则已经存在', 'error'); return }
      cfg.custom_rules.push({ match_type: state.customRuleForm.match_type, value, action: state.customRuleForm.action })
      addRuleBtn.disabled = true
      try {
        await window.go.app.App.SaveConfig(cfg)
        state.config = cfg
        state.customRuleForm.value = ''
        renderApp()
        showToast('规则已添加', 'success')
      } catch (e) { showToast(e.message || String(e), 'error') }
      finally { addRuleBtn.disabled = false }
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

  const diagnosticsToggle = document.querySelector('#diagnosticsToggle')
  if (diagnosticsToggle) {
    diagnosticsToggle.addEventListener('change', async () => {
      const cfg = JSON.parse(JSON.stringify(state.config))
      cfg.diagnostics_enabled = diagnosticsToggle.checked
      try {
        await window.go.app.App.SaveConfig(cfg)
        state.config = cfg
        await refreshDiagnostics()
        renderApp()
      } catch (e) {
        diagnosticsToggle.checked = !diagnosticsToggle.checked
        showToast(e.message || String(e), 'error')
      }
    })
  }

  const ipv6Toggle = document.querySelector('#ipv6Toggle')
  if (ipv6Toggle) {
    ipv6Toggle.addEventListener('change', async () => {
      const cfg = JSON.parse(JSON.stringify(state.config))
      cfg.ipv6_enabled = ipv6Toggle.checked
      ipv6Toggle.disabled = true
      try {
        await window.go.app.App.SaveConfig(cfg)
        state.config = cfg
        await refreshIPv6Status()
        renderApp()
        showToast(cfg.ipv6_enabled && !state.ipv6Status?.available ? '设备没有可用 IPv6，已自动使用 IPv4' : 'IPv6 设置已应用', 'success')
      } catch (e) {
        ipv6Toggle.checked = !ipv6Toggle.checked
        showToast(e.message || String(e), 'error')
      } finally {
        ipv6Toggle.disabled = false
      }
    })
  }

  const captureDomains = document.querySelector('#captureDomains')
  if (captureDomains) captureDomains.addEventListener('input', () => {
    state.captureDraft = captureDomains.value
    state.captureDraftDirty = true
  })
  const readCaptureDomains = () => (captureDomains?.value || '').split(/\r?\n|,/).map(item => item.trim().replace(/^\./, '')).filter(Boolean)
  const applyCaptureSettings = async enabled => {
    const domains = [...new Set(readCaptureDomains())]
    if (enabled && !domains.length) throw new Error('启用分析前请添加域名或关键词')
    await window.go.app.App.SetCaptureSettings(enabled, domains)
    await refreshConfig()
    state.captureDraft = domains.join('\n')
    state.captureDraftDirty = false
    state.captureEnabledDraft = enabled
    state.captureEnabledDirty = false
    await refreshCapture()
  }
  const captureEnabled = document.querySelector('#captureEnabled')
  if (captureEnabled) captureEnabled.addEventListener('change', async () => {
    state.captureEnabledDraft = captureEnabled.checked
    state.captureEnabledDirty = true
    captureEnabled.disabled = true
    try {
      await applyCaptureSettings(captureEnabled.checked)
      showToast(captureEnabled.checked ? (state.captureStatus?.running ? '分析器已启动' : '已启用，开启 TUN 后开始分析') : '分析器已关闭', 'success')
    } catch (e) {
      state.captureEnabledDraft = Boolean(state.config?.capture_enabled)
      state.captureEnabledDirty = false
      showToast(e.message || String(e), 'error')
      renderApp()
    } finally { captureEnabled.disabled = false }
  })
  const saveCapture = document.querySelector('#saveCapture')
  if (saveCapture) saveCapture.addEventListener('click', async () => {
    const enabled = state.captureEnabledDirty ? state.captureEnabledDraft : Boolean(captureEnabled?.checked)
    saveCapture.disabled = true
    try {
      await applyCaptureSettings(enabled)
      showToast(enabled && !state.captureStatus?.running ? '配置已保存，开启 TUN 后开始分析' : '分析配置已应用', 'success')
    } catch (e) { showToast(e.message || String(e), 'error') }
    finally { saveCapture.disabled = false }
  })
  const installCaptureCA = document.querySelector('#installCaptureCA')
  const captureCertLevel = document.querySelector('#captureCertLevel')
  if (installCaptureCA) installCaptureCA.addEventListener('click', async () => {
    installCaptureCA.disabled = true
    try {
      await window.go.app.App.InstallCaptureCertificate(captureCertLevel?.value || 'system')
      await refreshCapture()
      showToast(`分析证书已安装（${captureCertLevel?.value === 'user' ? '当前用户' : '系统级'}）`, 'success')
    } catch (e) { showToast(e.message || String(e), 'error') }
    finally { installCaptureCA.disabled = false }
  })
  const uninstallCaptureCA = document.querySelector('#uninstallCaptureCA')
  if (uninstallCaptureCA) uninstallCaptureCA.addEventListener('click', async () => {
    uninstallCaptureCA.disabled = true
    try {
      await window.go.app.App.UninstallCaptureCertificate(captureCertLevel?.value || 'system')
      await refreshCapture()
      showToast('分析证书已卸载', 'success')
    } catch (e) { showToast(e.message || String(e), 'error') }
    finally { uninstallCaptureCA.disabled = false }
  })
  const clearCapture = document.querySelector('#clearCapture')
  if (clearCapture) clearCapture.addEventListener('click', async () => {
    if (!window.confirm('确定删除全部抓包记录？此操作无法撤销。')) return
    await window.go.app.App.ClearCaptureFlows()
    state.selectedCaptureID = 0
    state.selectedCaptureFlow = null
    await refreshCapture()
  })
  const saveCaptureFile = document.querySelector('#saveCaptureFile')
  if (saveCaptureFile) saveCaptureFile.addEventListener('click', async () => {
    saveCaptureFile.disabled = true
    try {
      await window.go.app.App.SaveCaptureFlows()
      await refreshCapture()
      showToast('抓包已保存为 JSON', 'success')
    } catch (e) { showToast(e.message || String(e), 'error') }
    finally { saveCaptureFile.disabled = false }
  })
  document.querySelectorAll('[data-capture-id]').forEach(row => row.addEventListener('click', async () => {
    state.selectedCaptureID = Number(row.dataset.captureId)
    try { state.selectedCaptureFlow = await window.go.app.App.GetCaptureFlow(state.selectedCaptureID) }
    catch (e) { return showToast(e.message || String(e), 'error') }
    renderApp()
  }))
  const captureSearch = document.querySelector('#captureSearch')
  if (captureSearch) captureSearch.addEventListener('input', () => {
    state.captureSearch = captureSearch.value
    state.captureListScroll = 0
    renderApp()
  })
  document.querySelectorAll('[data-capture-filter]').forEach(button => button.addEventListener('click', () => {
    state.captureFilter = button.dataset.captureFilter
    state.captureListScroll = 0
    renderApp()
  }))
  document.querySelectorAll('[data-capture-tab]').forEach(button => button.addEventListener('click', () => {
    state.captureDetailTab = button.dataset.captureTab
    state.captureDetailScroll = 0
    renderApp()
  }))

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

  const toggleSelectNodes = document.querySelector('#toggleSelectNodes')
  const syncToggleSelectLabel = () => {
    if (!toggleSelectNodes) return
    const nodes = state.config?.nodes || []
    toggleSelectNodes.textContent = nodes.length > 0 && nodes.every(node => state.selectedNodes.has(node.id)) ? '反选' : '全选'
  }
  document.querySelectorAll('.node-select').forEach(input => {
    input.addEventListener('change', () => {
      if (input.checked) state.selectedNodes.add(input.dataset.id)
      else state.selectedNodes.delete(input.dataset.id)
      syncToggleSelectLabel()
    })
  })
  if (toggleSelectNodes) toggleSelectNodes.addEventListener('click', () => {
    const nodes = state.config?.nodes || []
    const allSelected = nodes.length > 0 && nodes.every(node => state.selectedNodes.has(node.id))
    nodes.forEach(node => {
      if (allSelected) state.selectedNodes.delete(node.id)
      else state.selectedNodes.add(node.id)
    })
    renderApp()
  })
  const selectedIDs = () => [...state.selectedNodes].filter(id => state.config?.nodes?.some(n => n.id === id))
  const copyNodeLinks = async ids => {
    const links = await window.go.app.App.ExportNodes(ids)
    if (!links?.length) throw new Error('没有可导出的节点')
    await window.runtime.ClipboardSetText(links.join('\n'))
    return links.length
  }
  const batchExport = document.querySelector('#batchExportNodes')
  if (batchExport) batchExport.addEventListener('click', async () => {
    const ids = selectedIDs()
    if (!ids.length) return showToast('请先选择节点', 'error')
    batchExport.disabled = true
    try {
      const count = await copyNodeLinks(ids)
      showToast(`已复制 ${count} 个节点链接`, 'success')
    } catch (e) {
      showToast(e.message || String(e), 'error')
    } finally {
      batchExport.disabled = false
    }
  })
  const batchDelete = document.querySelector('#batchDeleteNodes')
  if (batchDelete) batchDelete.addEventListener('click', async () => {
    const ids = selectedIDs()
    if (!ids.length) return showToast('请先选择节点', 'error')
    if (!confirm(`确定删除选中的 ${ids.length} 个节点吗？`)) return
    batchDelete.disabled = true
    try {
      await window.go.app.App.RemoveNodes(ids)
      ids.forEach(id => state.selectedNodes.delete(id))
      await refreshConfig()
      showToast('批量删除成功', 'success')
    } catch (e) { showToast(e.message || String(e), 'error') }
    finally { batchDelete.disabled = false }
  })
  const batchTest = document.querySelector('#batchTestNodes')
  if (batchTest) batchTest.addEventListener('click', async () => {
    const ids = selectedIDs()
    if (!ids.length) return showToast('请先选择节点', 'error')
    batchTest.disabled = true
    state.batchTesting = true
    try {
      for (const id of ids) {
        const result = await window.go.app.App.TestNode(id)
        state.nodeHealth[id] = result
        renderApp()
      }
      showToast('批量健康测试完成', 'success')
    } catch (e) {
      showToast(e.message || String(e), 'error')
    } finally {
      state.batchTesting = false
      renderApp()
    }
  })
  document.querySelectorAll('.filter-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      state.nodeFilter = btn.dataset.filter
      renderApp()
    })
  })
  document.querySelectorAll('.edit-node').forEach(btn => {
    btn.addEventListener('click', async () => {
      const node = state.config.nodes.find(item => item.id === btn.dataset.id)
      if (!node) return
      beginEditNode(node)
    })
  })
  document.querySelectorAll('.export-node').forEach(btn => {
    btn.addEventListener('click', async () => {
      btn.disabled = true
      try {
        await copyNodeLinks([btn.dataset.id])
        showToast('节点链接已复制', 'success')
      } catch (e) {
        showToast(e.message || String(e), 'error')
      } finally {
        btn.disabled = false
      }
    })
  })
  const cancelEdit = document.querySelector('#cancelEditNode')
  if (cancelEdit) cancelEdit.addEventListener('click', () => {
    state.editingNodeId = ''
    renderApp()
  })

  document.querySelectorAll('.test-node').forEach(btn => {
    btn.addEventListener('click', async () => {
      btn.disabled = true
      try {
        const result = await window.go.app.App.TestNode(btn.dataset.id)
        state.nodeHealth[btn.dataset.id] = result
        renderApp()
        showToast(result.message, (result.healthy ?? result.ok) ? 'success' : 'error')
      } catch (e) {
        showToast(e.message || String(e), 'error')
      } finally {
        btn.disabled = false
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

  const ruleSetFilter = document.querySelector('#ruleSetFilter')
  if (ruleSetFilter) ruleSetFilter.addEventListener('change', () => {
    state.ruleFilter = ruleSetFilter.value
    renderApp()
  })
  document.querySelectorAll('.rule-copy-url').forEach(btn => btn.addEventListener('click', async () => {
    const url = btn.dataset.url || ''
    if (!url) { showToast('这个规则集没有可用 URL', 'error'); return }
    try {
      await navigator.clipboard.writeText(url)
      showToast('规则集 URL 已复制', 'success')
    } catch (_) {
      showToast(url, 'info')
    }
  }))
  document.querySelectorAll('.edit-rule').forEach(btn => btn.addEventListener('click', () => {
    const rule = state.rules?.find(item => item.id === btn.dataset.id)
    if (!rule) return
    state.editingRuleSetID = rule.id
    state.ruleSetForm = { name: rule.name, url: rule.url }
    renderApp()
    document.querySelector('#ruleSetName')?.focus()
  }))

  const ruleSetName = document.querySelector('#ruleSetName')
  const ruleSetURL = document.querySelector('#ruleSetURL')
  if (ruleSetName) ruleSetName.addEventListener('input', () => { state.ruleSetForm.name = ruleSetName.value })
  if (ruleSetURL) ruleSetURL.addEventListener('input', () => { state.ruleSetForm.url = ruleSetURL.value })
  const addRuleSetBtn = document.querySelector('#addRuleSetBtn')
  if (addRuleSetBtn) addRuleSetBtn.addEventListener('click', async () => {
    const name = state.ruleSetForm.name.trim()
    const url = state.ruleSetForm.url.trim()
    if (!name || !url) { showToast('请输入规则集名称和 URL', 'error'); return }
    addRuleSetBtn.disabled = true
    const editing = Boolean(state.editingRuleSetID)
    try {
      if (state.editingRuleSetID) {
        await window.go.app.App.EditRuleSet(state.editingRuleSetID, name, url)
      } else {
        await window.go.app.App.AddRuleSet(name, url)
      }
      state.ruleSetForm = { name: '', url: '' }
      state.editingRuleSetID = ''
      await refreshRules()
      showToast(editing ? '规则集已修改' : '规则集已添加', 'success')
    } catch (e) { showToast(e.message || String(e), 'error') }
    finally { addRuleSetBtn.disabled = false }
  })
  const cancelEditRuleSet = document.querySelector('#cancelEditRuleSet')
  if (cancelEditRuleSet) cancelEditRuleSet.addEventListener('click', () => {
    state.editingRuleSetID = ''
    state.ruleSetForm = { name: '', url: '' }
    renderApp()
  })
  document.querySelectorAll('.toggle-rule').forEach(btn => btn.addEventListener('click', async () => {
    const rule = state.rules?.find(item => item.id === btn.dataset.id)
    if (!rule) return
    btn.disabled = true
    try {
      await window.go.app.App.SetRuleEnabled(rule.id, !rule.enabled)
      await refreshRules()
      showToast(rule.enabled ? '规则集已停用' : '规则集已启用', 'success')
    } catch (e) { showToast(e.message || String(e), 'error') }
    finally { btn.disabled = false }
  }))
  document.querySelectorAll('.delete-rule').forEach(btn => btn.addEventListener('click', async () => {
    if (!window.confirm('删除这个规则集及其本地文件？')) return
    btn.disabled = true
    try {
      await window.go.app.App.DeleteRuleSet(btn.dataset.id)
      await refreshRules()
      showToast('规则集已删除', 'success')
    } catch (e) { showToast(e.message || String(e), 'error') }
    finally { btn.disabled = false }
  }))
  const restoreDefaultRules = document.querySelector('#restoreDefaultRules')
  if (restoreDefaultRules) restoreDefaultRules.addEventListener('click', async () => {
    restoreDefaultRules.disabled = true
    try {
      await window.go.app.App.RestoreDefaultRuleSets()
      await refreshRules()
      showToast('默认规则集已恢复', 'success')
    } catch (e) { showToast(e.message || String(e), 'error') }
    finally { restoreDefaultRules.disabled = false }
  })

  const addFilterRuleBtn = document.querySelector('#addFilterRuleBtn')
  if (addFilterRuleBtn) addFilterRuleBtn.addEventListener('click', async () => {
    const matchType = document.querySelector('#filterRuleMatchType')?.value || 'domain_suffix'
    const value = document.querySelector('#filterRuleValue')?.value.trim() || ''
    const error = validateCustomRule(matchType, value)
    if (error && matchType !== 'ip_cidr') { showToast(error, 'error'); return }
    if (!value) { showToast('请输入过滤内容', 'error'); return }
    const cfg = JSON.parse(JSON.stringify(state.config))
    cfg.dns_filter_rules = cfg.dns_filter_rules || []
    const duplicate = cfg.dns_filter_rules.some(rule => rule.match_type === matchType && rule.value.toLowerCase() === value.toLowerCase())
    if (duplicate) { showToast('这条过滤规则已经存在', 'error'); return }
    cfg.dns_filter_rules.push({ match_type: matchType, value, action: 'block' })
    addFilterRuleBtn.disabled = true
    try {
      await window.go.app.App.SaveConfig(cfg)
      state.config = cfg
      state.dnsFilterFormOpen = false
      renderApp()
      showToast('过滤规则已添加', 'success')
    } catch (e) { showToast(e.message || String(e), 'error') }
    finally { addFilterRuleBtn.disabled = false }
  })
  const openFilterRuleBtn = document.querySelector('#openFilterRuleBtn')
  if (openFilterRuleBtn) openFilterRuleBtn.addEventListener('click', () => {
    state.dnsFilterFormOpen = true
    renderApp()
  })
  const cancelFilterRuleBtn = document.querySelector('#cancelFilterRuleBtn')
  if (cancelFilterRuleBtn) cancelFilterRuleBtn.addEventListener('click', () => {
    state.dnsFilterFormOpen = false
    renderApp()
  })
  document.querySelectorAll('.remove-filter-rule').forEach(btn => btn.addEventListener('click', async () => {
    const cfg = JSON.parse(JSON.stringify(state.config))
    cfg.dns_filter_rules.splice(Number(btn.dataset.index), 1)
    try {
      await window.go.app.App.SaveConfig(cfg)
      state.config = cfg
      renderApp()
      showToast('过滤规则已删除', 'success')
    } catch (e) { showToast(e.message || String(e), 'error') }
  }))

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
      if (!name || !server || !Number.isInteger(port) || port < 1 || port > 65535) {
        showToast('请填写名称、服务器和端口', 'error'); return
      }
      try {
        const node = {
          id: 'manual-' + Date.now(),
          name, server, port, protocol,
          settings: buildManualSettings(protocol, password),
        }
        if (state.editingNodeId) {
          node.id = state.editingNodeId
          await window.go.app.App.UpdateNode(state.editingNodeId, node)
          state.editingNodeId = ''
        } else {
          await window.go.app.App.AddNode(node)
        }
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
  document.querySelectorAll('#node-import input').forEach(input => {
    input.addEventListener('keydown', event => {
      if (event.key === 'Enter' && addNodeBtn && !addNodeBtn.disabled) {
        event.preventDefault()
        addNodeBtn.click()
      }
    })
  })
}

function beginEditNode(node) {
  const form = { ...state.manualForm }
  for (const key of Object.keys(form)) form[key] = typeof form[key] === 'boolean' ? false : ''
  form.name = node.name || ''
  form.server = node.server || ''
  form.port = String(node.port || '')
  for (const [key, value] of Object.entries(node.settings || {})) {
    if (key in form) form[key] = value === 'true' ? true : value
  }
  if (node.protocol === 'vless' || node.protocol === 'vmess') form.password = node.settings?.uuid || ''
  if (node.protocol === 'hysteria2') form.server_name = node.settings?.sni || ''
  state.manualProtocol = node.protocol
  state.manualForm = form
  state.editingNodeId = node.id
  state.view = 'nodes'
  renderApp()
  document.querySelector('#node-import')?.scrollIntoView({ behavior: 'smooth' })
}

// Wails runtime ready
window.addEventListener('DOMContentLoaded', async () => {
  if (window.go?.app?.App) {
    await refreshStatus()
    await refreshConfig()
    await refreshSingBoxVersion()
    await refreshIPv6Status()
    await refreshRules()
    await refreshDiagnostics()
    await refreshCapture()
    setInterval(refreshStatus, 2000)
    setInterval(refreshConfig, 1500)
    setInterval(refreshDiagnostics, 1500)
    setInterval(refreshCapture, 1500)
  } else {
    showToast('Wails 运行时未加载', 'error')
    renderApp()
  }
})
