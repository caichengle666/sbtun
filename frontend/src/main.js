
import './style.css'
import logoUrl from './assets/logo.svg'

const app = document.querySelector('#app')
const THEMES = [
  { id: 'dark', name: '深色' },
  { id: 'light', name: '浅色' },
  { id: 'glass', name: '毛玻璃' },
]
const savedTheme = localStorage.getItem('sbtun-theme')
const initialTheme = THEMES.some(theme => theme.id === savedTheme) ? savedTheme : 'dark'
const THEME_ICONS = { dark: '<span class="moon-icon" aria-hidden="true"></span>', light: '☀', glass: '<span class="glass-icon" aria-hidden="true"></span>' }
function nextTheme() {
  return THEMES[(THEMES.findIndex(theme => theme.id === state.theme) + 1) % THEMES.length]
}
const savedView = localStorage.getItem('sbtun-ui-view')
const initialView = ['overview', 'nodes', 'routing', 'rules', 'capture'].includes(savedView) ? savedView : 'overview'
const savedNodeFilter = localStorage.getItem('sbtun-ui-node-filter') || 'all'
const NODE_PROTOCOLS = [['hysteria2', 'Hysteria2'], ['vless', 'VLESS'], ['vmess', 'VMess'], ['trojan', 'Trojan'], ['shadowsocks', 'Shadowsocks'], ['tuic', 'TUIC'], ['anytls', 'AnyTLS'], ['wireguard', 'WireGuard'], ['socks', 'SOCKS'], ['http', 'HTTP']]
const TRAFFIC_SLOTS = 30
const NODE_TEST_CONCURRENCY = 4
const APP_UPDATE_INTERVAL = 6 * 60 * 60 * 1000
const AUTO_SWITCH_BADGE_SECONDS = 10 * 60
const validNodeFilters = ['all', ...NODE_PROTOCOLS.map(([id]) => id)]

const state = {
  theme: initialTheme,
  running: false,
  version: '',
  singBoxVersion: '',
  appUpdate: null,
  autoSwitch: null,
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
  manualForm: { url: '', name: '', server: '', port: '', password: '', username: '', method: '', network: '', server_name: '', alpn: '', flow: '', alter_id: '', security: '', packet_encoding: '', reality_public_key: '', reality_short_id: '', utls_fingerprint: '', transport_type: '', transport_path: '', transport_host: '', transport_service_name: '', plugin: '', plugin_opts: '', version: '5', up_mbps: '', down_mbps: '', server_ports: '', hop_interval: '', path: '', obfs_type: '', obfs_password: '', uuid: '', congestion_control: '', udp_relay_mode: '', private_key: '', public_key: '', pre_shared_key: '', local_address: '', reserved: '', mtu: '', allowed_ips: '', disable_path_mtu_discovery: false, udp_over_tcp: false, multiplex: false, insecure: false, tls: false },
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
  subUpdating: '',
  nodeHealth: loadNodeHealth(),
  selectedNodes: new Set(),
  batchTesting: false,
  nodeFilter: validNodeFilters.includes(savedNodeFilter) ? savedNodeFilter : 'all',
  nodeSearch: localStorage.getItem('sbtun-ui-node-search') || '',
  nodeFormOpen: false,
  nodeFormTab: 'link',
  nodeGroupBy: localStorage.getItem('sbtun-ui-node-group') === 'subscription' ? 'subscription' : 'protocol',
  nodeSort: localStorage.getItem('sbtun-ui-node-sort') === 'latency' ? 'latency' : 'default',
  nodeTesting: new Set(),
  rulesTab: localStorage.getItem('sbtun-ui-rules-tab') === 'manual' ? 'manual' : 'sets',
  traffic: { up: 0, down: 0 },
  trafficHistory: [],
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
  captureBodyViewer: null,
  nodeQRCode: null,
  captureSearch: '',
  captureFilter: 'all',
  captureListScroll: 0,
  captureDetailScroll: 0,
  captureBodyScroll: {},
  captureSettingsOpen: localStorage.getItem('sbtun-ui-capture-settings-open') !== 'false',
  view: initialView,
  statusRefreshing: false,
  configRefreshing: false,
}

const svgIcon = body => `<svg viewBox="0 0 24 24" width="17" height="17" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">${body}</svg>`
const NAV_ICONS = {
  overview: svgIcon('<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>'),
  nodes: svgIcon('<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18"/>'),
  routing: svgIcon('<circle cx="6" cy="6" r="2.5"/><circle cx="6" cy="18" r="2.5"/><circle cx="18" cy="12" r="2.5"/><path d="M6 8.5v7M8.3 7.2 15.7 11M8.3 16.8l7.4-3.8"/>'),
  rules: svgIcon('<path d="M4 6h16M7 12h10M10 18h4"/>'),
  capture: svgIcon('<path d="M3 12h4l3-8 4 16 3-8h4"/>'),
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
  tuic: [
    { key: 'uuid', label: 'UUID', placeholder: 'TUIC 用户 UUID', required: true },
    { key: 'password', label: '密码', type: 'password', placeholder: 'TUIC 密码' },
    { key: 'congestion_control', label: '拥塞控制', type: 'select', options: [['', '默认 cubic'], ['bbr', 'bbr'], ['new_reno', 'new_reno']] },
    { key: 'udp_relay_mode', label: 'UDP 转发', type: 'select', options: [['', '默认 native'], ['quic', 'quic']] },
    { key: 'server_name', label: 'TLS SNI', placeholder: '可选，默认服务器地址' },
    { key: 'alpn', label: 'TLS ALPN', placeholder: '可选，默认 h3' },
    { key: 'insecure', label: '跳过证书校验', type: 'checkbox' },
  ],
  anytls: [
    { key: 'password', label: '密码', type: 'password', placeholder: 'AnyTLS 密码', required: true },
    { key: 'server_name', label: 'TLS SNI', placeholder: '可选，默认服务器地址' },
    { key: 'alpn', label: 'TLS ALPN', placeholder: '逗号分隔，可选' },
    { key: 'utls_fingerprint', label: 'uTLS 指纹', placeholder: '可选，例如 chrome' },
    { key: 'insecure', label: '跳过证书校验', type: 'checkbox' },
  ],
  wireguard: [
    { key: 'private_key', label: '私钥', type: 'password', placeholder: '本机 PrivateKey', required: true },
    { key: 'public_key', label: '对端公钥', placeholder: '服务器 PublicKey', required: true },
    { key: 'local_address', label: '本机地址', placeholder: '例如 172.16.0.2/32,fd01::2/128', required: true },
    { key: 'pre_shared_key', label: '预共享密钥', type: 'password', placeholder: '可选' },
    { key: 'reserved', label: 'Reserved', placeholder: '可选，例如 1,2,3' },
    { key: 'mtu', label: 'MTU', type: 'number', placeholder: '默认 1408' },
    { key: 'allowed_ips', label: 'Allowed IPs', placeholder: '默认 0.0.0.0/0,::/0' },
  ],
}

document.documentElement.dataset.theme = state.theme
app.innerHTML = render()

function render() {
  return `
    <main class="shell">
      <aside class="sidebar">
        <div class="brand"><img class="brand-mark" src="${logoUrl}" alt="" /><span class="brand-name">sbtun</span></div>
        <div class="nav-caption">工作区</div>
        <nav class="nav-list">
          ${[
            ['overview', '运行概览'],
            ['nodes', '节点管理'],
            ['routing', '路由模式'],
            ['rules', '分流规则'],
            ['capture', '流量分析'],
          ].map(([id, label]) => `<button class="nav-item ${state.view === id ? 'active' : ''}" data-view="${id}" title="${label}"><span class="nav-icon" aria-hidden="true">${NAV_ICONS[id]}</span><span class="nav-label">${label}</span></button>`).join('')}
        </nav>
        <div class="sidebar-status">
          <div class="sidebar-status-line" title="${escapeHtml(statusLabel(state.statusState, state.statusMessage))}"><span class="dot ${dotClass(state.statusState)}"></span><span>${statusLabel(state.statusState, state.statusMessage)}</span></div>
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
            ${state.appUpdate?.has_update ? `<button id="appUpdate" class="update-pill" type="button" title="当前 v${escapeHtml(state.appUpdate.current_version)}，点击打开下载页"><span class="update-dot" aria-hidden="true"></span>新版本 v${escapeHtml(state.appUpdate.latest_version)}</button>` : ''}
            <button id="themeToggle" class="theme-toggle" type="button" aria-label="切换主题" title="切换到${nextTheme().name}主题">${THEME_ICONS[nextTheme().id]}</button>
            <button id="power" class="switch ${state.running ? 'on' : ''}">${state.running ? '关闭 TUN' : '开启 TUN'}</button>
          </div>
        </header>
        <section class="status-bar">
          <span class="dot ${dotClass(state.statusState)}"></span>
          <strong id="statusText">${statusLabel(state.statusState, state.statusMessage)}</strong>
          ${state.running && state.selectorSyncState !== 'idle' ? `<span class="badge selector-sync ${state.selectorSyncState}" title="${escapeHtml(state.selectorSyncMessage)}">${selectorSyncLabel(state.selectorSyncState)}</span>` : ''}
          ${renderAutoSwitchBadge()}
          <span class="status-context" id="statusNode">节点：${escapeHtml(state.config?.nodes?.find(node => node.id === activeNodeID())?.name || '未选择')}</span>
          <span class="status-context" id="statusMode">模式：${MODES.find(mode => mode.id === state.config?.routing_mode)?.name || '未设置'}</span>
          <span class="muted" id="statusTraffic">${formatTraffic(state.traffic)}</span>
        </section>
        ${renderView()}
      </section>
    </main>
    ${renderNodeModal()}
    ${renderCaptureBodyModal()}
    ${renderNodeQRCodeModal()}
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

function resetManualForm() {
  for (const key of Object.keys(state.manualForm)) {
    state.manualForm[key] = typeof state.manualForm[key] === 'boolean' ? false : ''
  }
  state.manualForm.version = '5'
  state.manualProtocol = 'vless'
}

function selectedNodeIDs() {
  return [...state.selectedNodes].filter(id => state.config?.nodes?.some(n => n.id === id))
}

function visibleNodeIDs() {
  const query = state.nodeSearch.trim().toLowerCase()
  return (state.config?.nodes || [])
    .filter(node => (state.nodeFilter === 'all' || node.protocol === state.nodeFilter) && (!query || `${node.name} ${node.server} ${node.port} ${node.protocol}`.toLowerCase().includes(query)))
    .map(node => node.id)
}

function renderView() {
  if (state.view === 'nodes') return renderNodesPage()
  if (state.view === 'routing') return renderRoutingPage()
  if (state.view === 'rules') return renderRulesPage()
  if (state.view === 'capture') return renderCapturePage()
  return renderOverview()
}

function renderOverview() {
  const nodes = state.config?.nodes || []
  const current = nodes.find(n => n.id === activeNodeID())
  const mode = MODES.find(item => item.id === state.config?.routing_mode)
  const rules = state.rules || []
  const enabledRules = rules.filter(rule => rule.enabled).length
  const testing = current && state.nodeTesting.has(current.id)
  return `<div class="overview-grid">
      <section class="card focus-card overview-node-panel">
        <div class="focus-card-main">
          <div class="section-kicker"><i class="dot ${state.running ? 'running' : ''}"></i>${state.running ? '当前代理节点' : '当前节点'}</div>
          <h2 class="overview-node-name" title="${escapeHtml(current?.name || '')}">${escapeHtml(current?.name || '未选择节点')}</h2>
          <p class="muted overview-node-server">${current ? escapeHtml(current.server) + ':' + current.port : '请先在节点页导入或添加节点'}</p>
          ${current ? `<div class="overview-tags"><span class="badge proxy">${escapeHtml(protocolLabel(current.protocol))}</span>${renderLatency(current.id)}</div>` : ''}
        </div>
        <div class="overview-node-side">
          <label class="overview-switch-label" for="quickNodeSelect">快速切换</label>
          <select id="quickNodeSelect" class="select" ${nodes.length && !state.nodeSwitching ? '' : 'disabled'}>${current ? '' : '<option value="">选择节点</option>'}${renderNodeOptions(nodes, current?.id)}</select>
          <button class="btn btn-ghost" id="testCurrentNode" ${current && !testing ? '' : 'disabled'}>${testing ? '测试中…' : '测试延迟'}</button>
        </div>
      </section>
      <section class="card overview-route-panel"><span class="section-kicker">当前路由</span><strong>${escapeHtml(mode?.name || '未设置')}</strong><div class="route-line"><span>路由策略</span><b>${escapeHtml(mode?.desc || '请先选择路由模式')}</b></div><div class="route-line"><span>IPv6</span><b class="${state.config?.ipv6_enabled ? 'good' : ''}">${state.config?.ipv6_enabled ? '已启用' : '未启用'}</b></div><button class="btn btn-ghost overview-route-btn" data-view="routing">更改路由模式</button></section>
      <section class="card traffic-panel"><div class="section-heading"><h2>实时流量</h2><span class="muted">最近 60 秒</span></div><div class="traffic-numbers"><span>上行<strong id="trafficUp">${formatBytes(state.traffic.up)}/s</strong></span><span>下行<strong id="trafficDown">${formatBytes(state.traffic.down)}/s</strong></span></div><div class="traffic-chart" id="trafficChart" role="img" aria-label="最近 60 秒实时流量">${renderTrafficChart()}</div><div class="traffic-chart-label"><span>60 秒前</span><span>现在</span></div></section>
    </div>
    <div class="overview-quick"><button data-view="nodes"><span>可用节点</span><strong>${nodes.length} 个　›</strong></button><button data-view="rules"><span>规则集</span><strong>已启用 ${enabledRules} / ${rules.length}　›</strong></button><button data-view="capture"><span>流量分析</span><strong>${state.captureStatus?.running ? '分析器运行中' : '分析器已停止'}　›</strong></button></div>`
}

function renderNodeOptions(nodes, selectedID) {
  const groups = nodeGroups(nodes, 'subscription')
  return groups.map(group => `<optgroup label="${escapeHtml(group.label)}">${group.nodes.map(n => `<option value="${n.id}" ${n.id === selectedID ? 'selected' : ''}>${escapeHtml(n.name)}</option>`).join('')}</optgroup>`).join('')
}

function renderTrafficChart() {
  const samples = state.trafficHistory.slice(-TRAFFIC_SLOTS)
  if (!samples.length) return '<span class="traffic-chart-empty">等待状态数据</span>'
  const max = Math.max(1, ...samples.flatMap(sample => [sample.up, sample.down]))
  const empty = '<span class="traffic-bar-pair empty"></span>'.repeat(TRAFFIC_SLOTS - samples.length)
  return empty + samples.map(sample => `<span class="traffic-bar-pair" title="↑ ${formatBytes(sample.up)}/s · ↓ ${formatBytes(sample.down)}/s"><i style="height:${Math.max(2, sample.up / max * 100)}%"></i><i style="height:${Math.max(2, sample.down / max * 100)}%"></i></span>`).join('')
}

function renderNodesPage() {
  const nodes = state.config?.nodes || []
  const visibleIDs = visibleNodeIDs()
  const allSelected = visibleIDs.length > 0 && visibleIDs.every(id => state.selectedNodes.has(id))
  const selected = selectedNodeIDs().length
  const subtitle = selected ? `已选 ${selected} 个 · 共 ${nodes.length} 个节点` : `${nodes.length} 个节点 · 点击节点即可切换`
  return `<section class="page-stack">
      <div class="section-heading node-page-heading"><div><h2>节点列表</h2><p class="muted">${subtitle}</p></div><span class="node-batch-actions"><button class="btn btn-ghost" id="toggleSelectNodes" ${visibleIDs.length ? '' : 'disabled'}>${allSelected ? '取消全选' : '全选'}</button><button class="btn btn-ghost" id="batchTestNodes" ${state.batchTesting || !visibleIDs.length ? 'disabled' : ''}>${state.batchTesting ? '测试中…' : selected ? '测试选中' : '测试全部'}</button><button class="btn btn-ghost" id="batchExportNodes" ${selected ? '' : 'disabled'}>导出选中</button><button class="btn btn-danger" id="batchDeleteNodes" ${selected ? '' : 'disabled'}>删除选中</button><button class="btn btn-primary" id="toggleNodeForm">导入 / 添加</button></span></div>
      ${renderSubscriptions()}
      <div id="nodePanel">${renderNodes()}</div>
  </section>`
}

function renderNodeModal() {
  if (!state.nodeFormOpen && !state.editingNodeId) return ''
  const editing = Boolean(state.editingNodeId)
  const tab = editing ? 'manual' : state.nodeFormTab
  const body = tab === 'link'
    ? `<p class="muted modal-hint">粘贴节点分享链接（vless:// vmess:// trojan:// ss:// hy2:// tuic:// anytls:// wg:// 等，可多行，一行一个），或订阅地址（支持 Base64、Clash YAML、sing-box JSON）。</p>
      <textarea id="nodeUrl" class="input node-url-input" rows="6" placeholder="每行一个链接或订阅地址">${escapeHtml(state.manualForm.url)}</textarea>`
    : `<div class="row"><select id="nodeProtocol" class="select">${NODE_PROTOCOLS.map(([id, label]) => `<option value="${id}" ${state.manualProtocol === id ? 'selected' : ''}>${label}</option>`).join('')}</select></div>
      <div class="row"><input id="nodeName" class="input" value="${escapeHtml(state.manualForm.name)}" placeholder="节点名称" /></div>
      <div class="row"><input id="nodeServer" class="input" value="${escapeHtml(state.manualForm.server)}" placeholder="服务器地址" /><input id="nodePort" class="input port-input" value="${escapeHtml(state.manualForm.port)}" placeholder="端口" /></div>
      ${renderManualAdvancedFields()}`
  return `<div class="modal" id="nodeModal" role="dialog" aria-modal="true" aria-labelledby="nodeModalTitle"><div class="modal-backdrop" data-close-node-modal></div>
    <section class="modal-panel node-modal" id="node-import">
      <header class="modal-header"><h2 id="nodeModalTitle">${editing ? '编辑节点' : '添加节点'}</h2><button class="icon-btn" data-close-node-modal aria-label="关闭" title="关闭">×</button></header>
      ${editing ? '' : `<div class="modal-tabs" role="tablist"><button class="modal-tab ${tab === 'link' ? 'active' : ''}" data-node-form-tab="link" role="tab">粘贴链接导入</button><button class="modal-tab ${tab === 'manual' ? 'active' : ''}" data-node-form-tab="manual" role="tab">手动填写</button></div>`}
      <div class="modal-body">${body}</div>
      <footer class="modal-footer"><button class="btn btn-ghost" data-close-node-modal>取消</button>${tab === 'link' ? '<button id="importBtn" class="btn btn-primary">导入</button>' : `<button id="addNodeBtn" class="btn btn-primary">${editing ? '保存修改' : '添加节点'}</button>`}</footer>
    </section>
  </div>`
}

function renderSubscriptions() {
  const subs = state.config?.subscriptions || []
  if (!subs.length) return ''
  const nodes = state.config?.nodes || []
  const busy = Boolean(state.subUpdating)
  const items = subs.map(s => {
    const count = nodes.filter(n => n.subscription === s.id).length
    const updated = s.updated_at ? '更新于 ' + new Date(s.updated_at).toLocaleString('zh-CN') : '尚未更新'
    const updating = state.subUpdating === s.id || state.subUpdating === '*'
    return `<li class="node-item subscription-item">
        <span class="badge proxy">订阅</span>
        <div class="node-info"><div class="node-row"><span class="node-name" title="${escapeHtml(s.name)}">${escapeHtml(s.name)}</span><span class="node-server">${count} 个节点 · ${escapeHtml(updated)}</span></div></div>
        <div class="node-actions"><button class="btn btn-ghost update-sub" data-id="${s.id}" ${busy ? 'disabled' : ''}>${updating ? '更新中...' : '更新'}</button><button class="btn btn-danger rm-sub" data-id="${s.id}" ${busy ? 'disabled' : ''}>删除</button></div>
      </li>`
  }).join('')
  return `<section class="card subscription-card"><div class="section-heading"><div><h2>订阅</h2><p class="muted">更新时用订阅里的最新节点替换旧节点，下载失败会保留原节点。</p></div>${subs.length > 1 ? `<button class="btn btn-primary" id="updateAllSubs" ${busy ? 'disabled' : ''}>${state.subUpdating === '*' ? '更新中...' : '全部更新'}</button>` : ''}</div><ul class="node-list">${items}</ul></section>`
}

function subscriptionResultText(r) {
  return `${r.name}：共 ${r.total} 个节点，新增 ${r.added}，移除 ${r.removed}`
}

function renderManualAdvancedFields() {
  const fields = NODE_FIELDS[state.manualProtocol] || []
  return `<div class="advanced-fields"><div class="field-hint">${state.manualProtocol.toUpperCase()} 专用参数</div>${fields.map(renderNodeField).join('')}${renderTransportFields()}</div>`
}

function renderTransportFields() {
  const type = state.manualForm.transport_type
  if (!type || !(NODE_FIELDS[state.manualProtocol] || []).some(field => field.key === 'transport_type')) return ''
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
  return `<div class="page-stack"><section class="card"><h2>选择路由模式</h2><div class="mode-grid">${MODES.map(m => `<button class="mode-btn ${state.config?.routing_mode === m.id ? 'active' : ''}" data-mode="${m.id}">${m.name}<span class="mode-desc">${m.desc}</span></button>`).join('')}</div><div class="network-options"><div class="network-options-title">网络选项</div><label class="setting-toggle-row"><span class="setting-toggle-copy"><strong>节点失效自动切换</strong><small>当前节点连续两次测试失败时，切换到延迟最低的可用节点，优先同一订阅</small></span><span class="toggle-switch"><input id="autoSwitchToggle" type="checkbox" ${state.config?.auto_switch_disabled ? '' : 'checked'}><span aria-hidden="true"></span></span></label><label class="setting-toggle-row"><span class="setting-toggle-copy"><strong>启用 IPv6</strong><small>${escapeHtml(state.ipv6Status?.message || '正在检测设备 IPv6')}</small></span><span class="toggle-switch"><input id="ipv6Toggle" type="checkbox" ${state.config?.ipv6_enabled ? 'checked' : ''}><span aria-hidden="true"></span></span></label><label class="setting-toggle-row"><span class="setting-toggle-copy"><strong>启用 WebRTC 防泄漏</strong><small>防止 WebRTC 通过直连暴露真实 IPv4/IPv6</small></span><span class="toggle-switch"><input id="webrtcProtectionToggle" type="checkbox" ${state.config?.webrtc_protection_enabled ? 'checked' : ''}><span aria-hidden="true"></span></span></label><label class="setting-toggle-row"><span class="setting-toggle-copy"><strong>启用诊断信息</strong><small>显示当前 selector 和运行节点，仅用于排查问题</small></span><span class="toggle-switch"><input id="diagnosticsToggle" type="checkbox" ${state.config?.diagnostics_enabled ? 'checked' : ''}><span aria-hidden="true"></span></span></label></div></section>${state.config?.diagnostics_enabled ? `<section class="card"><h2>运行诊断</h2><div class="diagnostics-grid"><span>当前配置节点</span><strong>${escapeHtml(diagnostics?.current_node_id || '未选择')}</strong><span>实际 selector</span><strong>${escapeHtml(diagnostics?.selector || '未读取')}</strong><span>状态</span><strong>${escapeHtml(diagnostics?.message || '读取中')}</strong></div></section>` : ''}
    ${state.config?.routing_mode === 'custom' ? `<section class="card"><div class="section-heading"><div><h2>自定义分流规则</h2><p class="muted">先选择匹配对象，再填写内容和处理方式。</p></div></div><div id="customRulesPanel">${renderCustomRules()}</div><div class="rule-editor"><label class="rule-field"><span>匹配对象</span><select id="ruleMatchType" class="select">${RULE_MATCH_TYPES.map(item => `<option value="${item.id}" ${item.id === matchType.id ? 'selected' : ''}>${item.label}</option>`).join('')}</select></label><label class="rule-field"><span>匹配内容</span><input id="ruleValue" class="input" value="${escapeHtml(state.customRuleForm.value)}" placeholder="${matchType.placeholder}" /></label><label class="rule-field"><span>处理方式</span><select id="ruleAction" class="select">${RULE_ACTIONS.map(item => `<option value="${item.id}" ${item.id === action.id ? 'selected' : ''}>${item.label}</option>`).join('')}</select></label><button id="addRuleBtn" class="btn btn-primary">添加规则</button><small class="rule-hint value-hint" id="ruleValueHint">${matchType.hint}</small><small class="rule-hint action-hint" id="ruleActionHint">${action.hint}</small></div><details class="rule-import" open><summary>批量导入规则</summary><div class="rule-import-body"><textarea id="ruleImport" class="input" rows="3" placeholder="每行一条，例如：proxy,domain_suffix,example.com"></textarea><button id="importRuleBtn" class="btn btn-ghost">导入</button></div></details></section>` : ''}
  </div>`
}

function renderRulesPage() {
  const filter = state.ruleFilter || 'all'
  return `<div class="page-stack rules-page">
    <div class="rules-tabs" role="tablist" aria-label="规则页面"><button class="filter-btn ${state.rulesTab === 'sets' ? 'active' : ''}" data-rules-tab="sets" role="tab" aria-selected="${state.rulesTab === 'sets'}">规则集</button><button class="filter-btn ${state.rulesTab === 'manual' ? 'active' : ''}" data-rules-tab="manual" role="tab" aria-selected="${state.rulesTab === 'manual'}">手动过滤</button></div>
    ${state.rulesTab === 'sets' ? `
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
          <div class="rule-form-actions"><button id="addRuleSetBtn" class="btn btn-primary">${state.editingRuleSetID ? '保存修改' : '添加规则集'}</button>${state.editingRuleSetID ? '<button id="cancelEditRuleSet" class="btn btn-ghost">取消</button>' : '<a class="btn btn-ghost rule-source-link" href="https://github.com/razaxq/dns-blocklists-sing-box/blob/main/README_zh.md" target="_blank" rel="noopener noreferrer">查找规则集</a><button id="restoreDefaultRules" class="btn btn-ghost">恢复默认规则</button>'}</div>
        </aside>
      </div>
    </section>
    ` : `
    <section class="card dns-filter-card">
      <div class="section-heading"><div><h2>手动 DNS 过滤规则</h2><p class="muted">单独添加域名或 IP，命中后直接阻断。优先级高于规则集。</p></div><button id="openFilterRuleBtn" class="btn btn-primary">添加过滤规则</button></div>
      ${state.dnsFilterFormOpen ? `<div class="dns-filter-editor"><label class="rule-form-field"><span>匹配对象</span><select id="filterRuleMatchType" class="select">${RULE_MATCH_TYPES.filter(item => item.id !== 'port').map(item => `<option value="${item.id}">${item.label}</option>`).join('')}</select></label><label class="rule-form-field"><span>匹配内容</span><input id="filterRuleValue" class="input" placeholder="例如 ads.example.com" /></label><div class="rule-form-actions"><button id="addFilterRuleBtn" class="btn btn-primary">添加规则</button><button id="cancelFilterRuleBtn" class="btn btn-ghost">取消</button></div></div>` : ''}
      ${renderDNSFilterRules()}
      <div class="rules-toolbar dns-filter-toolbar"><span class="rules-summary-note">共 ${(state.config?.dns_filter_rules || []).length} 条手动过滤规则</span></div>
    </section>
    `}
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
      <label class="capture-toggle"><span>启用分析</span><span class="toggle-switch"><input id="captureEnabled" type="checkbox" ${enabled ? 'checked' : ''}><span aria-hidden="true"></span></span></label>
    </section>
    <details class="card capture-settings" id="captureSettings" ${state.captureSettingsOpen ? 'open' : ''}>
      <summary class="capture-settings-summary"><div><h2>抓包规则与证书</h2><p class="muted">每行一个域名或关键词；输入 * 抓取全部 HTTP/HTTPS 请求。</p></div><span class="capture-settings-actions" aria-hidden="true">⌄</span></summary>
      <div class="capture-settings-content"><div class="section-heading"><span></span><button id="saveCapture" class="btn btn-primary">保存并应用</button></div>
      <textarea id="captureDomains" class="input" rows="4" placeholder="例如 example.com、google；输入 * 抓取全部 HTTP/HTTPS">${escapeHtml(domains)}</textarea>
      <div class="capture-settings-footer"><div class="capture-cert"><span class="badge ${status.certificate_installed ? 'direct' : ''}">${status.certificate_installed ? `证书已安装（${escapeHtml(status.certificate_level || '未知级别')}）` : '证书未安装'}</span>${status.user_certificate_supported ? '<select id="captureCertLevel" class="input capture-cert-level"><option value="system">系统级</option><option value="user">当前用户</option></select>' : '<span class="muted">Linux 使用系统级证书</span>'}<button id="installCaptureCA" class="btn btn-ghost">安装系统证书</button><button id="uninstallCaptureCA" class="btn btn-ghost">卸载系统证书</button></div><span class="capture-storage" title="${escapeHtml(status.storage_path || '')}">${escapeHtml(status.storage_path || 'capture.json')}</span></div>
      </div>
    </details>
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
    case 'request-body': content = `<div class="capture-tab-content"><h2>请求正文${flow.request_truncated ? '（已截断）' : ''}</h2>${renderCaptureBody(flow, 'request-body')}</div>`; break
    case 'response-body': content = `<div class="capture-tab-content"><h2>响应正文${flow.response_truncated ? '（已截断）' : ''}</h2>${renderCaptureBody(flow, 'response-body')}</div>`; break
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

function renderCaptureBody(flow, tab) {
  const response = tab === 'response-body'
  const body = response ? flow.response_body : flow.request_body
  const encoding = response ? flow.response_encoding : flow.request_encoding
  const headers = response ? flow.response_headers : flow.request_headers
  const contentType = captureHeader(headers, 'content-type')
  if (!body) return '<div class="empty">无</div>'
  const formatted = formatCaptureBody(body, encoding, contentType)
  return `<div class="capture-body-viewer"><div class="capture-body-toolbar"><span class="capture-body-meta">${escapeHtml(contentType || (encoding === 'base64' ? '二进制内容' : '文本内容'))} · ${formatBytes(body.length)}</span><span>${encoding === 'base64' ? '<span class="badge">Base64 编码</span>' : ''}${formatted.pretty ? '<span class="badge direct">已格式化</span>' : ''}</span><button class="btn btn-ghost capture-expand-body" data-capture-body-id="${flow.id}" data-capture-body-tab="${tab}">展开查看</button></div><pre data-capture-body-scroll="${flow.id}:${tab}">${escapeHtml(formatted.text)}</pre></div>`
}

function renderNodeQRCodeModal() {
  const qr = state.nodeQRCode
  if (!qr) return ''
  return `<div class="modal" id="nodeQRCodeModal" role="dialog" aria-modal="true" aria-labelledby="nodeQRCodeTitle"><div class="modal-backdrop" data-close-node-qrcode></div>
    <section class="modal-panel qrcode-modal">
      <header class="modal-header"><h2 id="nodeQRCodeTitle">${escapeHtml(qr.name)}</h2><button class="icon-btn" data-close-node-qrcode aria-label="关闭" title="关闭">×</button></header>
      <div class="modal-body qrcode-body"><img class="qrcode-image" src="${qr.image}" alt="节点分享链接二维码"><p class="muted modal-hint">用手机客户端扫码即可导入该节点</p><code class="qrcode-link">${escapeHtml(qr.link)}</code></div>
      <footer class="modal-footer"><button class="btn btn-ghost" data-close-node-qrcode>关闭</button><button class="btn btn-primary" id="copyNodeQRCodeLink">复制链接</button></footer>
    </section></div>`
}

function closeNodeQRCode() {
  state.nodeQRCode = null
  renderApp()
}

function renderCaptureBodyModal() {
  const viewer = state.captureBodyViewer
  if (!viewer) return ''
  const formatted = formatCaptureBody(viewer.body, viewer.encoding, viewer.contentType, viewer.mode === 'pretty')
  return `<div class="capture-body-modal" id="captureBodyModal" role="dialog" aria-modal="true" aria-labelledby="captureBodyTitle"><div class="capture-body-modal-backdrop" data-close-capture-body></div><section class="capture-body-modal-panel"><header class="capture-body-modal-header"><div><h2 id="captureBodyTitle">${escapeHtml(viewer.title)}</h2><p class="muted">${escapeHtml(viewer.contentType || (viewer.encoding === 'base64' ? '二进制内容' : '文本内容'))} · ${formatBytes(viewer.body.length)}${viewer.truncated ? ' · 已截断' : ''}</p></div><button class="icon-btn" data-close-capture-body aria-label="关闭正文查看器" title="关闭">×</button></header><div class="capture-body-modal-actions"><button class="btn btn-ghost ${viewer.mode === 'pretty' ? 'active' : ''}" data-capture-body-mode="pretty">美化</button><button class="btn btn-ghost ${viewer.mode === 'raw' ? 'active' : ''}" data-capture-body-mode="raw">原文</button><button class="btn btn-primary" data-copy-capture-body>复制</button></div><pre class="capture-body-modal-content">${escapeHtml(formatted.text)}</pre></section></div>`
}

function captureHeader(headers, name) {
  const key = Object.keys(headers || {}).find(item => item.toLowerCase() === name.toLowerCase())
  return key ? (headers[key] || []).join(', ') : ''
}

function formatCaptureBody(body, encoding, contentType, pretty = true) {
  const prefix = encoding === 'base64' ? 'Base64\n' : ''
  if (!pretty || encoding === 'base64') return { text: prefix + body, pretty: false }
  const mediaType = (contentType || '').split(';', 1)[0].trim().toLowerCase()
  if (mediaType.includes('json') || looksLikeJSON(body)) {
    try { return { text: JSON.stringify(JSON.parse(body), null, 2), pretty: true } } catch (_) {}
  }
  if (mediaType === 'application/x-www-form-urlencoded') {
    try { return { text: new URLSearchParams(body).toString().replaceAll('&', '\n'), pretty: true } } catch (_) {}
  }
  return { text: body, pretty: false }
}

function looksLikeJSON(body) {
  const value = body.trim()
  return (value.startsWith('{') && value.endsWith('}')) || (value.startsWith('[') && value.endsWith(']'))
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
      <div class="rule-set-main"><div class="rule-set-title"><strong>${escapeHtml(r.name)}</strong><span class="rule-tag ${r.source === 'default' ? 'default' : 'custom'}">${r.source === 'default' ? '默认' : '用户添加'}</span></div><div class="rule-set-file">${r.exists ? escapeHtml(formatRuleInfo(r)) : `<button class="rule-inline-download update-rule" data-id="${r.id}" ${state.ruleUpdating || state.allRulesUpdating ? 'disabled' : ''}>${state.ruleUpdating === r.id ? '下载中...' : '立即下载'}</button>`}</div></div>
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
  const filterBtns = [['all', '全部'], ...NODE_PROTOCOLS.map(([id, label]) => [id, id === 'shadowsocks' ? 'SS' : label])]
  const toolsHtml = `<div class="node-tools">
      <div class="node-tools-row"><input id="nodeSearch" class="input" value="${escapeHtml(state.nodeSearch)}" placeholder="搜索名称、服务器或协议" aria-label="搜索节点">
        <select id="nodeGroupBy" class="select node-tool-select" aria-label="分组方式"><option value="protocol" ${state.nodeGroupBy === 'protocol' ? 'selected' : ''}>按协议分组</option><option value="subscription" ${state.nodeGroupBy === 'subscription' ? 'selected' : ''}>按订阅分组</option></select>
        <select id="nodeSort" class="select node-tool-select" aria-label="排序方式"><option value="default" ${state.nodeSort === 'default' ? 'selected' : ''}>默认顺序</option><option value="latency" ${state.nodeSort === 'latency' ? 'selected' : ''}>延迟从低到高</option></select></div>
      <div class="node-filters">${filterBtns.map(([id, label]) => {
    const count = id === 'all' ? allNodes.length : allNodes.filter(n => n.protocol === id).length
    if (!count && id !== 'all' && state.nodeFilter !== id) return ''
    return `<button class="filter-btn ${state.nodeFilter === id ? 'active' : ''}" data-filter="${id}">${label} (${count})</button>`
  }).join('')}</div></div>`
  const visible = new Set(visibleNodeIDs())
  const filtered = allNodes.filter(n => visible.has(n.id))
  if (!allNodes.length) return toolsHtml + '<div class="empty">还没有节点，点右上角“导入 / 添加”</div>'
  if (!filtered.length) return toolsHtml + '<div class="empty">没有匹配的节点</div>'
  const bySubscription = state.nodeGroupBy === 'subscription'
  const row = n => {
    const current = currentNodeID === n.id
    const testing = state.nodeTesting.has(n.id)
    return `<li class="node-item ${current ? 'active' : ''} ${state.nodeSwitching ? 'busy' : ''}" data-node-id="${n.id}" tabindex="0" title="${current ? '当前节点' : '点击切换到此节点'}">
        <input class="node-select" type="checkbox" data-id="${n.id}" ${state.selectedNodes.has(n.id) ? 'checked' : ''} aria-label="选择 ${escapeHtml(n.name)}" />
        <div class="node-info">
          <div class="node-row"><span class="node-name">${escapeHtml(n.name)}</span>${current ? '<span class="node-current-tag">当前</span>' : ''}</div>
          <div class="node-meta">${bySubscription ? `<span class="node-proto">${escapeHtml(protocolLabel(n.protocol))}</span>` : ''}<span class="node-server">${escapeHtml(n.server)}:${n.port}</span></div>
        </div>
        ${renderLatency(n.id)}
        <div class="node-actions">
          <button class="btn btn-ghost test-node" data-id="${n.id}" ${testing ? 'disabled' : ''}>测试</button>
          <details class="rule-menu node-menu" data-rule-menu-id="node-${n.id}"><summary aria-label="节点操作" title="更多操作">⋯</summary><div class="rule-menu-list"><button class="rule-menu-action edit-node" data-id="${n.id}">编辑</button><button class="rule-menu-action export-node" data-id="${n.id}">复制分享链接</button><button class="rule-menu-action qrcode-node" data-id="${n.id}">显示二维码</button><button class="rule-menu-action danger rm-node" data-id="${n.id}">删除</button></div></details>
        </div>
      </li>`
  }
  const groupsHtml = nodeGroups(filtered, state.nodeGroupBy).map(group => {
    const nodes = state.nodeSort === 'latency' ? [...group.nodes].sort((a, b) => latencySortValue(a.id) - latencySortValue(b.id)) : group.nodes
    return `<div class="node-group"><div class="node-group-header"><span class="badge proxy" title="${escapeHtml(group.label)}">${escapeHtml(group.label)}</span><span class="node-group-count">${nodes.length} 个</span></div><ul class="node-list">${nodes.map(row).join('')}</ul></div>`
  }).join('')
  return toolsHtml + groupsHtml
}

function protocolLabel(protocol) {
  return NODE_PROTOCOLS.find(([id]) => id === protocol)?.[1] || String(protocol || '未知').toUpperCase()
}

// nodeGroups keeps protocol groups in NODE_PROTOCOLS order and subscription
// groups in subscription order, with manually added nodes last.
function nodeGroups(nodes, groupBy) {
  const subs = state.config?.subscriptions || []
  const keyOf = n => groupBy === 'subscription' ? (n.subscription || '') : (n.protocol || 'unknown')
  const order = groupBy === 'subscription' ? [...subs.map(s => s.id), ''] : NODE_PROTOCOLS.map(([id]) => id)
  const labelOf = key => groupBy === 'subscription'
    ? (key ? subs.find(s => s.id === key)?.name || '已删除的订阅' : '手动添加')
    : protocolLabel(key)
  const groups = new Map()
  for (const n of nodes) {
    const key = keyOf(n)
    if (!groups.has(key)) groups.set(key, [])
    groups.get(key).push(n)
  }
  const rank = key => { const i = order.indexOf(key); return i === -1 ? order.length : i }
  return [...groups.entries()].sort((a, b) => rank(a[0]) - rank(b[0])).map(([key, list]) => ({ key, label: labelOf(key), nodes: list }))
}

function loadNodeHealth() {
  try {
    const saved = JSON.parse(localStorage.getItem('sbtun-ui-node-health') || '{}')
    return saved && typeof saved === 'object' ? saved : {}
  } catch {
    return {}
  }
}

function saveNodeHealth() {
  const ids = new Set((state.config?.nodes || []).map(n => n.id))
  if (state.config) {
    for (const id of Object.keys(state.nodeHealth)) if (!ids.has(id)) delete state.nodeHealth[id]
  }
  localStorage.setItem('sbtun-ui-node-health', JSON.stringify(state.nodeHealth))
}

// nodeLatency prefers the proxied URL delay; without one (no kernel to run a
// URL test) it falls back to the TCP handshake time.
function nodeLatency(id) {
  const health = state.nodeHealth[id]
  if (!health) return null
  if (health.url?.ok) return { ok: true, ms: health.url.latency_ms || 0, label: 'URL 延迟' }
  if (!health.url?.message && health.port?.ok) return { ok: true, ms: health.transport === 'udp' ? 0 : health.port.latency_ms || 0, label: health.transport === 'udp' ? 'UDP 可发送' : 'TCPing' }
  return { ok: false, ms: 0, label: health.message || '不可用' }
}

function healthDetail(id) {
  const health = state.nodeHealth[id]
  if (!health) return ''
  const line = (label, result) => result?.message ? `${label}：${result.message}${result.latency_ms ? ` (${result.latency_ms} ms)` : ''}` : ''
  return [line('Ping', health.ping), line(health.transport === 'udp' ? 'UDP' : 'TCPing', health.port), line('URL', health.url)].filter(Boolean).join('\n') || health.message || ''
}

function renderLatency(id) {
  if (state.nodeTesting.has(id)) return '<span class="latency testing">测试中</span>'
  const latency = nodeLatency(id)
  if (!latency) return '<span class="latency none">未测试</span>'
  const title = escapeHtml(healthDetail(id))
  if (!latency.ok) return `<span class="latency bad" title="${title}">不可用</span>`
  if (!latency.ms) return `<span class="latency good" title="${title}">可用</span>`
  const level = latency.ms < 300 ? 'good' : latency.ms < 800 ? 'mid' : 'bad'
  return `<span class="latency ${level}" title="${title}">${latency.ms} ms</span>`
}

function latencySortValue(id) {
  const latency = nodeLatency(id)
  if (!latency) return Number.MAX_SAFE_INTEGER
  if (!latency.ok) return Number.MAX_SAFE_INTEGER - 1
  return latency.ms || 1
}

async function testNodes(ids) {
  const queue = ids.filter(id => !state.nodeTesting.has(id))
  if (!queue.length) return
  queue.forEach(id => state.nodeTesting.add(id))
  renderApp()
  const worker = async () => {
    while (queue.length) {
      const id = queue.shift()
      try {
        state.nodeHealth[id] = await window.go.app.App.TestNode(id)
      } catch (e) {
        state.nodeHealth[id] = { node_id: id, healthy: false, message: e.message || String(e) }
      }
      state.nodeTesting.delete(id)
      saveNodeHealth()
      renderApp()
    }
  }
  await Promise.all(Array.from({ length: Math.min(NODE_TEST_CONCURRENCY, queue.length) }, worker))
}

async function selectNode(id) {
  if (!id || state.nodeSwitching || id === activeNodeID()) return
  state.nodeSwitching = true
  renderApp()
  try {
    await window.go.app.App.SelectNode(id)
    await refreshConfig()
    await refreshStatus()
    await refreshRules()
    showToast('已切换节点', 'success')
  } catch (e) {
    showToast(e.message || String(e), 'error')
  } finally {
    state.nodeSwitching = false
    renderApp()
  }
}

function closeNodeModal() {
  if (state.editingNodeId) resetManualForm()
  state.editingNodeId = ''
  state.nodeFormOpen = false
  renderApp()
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
    state.trafficHistory.push({ up: state.traffic.up, down: state.traffic.down })
    if (state.trafficHistory.length > TRAFFIC_SLOTS) state.trafficHistory.shift()
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

function recentAutoSwitch() {
  const event = state.autoSwitch?.event
  if (!event?.seq || !event.ok || !state.running) return null
  return Date.now() / 1000 - event.at <= AUTO_SWITCH_BADGE_SECONDS ? event : null
}

function renderAutoSwitchBadge() {
  const event = recentAutoSwitch()
  return event ? `<span class="badge auto-switched" title="${escapeHtml(event.message)}">已自动切换</span>` : ''
}

// refreshAutoSwitch toasts new failover events and merges the latencies the
// background monitor measured into the node list.
async function refreshAutoSwitch() {
  try {
    const previous = state.autoSwitch
    const next = await window.go.app.App.GetAutoSwitchStatus()
    state.autoSwitch = next
    const event = next.event
    const newEvent = previous && event?.seq && event.seq !== previous.event?.seq
    let changed = newEvent || !!recentAutoSwitch() !== !!document.querySelector('.badge.auto-switched')
    if (next.health_seq && next.health_seq !== previous?.health_seq) {
      for (const health of await window.go.app.App.GetBackgroundNodeHealth()) {
        if (!state.nodeTesting.has(health.node_id)) state.nodeHealth[health.node_id] = health
      }
      saveNodeHealth()
      changed = true
    }
    if (changed) renderApp()
    // The toast element is part of the rendered page, so show it after rendering.
    if (newEvent) showToast(event.message, event.ok ? 'success' : 'error')
  } catch (e) {
    console.error('read auto switch status failed:', e)
  }
}

async function checkAppUpdate() {
  try {
    state.appUpdate = await window.go.app.App.CheckAppUpdate()
    renderApp()
  } catch (e) {
    console.error('check sbtun update failed:', e)
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
  const dot = document.querySelector('.status-bar .dot')
  if (!statusText || !power || !dot) {
    renderApp()
    return
  }
  statusText.textContent = statusLabel(state.statusState, state.statusMessage)
  if (statusTraffic) statusTraffic.textContent = formatTraffic(state.traffic)
  const statusNode = document.querySelector('#statusNode')
  const statusMode = document.querySelector('#statusMode')
  const current = state.config?.nodes?.find(node => node.id === activeNodeID())
  const mode = MODES.find(item => item.id === state.config?.routing_mode)
  if (statusNode) statusNode.textContent = '节点：' + (current?.name || '未选择')
  if (statusMode) statusMode.textContent = '模式：' + (mode?.name || '未设置')
  const trafficUp = document.querySelector('#trafficUp')
  const trafficDown = document.querySelector('#trafficDown')
  if (trafficUp) trafficUp.textContent = formatBytes(state.traffic.up) + '/s'
  if (trafficDown) trafficDown.textContent = formatBytes(state.traffic.down) + '/s'
  const trafficChart = document.querySelector('#trafficChart')
  if (trafficChart) trafficChart.innerHTML = renderTrafficChart()
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
    document.addEventListener('keydown', event => {
      if (event.key !== 'Escape' || state.captureBodyViewer) return
      if (state.nodeQRCode) closeNodeQRCode()
      else if (state.nodeFormOpen || state.editingNodeId) closeNodeModal()
    })
    state.ruleMenuDocumentBound = true
  }
  document.querySelector('#appUpdate')?.addEventListener('click', () => {
    if (state.appUpdate?.release_url) window.runtime.BrowserOpenURL(state.appUpdate.release_url)
  })
  const themeToggle = document.querySelector('#themeToggle')
  if (themeToggle) {
    themeToggle.addEventListener('click', () => {
      state.theme = nextTheme().id
      localStorage.setItem('sbtun-theme', state.theme)
      renderApp()
    })
  }
  document.querySelectorAll('[data-view]').forEach(item => {
    item.addEventListener('click', async () => {
      state.view = item.dataset.view
      localStorage.setItem('sbtun-ui-view', state.view)
      if (state.view === 'routing') await refreshIPv6Status()
      if (state.view === 'capture') await refreshCapture()
      renderApp()
    })
  })
  const nodeSearch = document.querySelector('#nodeSearch')
  if (nodeSearch) nodeSearch.addEventListener('input', () => {
    state.nodeSearch = nodeSearch.value
    localStorage.setItem('sbtun-ui-node-search', state.nodeSearch)
    renderApp()
  })
  const toggleNodeForm = document.querySelector('#toggleNodeForm')
  if (toggleNodeForm) toggleNodeForm.addEventListener('click', () => {
    resetManualForm()
    state.editingNodeId = ''
    state.nodeFormTab = 'link'
    state.nodeFormOpen = true
    renderApp()
    document.querySelector('#nodeUrl')?.focus()
  })
  document.querySelectorAll('[data-close-node-modal]').forEach(element => element.addEventListener('click', closeNodeModal))
  document.querySelectorAll('[data-node-form-tab]').forEach(button => button.addEventListener('click', () => {
    state.nodeFormTab = button.dataset.nodeFormTab
    renderApp()
  }))
  const nodeGroupBy = document.querySelector('#nodeGroupBy')
  if (nodeGroupBy) nodeGroupBy.addEventListener('change', () => {
    state.nodeGroupBy = nodeGroupBy.value
    localStorage.setItem('sbtun-ui-node-group', state.nodeGroupBy)
    renderApp()
  })
  const nodeSort = document.querySelector('#nodeSort')
  if (nodeSort) nodeSort.addEventListener('change', () => {
    state.nodeSort = nodeSort.value
    localStorage.setItem('sbtun-ui-node-sort', state.nodeSort)
    renderApp()
  })
  const quickNodeSelect = document.querySelector('#quickNodeSelect')
  if (quickNodeSelect) quickNodeSelect.addEventListener('change', () => selectNode(quickNodeSelect.value))
  const testCurrentNode = document.querySelector('#testCurrentNode')
  if (testCurrentNode) testCurrentNode.addEventListener('click', () => testNodes([activeNodeID()].filter(Boolean)))
  document.querySelectorAll('[data-rules-tab]').forEach(button => button.addEventListener('click', () => {
    state.rulesTab = button.dataset.rulesTab
    localStorage.setItem('sbtun-ui-rules-tab', state.rulesTab)
    renderApp()
  }))
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

  const autoSwitchToggle = document.querySelector('#autoSwitchToggle')
  if (autoSwitchToggle) {
    autoSwitchToggle.addEventListener('change', async () => {
      const enabled = autoSwitchToggle.checked
      try {
        await window.go.app.App.SetAutoSwitch(enabled)
        await refreshConfig()
        showToast(enabled ? '已开启自动切换' : '已关闭自动切换', 'success')
      } catch (e) {
        autoSwitchToggle.checked = !enabled
        showToast(e.message || String(e), 'error')
      }
    })
  }
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

  const webrtcProtectionToggle = document.querySelector('#webrtcProtectionToggle')
  if (webrtcProtectionToggle) {
    webrtcProtectionToggle.addEventListener('change', async () => {
      const cfg = JSON.parse(JSON.stringify(state.config))
      cfg.webrtc_protection_enabled = webrtcProtectionToggle.checked
      webrtcProtectionToggle.disabled = true
      try {
        await window.go.app.App.SaveConfig(cfg)
        state.config = cfg
        renderApp()
        showToast(cfg.webrtc_protection_enabled ? 'WebRTC 防泄漏已启用' : 'WebRTC 防泄漏已关闭', 'success')
      } catch (e) {
        webrtcProtectionToggle.checked = !webrtcProtectionToggle.checked
        showToast(e.message || String(e), 'error')
      } finally {
        webrtcProtectionToggle.disabled = false
      }
    })
  }

  const captureDomains = document.querySelector('#captureDomains')
  const captureSettings = document.querySelector('#captureSettings')
  if (captureSettings) captureSettings.addEventListener('toggle', () => {
    state.captureSettingsOpen = captureSettings.open
    localStorage.setItem('sbtun-ui-capture-settings-open', String(captureSettings.open))
  })
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
    const domains = [...new Set(readCaptureDomains())]
    if (captureEnabled.checked && !domains.length) {
      captureEnabled.checked = false
      state.captureEnabledDraft = false
      state.captureEnabledDirty = false
      showToast('请先填写抓包规则；输入 * 可分析全部 HTTP/HTTPS 请求', 'error')
      captureDomains?.focus()
      return
    }
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
  document.querySelectorAll('.capture-expand-body').forEach(button => button.addEventListener('click', () => {
    const flow = state.selectedCaptureFlow?.id === Number(button.dataset.captureBodyId)
      ? state.selectedCaptureFlow
      : state.captureFlows.find(item => item.id === Number(button.dataset.captureBodyId))
    if (!flow) return
    const response = button.dataset.captureBodyTab === 'response-body'
    state.captureBodyViewer = {
      title: response ? '响应正文' : '请求正文',
      body: response ? flow.response_body : flow.request_body,
      encoding: response ? flow.response_encoding : flow.request_encoding,
      contentType: captureHeader(response ? flow.response_headers : flow.request_headers, 'content-type'),
      truncated: response ? flow.response_truncated : flow.request_truncated,
      mode: 'pretty',
    }
    renderApp()
  }))
  document.querySelectorAll('[data-close-capture-body]').forEach(element => element.addEventListener('click', () => {
    state.captureBodyViewer = null
    renderApp()
  }))
  document.querySelectorAll('[data-capture-body-mode]').forEach(button => button.addEventListener('click', () => {
    if (!state.captureBodyViewer) return
    state.captureBodyViewer.mode = button.dataset.captureBodyMode
    renderApp()
  }))
  const copyCaptureBody = document.querySelector('[data-copy-capture-body]')
  if (copyCaptureBody) copyCaptureBody.addEventListener('click', async () => {
    if (!state.captureBodyViewer) return
    try {
      await window.runtime.ClipboardSetText(state.captureBodyViewer.body)
      showToast('正文已复制', 'success')
    } catch (e) {
      showToast(e.message || String(e), 'error')
    }
  })
  if (state.captureBodyViewer) {
    document.addEventListener('keydown', function closeCaptureBodyOnEscape(event) {
      if (event.key !== 'Escape' || !state.captureBodyViewer) return
      state.captureBodyViewer = null
      renderApp()
      document.removeEventListener('keydown', closeCaptureBodyOnEscape)
    }, { once: true })
  }

  document.querySelectorAll('.node-item[data-node-id]').forEach(row => {
    row.addEventListener('click', event => {
      if (event.target.closest('button, input, details, a')) return
      selectNode(row.dataset.nodeId)
    })
    row.addEventListener('keydown', event => {
      if (event.target !== row || (event.key !== 'Enter' && event.key !== ' ')) return
      event.preventDefault()
      selectNode(row.dataset.nodeId)
    })
  })

  document.querySelectorAll('.update-sub').forEach(btn => {
    btn.addEventListener('click', async () => {
      state.subUpdating = btn.dataset.id
      renderApp()
      try {
        const result = await window.go.app.App.UpdateSubscription(btn.dataset.id)
        showToast('订阅已更新 · ' + subscriptionResultText(result), 'success')
      } catch (e) {
        showToast('订阅更新失败: ' + (e.message || String(e)), 'error')
      } finally {
        state.subUpdating = ''
        await refreshConfig()
        renderApp()
      }
    })
  })

  const updateAllSubs = document.querySelector('#updateAllSubs')
  if (updateAllSubs) {
    updateAllSubs.addEventListener('click', async () => {
      state.subUpdating = '*'
      renderApp()
      try {
        const results = await window.go.app.App.UpdateAllSubscriptions()
        const failed = results.filter(r => r.error)
        if (failed.length) showToast('部分订阅更新失败: ' + failed.map(r => `${r.name || r.id}: ${r.error}`).join('; '), 'error')
        else showToast('全部订阅已更新', 'success')
      } catch (e) {
        showToast(e.message || String(e), 'error')
      } finally {
        state.subUpdating = ''
        await refreshConfig()
        renderApp()
      }
    })
  }

  document.querySelectorAll('.rm-sub').forEach(btn => {
    btn.addEventListener('click', async () => {
      const sub = state.config?.subscriptions?.find(item => item.id === btn.dataset.id)
      if (!window.confirm(`确定删除订阅“${sub?.name || '此订阅'}”及其全部节点吗？`)) return
      try {
        await window.go.app.App.RemoveSubscription(btn.dataset.id)
        await refreshConfig()
        showToast('订阅已删除', 'success')
      } catch (e) {
        showToast(e.message || String(e), 'error')
      }
    })
  })

  document.querySelectorAll('.rm-node').forEach(btn => {
    btn.addEventListener('click', async () => {
      const node = state.config?.nodes?.find(item => item.id === btn.dataset.id)
      if (!window.confirm(`确定删除节点“${node?.name || '此节点'}”吗？`)) return
      try {
        await window.go.app.App.RemoveNode(btn.dataset.id)
        state.selectedNodes.delete(btn.dataset.id)
        await refreshConfig()
        await refreshRules()
        saveNodeHealth()
        renderApp()
        showToast('节点已删除', 'success')
      } catch (e) {
        showToast(e.message, 'error')
      }
    })
  })

  const toggleSelectNodes = document.querySelector('#toggleSelectNodes')
  document.querySelectorAll('.node-select').forEach(input => {
    input.addEventListener('change', () => {
      if (input.checked) state.selectedNodes.add(input.dataset.id)
      else state.selectedNodes.delete(input.dataset.id)
      renderApp()
    })
  })
  if (toggleSelectNodes) toggleSelectNodes.addEventListener('click', () => {
    const ids = visibleNodeIDs()
    const allSelected = ids.length > 0 && ids.every(id => state.selectedNodes.has(id))
    ids.forEach(id => {
      if (allSelected) state.selectedNodes.delete(id)
      else state.selectedNodes.add(id)
    })
    renderApp()
  })
  const selectedIDs = selectedNodeIDs
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
      saveNodeHealth()
      renderApp()
      showToast('批量删除成功', 'success')
    } catch (e) { showToast(e.message || String(e), 'error') }
    finally { batchDelete.disabled = false }
  })
  const batchTest = document.querySelector('#batchTestNodes')
  if (batchTest) batchTest.addEventListener('click', async () => {
    const selected = selectedIDs()
    const ids = selected.length ? selected : visibleNodeIDs()
    if (!ids.length) return
    state.batchTesting = true
    try {
      await testNodes(ids)
      const ok = ids.filter(id => nodeLatency(id)?.ok).length
      showToast(`测试完成：${ok} / ${ids.length} 个可用`, ok ? 'success' : 'error')
    } finally {
      state.batchTesting = false
      renderApp()
    }
  })
  document.querySelectorAll('.node-filters .filter-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      state.nodeFilter = btn.dataset.filter
      localStorage.setItem('sbtun-ui-node-filter', state.nodeFilter)
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
  document.querySelectorAll('.qrcode-node').forEach(btn => {
    btn.addEventListener('click', async () => {
      const node = state.config.nodes.find(item => item.id === btn.dataset.id)
      if (!node) return
      btn.disabled = true
      try {
        const qr = await window.go.app.App.NodeQRCode(node.id)
        state.nodeQRCode = { name: node.name, link: qr.link, image: qr.image }
        renderApp()
      } catch (e) {
        showToast(e.message || String(e), 'error')
      } finally {
        btn.disabled = false
      }
    })
  })
  document.querySelectorAll('[data-close-node-qrcode]').forEach(element => element.addEventListener('click', closeNodeQRCode))
  document.querySelector('#copyNodeQRCodeLink')?.addEventListener('click', async () => {
    if (!state.nodeQRCode) return
    try {
      await window.runtime.ClipboardSetText(state.nodeQRCode.link)
      showToast('节点链接已复制', 'success')
    } catch (e) {
      showToast(e.message || String(e), 'error')
    }
  })
  document.querySelectorAll('.test-node').forEach(btn => {
    btn.addEventListener('click', async () => {
      await testNodes([btn.dataset.id])
      const latency = nodeLatency(btn.dataset.id)
      if (latency && !latency.ok) showToast(latency.label, 'error')
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
      const links = state.manualForm.url.split(/\r?\n/).map(line => line.trim()).filter(Boolean)
      if (!links.length) { showToast('请输入节点或订阅链接', 'error'); return }
      importBtn.disabled = true
      importBtn.textContent = '导入中…'
      let added = 0
      const failed = []
      for (const link of links) {
        try {
          added += await window.go.app.App.ImportSubscription(link)
        } catch (e) {
          failed.push({ link, error: e.message || String(e) })
        }
      }
      await refreshConfig()
      await refreshRules()
      // 失败的链接留在输入框里，方便修改后重试。
      state.manualForm.url = failed.map(item => item.link).join('\n')
      state.nodeFormOpen = failed.length > 0
      renderApp()
      if (!failed.length) showToast(`导入成功，新增 ${added} 个节点`, 'success')
      else if (failed.length === links.length) showToast('导入失败：' + failed[0].error, 'error')
      else showToast(`新增 ${added} 个节点，${failed.length} 条失败：${failed[0].error}`, 'error')
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
      const editing = Boolean(state.editingNodeId)
      try {
        const node = {
          id: 'manual-' + Date.now(),
          name, server, port, protocol,
          settings: buildManualSettings(protocol, password),
        }
        if (editing) {
          node.id = state.editingNodeId
          await window.go.app.App.UpdateNode(state.editingNodeId, node)
          state.editingNodeId = ''
        } else {
          await window.go.app.App.AddNode(node)
        }
        await refreshConfig()
        await refreshRules()
        showToast(editing ? '节点已修改' : '节点已添加', 'success')
        resetManualForm()
        state.nodeFormOpen = false
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
  state.nodeFormOpen = true
  renderApp()
  document.querySelector('#nodeName')?.focus()
}

const MAXIMISE_ICON = '<svg viewBox="0 0 10 10"><rect x=".5" y=".5" width="9" height="9" /></svg>'
const RESTORE_ICON = '<svg viewBox="0 0 10 10"><path d="M2.5 2.5V.5h7v7h-2" /><rect x=".5" y="2.5" width="7" height="7" /></svg>'

// The window has no system title bar; macOS keeps its own traffic-light buttons.
async function setupTitlebar() {
  const runtime = window.runtime
  if (!runtime) return
  const platform = (await runtime.Environment().catch(() => ({}))).platform || ''
  document.documentElement.dataset.platform = platform
  if (platform === 'darwin') return
  const maximise = document.querySelector('[data-window="maximise"]')
  const syncMaximise = async () => {
    const maximised = await runtime.WindowIsMaximised().catch(() => false)
    maximise.innerHTML = maximised ? RESTORE_ICON : MAXIMISE_ICON
    maximise.title = maximised ? '还原' : '最大化'
    maximise.setAttribute('aria-label', maximise.title)
  }
  document.querySelector('[data-window="minimise"]').addEventListener('click', () => runtime.WindowMinimise())
  maximise.addEventListener('click', () => runtime.WindowToggleMaximise())
  // Closing hides to the tray, the same as the old system close button.
  document.querySelector('[data-window="close"]').addEventListener('click', () => runtime.WindowHide())
  document.querySelector('.titlebar').addEventListener('dblclick', event => {
    if (!event.target.closest('.window-controls')) runtime.WindowToggleMaximise()
  })
  window.addEventListener('resize', syncMaximise)
  syncMaximise()
}

// Wails runtime ready
window.addEventListener('DOMContentLoaded', async () => {
  setupTitlebar()
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
    setInterval(refreshAutoSwitch, 2000)
    checkAppUpdate()
    setInterval(checkAppUpdate, APP_UPDATE_INTERVAL)
  } else {
    showToast('Wails 运行时未加载', 'error')
    renderApp()
  }
})
