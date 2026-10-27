
import './style.css'
import './mobile.css'
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
// 安卓 APK：VPN 由系统创建，没有流量分析（抓包要装证书）和桌面窗口。
const IS_ANDROID = document.documentElement.dataset.platform === 'android'
const VIEWS = IS_ANDROID ? ['overview', 'nodes', 'routing', 'rules'] : ['overview', 'nodes', 'routing', 'rules', 'capture']
const TUN_LABEL = IS_ANDROID ? 'VPN' : 'TUN'
const savedView = localStorage.getItem('sbtun-ui-view')
const initialView = VIEWS.includes(savedView) ? savedView : 'overview'
const savedNodeFilter = localStorage.getItem('sbtun-ui-node-filter') || 'all'
const NODE_PROTOCOLS = [['hysteria2', 'Hysteria2'], ['hysteria', 'Hysteria（第一版）'], ['vless', 'VLESS'], ['vmess', 'VMess'], ['trojan', 'Trojan'], ['shadowsocks', 'Shadowsocks'], ['tuic', 'TUIC'], ['anytls', 'AnyTLS'], ['wireguard', 'WireGuard'], ['socks', 'SOCKS'], ['http', 'HTTP']]
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
  appUpdating: false,
  autoSwitch: null,
  singBoxUpdating: false,
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
  customRuleForm: { match_type: 'domain_suffix', value: '', action: 'proxy', node_id: '' },
  customRuleImport: '',
  rules: null,
  ruleSetForm: { name: '', url: '', action: 'block', node_id: '' },
  editingRuleSetID: '',
  ruleFilter: 'all',
  ruleUpdating: '',
  allRulesUpdating: false,
  documentEventsBound: false,
  dnsFilterFormOpen: false,
  filterRuleForm: { match_type: 'domain_suffix', value: '' },
  // 正在修改的规则（原样保存，用于在配置里定位），null 表示新增
  editingCustomRule: null,
  editingFilterRule: null,
  nodeSwitching: false,
  subUpdating: '',
  nodeHealth: loadNodeHealth(),
  selectedNodes: new Set(),
  batchTesting: false,
  nodeRegion: localStorage.getItem('sbtun-ui-node-region') || 'all',
  nodeFilter: validNodeFilters.includes(savedNodeFilter) ? savedNodeFilter : 'all',
  nodeSearch: localStorage.getItem('sbtun-ui-node-search') || '',
  nodeFormOpen: false,
  nodeFormTab: 'link',
  nodeGroupBy: ['subscription', 'region'].includes(localStorage.getItem('sbtun-ui-node-group')) ? localStorage.getItem('sbtun-ui-node-group') : 'protocol',
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
  // 云端备份弹窗：form 是正在编辑的 WebDAV 设置，list 是云端备份列表
  backup: null,
  captureSearch: '',
  captureFilter: 'all',
  captureListScroll: 0,
  captureDetailScroll: 0,
  captureBodyScroll: {},
  captureSettingsOpen: localStorage.getItem('sbtun-ui-capture-settings-open') !== 'false',
  view: initialView,
  statusRefreshing: false,
  configReading: 0,
  // 进行中的操作（防重复点击），render 里据此输出 disabled
  busy: new Set(),
  // 用户正在输入时推迟的后台重绘
  renderPending: false,
}

const svgIcon = body => `<svg viewBox="0 0 24 24" width="17" height="17" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">${body}</svg>`
const POWER_ICON = '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3v8"/><path d="M6.3 6.8a8 8 0 1 0 11.4 0"/></svg>'
const NAV_ICONS = {
  overview: svgIcon('<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>'),
  nodes: svgIcon('<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18"/>'),
  routing: svgIcon('<circle cx="6" cy="6" r="2.5"/><circle cx="6" cy="18" r="2.5"/><circle cx="18" cy="12" r="2.5"/><path d="M6 8.5v7M8.3 7.2 15.7 11M8.3 16.8l7.4-3.8"/>'),
  rules: svgIcon('<path d="M4 6h16M7 12h10M10 18h4"/>'),
  capture: svgIcon('<path d="M3 12h4l3-8 4 16 3-8h4"/>'),
}

const MODES = [
  { id: 'smart', name: '智能分流', desc: '国内直连，国外走代理' },
  { id: 'global', name: '全局代理', desc: '除局域网外全部走代理' },
  { id: 'direct', name: '全局直连', desc: '全部直连，只保留阻断规则' },
]

const RULE_MATCH_TYPES = [
  { id: 'domain_suffix', label: '域名后缀', hint: '匹配以该后缀结尾的域名', placeholder: '例如 example.com' },
  { id: 'domain_keyword', label: '域名关键词', hint: '匹配域名中包含的关键词', placeholder: '例如 google' },
  { id: 'domain', label: '完整域名', hint: '只匹配一个完整域名', placeholder: '例如 www.example.com' },
  { id: 'ip_cidr', label: 'IP 网段', hint: '填写 IPv4 或 IPv6 网段', placeholder: '例如 192.168.1.0/24' },
  { id: 'port', label: '端口', hint: '只填写一个端口号', placeholder: '例如 443' },
]

// 节点落地地区：国旗 + 中文名。国旗用两个区域指示符拼出，Windows 靠 style.css 里的 Twemoji 国旗字体显示。
const REGION_NAMES = { HK: '香港', TW: '台湾', JP: '日本', KR: '韩国', SG: '新加坡', US: '美国', GB: '英国', DE: '德国', FR: '法国', NL: '荷兰', CA: '加拿大', AU: '澳大利亚', IN: '印度', TR: '土耳其', RU: '俄罗斯', MY: '马来西亚', TH: '泰国', ID: '印尼', MO: '澳门', VN: '越南', PH: '菲律宾', AR: '阿根廷', BR: '巴西', AE: '阿联酋', IT: '意大利', ES: '西班牙', CH: '瑞士', SE: '瑞典', PL: '波兰', UA: '乌克兰', IL: '以色列', MX: '墨西哥', CL: '智利', ZA: '南非', NZ: '新西兰', IE: '爱尔兰', FI: '芬兰', NO: '挪威', DK: '丹麦', AT: '奥地利', BE: '比利时', PT: '葡萄牙', CZ: '捷克', RO: '罗马尼亚', HU: '匈牙利', KZ: '哈萨克斯坦', PK: '巴基斯坦', BD: '孟加拉', SA: '沙特', EG: '埃及', NG: '尼日利亚', CO: '哥伦比亚', PE: '秘鲁', LU: '卢森堡', IS: '冰岛', GR: '希腊', BG: '保加利亚', RS: '塞尔维亚', LT: '立陶宛', LV: '拉脱维亚', EE: '爱沙尼亚', MN: '蒙古', KH: '柬埔寨', MM: '缅甸', LA: '老挝', NP: '尼泊尔', LK: '斯里兰卡', MD: '摩尔多瓦', GE: '格鲁吉亚', AM: '亚美尼亚', AZ: '阿塞拜疆' }
// 没收录的地区用系统的中文地区名兜底（旧 WebView 没有 Intl.DisplayNames 就显示代码）
const regionDisplay = (() => { try { return new Intl.DisplayNames(['zh-CN'], { type: 'region' }) } catch { return null } })()
const regionName = code => { if (REGION_NAMES[code]) return REGION_NAMES[code]; try { return regionDisplay?.of(code) || code } catch { return code } }
const regionFlag = code => /^[A-Z]{2}$/.test(code) ? String.fromCodePoint(...[...code].map(c => 0x1F1A5 + c.charCodeAt(0))) : ''
const regionLabel = code => `<span class="region-flag">${regionFlag(code)}</span>${escapeHtml(regionName(code))}`
// 按节点数从多到少列出现有地区
function nodeRegions() {
  const counts = new Map()
  for (const n of state.config?.nodes || []) if (n.country) counts.set(n.country, (counts.get(n.country) || 0) + 1)
  return [...counts.entries()].sort((a, b) => b[1] - a[1]).map(([code, count]) => ({ code, count }))
}

// 节点组：地区 / 机场(订阅) / 协议 / 名称关键词 组合筛选；同一项内任选其一，不同项要同时满足，空=不限
const groupWords = text => (text || '').toLowerCase().split(/[\s,，]+/).filter(Boolean)
function groupMatches(g, n) {
  const name = (n.name || '').toLowerCase()
  if (g.regions?.length && !g.regions.includes(n.country)) return false
  if (g.subscriptions?.length && !g.subscriptions.includes(n.subscription || '')) return false
  if (g.protocols?.length && !g.protocols.includes(n.protocol)) return false
  const include = groupWords(g.keyword), exclude = groupWords(g.exclude)
  if (include.length && !include.some(w => name.includes(w))) return false
  return !exclude.some(w => name.includes(w))
}
const groupNodes = g => (state.config?.nodes || []).filter(n => groupMatches(g, n))
const subscriptionName = id => id ? ((state.config?.subscriptions || []).find(sub => sub.id === id)?.name || '已删除的订阅') : '手动添加'
function groupConditions(g) {
  const parts = []
  if (g.subscriptions?.length) parts.push(g.subscriptions.map(subscriptionName).map(escapeHtml).join(' / '))
  if (g.regions?.length) parts.push(g.regions.map(regionLabel).join(' / '))
  if (g.protocols?.length) parts.push(g.protocols.map(protocolLabel).map(escapeHtml).join(' / '))
  if (g.keyword) parts.push('含「' + escapeHtml(g.keyword) + '」')
  if (g.exclude) parts.push('不含「' + escapeHtml(g.exclude) + '」')
  return parts.length ? parts.map(part => `<span class="group-cond">${part}</span>`).join('') : '<span class="group-cond">全部节点</span>'
}

const RULE_ACTIONS = [
  { id: 'proxy', label: '代理', hint: '匹配流量通过当前节点' },
  { id: 'direct', label: '直连', hint: '匹配流量直接连接' },
  { id: 'block', label: '阻断', hint: '匹配流量直接拒绝' },
  { id: 'node', label: '指定节点…', hint: '固定走这个节点；节点被删除后改走当前节点' },
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
    { key: 'utls_fingerprint', label: 'uTLS 指纹', placeholder: '可选，例如 chrome' },
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
  hysteria: [
    { key: 'password', label: '认证密码', type: 'password', placeholder: 'auth_str，可选' },
    { key: 'server_name', label: 'TLS SNI', placeholder: '可选，默认服务器地址' },
    { key: 'alpn', label: 'TLS ALPN', placeholder: '可选，默认 hysteria' },
    { key: 'up_mbps', label: '上行 Mbps', type: 'number', placeholder: '不填默认 20' },
    { key: 'down_mbps', label: '下行 Mbps', type: 'number', placeholder: '不填默认 100' },
    { key: 'server_ports', label: '跳跃端口', placeholder: '例如 2000-3000 或 443,8443' },
    { key: 'obfs_password', label: 'Obfs 密码', type: 'password', placeholder: '服务端开了 xplus 混淆时填写' },
    { key: 'insecure', label: '跳过证书校验', type: 'checkbox' },
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

// 旧内核（Chrome < 84）不支持 flex 容器的 gap，检测后由 CSS 用 margin 兜底
function supportsFlexGap() {
  const box = document.createElement('div')
  box.style.cssText = 'display:flex;flex-direction:column;row-gap:1px;position:absolute;visibility:hidden'
  box.appendChild(document.createElement('div'))
  box.appendChild(document.createElement('div'))
  document.body.appendChild(box)
  const ok = box.scrollHeight === 1
  document.body.removeChild(box)
  return ok
}
if (!supportsFlexGap()) document.documentElement.classList.add('no-flex-gap')

document.documentElement.dataset.theme = state.theme
app.innerHTML = render()

function render() {
  return `
    <main class="shell view-${state.view}">
      <aside class="sidebar">
        <div class="brand"><img class="brand-mark" src="${logoUrl}" alt="" /><span class="brand-name">sbtun</span></div>
        <div class="nav-caption">工作区</div>
        <nav class="nav-list">
          ${VIEWS.map(id => [id, viewLabel(id)]).map(([id, label]) => `<button class="nav-item ${state.view === id ? 'active' : ''}" data-view="${id}" title="${label}"><span class="nav-icon" aria-hidden="true">${NAV_ICONS[id]}</span><span class="nav-label">${label}</span></button>`).join('')}
        </nav>
        <div class="sidebar-status">
          <div class="sidebar-status-line" title="${escapeHtml(statusLabel(state.statusState, state.statusMessage))}"><span class="dot ${dotClass(state.statusState)}"></span><span>${escapeHtml(statusLabel(state.statusState, state.statusMessage))}</span></div>
          <div class="sidebar-versions">
            ${state.version ? `<span class="app-version" title="管理器版本 v${escapeHtml(state.version)}">程序 v${escapeHtml(state.version)}</span>` : ''}
            <span class="app-version" title="sing-box 内核版本 ${state.singBoxVersion ? 'v' + escapeHtml(state.singBoxVersion) : '读取中'}">内核 ${state.singBoxVersion ? 'v' + escapeHtml(state.singBoxVersion) : '--'}</span>
          </div>
          ${IS_ANDROID ? '' : `<div class="update-actions">
            <button id="updateApp" class="btn btn-ghost kernel-update-btn" title="下载 sbtun 最新版并自动重启（同 sbtun upgrade）" ${state.appUpdating ? 'disabled' : ''}>${state.appUpdating ? '正在更新…' : '更新程序'}</button>
            <button id="updateSingBox" class="btn btn-ghost kernel-update-btn" title="检查 sing-box 官方稳定版并更新（同 sbtun kernel update）" ${state.singBoxUpdating ? 'disabled' : ''}>${state.singBoxUpdating ? '正在更新…' : '更新内核'}</button>
          </div>`}
        </div>
      </aside>

      <section class="workspace">
        <header class="topbar">
          <div>
            <div class="eyebrow">sbtun / ${viewLabel(state.view)}</div>
            <h1>${viewTitle(state.view)}</h1>
          </div>
          <div class="topbar-actions">
            ${state.appUpdate?.has_update ? `<button id="appUpdate" class="update-pill" type="button" title="当前 v${escapeHtml(state.appUpdate.current_version)}，${IS_ANDROID ? '点击打开下载页' : '点击下载并自动重启'}" ${state.appUpdating ? 'disabled' : ''}><span class="update-dot" aria-hidden="true"></span>${state.appUpdating ? '正在更新…' : '新版本 v' + escapeHtml(state.appUpdate.latest_version)}</button>` : ''}
            <button id="themeToggle" class="theme-toggle" type="button" aria-label="切换主题" title="切换到${nextTheme().name}主题">${THEME_ICONS[nextTheme().id]}</button>
            <button id="power" data-power class="switch ${state.running ? 'on' : ''}" ${busyAttr('power')}>${powerLabel()}</button>
          </div>
        </header>
        <section class="status-bar">
          <span class="dot ${dotClass(state.statusState)}"></span>
          <strong id="statusText">${escapeHtml(statusLabel(state.statusState, state.statusMessage))}</strong>
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
    ${renderBackupModal()}
    ${renderGroupModal()}
  `
}

function powerLabel() {
  return (state.running ? '关闭 ' : '开启 ') + TUN_LABEL
}

function viewLabel(view) {
  return { overview: '运行概览', nodes: '节点管理', routing: '路由模式', rules: '分流规则', capture: '流量分析' }[view] || '运行概览'
}

function viewTitle(view) {
  return { overview: '运行概览', nodes: '节点管理', routing: '路由模式', rules: '规则集与过滤', capture: 'HTTP 请求分析' }[view] || '运行概览'
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
    .filter(node => protoMatches(node) && regionMatches(node) && (!query || `${node.name} ${node.server} ${node.port} ${node.protocol} ${node.country || ''} ${regionName(node.country || '')}`.toLowerCase().includes(query)))
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
  return `${renderMobileHero(current, mode)}<div class="overview-grid">
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
    <div class="overview-quick"><button data-view="nodes"><span>可用节点</span><strong>${nodes.length} 个　›</strong></button><button data-view="rules"><span>规则集</span><strong>已启用 ${enabledRules} / ${rules.length}　›</strong></button>${IS_ANDROID ? '' : `<button data-view="capture"><span>流量分析</span><strong>${state.captureStatus?.running ? '分析器运行中' : '分析器已停止'}　›</strong></button>`}</div>`
}

// 手机竖屏的概览顶部：大号连接按钮 + 状态 + 实时速度（宽屏时由 CSS 隐藏）。

function heroState() {
  if (state.statusState === 'starting') return ['starting', '正在连接…']
  if (state.statusState === 'stopping') return ['starting', '正在断开…']
  if (state.statusState === 'error') return ['error', '连接出错']
  return state.running ? ['on', '已连接'] : ['off', '未连接']
}

function renderMobileHero(current, mode) {
  const [cls, label] = heroState()
  return `<section class="mobile-hero ${cls}">
      <button class="power-orb" data-power type="button" aria-label="${powerLabel()}" ${busyAttr('power')}>${POWER_ICON}</button>
      <div class="hero-copy">
        <strong id="heroState">${label}</strong>
        <span class="hero-sub">${escapeHtml(current?.name || '未选择节点')} · ${escapeHtml(mode?.name || '未设置')}</span>
      </div>
      <div class="hero-speed"><span>↑ <b id="heroUp">${formatBytes(state.traffic.up)}/s</b></span><span>↓ <b id="heroDown">${formatBytes(state.traffic.down)}/s</b></span></div>
    </section>`
}

function updateMobileHero() {
  const hero = document.querySelector('.mobile-hero')
  if (!hero) return
  const [cls, label] = heroState()
  hero.className = 'mobile-hero ' + cls
  hero.querySelector('#heroState').textContent = label
  hero.querySelector('#heroUp').textContent = formatBytes(state.traffic.up) + '/s'
  hero.querySelector('#heroDown').textContent = formatBytes(state.traffic.down) + '/s'
  const orb = hero.querySelector('.power-orb')
  orb.setAttribute('aria-label', powerLabel())
  orb.disabled = state.busy.has('power')
}

function renderNodeOptions(nodes, selectedID) {
  const groups = nodeGroups(nodes, 'subscription')
  return groups.map(group => `<optgroup label="${escapeHtml(group.label)}">${group.nodes.map(n => `<option value="${escapeHtml(n.id)}" ${n.id === selectedID ? 'selected' : ''}>${escapeHtml(n.name)}</option>`).join('')}</optgroup>`).join('')
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
      <div class="section-heading node-page-heading"><div><h2>节点列表</h2><p class="muted">${subtitle}</p></div><span class="node-batch-actions"><button class="btn btn-ghost" id="openBackup" title="备份到 WebDAV 或从云端恢复">云端备份</button><button class="btn btn-ghost" id="toggleSelectNodes" ${visibleIDs.length ? '' : 'disabled'}>${allSelected ? '取消全选' : '全选'}</button><button class="btn btn-ghost" id="batchTestNodes" ${state.batchTesting || !visibleIDs.length ? 'disabled' : ''}>${state.batchTesting ? '测试中…' : selected ? '测试选中' : '测试全部'}</button><button class="btn btn-ghost" id="batchExportNodes" ${selected && !state.busy.has('export') ? '' : 'disabled'}>导出选中</button><button class="btn btn-danger" id="batchDeleteNodes" ${selected && !state.busy.has('delete') ? '' : 'disabled'}>删除选中</button>${IS_ANDROID ? `<button class="btn btn-ghost" data-scan-qr ${busyAttr('scan')}>扫码导入</button>` : ''}<button class="btn btn-primary" id="toggleNodeForm">导入 / 添加</button></span></div>
      ${renderSubscriptions()}
      <div id="nodePanel">${renderNodes()}</div>
  </section>`
}

function renderNodeModal() {
  if (!state.nodeFormOpen && !state.editingNodeId) return ''
  const editing = Boolean(state.editingNodeId)
  const tab = state.nodeFormTab
  const importing = state.busy.has('import')
  const body = tab === 'link'
    ? editing
      ? `<p class="muted modal-hint">粘贴这个节点新的分享链接（只能一条），保存后用链接里的参数整体替换，节点所属订阅不变。</p>
      <textarea id="nodeUrl" class="input node-url-input" rows="4" placeholder="vless:// vmess:// trojan:// ss:// hy2:// …">${escapeHtml(state.manualForm.url)}</textarea>`
      : `<p class="muted modal-hint">粘贴节点分享链接（vless:// vmess:// trojan:// ss:// hy2:// hysteria:// tuic:// anytls:// wg:// 等，可多行，一行一个），或订阅地址（支持 Base64、Clash YAML、sing-box JSON）。</p>
      <textarea id="nodeUrl" class="input node-url-input" rows="6" placeholder="每行一个链接或订阅地址">${escapeHtml(state.manualForm.url)}</textarea>`
    : `<div class="row"><select id="nodeProtocol" class="select">${NODE_PROTOCOLS.map(([id, label]) => `<option value="${id}" ${state.manualProtocol === id ? 'selected' : ''}>${label}</option>`).join('')}</select></div>
      <div class="row"><input id="nodeName" class="input" value="${escapeHtml(state.manualForm.name)}" placeholder="节点名称" /></div>
      <div class="row"><input id="nodeServer" class="input" value="${escapeHtml(state.manualForm.server)}" placeholder="服务器地址" /><input id="nodePort" class="input port-input" value="${escapeHtml(state.manualForm.port)}" placeholder="端口" /></div>
      ${renderManualAdvancedFields()}`
  return `<div class="modal" id="nodeModal" role="dialog" aria-modal="true" aria-labelledby="nodeModalTitle"><div class="modal-backdrop" data-close-node-modal></div>
    <section class="modal-panel node-modal" id="node-import">
      <header class="modal-header"><h2 id="nodeModalTitle">${editing ? '编辑节点' : '添加节点'}</h2><button class="icon-btn" data-close-node-modal aria-label="关闭" title="关闭">×</button></header>
      <div class="modal-tabs" role="tablist"><button class="modal-tab ${tab === 'link' ? 'active' : ''}" data-node-form-tab="link" role="tab">${editing ? '粘贴链接替换' : '粘贴链接导入'}</button><button class="modal-tab ${tab === 'manual' ? 'active' : ''}" data-node-form-tab="manual" role="tab">手动填写</button></div>
      <div class="modal-body">${body}</div>
      <footer class="modal-footer"><button class="btn btn-ghost" data-close-node-modal>取消</button>${tab === 'link' ? `${IS_ANDROID && !editing ? `<button class="btn btn-ghost" data-scan-qr ${busyAttr('scan')}>扫码</button>` : ''}<button id="importBtn" class="btn btn-primary" ${importing ? 'disabled' : ''}>${importing ? (editing ? '保存中…' : '导入中…') : editing ? '保存修改' : '导入'}</button>` : `<button id="addNodeBtn" class="btn btn-primary" ${busyAttr('node')}>${editing ? '保存修改' : '添加节点'}</button>`}</footer>
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
    const usage = subscriptionUsage(s.usage)
    return `<li class="node-item subscription-item">
        <span class="badge proxy">订阅</span>
        <div class="node-info"><div class="node-row"><span class="node-name" title="${escapeHtml(s.name)}">${escapeHtml(s.name)}</span><span class="node-server">${count} 个节点 · ${escapeHtml(updated)}</span></div>${usage ? `<div class="sub-usage${usage.warn ? ' warn' : ''}">${escapeHtml(usage.text)}</div>` : ''}</div>
        <div class="node-actions"><button class="btn btn-ghost update-sub" data-id="${escapeHtml(s.id)}" ${busy ? 'disabled' : ''}>${updating ? '更新中...' : '更新'}</button><button class="btn btn-danger rm-sub" data-id="${escapeHtml(s.id)}" ${busy ? 'disabled' : ''}>删除</button></div>
      </li>`
  }).join('')
  const cfg = state.config || {}
  const hours = cfg.subscription_auto_update_disabled ? 0 : cfg.subscription_update_hours || 24
  const choices = [...new Set([0, 6, 12, 24, 48, hours])].sort((a, b) => a - b)
  const autoSelect = `<select id="subAutoUpdate" class="select sub-auto-select" title="订阅自动更新">${choices.map(h => `<option value="${h}" ${h === hours ? 'selected' : ''}>${h ? `每 ${h} 小时自动更新` : '不自动更新'}</option>`).join('')}</select>`
  return `<section class="card subscription-card"><div class="section-heading"><div><h2>订阅</h2><p class="muted">更新时用订阅里的最新节点替换旧节点，下载失败会保留原节点。</p></div><span class="node-batch-actions">${autoSelect}${subs.length > 1 ? `<button class="btn btn-primary" id="updateAllSubs" ${busy ? 'disabled' : ''}>${state.subUpdating === '*' ? '更新中...' : '全部更新'}</button>` : ''}</span></div><ul class="node-list">${items}</ul></section>`
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
  return `<label class="setting-field"><span>${label}</span><input id="setting-${key}" data-setting="${key}" class="input" value="${escapeHtml(state.manualForm[key] || '')}" placeholder="${placeholder}" /></label>`
}

function renderNodeField(field) {
  const value = state.manualForm[field.key] ?? ''
  if (field.type === 'checkbox') {
    return `<label class="check-field"><input id="setting-${field.key}" data-setting="${field.key}" type="checkbox" ${value ? 'checked' : ''} /> ${field.label}</label>`
  }
  if (field.type === 'select') {
    // 导入的值不在预设选项里时也要显示出来，否则保存时会被改成第一个选项。
    const options = value && !field.options.some(([key]) => key === value) ? [...field.options, [value, value]] : field.options
    return `<label class="setting-field"><span>${field.label}</span><select id="setting-${field.key}" data-setting="${field.key}" class="select">${options.map(([key, label]) => `<option value="${escapeHtml(key)}" ${value === key ? 'selected' : ''}>${escapeHtml(label)}</option>`).join('')}</select></label>`
  }
  return `<label class="setting-field"><span>${field.label}</span><input id="setting-${field.key}" data-setting="${field.key}" class="input" type="${field.type || 'text'}" value="${escapeHtml(value)}" placeholder="${field.placeholder || ''}" ${field.required ? 'required' : ''} /></label>`
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
  if ((protocol === 'hysteria' || protocol === 'hysteria2') && settings.server_name) {
    settings.sni = settings.server_name
    delete settings.server_name
  }
  for (const key of transportKeys(protocol, settings.transport_type)) {
    const value = String(form[key] || '').trim()
    if (value) settings[key] = value
  }
  return settings
}

// transportKeys 返回当前传输方式在表单里显示的参数（Host/Path 或 Service Name）。
function transportKeys(protocol, type) {
  if (!type || !(NODE_FIELDS[protocol] || []).some(field => field.key === 'transport_type')) return []
  if (type === 'grpc') return ['transport_service_name']
  return ['transport_host', 'transport_path']
}

// 表单负责的设置项；编辑节点时其余字段（表单里没有的）原样保留。
function formSettingKeys(protocol) {
  const keys = new Set((NODE_FIELDS[protocol] || []).map(field => field.key))
  if (keys.has('transport_type')) ['transport_host', 'transport_path', 'transport_service_name'].forEach(key => keys.add(key))
  if (protocol === 'vless' || protocol === 'vmess') { keys.delete('password'); keys.add('uuid') }
  if (protocol === 'hysteria' || protocol === 'hysteria2') { keys.delete('server_name'); keys.add('sni') }
  return keys
}

function renderRoutingPage() {
  const matchType = RULE_MATCH_TYPES.find(item => item.id === state.customRuleForm.match_type) || RULE_MATCH_TYPES[0]
  const diagnostics = state.diagnostics
  return `<div class="page-stack"><section class="card"><h2>选择路由模式</h2><div class="mode-grid">${MODES.map(m => `<button class="mode-btn ${state.config?.routing_mode === m.id ? 'active' : ''}" data-mode="${m.id}">${m.name}<span class="mode-desc">${m.desc}</span></button>`).join('')}</div><div class="network-options"><div class="network-options-title">网络选项</div><label class="setting-toggle-row"><span class="setting-toggle-copy"><strong>节点失效自动切换</strong><small>当前节点连续两次测试失败时，切换到延迟最低的可用节点，优先同一订阅</small></span><span class="toggle-switch"><input id="autoSwitchToggle" type="checkbox" ${state.config?.auto_switch_disabled ? '' : 'checked'}><span aria-hidden="true"></span></span></label><label class="setting-toggle-row"><span class="setting-toggle-copy"><strong>启用 IPv6</strong><small>${escapeHtml(state.ipv6Status?.message || '正在检测设备 IPv6')}</small></span><span class="toggle-switch"><input id="ipv6Toggle" type="checkbox" ${state.config?.ipv6_enabled ? 'checked' : ''} ${busyAttr('ipv6')}><span aria-hidden="true"></span></span></label><label class="setting-toggle-row" data-desktop-only><span class="setting-toggle-copy"><strong>启用 WebRTC 防泄漏</strong><small>防止 WebRTC 通过直连暴露真实 IPv4/IPv6</small></span><span class="toggle-switch"><input id="webrtcProtectionToggle" type="checkbox" ${state.config?.webrtc_protection_enabled ? 'checked' : ''} ${busyAttr('webrtc')}><span aria-hidden="true"></span></span></label><label class="setting-toggle-row"><span class="setting-toggle-copy"><strong>启用诊断信息</strong><small>显示当前 selector 和运行节点，仅用于排查问题</small></span><span class="toggle-switch"><input id="diagnosticsToggle" type="checkbox" ${state.config?.diagnostics_enabled ? 'checked' : ''} ${busyAttr('diagnostics')}><span aria-hidden="true"></span></span></label></div></section>${state.config?.diagnostics_enabled ? `<section class="card"><h2>运行诊断</h2><div class="diagnostics-grid"><span>当前配置节点</span><strong>${escapeHtml(diagnostics?.current_node_id || '未选择')}</strong><span>实际 selector</span><strong>${escapeHtml(diagnostics?.selector || '未读取')}</strong><span>状态</span><strong>${escapeHtml(diagnostics?.message || '读取中')}</strong></div></section>` : ''}
    ${renderNodeGroupsCard()}
    ${`<section class="card"><div class="section-heading"><div><h2>我的分流规则</h2><p class="muted">${state.config?.routing_mode === 'direct' ? '全局直连模式下只有去向为“阻断”的规则生效。' : '优先于路由模式生效；规则集在“分流规则”页添加，也可以选去向。'}</p></div></div><div id="customRulesPanel">${renderCustomRules()}</div><div class="rule-editor ${state.customRuleForm.action === 'node' ? 'with-node' : ''}"><label class="rule-field"><span>匹配对象</span><select id="ruleMatchType" class="select">${RULE_MATCH_TYPES.map(item => `<option value="${item.id}" ${item.id === matchType.id ? 'selected' : ''}>${item.label}</option>`).join('')}</select></label><label class="rule-field"><span>匹配内容</span><input id="ruleValue" class="input" value="${escapeHtml(state.customRuleForm.value)}" placeholder="${matchType.placeholder}" /></label><label class="rule-field"><span>去向</span>${renderRuleActionSelect()}</label>${state.customRuleForm.action === 'node' ? renderRuleNodeSelect() : ''}<button id="addRuleBtn" class="btn btn-primary" ${busyAttr('customRule')}>${state.editingCustomRule ? '保存修改' : '添加规则'}</button>${state.editingCustomRule ? '<button id="cancelEditRule" class="btn btn-ghost">取消</button>' : ''}<small class="rule-hint value-hint" id="ruleValueHint">${matchType.hint}</small><small class="rule-hint action-hint" id="ruleActionHint">${ruleActionHint(state.customRuleForm.action)}</small></div><details class="rule-import" open><summary>批量导入规则</summary><div class="rule-import-body"><textarea id="ruleImport" class="input" rows="3" placeholder="粘贴规则或填链接：Clash/Surge 配置、ACL4SSR 模板都行；引用的 RULE-SET 列表会各自变成一个规则集"></textarea><button id="importRuleBtn" class="btn btn-ghost" ${busyAttr('importRule')}>导入</button></div></details></section>`}
  </div>`
}

function renderRulesPage() {
  const filter = state.ruleFilter || 'all'
  return `<div class="page-stack rules-page">
    <div class="rules-tabs" role="tablist" aria-label="规则页面"><button class="filter-btn ${state.rulesTab === 'sets' ? 'active' : ''}" data-rules-tab="sets" role="tab" aria-selected="${state.rulesTab === 'sets'}">规则集</button><button class="filter-btn ${state.rulesTab === 'manual' ? 'active' : ''}" data-rules-tab="manual" role="tab" aria-selected="${state.rulesTab === 'manual'}">手动过滤</button></div>
    ${state.rulesTab === 'sets' ? `
    <section class="card rules-card">
      <div class="section-heading rules-page-heading">
        <div><h2>规则集</h2><p class="muted">内置规则只用于智能分流；自己添加的整份生效、整份更新，可以选去向。</p></div>
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
          <div class="rules-toolbar"><span class="rules-summary-note">更新失败时保留旧文件。</span><button id="updateAllRules" class="btn btn-primary" ${state.ruleUpdating || state.allRulesUpdating ? 'disabled' : ''}>${state.allRulesUpdating ? '更新中...' : '更新全部规则集'}</button></div>
        </div>
        <aside class="rule-set-form card">
          <h3>${state.editingRuleSetID ? '编辑规则集' : '添加规则集'}</h3>
          <p class="muted">支持 .srs、Clash/Surge 规则列表、域名或 IP 列表的链接。</p>
          <label class="rule-form-field"><span>规则集名称</span><input id="ruleSetName" class="input" value="${escapeHtml(state.ruleSetForm.name)}" placeholder="例如 广告过滤规则" /></label>
          <label class="rule-form-field"><span>规则集 URL</span><input id="ruleSetURL" class="input" value="${escapeHtml(state.ruleSetForm.url)}" placeholder="https://example.com/rules.srs" /></label>
          ${state.rules?.find(r => r.id === state.editingRuleSetID)?.source === 'default' ? '' : `<label class="rule-form-field"><span>去向</span>${renderTargetSelect('ruleSetAction', state.ruleSetForm.action, { blockFirst: true })}</label>${state.ruleSetForm.action === 'node' ? renderNodePicker('ruleSetNode', state.ruleSetForm.node_id) : ''}`}
          <div class="rule-form-actions"><button id="addRuleSetBtn" class="btn btn-primary" ${busyAttr('ruleSet')}>${state.editingRuleSetID ? '保存修改' : '添加规则集'}</button>${state.editingRuleSetID ? '<button id="cancelEditRuleSet" class="btn btn-ghost">取消</button>' : `<a class="btn btn-ghost rule-source-link" href="https://github.com/razaxq/dns-blocklists-sing-box/blob/main/README_zh.md" target="_blank" rel="noopener noreferrer">查找规则集</a><button id="restoreDefaultRules" class="btn btn-ghost" ${busyAttr('ruleSet')}>恢复默认规则</button>`}</div>
        </aside>
      </div>
    </section>
    ` : `
    <section class="card dns-filter-card">
      <div class="section-heading"><div><h2>手动 DNS 过滤规则</h2><p class="muted">单独添加域名或 IP，命中后直接阻断，优先级最高。</p></div><button id="openFilterRuleBtn" class="btn btn-primary">添加过滤规则</button></div>
      ${state.dnsFilterFormOpen ? `<div class="dns-filter-editor"><label class="rule-form-field"><span>匹配对象</span><select id="filterRuleMatchType" class="select">${RULE_MATCH_TYPES.filter(item => item.id !== 'port').map(item => `<option value="${item.id}" ${item.id === state.filterRuleForm.match_type ? 'selected' : ''}>${item.label}</option>`).join('')}</select></label><label class="rule-form-field"><span>匹配内容</span><input id="filterRuleValue" class="input" value="${escapeHtml(state.filterRuleForm.value)}" placeholder="例如 ads.example.com" /></label><div class="rule-form-actions"><button id="addFilterRuleBtn" class="btn btn-primary" ${busyAttr('filterRule')}>${state.editingFilterRule ? '保存修改' : '添加规则'}</button><button id="cancelFilterRuleBtn" class="btn btn-ghost">取消</button></div></div>` : ''}
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
      <label class="capture-toggle"><span>启用分析</span><span class="toggle-switch"><input id="captureEnabled" type="checkbox" ${enabled ? 'checked' : ''} ${busyAttr('capture')}><span aria-hidden="true"></span></span></label>
    </section>
    <details class="card capture-settings" id="captureSettings" ${state.captureSettingsOpen ? 'open' : ''}>
      <summary class="capture-settings-summary"><div><h2>抓包规则与证书</h2><p class="muted">每行一个域名或关键词；输入 * 抓取全部 HTTP/HTTPS 请求。</p></div><span class="capture-settings-actions" aria-hidden="true">⌄</span></summary>
      <div class="capture-settings-content"><div class="section-heading"><span></span><button id="saveCapture" class="btn btn-primary" ${busyAttr('capture')}>保存并应用</button></div>
      <textarea id="captureDomains" class="input" rows="4" placeholder="例如 example.com、google；输入 * 抓取全部 HTTP/HTTPS">${escapeHtml(domains)}</textarea>
      <div class="capture-settings-footer"><div class="capture-cert"><span class="badge ${status.certificate_installed ? 'direct' : ''}">${status.certificate_installed ? `证书已安装（${escapeHtml(status.certificate_level || '未知级别')}）` : '证书未安装'}</span>${status.user_certificate_supported ? '<select id="captureCertLevel" class="input capture-cert-level"><option value="system">系统级</option><option value="user">当前用户</option></select>' : '<span class="muted">Linux 使用系统级证书</span>'}<button id="installCaptureCA" class="btn btn-ghost" ${busyAttr('captureCA')}>安装系统证书</button><button id="uninstallCaptureCA" class="btn btn-ghost" ${busyAttr('captureCA')}>卸载系统证书</button></div><span class="capture-storage" title="${escapeHtml(status.storage_path || '')}">${escapeHtml(status.storage_path || 'capture.json')}</span></div>
      </div>
    </details>
    <section class="card capture-records">
      <div class="section-heading capture-records-heading"><div><h2>请求记录</h2><p class="muted">显示 ${visibleFlows.length} 条，共 ${state.captureFlows.length} 条</p></div><div class="capture-record-actions"><button id="saveCaptureFile" class="btn btn-ghost" ${busyAttr('captureFile')}>保存记录</button><button id="clearCapture" class="btn btn-danger" ${busyAttr('captureFile')}>删除全部</button></div></div>
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
      <div class="modal-body qrcode-body"><img class="qrcode-image" src="${escapeHtml(qr.image)}" alt="节点分享链接二维码"><p class="muted modal-hint">用手机客户端扫码即可导入该节点</p><code class="qrcode-link">${escapeHtml(qr.link)}</code></div>
      <footer class="modal-footer"><button class="btn btn-ghost" data-close-node-qrcode>关闭</button><button class="btn btn-primary" id="copyNodeQRCodeLink">复制链接</button></footer>
    </section></div>`
}

function renderBackupModal() {
  const b = state.backup
  if (!b) return ''
  const f = b.form
  const busy = Boolean(b.busy)
  const field = (key, label, type, placeholder) => `<label class="setting-field"><span>${label}</span><input id="backup-${key}" class="input" data-backup-field="${key}" type="${type}" value="${escapeHtml(f[key] || '')}" placeholder="${placeholder}" autocomplete="off" /></label>`
  const list = !b.saved
    ? '<p class="muted">保存 WebDAV 设置后可以备份和恢复。</p>'
    : b.listError
      ? `<p class="muted">${escapeHtml(b.listError)}</p>`
      : !b.list
        ? '<p class="muted">读取中…</p>'
        : !b.list.length
          ? '<p class="muted">云端还没有备份。</p>'
          : `<ul class="node-list backup-list">${b.list.map(item => `<li class="node-item"><div class="node-info"><div class="node-row"><span class="node-name" title="${escapeHtml(item.name)}">${escapeHtml(item.modified || item.name)}</span><span class="node-server">${escapeHtml(item.name)} · ${(item.size / 1024).toFixed(1)} KB${item.encrypted ? ' · 已加密' : ''}</span></div></div><div class="node-actions"><button class="btn btn-ghost restore-backup" data-name="${escapeHtml(item.name)}" ${busy ? 'disabled' : ''}>${b.busy === item.name ? '恢复中…' : '恢复'}</button><button class="btn btn-danger delete-backup" data-name="${escapeHtml(item.name)}" ${busy ? 'disabled' : ''}>${b.busy === 'delete:' + item.name ? '删除中…' : '删除'}</button></div></li>`).join('')}</ul>`
  return `<div class="modal" id="backupModal" role="dialog" aria-modal="true" aria-labelledby="backupModalTitle"><div class="modal-backdrop" data-close-backup></div>
    <section class="modal-panel backup-modal">
      <header class="modal-header"><h2 id="backupModalTitle">云端备份（WebDAV）</h2><button class="icon-btn" data-close-backup aria-label="关闭" title="关闭">×</button></header>
      <div class="modal-body">
        <p class="muted modal-hint">备份手动添加的节点、订阅地址、规则和设置；订阅节点不备份，恢复时按订阅地址重新拉取。坚果云地址填 https://dav.jianguoyun.com/dav/sbtun/，密码用“第三方应用密码”。</p>
        ${field('url', 'WebDAV 地址', 'text', 'https://…/dav/sbtun/')}
        ${field('username', '用户名', 'text', '账号')}
        ${field('password', '密码', 'password', '密码或应用密码')}
        ${field('encrypt_password', '加密密码', 'password', '可选，填了就加密备份，恢复时要用同一个')}
        <label class="setting-toggle-row"><span class="setting-toggle-copy"><strong>自动备份</strong><small>sbtun 运行时每分钟检查一次，手动节点、订阅地址、规则或设置变了才上传；切换节点、订阅节点更新不算变化</small></span><span class="toggle-switch"><input id="backupAutoToggle" type="checkbox" ${f.auto_backup ? 'checked' : ''}><span aria-hidden="true"></span></span></label>
        <div class="section-heading backup-list-heading"><div><h2>云端备份</h2><p class="muted">保留最近 10 份，恢复会整体覆盖本机的节点、订阅、规则和设置。</p></div></div>
        ${list}
      </div>
      <footer class="modal-footer">${b.saved ? '<button class="btn btn-ghost" id="clearBackup" ' + (busy ? 'disabled' : '') + '>清除设置</button>' : ''}<button class="btn btn-ghost" id="saveBackupSettings" ${busy ? 'disabled' : ''}>${b.busy === 'save' ? '连接中…' : '保存并测试'}</button><button class="btn btn-primary" id="backupNow" ${busy || !b.saved ? 'disabled' : ''}>${b.busy === 'backup' ? '备份中…' : '立即备份'}</button></footer>
    </section></div>`
}

function backupFormFromConfig() {
  const w = state.config?.webdav
  return { url: w?.url || '', username: w?.username || '', password: w?.password || '', encrypt_password: w?.encrypt_password || '', auto_backup: Boolean(w?.auto_backup) }
}

async function openBackup() {
  state.backup = { form: backupFormFromConfig(), saved: Boolean(state.config?.webdav?.url), list: null, listError: '', busy: '' }
  renderApp()
  await refreshBackupList()
}

async function refreshBackupList() {
  const b = state.backup
  if (!b || !b.saved) return
  try {
    b.list = await window.go.app.App.ListWebDAVBackups() || []
    b.listError = ''
  } catch (e) {
    b.list = null
    b.listError = errorText(e)
  }
  if (state.backup === b) renderApp()
}

function closeBackup() {
  state.backup = null
  renderApp()
}

async function runBackupAction(name, action) {
  const b = state.backup
  if (!b || b.busy) return
  b.busy = name
  renderApp()
  try {
    await action(b)
  } catch (e) {
    showToast(errorText(e), 'error')
  } finally {
    b.busy = ''
    if (state.backup === b) renderApp()
  }
}

async function saveBackupSettings(b, form, message) {
  await window.go.app.App.TestWebDAV(form)
  await window.go.app.App.SaveWebDAVSettings(form)
  await refreshConfig()
  b.form = { ...form }
  b.saved = true
  showToast(message, 'success')
  await refreshBackupList()
}

function bindBackupModal() {
  document.querySelector('#openBackup')?.addEventListener('click', openBackup)
  const b = state.backup
  if (!b) return
  document.querySelectorAll('[data-close-backup]').forEach(element => element.addEventListener('click', closeBackup))
  document.querySelectorAll('[data-backup-field]').forEach(input => input.addEventListener('input', () => {
    b.form[input.dataset.backupField] = input.value
  }))
  document.querySelector('#saveBackupSettings')?.addEventListener('click', () => runBackupAction('save', b => saveBackupSettings(b, { ...b.form }, b.form.encrypt_password ? 'WebDAV 已连接并保存' : 'WebDAV 已保存，没有设置加密密码，备份以明文保存')))
  document.querySelector('#backupAutoToggle')?.addEventListener('change', event => {
    const enabled = event.target.checked
    b.form.auto_backup = enabled
    if (!b.saved) return // 还没保存设置时，跟着“保存并测试”一起生效
    runBackupAction('save', b => saveBackupSettings(b, { ...backupFormFromConfig(), auto_backup: enabled }, enabled ? '自动备份已开启' : '自动备份已关闭')).then(() => {
      if (state.backup === b) b.form.auto_backup = Boolean(state.config?.webdav?.auto_backup)
    })
  })
  document.querySelector('#backupNow')?.addEventListener('click', () => runBackupAction('backup', async b => {
    const name = await window.go.app.App.BackupToWebDAV()
    showToast('已备份：' + name, 'success')
    await refreshBackupList()
  }))
  document.querySelector('#clearBackup')?.addEventListener('click', () => {
    if (!window.confirm('清除本机保存的 WebDAV 设置？云端已有的备份不会删除。')) return
    runBackupAction('clear', async b => {
      await window.go.app.App.SaveWebDAVSettings({ url: '', username: '', password: '', encrypt_password: '', auto_backup: false })
      await refreshConfig()
      b.form = backupFormFromConfig()
      b.saved = false
      b.list = null
      showToast('已清除 WebDAV 设置', 'success')
    })
  })
  document.querySelectorAll('.restore-backup').forEach(btn => btn.addEventListener('click', () => {
    const name = btn.dataset.name
    if (!window.confirm('用这份备份覆盖本机的节点、订阅、规则和设置？订阅节点会按地址重新拉取。')) return
    runBackupAction(name, async () => {
      const warning = await window.go.app.App.RestoreFromWebDAV(name)
      await refreshConfig()
      await refreshRules()
      showToast(warning ? '已恢复，' + warning : '已恢复备份', warning ? '' : 'success')
    })
  }))
  document.querySelectorAll('.delete-backup').forEach(btn => btn.addEventListener('click', () => {
    const name = btn.dataset.name
    if (!window.confirm('删除这份云端备份？删除后无法找回。')) return
    runBackupAction('delete:' + name, async b => {
      await window.go.app.App.DeleteWebDAVBackup(name)
      b.list = (b.list || []).filter(item => item.name !== name)
      showToast('已删除备份', 'success')
    })
  }))
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
    try { return { text: new URLSearchParams(body).toString().replace(/&/g, '\n'), pretty: true } } catch (_) {}
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
  const units = ['KB', 'MB', 'GB', 'TB', 'PB']
  let v = n / 1024
  let i = 0
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++ }
  return (v >= 100 ? v.toFixed(0) : v.toFixed(1)) + ' ' + units[i]
}

// 与 Go 端 SubscriptionUsageText 一致：剩余流量与到期时间；已过期、7 天内到期或剩余不足 10% 时 warn。
function subscriptionUsage(u) {
  if (!u) return null
  const parts = []
  let warn = false
  if (u.total > 0) {
    const left = u.total - u.upload - u.download
    if (left <= 0) { parts.push('流量已用完'); warn = true }
    else { parts.push(`剩余 ${formatBytes(left)} / 共 ${formatBytes(u.total)}`); warn = left * 10 < u.total }
  }
  if (u.expire > 0) {
    const left = u.expire * 1000 - Date.now()
    if (left <= 0) { parts.push('已过期'); warn = true }
    else if (left < 7 * 86400000) { parts.push(`${Math.floor(left / 86400000) + 1} 天后到期`); warn = true }
    else parts.push(new Date(u.expire * 1000).toLocaleDateString('zh-CN') + ' 到期')
  }
  return parts.length ? { text: parts.join(' · '), warn } : null
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
  return `<div class="rule-set-list">${rules.map(r => {
    const id = escapeHtml(r.id)
    return `
    <div class="rule-set-row">
      <button class="rule-switch ${r.enabled ? 'on' : ''} toggle-rule" data-id="${id}" ${busyAttr('rule:' + r.id)} aria-label="${r.enabled ? '停用' : '启用'} ${escapeHtml(r.name)}"><span></span></button>
      <div class="rule-set-main"><div class="rule-set-title"><strong>${escapeHtml(r.name)}</strong><span class="rule-tag ${r.source === 'default' ? 'default' : 'custom'}">${r.source === 'default' ? '默认' : '用户添加'}</span></div><div class="rule-set-file">${r.source === 'default' ? '' : ruleTarget(r.action || 'block') + ' '}${r.exists ? escapeHtml(formatRuleInfo(r)) : `<button class="rule-inline-download update-rule" data-id="${id}" ${state.ruleUpdating || state.allRulesUpdating ? 'disabled' : ''}>${state.ruleUpdating === r.id ? '下载中...' : '立即下载'}</button>`}</div></div>
      <span class="rule-tag ${r.exists ? 'ready' : 'missing'}">${r.exists ? '已下载' : '未下载'}</span>
      <span class="rule-set-date">${r.updated_at ? escapeHtml(new Date(r.updated_at).toLocaleDateString('zh-CN')) : '尚未更新'}</span>
      <details class="rule-menu" data-rule-menu-id="${id}"><summary aria-label="规则集操作">⋯</summary><div class="rule-menu-list"><button class="rule-menu-action update-rule" data-id="${id}" ${state.ruleUpdating || state.allRulesUpdating ? 'disabled' : ''}>${state.ruleUpdating === r.id ? '更新中...' : '更新'}</button><button class="rule-menu-action edit-rule" data-id="${id}">编辑</button><button class="rule-menu-action rule-copy-url" data-url="${escapeHtml(r.url || '')}">查看 URL</button><button class="rule-menu-action danger delete-rule" data-id="${id}" ${busyAttr('rule:' + r.id)}>删除</button></div></details>
    </div>`
  }).join('')}</div>`
}

function renderDNSFilterRules() {
  const rules = state.config?.dns_filter_rules || []
  if (!rules.length) return '<div class="dns-empty empty">暂无手动 DNS 过滤规则</div>'
  return `<div class="dns-filter-table"><div class="dns-filter-head"><span>状态</span><span>匹配对象</span><span>匹配内容</span><span>处理</span><span></span></div>${rules.map((rule, index) => `<div class="dns-filter-row"><span class="rule-tag ready">已启用</span><span>${escapeHtml(ruleMatchLabel(rule.match_type))}</span><strong>${escapeHtml(rule.value)}</strong><span class="rule-tag block">阻断</span><span class="dns-filter-actions"><button class="btn btn-ghost edit-filter-rule" data-index="${index}">修改</button><button class="btn btn-ghost remove-filter-rule" data-index="${index}">删除</button></span></div>`).join('')}</div>`
}

function formatRuleInfo(rule) {
  const size = Number(rule.size || 0).toLocaleString('zh-CN') + ' bytes'
  const updated = rule.updated_at ? new Date(rule.updated_at).toLocaleString('zh-CN') : '时间未知'
  const sha = rule.sha256 ? ' · SHA256 ' + rule.sha256 : ''
  return size + ' · ' + updated + sha
}

function renderNodes() {
  const allNodes = state.config?.nodes || []
  const filterBtns = [['all', '全部'], ...NODE_PROTOCOLS.map(([id, label]) => [id, id === 'shadowsocks' ? 'SS' : label])]
  const toolsHtml = `<div class="node-tools">
      <div class="node-tools-row"><input id="nodeSearch" class="input" value="${escapeHtml(state.nodeSearch)}" placeholder="搜索名称、服务器或协议" aria-label="搜索节点">
        <select id="nodeGroupBy" class="select node-tool-select" aria-label="分组方式"><option value="protocol" ${state.nodeGroupBy === 'protocol' ? 'selected' : ''}>按协议分组</option><option value="subscription" ${state.nodeGroupBy === 'subscription' ? 'selected' : ''}>按订阅分组</option><option value="region" ${state.nodeGroupBy === 'region' ? 'selected' : ''}>按地区分组</option></select>
        <select id="nodeSort" class="select node-tool-select" aria-label="排序方式"><option value="default" ${state.nodeSort === 'default' ? 'selected' : ''}>默认顺序</option><option value="latency" ${state.nodeSort === 'latency' ? 'selected' : ''}>延迟从低到高</option></select></div>
      <div class="node-filters">${filterBtns.map(([id, label]) => {
    const pool = allNodes.filter(regionMatches)
    const count = id === 'all' ? pool.length : pool.filter(n => n.protocol === id).length
    if (!allNodes.some(n => n.protocol === id) && id !== 'all' && state.nodeFilter !== id) return ''
    return `<button class="filter-btn ${state.nodeFilter === id ? 'active' : ''} ${count ? '' : 'empty-chip'}" data-filter="${id}">${label} (${count})</button>`
  }).join('')}</div>${renderRegionChips()}</div>`
  // 列表单独放一层：搜索时只重绘列表，不动搜索框（避免打断输入法）
  return toolsHtml + `<div id="nodeList">${renderNodeList()}</div>`
}

// 协议和地区两排筛选叠加：每排的数字按另一排当前的筛选来数，点哪个都不会出现“写着 1 个却是空的”。
// 记住的地区已经没有节点时当作“全部”，免得列表空着又找不到筛选条。
function activeRegionFilter() {
  const r = state.nodeRegion
  if (r === 'none') return (state.config?.nodes || []).some(n => !n.country) ? r : 'all'
  return nodeRegions().some(item => item.code === r) ? r : 'all'
}
// 用函数声明：启动时若停在节点页，render() 在模块加载阶段就会调用它们（const 箭头函数此时还在暂时性死区）
function protoMatches(n) { return state.nodeFilter === 'all' || n.protocol === state.nodeFilter }
function regionMatches(n) { const r = activeRegionFilter(); return r === 'all' || (r === 'none' ? !n.country : n.country === r) }

function renderRegionChips() {
  const all = state.config?.nodes || []
  const regions = nodeRegions()
  const unknownAll = all.filter(n => !n.country)
  if (!regions.length && !unknownAll.length) return ''
  const pool = all.filter(protoMatches)
  const active = activeRegionFilter()
  const chip = (value, label, count) => `<button class="filter-btn ${active === value ? 'active' : ''} ${count ? '' : 'empty-chip'}" data-region="${escapeHtml(value)}">${label} ${count}</button>`
  const unknown = pool.filter(n => !n.country).length
  const detect = unknownAll.length ? `<button class="btn btn-ghost btn-small" id="detectRegions" ${state.batchTesting ? 'disabled' : ''} title="测速一次，成功的节点会查出落地 IP 和地区；测速失败的节点查不了">${state.batchTesting ? '检测中…' : `检测地区（${unknownAll.length}）`}</button>` : ''
  return `<div class="node-filters region-filters"><span class="region-filters-label">地区</span>${chip('all', '全部', pool.length)}${regions.map(({ code }) => chip(code, regionLabel(code), pool.filter(n => n.country === code).length)).join('')}${unknownAll.length ? chip('none', '未检测', unknown) : ''}${detect}</div>`
}

function renderNodeList() {
  const allNodes = state.config?.nodes || []
  const currentNodeID = activeNodeID()
  // 链接粘到了搜索框里：提示一下，并给个一键导入。
  if (/^[a-z][a-z0-9+.-]*:\/\//i.test(state.nodeSearch.trim())) return '<div class="empty">这里是搜索框，只用来筛选节点。<br><button class="btn btn-primary" id="importFromSearch">导入这个链接</button></div>'
  const visible = new Set(visibleNodeIDs())
  const filtered = allNodes.filter(n => visible.has(n.id))
  if (!allNodes.length) return '<div class="empty">还没有节点，点右上角“导入 / 添加”</div>'
  if (!filtered.length) return '<div class="empty">没有匹配的节点<br><button class="btn btn-ghost" id="clearNodeFilters">清除筛选</button></div>'
  const bySubscription = state.nodeGroupBy === 'subscription'
  const row = n => {
    const current = currentNodeID === n.id
    const testing = state.nodeTesting.has(n.id)
    const id = escapeHtml(n.id)
    return `<li class="node-item ${current ? 'active' : ''} ${state.nodeSwitching ? 'busy' : ''}" data-node-id="${id}" tabindex="0" title="${current ? '当前节点' : '点击切换到此节点'}">
        <input class="node-select" type="checkbox" data-id="${id}" ${state.selectedNodes.has(n.id) ? 'checked' : ''} aria-label="选择 ${escapeHtml(n.name)}" />
        <div class="node-info">
          <div class="node-row"><span class="node-name">${escapeHtml(n.name)}</span>${current ? '<span class="node-current-tag">当前</span>' : ''}</div>
          <div class="node-meta">${bySubscription ? `<span class="node-proto">${escapeHtml(protocolLabel(n.protocol))}</span>` : ''}${n.country && state.nodeGroupBy !== 'region' ? `<span class="node-region ${n.exit_ip ? '' : 'guessed'}" title="${n.exit_ip ? '落地 IP ' + escapeHtml(n.exit_ip) : '按节点名推测，测速后查落地 IP'}">${regionLabel(n.country)}</span>` : ''}<span class="node-server">${escapeHtml(n.server)}:${n.port}</span>${ruleCount(n.id) ? `<span class="node-rule-count" title="有 ${ruleCount(n.id)} 条分流规则指定走这个节点">${ruleCount(n.id)} 条规则</span>` : ''}</div>
        </div>
        ${renderLatency(n.id)}
        <div class="node-actions">
          <button class="btn btn-ghost test-node desktop-only" data-id="${id}" ${testing ? 'disabled' : ''}>测试</button>
          <details class="rule-menu node-menu" data-rule-menu-id="node-${id}"><summary aria-label="节点操作" title="更多操作">⋯</summary><div class="rule-menu-list"><button class="rule-menu-action test-node mobile-only" data-id="${id}" ${testing ? 'disabled' : ''}>测试延迟</button><button class="rule-menu-action edit-node" data-id="${id}">编辑</button><button class="rule-menu-action route-to-node" data-id="${id}">让网站走这个节点…</button><button class="rule-menu-action export-node" data-id="${id}" ${busyAttr('node:' + n.id)}>复制分享链接</button><button class="rule-menu-action qrcode-node" data-id="${id}" ${busyAttr('node:' + n.id)}>显示二维码</button><button class="rule-menu-action danger rm-node" data-id="${id}" ${busyAttr('node:' + n.id)}>删除</button></div></details>
        </div>
      </li>`
  }
  const groupsHtml = nodeGroups(filtered, state.nodeGroupBy).map(group => {
    const nodes = state.nodeSort === 'latency' ? [...group.nodes].sort((a, b) => latencySortValue(a.id) - latencySortValue(b.id)) : group.nodes
    return `<div class="node-group"><div class="node-group-header"><span class="badge proxy" title="${escapeHtml(group.label)}">${escapeHtml(group.label)}</span><span class="node-group-count">${nodes.length} 个</span></div><ul class="node-list">${nodes.map(row).join('')}</ul></div>`
  }).join('')
  return groupsHtml
}

function protocolLabel(protocol) {
  return NODE_PROTOCOLS.find(([id]) => id === protocol)?.[1] || String(protocol || '未知').toUpperCase()
}

// nodeGroups keeps protocol groups in NODE_PROTOCOLS order and subscription
// groups in subscription order, with manually added nodes last.
function nodeGroups(nodes, groupBy) {
  const subs = state.config?.subscriptions || []
  const keyOf = n => groupBy === 'subscription' ? (n.subscription || '') : groupBy === 'region' ? (n.country || '') : (n.protocol || 'unknown')
  const order = groupBy === 'subscription' ? [...subs.map(s => s.id), ''] : groupBy === 'region' ? [...nodeRegions().map(r => r.code), ''] : NODE_PROTOCOLS.map(([id]) => id)
  const labelOf = key => groupBy === 'subscription'
    ? (key ? subs.find(s => s.id === key)?.name || '已删除的订阅' : '手动添加')
    : groupBy === 'region' ? (key ? `${regionFlag(key)} ${regionName(key)}` : '未检测地区') : protocolLabel(key)
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
  backgroundRender()
  let exitFound = false
  const worker = async () => {
    while (queue.length) {
      const id = queue.shift()
      try {
        const result = await window.go.app.App.TestNode(id)
        state.nodeHealth[id] = result
        // 第一次测速成功会顺便查到落地 IP 和地区，后端已保存，这里先同步到界面
        const node = result.exit_ip && (state.config?.nodes || []).find(n => n.id === id)
        if (node) Object.assign(node, { country: result.country, exit_ip: result.exit_ip }), exitFound = true
      } catch (e) {
        state.nodeHealth[id] = { node_id: id, healthy: false, message: errorText(e) }
      }
      state.nodeTesting.delete(id)
      saveNodeHealth()
      backgroundRender()
    }
  }
  await Promise.all(Array.from({ length: Math.min(NODE_TEST_CONCURRENCY, queue.length) }, worker))
  if (exitFound) await refreshConfig().catch(() => {})
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
    showToast(errorText(e), 'error')
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

// 错误转文字：桌面版 Wails reject 的是字符串，安卓桥是 Error，Go 的 error 序列化后可能是对象
function errorText(e, fallback = '未知错误') {
  if (e == null) return fallback
  if (typeof e === 'string') return e || fallback
  if (e.message) return String(e.message)
  if (e.error) return String(e.error)
  if (typeof e === 'object') {
    const text = JSON.stringify(e)
    return text && text !== '{}' ? text : fallback
  }
  return String(e)
}

function busyAttr(key) {
  return state.busy.has(key) ? 'disabled' : ''
}

// 防重复操作：同一个 key 进行中时忽略再次触发；按钮禁用由 render 按 state.busy 输出，重绘后也不会恢复可点
async function runOnce(key, action, el) {
  if (state.busy.has(key)) return
  state.busy.add(key)
  if (el) el.disabled = true
  try {
    await action()
  } catch (e) {
    showToast(errorText(e), 'error')
  } finally {
    state.busy.delete(key)
    renderApp()
  }
}

// 用户正在输入（文本框、下拉框获得焦点）时，后台轮询不整页重绘，等失焦后再补
function userEditing() {
  const el = document.activeElement
  if (!el || !app.contains(el)) return false
  return el.tagName === 'TEXTAREA' || el.tagName === 'SELECT' || (el.tagName === 'INPUT' && !/^(checkbox|radio|button|submit)$/.test(el.type))
}

function backgroundRender() {
  if (userEditing()) state.renderPending = true
  else renderApp()
}

// 配置读写序号：本地写入或发起新读取后，之前发出的读取结果作废，避免过期数据覆盖刚保存的值
let configSeq = 0

// 修改配置：以后端最新配置为底只改自己那部分，不拿可能过期的 state.config 整份覆盖
async function updateConfig(mutate) {
  const cfg = await window.go.app.App.GetConfig()
  mutate(cfg)
  configSeq++
  await window.go.app.App.SaveConfig(cfg)
  configSeq++
  state.config = cfg
  return cfg
}

// 按内容删除规则：下标来自可能过期的 state.config，不能直接按下标删
function ruleIndex(rules, target) {
  return (rules || []).findIndex(rule => rule.match_type === target.match_type && rule.value === target.value && rule.action === target.action)
}

function removeRule(list, target) {
  const rules = list || []
  const index = ruleIndex(rules, target)
  return index === -1 ? rules : rules.filter((_, i) => i !== index)
}

// saveRule 新增或替换（editing 为被修改的原规则）一条规则；same 判断是否与已有规则重复。
function saveRule(list, rule, editing, same, duplicateMessage) {
  const rules = list || []
  const index = editing ? ruleIndex(rules, editing) : -1
  if (editing && index === -1) throw new Error('这条规则已被删除或改动，请重新选择')
  if (rules.some((existing, i) => i !== index && same(existing))) throw new Error(duplicateMessage)
  return index === -1 ? [...rules, rule] : rules.map((existing, i) => i === index ? rule : existing)
}

// 文本框输入：输入法组字期间不处理，组字结束再处理一次
function onTextInput(input, apply) {
  let composing = false
  input.addEventListener('compositionstart', () => { composing = true })
  input.addEventListener('compositionend', () => { composing = false; apply() })
  input.addEventListener('input', event => { if (!composing && !event.isComposing) apply() })
}

function showToast(msg, type = '') {
  msg = String(msg)
  const t = document.querySelector('#toast')
  t.textContent = msg
  t.className = 'toast ' + type
  t.style.display = 'block'
  clearTimeout(t._timer)
  t._timer = setTimeout(() => { t.style.display = 'none' }, Math.min(10000, 3000 + msg.length * 80)) // 长提示多停留一会儿
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
    if (previousNodeID !== activeNodeID() || selectorSyncChanged) backgroundRender()
    updateStatusView()
  } catch (e) {
    showToast('获取状态失败: ' + errorText(e), 'error')
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
    if (changed) backgroundRender()
    if (newEvent) showToast(event.message, event.ok ? 'success' : 'error')
  } catch (e) {
    console.error('read auto switch status failed:', e)
  }
}

async function checkAppUpdate() {
  try {
    state.appUpdate = await window.go.app.App.CheckAppUpdate()
    backgroundRender()
  } catch (e) {
    console.error('check sbtun update failed:', e)
  }
}

// 下载新版替换程序文件，问过用户后启动新版本并退出当前进程（代理在运行时新版本会自动重连）。
async function updateApp() {
  if (state.appUpdating) return
  state.appUpdating = true
  renderApp()
  try {
    const result = await window.go.app.App.UpdateApp()
    if (!result.updated) {
      if (state.appUpdate) state.appUpdate.has_update = false
      showToast(result.message || '已是最新版本', 'success')
      return
    }
    const hint = state.running ? '代理会断开几秒，重启后自动重连。' : ''
    if (!window.confirm(`${result.message}，现在重启 sbtun 生效吗？${hint}`)) {
      if (state.appUpdate) state.appUpdate.has_update = false
      showToast(result.message + '，下次打开 sbtun 时生效', 'success')
      return
    }
    showToast('正在重启…', 'success')
    await window.go.app.App.RestartApp()
  } catch (e) {
    showToast('更新程序失败：' + errorText(e), 'error')
  } finally {
    state.appUpdating = false
    renderApp()
  }
}

async function refreshSingBoxVersion() {
  try {
    state.singBoxVersion = await window.go.app.App.SingBoxVersion()
    backgroundRender()
  } catch (e) {
    console.error('read sing-box version failed:', e)
  }
}

function renderCustomRules() {
  const rules = state.config?.custom_rules || []
  if (!rules.length) return '<div class="empty">还没有分流规则</div>'
  return rules.map((rule, index) => `
    <div class="node-item rule-item ${state.editingCustomRule && ruleIndex(rules, state.editingCustomRule) === index ? 'editing' : ''}">
      <span class="badge">${escapeHtml(ruleMatchLabel(rule.match_type))}</span>
      <span class="rule-content"><span class="node-name">${escapeHtml(rule.value)}</span>${ruleTarget(rule.action)}</span>
      <button class="btn btn-ghost edit-custom-rule" data-index="${index}">修改</button>
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
    backgroundRender()
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
  power.textContent = powerLabel()
  power.className = 'switch ' + (state.running ? 'on' : '')
  power.disabled = state.busy.has('power')
  updateMobileHero()
  dot.className = 'dot ' + dotClass(state.statusState)
}

async function refreshConfig() {
  const seq = ++configSeq
  state.configReading++
  try {
    const next = await window.go.app.App.GetConfig()
    if (seq !== configSeq) return // 期间有新的读取或本地写入，这次结果已过期
    const changed = JSON.stringify(state.config) !== JSON.stringify(next)
    state.config = next
    if (changed) backgroundRender()
  } catch (e) {
    showToast('获取配置失败: ' + errorText(e), 'error')
  } finally {
    state.configReading--
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

function ruleCount(nodeID) { return (state.config?.custom_rules || []).filter(rule => rule.action === 'node:' + nodeID).length }

// 规则去向：代理/直连/阻断、某个分组(sub:)或地区(region:)（组内自动选延迟最低）、某个节点（节点没了就标红，实际走当前节点）
function ruleTarget(action) {
  if (action.startsWith('region:')) {
    const code = action.slice(7)
    return `<span class="rule-target region">${regionLabel(code)} · 自动选最快</span>`
  }
  if (action.startsWith('group:')) {
    const g = (state.config?.node_groups || []).find(item => item.id === action.slice(6))
    return g
      ? `<span class="rule-target region">节点组 · ${escapeHtml(g.name)} · ${groupNodes(g).length} 个节点</span>`
      : '<span class="rule-target missing" title="节点组已被删除，暂时走当前节点">节点组已删除 · 暂走代理</span>'
  }
  if (action.startsWith('proto:')) return `<span class="rule-target region">${escapeHtml(protocolLabel(action.slice(6)))} 节点 · 自动选最快</span>`
  if (action.startsWith('sub:')) {
    const sub = (state.config?.subscriptions || []).find(item => item.id === action.slice(4))
    return sub
      ? `<span class="rule-target region">机场 · ${escapeHtml(sub.name)} · 自动选最快</span>`
      : '<span class="rule-target missing" title="指定的订阅已被删除，暂时走当前节点">订阅已删除 · 暂走代理</span>'
  }
  if (action.startsWith('node:')) {
    const node = (state.config?.nodes || []).find(n => n.id === action.slice(5))
    return node
      ? `<span class="rule-target node">节点 · ${node.country ? regionFlag(node.country) + ' ' : ''}${escapeHtml(node.name)}</span>`
      : '<span class="rule-target missing" title="指定的节点已被删除，暂时走当前节点">节点已失效 · 暂走代理</span>'
  }
  return `<span class="rule-target ${escapeHtml(action)}">${escapeHtml(ruleActionLabel(action))}</span>`
}

function renderRuleActionSelect() {
  return renderTargetSelect('ruleAction', state.customRuleForm.action, { newGroup: true })
}

function renderRuleNodeSelect() {
  return renderNodePicker('ruleNode', state.customRuleForm.node_id)
}

// 去向下拉：代理/直连/阻断、节点组、协议、机场、地区（组内自动选最快）、指定节点。
function renderTargetSelect(id, value, { newGroup = false, blockFirst = false } = {}) {
  let basic = RULE_ACTIONS.filter(item => item.id !== 'node')
  if (blockFirst) basic = [...basic.filter(item => item.id === 'block'), ...basic.filter(item => item.id !== 'block')]
  const regions = nodeRegions()
  const nodes = state.config?.nodes || []
  const subs = (state.config?.subscriptions || []).map(sub => ({ ...sub, count: nodes.filter(n => n.subscription === sub.id).length })).filter(sub => sub.count)
  const groups = state.config?.node_groups || []
  const protocols = [...new Set(nodes.map(n => n.protocol))]
  const opt = (v, label) => `<option value="${escapeHtml(v)}" ${value === v ? 'selected' : ''}>${label}</option>`
  return `<select id="${id}" class="select">${basic.map(item => opt(item.id, item.label)).join('')}${groups.length ? `<optgroup label="我的节点组">${groups.map(g => opt('group:' + g.id, `${escapeHtml(g.name)}（${groupNodes(g).length} 个节点）`)).join('')}</optgroup>` : ''}${protocols.length > 1 ? `<optgroup label="按协议（自动选延迟最低）">${protocols.map(pr => opt('proto:' + pr, `${escapeHtml(protocolLabel(pr))}（${nodes.filter(n => n.protocol === pr).length} 个节点）`)).join('')}</optgroup>` : ''}${subs.length ? `<optgroup label="按机场（自动选延迟最低）">${subs.map(sub => opt('sub:' + sub.id, `${escapeHtml(sub.name)}（${sub.count} 个节点）`)).join('')}</optgroup>` : ''}${regions.length ? `<optgroup label="按地区（自动选延迟最低）">${regions.map(({ code, count }) => opt('region:' + code, `${regionFlag(code)} ${escapeHtml(regionName(code))}（${count} 个节点）`)).join('')}</optgroup>` : ''}${opt('node', '指定节点…')}${newGroup ? '<option value="new-group">＋ 新建节点组…</option>' : ''}</select>`
}

function renderNodePicker(id, value) {
  const groups = nodeGroups(state.config?.nodes || [], 'subscription')
  return `<label class="rule-field rule-node-field"><span>节点</span><select id="${id}" class="select"><option value="">选择节点</option>${groups.map(group => `<optgroup label="${escapeHtml(group.label)}">${group.nodes.map(n => `<option value="${escapeHtml(n.id)}" ${value === n.id ? 'selected' : ''}>${n.country ? regionFlag(n.country) + ' ' : ''}${escapeHtml(n.name)}</option>`).join('')}</optgroup>`).join('')}</select></label>`
}

// 表单里“指定节点”拆成 node + node_id 两个值，存盘时合成 node:<id>；反过来也一样。
function splitTarget(action) {
  return action && action.startsWith('node:') ? { action: 'node', node_id: action.slice(5) } : { action: action || 'block', node_id: '' }
}

function ruleActionHint(action) {
  if (action.startsWith('region:')) return `走${regionName(action.slice(7))}节点里延迟最低的；某个节点挂了自动换下一个`
  if (action.startsWith('group:')) return '走这个节点组里延迟最低的节点；组里的节点会随订阅更新自动增减'
  if (action.startsWith('proto:')) return `走 ${protocolLabel(action.slice(6))} 节点里延迟最低的；某个节点挂了自动换下一个`
  if (action.startsWith('sub:')) return '走这个机场里延迟最低的节点；某个节点挂了自动换下一个'
  return (RULE_ACTIONS.find(item => item.id === action) || RULE_ACTIONS[0]).hint
}

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

// 只在开了诊断且停在路由页时读取，结果变了才重绘
async function refreshDiagnostics() {
  if (!state.config?.diagnostics_enabled || state.view !== 'routing') return
  const previous = JSON.stringify(state.diagnostics)
  try {
    state.diagnostics = await window.go.app.App.GetDiagnostics()
  } catch (e) {
    state.diagnostics = { message: errorText(e) }
  }
  if (JSON.stringify(state.diagnostics) !== previous) backgroundRender()
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
    if (state.view === 'capture') backgroundRender()
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

// 安卓扫码：识别到的内容放进“粘贴链接导入”，然后直接导入。
function scanAndImport() {
  return runOnce('scan', async () => {
    let text = ''
    try {
      text = String(await window.runtime.ScanQRCode() || '').trim()
    } catch (e) {
      throw new Error('扫码失败：' + errorText(e))
    }
    if (!text) return
    const keep = state.nodeFormOpen && !state.editingNodeId && state.nodeFormTab === 'link' ? state.manualForm.url.trim() : ''
    if (!keep) resetManualForm()
    state.manualForm.url = [keep, text].filter(Boolean).join('\n')
    state.editingNodeId = ''
    state.nodeFormTab = 'link'
    state.nodeFormOpen = true
    renderApp()
    document.querySelector('#importBtn')?.click()
  })
}

function renderApp() {
  state.renderPending = false
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
  syncNativeTheme()
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

// 安卓：系统状态栏/导航栏的图标颜色跟着主题走。
let nativeTheme = ''
function syncNativeTheme() {
  if (!IS_ANDROID || nativeTheme === state.theme) return
  nativeTheme = state.theme
  window.SbtunNative?.setTheme?.(state.theme !== 'light')
}

// 安卓返回键：先关弹窗，再回到概览；都没有时返回 false，由 APK 把应用退到后台。
window.__sbtunBack = () => {
  if (state.backup) closeBackup()
  else if (state.nodeQRCode) closeNodeQRCode()
  else if (state.nodeFormOpen || state.editingNodeId) closeNodeModal()
  else if (state.view !== 'overview') {
    state.view = 'overview'
    localStorage.setItem('sbtun-ui-view', state.view)
    renderApp()
  } else return false
  return true
}

function bindEvents() {
  if (!state.documentEventsBound) {
    document.addEventListener('click', event => {
      const clickedMenu = event.target.closest?.('.rule-menu')
      document.querySelectorAll('.rule-menu[open]').forEach(menu => {
        if (!clickedMenu || menu !== clickedMenu) menu.open = false
      })
    })
    document.addEventListener('keydown', event => {
      if (event.key !== 'Escape') return
      if (state.captureBodyViewer) {
        state.captureBodyViewer = null
        renderApp()
      } else if (state.backup) closeBackup()
      else if (state.nodeQRCode) closeNodeQRCode()
      else if (state.nodeFormOpen || state.editingNodeId) closeNodeModal()
    })
    // 输入框失焦后补上被推迟的后台重绘
    app.addEventListener('focusout', () => setTimeout(() => {
      if (state.renderPending && !userEditing()) renderApp()
    }))
    state.documentEventsBound = true
  }
  document.querySelector('#appUpdate')?.addEventListener('click', () => {
    if (!IS_ANDROID) updateApp()
    else if (state.appUpdate?.release_url) window.runtime.BrowserOpenURL(state.appUpdate.release_url)
  })
  document.querySelector('#updateApp')?.addEventListener('click', updateApp)
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
      if (state.view === 'routing') {
        await refreshIPv6Status()
        await refreshDiagnostics()
      }
      if (state.view === 'capture') await refreshCapture()
      renderApp()
    })
  })
  const nodeSearch = document.querySelector('#nodeSearch')
  if (nodeSearch) onTextInput(nodeSearch, () => {
    state.nodeSearch = nodeSearch.value
    localStorage.setItem('sbtun-ui-node-search', state.nodeSearch)
    const list = document.querySelector('#nodeList')
    if (!list) return renderApp()
    list.innerHTML = renderNodeList()
    bindNodeList(list)
    updateNodeHeading()
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
  document.querySelector('#updateSingBox')?.addEventListener('click', async () => {
    if (state.singBoxUpdating) return
    state.singBoxUpdating = true
    renderApp()
    try {
      const result = await window.go.app.App.UpdateSingBox()
      state.singBoxVersion = result.current_version || state.singBoxVersion
      showToast(result.message || 'sing-box 已是最新版本', 'success')
    } catch (e) {
      showToast('更新内核失败：' + errorText(e), 'error')
    } finally {
      state.singBoxUpdating = false
      renderApp()
    }
  })
  for (const [id, key] of [['nodeUrl', 'url'], ['nodeName', 'name'], ['nodeServer', 'server'], ['nodePort', 'port']]) {
    const input = document.querySelector('#' + id)
    if (input) input.addEventListener('input', () => { state.manualForm[key] = input.value })
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
    ruleAction.addEventListener('change', () => {
      if (ruleAction.value === 'new-group') { openGroupModal(null, true); return }
      state.customRuleForm.action = ruleAction.value
      renderApp()
    })
  }
  bindGroupEvents()
  const ruleNode = document.querySelector('#ruleNode')
  if (ruleNode) ruleNode.addEventListener('change', () => { state.customRuleForm.node_id = ruleNode.value })
  document.querySelectorAll('[data-region]').forEach(btn => btn.addEventListener('click', () => {
    state.nodeRegion = btn.dataset.region
    localStorage.setItem('sbtun-ui-node-region', state.nodeRegion)
    renderApp()
  }))
  document.querySelectorAll('.route-to-node').forEach(btn => btn.addEventListener('click', () => {
    Object.assign(state.customRuleForm, { action: 'node', node_id: btn.dataset.id })
    state.view = 'routing'
    renderApp()
  }))
  const addRuleBtn = document.querySelector('#addRuleBtn')
  if (addRuleBtn) addRuleBtn.addEventListener('click', () => {
    const { match_type: matchType, node_id: nodeID } = state.customRuleForm
    const action = state.customRuleForm.action === 'node' ? 'node:' + nodeID : state.customRuleForm.action
    const value = state.customRuleForm.value.trim()
    const error = action === 'node:' ? '请选择节点' : validateCustomRule(matchType, value)
    if (error) return showToast(error, 'error')
    runOnce('customRule', async () => {
      const editing = state.editingCustomRule
      await updateConfig(cfg => {
        cfg.custom_rules = saveRule(cfg.custom_rules, { match_type: matchType, value, action }, editing,
          rule => rule.match_type === matchType && rule.value.toLowerCase() === value.toLowerCase() && rule.action === action, '这条规则已经存在')
      })
      state.customRuleForm.value = ''
      state.editingCustomRule = null
      showToast(editing ? '规则已修改' : '规则已添加', 'success')
    }, addRuleBtn)
  })
  const ruleImport = document.querySelector('#ruleImport')
  if (ruleImport) {
    ruleImport.value = state.customRuleImport
    ruleImport.addEventListener('input', () => { state.customRuleImport = ruleImport.value })
  }
  const importRuleBtn = document.querySelector('#importRuleBtn')
  if (importRuleBtn) importRuleBtn.addEventListener('click', () => {
    const source = state.customRuleImport.trim()
    if (!source) return showToast('请输入规则链接、文件路径或文本', 'error')
    runOnce('importRule', async () => {
      const result = await window.go.app.App.ImportCustomRules(source)
      state.customRuleImport = ''
      await refreshConfig()
      if (result.rule_sets) await refreshRules()
      const parts = [result.rules && result.rules + ' 条规则', result.rule_sets && result.rule_sets + ' 个规则集（在“分流规则”页）'].filter(Boolean)
      let text = parts.length ? '已导入 ' + parts.join('、') : '没有导入新内容'
      if (result.existing) text += '，' + result.existing + ' 个规则集已存在'
      if (result.failed?.length) text += '；' + result.failed.length + ' 个下载失败：' + result.failed[0]
      showToast(text, result.failed?.length ? 'error' : 'success')
    }, importRuleBtn)
  })
  document.querySelectorAll('.edit-custom-rule').forEach(btn => btn.addEventListener('click', () => {
    const rule = state.config?.custom_rules?.[Number(btn.dataset.index)]
    if (!rule) return
    const node = rule.action.startsWith('node:')
    state.editingCustomRule = { ...rule }
    state.customRuleForm = { match_type: rule.match_type, value: rule.value, action: node ? 'node' : rule.action, node_id: node ? rule.action.slice(5) : '' }
    renderApp()
    document.querySelector('#ruleValue')?.focus()
    document.querySelector('.rule-editor')?.scrollIntoView({ block: 'nearest' })
  }))
  document.querySelector('#cancelEditRule')?.addEventListener('click', () => {
    state.editingCustomRule = null
    state.customRuleForm = { match_type: 'domain_suffix', value: '', action: 'proxy', node_id: '' }
    renderApp()
  })
  document.querySelectorAll('.remove-custom-rule').forEach(btn => btn.addEventListener('click', () => {
    const target = state.config?.custom_rules?.[Number(btn.dataset.index)]
    if (!target) return
    runOnce('customRule', async () => {
      await updateConfig(cfg => { cfg.custom_rules = removeRule(cfg.custom_rules, target) })
      showToast('规则已删除', 'success')
    }, btn)
  }))
  document.querySelectorAll('[data-power]').forEach(power => power.addEventListener('click', () => {
    runOnce('power', async () => {
      updateStatusView()
      try {
        if (state.running) await window.go.app.App.Stop()
        else await window.go.app.App.Start()
      } finally {
        await refreshStatus()
      }
    })
  }))

  document.querySelectorAll('.mode-btn').forEach(btn => {
    btn.addEventListener('click', async () => {
      const mode = btn.dataset.mode
      try {
        await window.go.app.App.SetRoutingMode(mode)
        await refreshConfig()
        await refreshRules()
        showToast('路由模式已切换', 'success')
      } catch (e) {
        showToast(errorText(e), 'error')
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
        showToast(errorText(e), 'error')
      }
    })
  }
  // 开关失败时 runOnce 结束会按 state.config 重绘，勾选状态自动回到实际值
  const diagnosticsToggle = document.querySelector('#diagnosticsToggle')
  if (diagnosticsToggle) diagnosticsToggle.addEventListener('change', () => {
    const enabled = diagnosticsToggle.checked
    runOnce('diagnostics', async () => {
      await updateConfig(cfg => { cfg.diagnostics_enabled = enabled })
      if (!enabled) state.diagnostics = null
      await refreshDiagnostics()
    }, diagnosticsToggle)
  })
  const ipv6Toggle = document.querySelector('#ipv6Toggle')
  if (ipv6Toggle) ipv6Toggle.addEventListener('change', () => {
    const enabled = ipv6Toggle.checked
    runOnce('ipv6', async () => {
      await updateConfig(cfg => { cfg.ipv6_enabled = enabled })
      await refreshIPv6Status()
      showToast(enabled && !state.ipv6Status?.available ? '设备没有可用 IPv6，已自动使用 IPv4' : 'IPv6 设置已应用', 'success')
    }, ipv6Toggle)
  })
  const webrtcProtectionToggle = document.querySelector('#webrtcProtectionToggle')
  if (webrtcProtectionToggle) webrtcProtectionToggle.addEventListener('change', () => {
    const enabled = webrtcProtectionToggle.checked
    runOnce('webrtc', async () => {
      await updateConfig(cfg => { cfg.webrtc_protection_enabled = enabled })
      showToast(enabled ? 'WebRTC 防泄漏已启用' : 'WebRTC 防泄漏已关闭', 'success')
    }, webrtcProtectionToggle)
  })

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
  if (captureEnabled) captureEnabled.addEventListener('change', () => {
    const enabled = captureEnabled.checked
    if (enabled && !readCaptureDomains().length) {
      captureEnabled.checked = false
      state.captureEnabledDraft = false
      state.captureEnabledDirty = false
      showToast('请先填写抓包规则；输入 * 可分析全部 HTTP/HTTPS 请求', 'error')
      captureDomains?.focus()
      return
    }
    state.captureEnabledDraft = enabled
    state.captureEnabledDirty = true
    runOnce('capture', async () => {
      try {
        await applyCaptureSettings(enabled)
      } catch (e) {
        state.captureEnabledDraft = Boolean(state.config?.capture_enabled)
        state.captureEnabledDirty = false
        throw e
      }
      showToast(enabled ? (state.captureStatus?.running ? '分析器已启动' : '已启用，开启 TUN 后开始分析') : '分析器已关闭', 'success')
    }, captureEnabled)
  })
  const saveCapture = document.querySelector('#saveCapture')
  if (saveCapture) saveCapture.addEventListener('click', () => {
    const enabled = state.captureEnabledDirty ? state.captureEnabledDraft : Boolean(captureEnabled?.checked)
    runOnce('capture', async () => {
      await applyCaptureSettings(enabled)
      showToast(enabled && !state.captureStatus?.running ? '配置已保存，开启 TUN 后开始分析' : '分析配置已应用', 'success')
    }, saveCapture)
  })
  const captureCertLevel = document.querySelector('#captureCertLevel')
  const certLevel = () => captureCertLevel?.value || 'system'
  document.querySelector('#installCaptureCA')?.addEventListener('click', event => runOnce('captureCA', async () => {
    const level = certLevel()
    await window.go.app.App.InstallCaptureCertificate(level)
    await refreshCapture()
    showToast(`分析证书已安装（${level === 'user' ? '当前用户' : '系统级'}）`, 'success')
  }, event.currentTarget))
  document.querySelector('#uninstallCaptureCA')?.addEventListener('click', event => runOnce('captureCA', async () => {
    await window.go.app.App.UninstallCaptureCertificate(certLevel())
    await refreshCapture()
    showToast('分析证书已卸载', 'success')
  }, event.currentTarget))
  document.querySelector('#clearCapture')?.addEventListener('click', event => {
    if (!window.confirm('确定删除全部抓包记录？此操作无法撤销。')) return
    runOnce('captureFile', async () => {
      await window.go.app.App.ClearCaptureFlows()
      state.selectedCaptureID = 0
      state.selectedCaptureFlow = null
      await refreshCapture()
    }, event.currentTarget)
  })
  document.querySelector('#saveCaptureFile')?.addEventListener('click', event => runOnce('captureFile', async () => {
    await window.go.app.App.SaveCaptureFlows()
    await refreshCapture()
    showToast('抓包已保存为 JSON', 'success')
  }, event.currentTarget))
  document.querySelectorAll('[data-capture-id]').forEach(row => row.addEventListener('click', async () => {
    state.selectedCaptureID = Number(row.dataset.captureId)
    try { state.selectedCaptureFlow = await window.go.app.App.GetCaptureFlow(state.selectedCaptureID) }
    catch (e) { return showToast(errorText(e), 'error') }
    renderApp()
  }))
  const captureSearch = document.querySelector('#captureSearch')
  if (captureSearch) onTextInput(captureSearch, () => {
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
      showToast(errorText(e), 'error')
    }
  })

  document.querySelectorAll('.update-sub').forEach(btn => {
    btn.addEventListener('click', async () => {
      if (state.subUpdating) return
      state.subUpdating = btn.dataset.id
      renderApp()
      try {
        const result = await window.go.app.App.UpdateSubscription(btn.dataset.id)
        showToast('订阅已更新 · ' + subscriptionResultText(result), 'success')
      } catch (e) {
        showToast('订阅更新失败: ' + errorText(e), 'error')
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
      if (state.subUpdating) return
      state.subUpdating = '*'
      renderApp()
      try {
        const results = await window.go.app.App.UpdateAllSubscriptions()
        const failed = results.filter(r => r.error)
        if (failed.length) showToast('部分订阅更新失败: ' + failed.map(r => `${r.name || r.id}: ${errorText(r.error)}`).join('; '), 'error')
        else showToast('全部订阅已更新', 'success')
      } catch (e) {
        showToast(errorText(e), 'error')
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
        showToast(errorText(e), 'error')
      }
    })
  })

  bindNodeList(document)
  const toggleSelectNodes = document.querySelector('#toggleSelectNodes')
  if (toggleSelectNodes) toggleSelectNodes.addEventListener('click', () => {
    const ids = visibleNodeIDs()
    const allSelected = ids.length > 0 && ids.every(id => state.selectedNodes.has(id))
    ids.forEach(id => {
      if (allSelected) state.selectedNodes.delete(id)
      else state.selectedNodes.add(id)
    })
    renderApp()
  })
  const batchExport = document.querySelector('#batchExportNodes')
  if (batchExport) batchExport.addEventListener('click', () => {
    const ids = selectedNodeIDs()
    if (!ids.length) return showToast('请先选择节点', 'error')
    runOnce('export', async () => {
      const count = await copyNodeLinks(ids)
      showToast(`已复制 ${count} 个节点链接`, 'success')
    }, batchExport)
  })
  const batchDelete = document.querySelector('#batchDeleteNodes')
  if (batchDelete) batchDelete.addEventListener('click', () => {
    const ids = selectedNodeIDs()
    if (!ids.length) return showToast('请先选择节点', 'error')
    if (!confirm(`确定删除选中的 ${ids.length} 个节点吗？`)) return
    runOnce('delete', async () => {
      await window.go.app.App.RemoveNodes(ids)
      ids.forEach(id => state.selectedNodes.delete(id))
      await refreshConfig()
      saveNodeHealth()
      showToast('批量删除成功', 'success')
    }, batchDelete)
  })
  const batchTest = document.querySelector('#batchTestNodes')
  if (batchTest) batchTest.addEventListener('click', async () => {
    if (state.batchTesting) return
    const selected = selectedNodeIDs()
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
  document.querySelector('#detectRegions')?.addEventListener('click', async () => {
    if (state.batchTesting) return
    const ids = (state.config?.nodes || []).filter(n => !n.country).map(n => n.id)
    state.batchTesting = true
    try {
      await testNodes(ids)
      const left = (state.config?.nodes || []).filter(n => !n.country).length
      showToast(left ? `已检测 ${ids.length - left} 个，${left} 个测速失败查不了地区` : `已检测 ${ids.length} 个节点的地区`, left ? 'error' : 'success')
    } finally {
      state.batchTesting = false
      renderApp()
    }
  })
  document.querySelectorAll('[data-filter]').forEach(btn => {
    btn.addEventListener('click', () => {
      state.nodeFilter = btn.dataset.filter
      localStorage.setItem('sbtun-ui-node-filter', state.nodeFilter)
      renderApp()
    })
  })
  document.querySelectorAll('[data-close-node-qrcode]').forEach(element => element.addEventListener('click', closeNodeQRCode))
  bindBackupModal()
  document.querySelector('#copyNodeQRCodeLink')?.addEventListener('click', async () => {
    if (!state.nodeQRCode) return
    try {
      await window.runtime.ClipboardSetText(state.nodeQRCode.link)
      showToast('节点链接已复制', 'success')
    } catch (e) {
      showToast(errorText(e), 'error')
    }
  })

  document.querySelectorAll('.update-rule').forEach(btn => {
    btn.addEventListener('click', async () => {
      if (state.ruleUpdating || state.allRulesUpdating) return
      state.ruleUpdating = btn.dataset.id
      renderApp()
      try {
        await window.go.app.App.UpdateRule(btn.dataset.id)
        showToast('规则集更新成功', 'success')
      } catch (e) {
        showToast(errorText(e), 'error')
      } finally {
        state.ruleUpdating = ''
        await refreshRules()
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
      await window.runtime.ClipboardSetText(url)
      showToast('规则集 URL 已复制', 'success')
    } catch (_) {
      showToast(url, 'info')
    }
  }))
  document.querySelectorAll('.edit-rule').forEach(btn => btn.addEventListener('click', () => {
    const rule = state.rules?.find(item => item.id === btn.dataset.id)
    if (!rule) return
    state.editingRuleSetID = rule.id
    state.ruleSetForm = { name: rule.name, url: rule.url, ...splitTarget(rule.action) }
    renderApp()
    document.querySelector('#ruleSetName')?.focus()
  }))

  const ruleSetName = document.querySelector('#ruleSetName')
  const ruleSetURL = document.querySelector('#ruleSetURL')
  if (ruleSetName) ruleSetName.addEventListener('input', () => { state.ruleSetForm.name = ruleSetName.value })
  if (ruleSetURL) ruleSetURL.addEventListener('input', () => { state.ruleSetForm.url = ruleSetURL.value })
  const ruleSetAction = document.querySelector('#ruleSetAction')
  if (ruleSetAction) ruleSetAction.addEventListener('change', () => {
    state.ruleSetForm.action = ruleSetAction.value
    renderApp()
  })
  const ruleSetNode = document.querySelector('#ruleSetNode')
  if (ruleSetNode) ruleSetNode.addEventListener('change', () => { state.ruleSetForm.node_id = ruleSetNode.value })
  const addRuleSetBtn = document.querySelector('#addRuleSetBtn')
  if (addRuleSetBtn) addRuleSetBtn.addEventListener('click', () => {
    const name = state.ruleSetForm.name.trim()
    const url = state.ruleSetForm.url.trim()
    if (!name || !url) return showToast('请输入规则集名称和 URL', 'error')
    const { action: kind, node_id: nodeID } = state.ruleSetForm
    if (kind === 'node' && !nodeID) return showToast('请选择节点', 'error')
    const action = kind === 'node' ? 'node:' + nodeID : kind
    const editingID = state.editingRuleSetID
    runOnce('ruleSet', async () => {
      if (editingID) await window.go.app.App.EditRuleSet(editingID, name, url, action)
      else await window.go.app.App.AddRuleSet(name, url, action)
      state.ruleSetForm = { name: '', url: '', action: 'block', node_id: '' }
      state.editingRuleSetID = ''
      await refreshRules()
      showToast(editingID ? '规则集已修改' : '规则集已添加', 'success')
    }, addRuleSetBtn)
  })
  const cancelEditRuleSet = document.querySelector('#cancelEditRuleSet')
  if (cancelEditRuleSet) cancelEditRuleSet.addEventListener('click', () => {
    state.editingRuleSetID = ''
    state.ruleSetForm = { name: '', url: '', action: 'block', node_id: '' }
    renderApp()
  })
  document.querySelectorAll('.toggle-rule').forEach(btn => btn.addEventListener('click', () => {
    const rule = state.rules?.find(item => item.id === btn.dataset.id)
    if (!rule) return
    runOnce('rule:' + rule.id, async () => {
      await window.go.app.App.SetRuleEnabled(rule.id, !rule.enabled)
      await refreshRules()
      showToast(rule.enabled ? '规则集已停用' : '规则集已启用', 'success')
    }, btn)
  }))
  document.querySelectorAll('.delete-rule').forEach(btn => btn.addEventListener('click', () => {
    if (!window.confirm('删除这个规则集及其本地文件？')) return
    runOnce('rule:' + btn.dataset.id, async () => {
      await window.go.app.App.DeleteRuleSet(btn.dataset.id)
      await refreshRules()
      showToast('规则集已删除', 'success')
    }, btn)
  }))
  const restoreDefaultRules = document.querySelector('#restoreDefaultRules')
  if (restoreDefaultRules) restoreDefaultRules.addEventListener('click', () => runOnce('ruleSet', async () => {
    await window.go.app.App.RestoreDefaultRuleSets()
    await refreshRules()
    showToast('默认规则集已恢复', 'success')
  }, restoreDefaultRules))

  const filterRuleMatchType = document.querySelector('#filterRuleMatchType')
  const filterRuleValue = document.querySelector('#filterRuleValue')
  if (filterRuleMatchType) filterRuleMatchType.addEventListener('change', () => { state.filterRuleForm.match_type = filterRuleMatchType.value })
  if (filterRuleValue) filterRuleValue.addEventListener('input', () => { state.filterRuleForm.value = filterRuleValue.value })
  const addFilterRuleBtn = document.querySelector('#addFilterRuleBtn')
  if (addFilterRuleBtn) addFilterRuleBtn.addEventListener('click', () => {
    const matchType = state.filterRuleForm.match_type
    const value = state.filterRuleForm.value.trim()
    if (!value) return showToast('请输入过滤内容', 'error')
    const error = validateCustomRule(matchType, value)
    if (error && matchType !== 'ip_cidr') return showToast(error, 'error')
    runOnce('filterRule', async () => {
      const editing = state.editingFilterRule
      await updateConfig(cfg => {
        cfg.dns_filter_rules = saveRule(cfg.dns_filter_rules, { match_type: matchType, value, action: 'block' }, editing,
          rule => rule.match_type === matchType && rule.value.toLowerCase() === value.toLowerCase(), '这条过滤规则已经存在')
      })
      state.filterRuleForm = { match_type: 'domain_suffix', value: '' }
      state.dnsFilterFormOpen = false
      state.editingFilterRule = null
      showToast(editing ? '过滤规则已修改' : '过滤规则已添加', 'success')
    }, addFilterRuleBtn)
  })
  const openFilterRuleBtn = document.querySelector('#openFilterRuleBtn')
  if (openFilterRuleBtn) openFilterRuleBtn.addEventListener('click', () => {
    state.editingFilterRule = null
    state.filterRuleForm = { match_type: 'domain_suffix', value: '' }
    state.dnsFilterFormOpen = true
    renderApp()
  })
  const cancelFilterRuleBtn = document.querySelector('#cancelFilterRuleBtn')
  if (cancelFilterRuleBtn) cancelFilterRuleBtn.addEventListener('click', () => {
    state.dnsFilterFormOpen = false
    state.editingFilterRule = null
    state.filterRuleForm = { match_type: 'domain_suffix', value: '' }
    renderApp()
  })
  document.querySelectorAll('.edit-filter-rule').forEach(btn => btn.addEventListener('click', () => {
    const rule = state.config?.dns_filter_rules?.[Number(btn.dataset.index)]
    if (!rule) return
    state.editingFilterRule = { ...rule }
    state.filterRuleForm = { match_type: rule.match_type, value: rule.value }
    state.dnsFilterFormOpen = true
    renderApp()
    document.querySelector('#filterRuleValue')?.focus()
  }))
  document.querySelectorAll('.remove-filter-rule').forEach(btn => btn.addEventListener('click', () => {
    const target = state.config?.dns_filter_rules?.[Number(btn.dataset.index)]
    if (!target) return
    runOnce('filterRule', async () => {
      await updateConfig(cfg => { cfg.dns_filter_rules = removeRule(cfg.dns_filter_rules, target) })
      showToast('过滤规则已删除', 'success')
    }, btn)
  }))

  document.querySelector('#updateAllRules')?.addEventListener('click', async () => {
    if (state.ruleUpdating || state.allRulesUpdating) return
    state.allRulesUpdating = true
    renderApp()
    try {
      // Go 的 []error 序列化后可能是空对象，字符串或对象都尽量取出文字
      const errors = await window.go.app.App.UpdateAllRules() || []
      if (errors.length) {
        const texts = errors.map(e => errorText(e, '')).filter(Boolean)
        showToast(`部分规则集更新失败（${errors.length} 个）` + (texts.length ? '：' + texts.join('；') : '，可在列表里单独更新查看原因'), 'error')
      } else {
        showToast('全部规则集更新成功', 'success')
      }
    } catch (e) {
      showToast(errorText(e), 'error')
    } finally {
      state.allRulesUpdating = false
      await refreshRules()
    }
  })

  document.querySelector('#subAutoUpdate')?.addEventListener('change', async event => {
    const hours = Number(event.target.value)
    try {
      await window.go.app.App.SetSubscriptionAutoUpdate(hours)
      await refreshConfig()
      renderApp()
      showToast(hours ? `订阅将每 ${hours} 小时自动更新` : '已关闭订阅自动更新', 'success')
    } catch (e) {
      showToast('设置失败：' + errorText(e), 'error')
    }
  })

  document.querySelectorAll('[data-scan-qr]').forEach(button => button.addEventListener('click', scanAndImport))

  const importBtn = document.querySelector('#importBtn')
  if (importBtn) importBtn.addEventListener('click', () => {
    const links = state.manualForm.url.split(/\r?\n/).map(line => line.trim()).filter(Boolean)
    if (!links.length) return showToast('请输入节点或订阅链接', 'error')
    const editingID = state.editingNodeId
    if (editingID && links.length !== 1) return showToast('编辑节点时只能粘贴一条节点链接', 'error')
    runOnce('import', async () => {
      renderApp() // 显示“导入中…”
      if (editingID) {
        await window.go.app.App.UpdateNodeFromLink(editingID, links[0])
        await refreshConfig()
        await refreshRules()
        resetManualForm()
        state.editingNodeId = ''
        state.nodeFormOpen = false
        showToast('节点已按链接更新', 'success')
        return
      }
      let added = 0
      const failed = []
      for (const link of links) {
        try {
          added += await window.go.app.App.ImportSubscription(link)
        } catch (e) {
          failed.push({ link, error: errorText(e) })
        }
      }
      await refreshConfig()
      await refreshRules()
      // 失败的链接留在输入框里，方便修改后重试。
      state.manualForm.url = failed.map(item => item.link).join('\n')
      state.nodeFormOpen = failed.length > 0
      if (!failed.length && !added) showToast('节点已存在，没有重复添加', 'success')
      else if (!failed.length) showToast(`导入成功，新增 ${added} 个节点`, 'success')
      else if (failed.length === links.length) showToast('导入失败：' + failed[0].error, 'error')
      else showToast(`新增 ${added} 个节点，${failed.length} 条失败：${failed[0].error}`, 'error')
    }, importBtn)
  })

  const addNodeBtn = document.querySelector('#addNodeBtn')
  const nodeProtocol = document.querySelector('#nodeProtocol')
  if (nodeProtocol) {
    nodeProtocol.addEventListener('change', () => {
      state.manualProtocol = nodeProtocol.value
      renderApp()
    })
  }
  if (addNodeBtn) addNodeBtn.addEventListener('click', () => {
    const name = state.manualForm.name.trim()
    const server = state.manualForm.server.trim()
    const port = parseInt(state.manualForm.port.trim(), 10)
    const protocol = state.manualProtocol
    if (!name || !server || !Number.isInteger(port) || port < 1 || port > 65535) {
      return showToast('请填写名称、服务器和端口', 'error')
    }
    const editingID = state.editingNodeId
    runOnce('node', async () => {
      const node = { id: editingID || 'manual-' + Date.now(), name, server, port, protocol, settings: buildManualSettings(protocol, state.manualForm.password.trim()) }
      if (editingID) {
        // 同协议编辑时保留表单里没有的设置项
        const original = state.config?.nodes?.find(n => n.id === editingID)
        if (original?.protocol === protocol) {
          const managed = formSettingKeys(protocol)
          const kept = {}
          for (const [key, value] of Object.entries(original.settings || {})) if (!managed.has(key)) kept[key] = value
          node.settings = { ...kept, ...node.settings }
        }
        await window.go.app.App.UpdateNode(editingID, node)
        state.editingNodeId = ''
      } else {
        await window.go.app.App.AddNode(node)
      }
      await refreshConfig()
      await refreshRules()
      showToast(editingID ? '节点已修改' : '节点已添加', 'success')
      resetManualForm()
      state.nodeFormOpen = false
    }, addNodeBtn)
  })
  document.querySelectorAll('#node-import input').forEach(input => {
    input.addEventListener('keydown', event => {
      if (event.key === 'Enter' && addNodeBtn) {
        event.preventDefault()
        addNodeBtn.click()
      }
    })
  })
}

// 节点列表里的事件；搜索时只重绘列表，单独重新绑定
function bindNodeList(root) {
  root.querySelector('#clearNodeFilters')?.addEventListener('click', () => {
    state.nodeFilter = 'all'
    state.nodeRegion = 'all'
    state.nodeSearch = ''
    for (const key of ['sbtun-ui-node-filter', 'sbtun-ui-node-region', 'sbtun-ui-node-search']) localStorage.removeItem(key)
    renderApp()
  })
  root.querySelector('#importFromSearch')?.addEventListener('click', () => {
    resetManualForm()
    state.manualForm.url = state.nodeSearch.trim()
    state.nodeSearch = ''
    localStorage.setItem('sbtun-ui-node-search', '')
    state.editingNodeId = ''
    state.nodeFormTab = 'link'
    state.nodeFormOpen = true
    renderApp()
    document.querySelector('#importBtn')?.click()
  })
  root.querySelectorAll('.node-item[data-node-id]').forEach(row => {
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
  root.querySelectorAll('.node-select').forEach(input => input.addEventListener('change', () => {
    if (input.checked) state.selectedNodes.add(input.dataset.id)
    else state.selectedNodes.delete(input.dataset.id)
    renderApp()
  }))
  root.querySelectorAll('.rm-node').forEach(btn => btn.addEventListener('click', () => {
    const id = btn.dataset.id
    const node = state.config?.nodes?.find(item => item.id === id)
    if (!window.confirm(`确定删除节点“${node?.name || '此节点'}”吗？`)) return
    runOnce('node:' + id, async () => {
      await window.go.app.App.RemoveNode(id)
      state.selectedNodes.delete(id)
      await refreshConfig()
      await refreshRules()
      saveNodeHealth()
      showToast('节点已删除', 'success')
    }, btn)
  }))
  root.querySelectorAll('.edit-node').forEach(btn => btn.addEventListener('click', () => {
    const node = state.config?.nodes?.find(item => item.id === btn.dataset.id)
    if (node) beginEditNode(node)
  }))
  root.querySelectorAll('.export-node').forEach(btn => btn.addEventListener('click', () => runOnce('node:' + btn.dataset.id, async () => {
    await copyNodeLinks([btn.dataset.id])
    showToast('节点链接已复制', 'success')
  }, btn)))
  root.querySelectorAll('.qrcode-node').forEach(btn => btn.addEventListener('click', () => {
    const node = state.config?.nodes?.find(item => item.id === btn.dataset.id)
    if (!node) return
    runOnce('node:' + node.id, async () => {
      const qr = await window.go.app.App.NodeQRCode(node.id)
      state.nodeQRCode = { name: node.name, link: qr.link, image: qr.image }
    }, btn)
  }))
  root.querySelectorAll('.test-node').forEach(btn => btn.addEventListener('click', async () => {
    await testNodes([btn.dataset.id])
    const latency = nodeLatency(btn.dataset.id)
    if (latency && !latency.ok) showToast(latency.label, 'error')
  }))
}

// 搜索只重绘列表时，同步标题栏里依赖可见节点的按钮
function updateNodeHeading() {
  const ids = visibleNodeIDs()
  const toggle = document.querySelector('#toggleSelectNodes')
  const batchTest = document.querySelector('#batchTestNodes')
  if (toggle) {
    toggle.disabled = !ids.length
    toggle.textContent = ids.length && ids.every(id => state.selectedNodes.has(id)) ? '取消全选' : '全选'
  }
  if (batchTest) batchTest.disabled = state.batchTesting || !ids.length
}

async function copyNodeLinks(ids) {
  const links = await window.go.app.App.ExportNodes(ids)
  if (!links?.length) throw new Error('没有可导出的节点')
  await window.runtime.ClipboardSetText(links.join('\n'))
  return links.length
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
  if (node.protocol === 'hysteria' || node.protocol === 'hysteria2') form.server_name = node.settings?.sni || ''
  state.manualProtocol = node.protocol
  state.manualForm = form
  state.editingNodeId = node.id
  state.nodeFormTab = 'manual'
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
  if (platform === 'darwin' || platform === 'android') return
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
    if (!IS_ANDROID) await refreshCapture()
    setInterval(refreshStatus, 2000)
    setInterval(() => { if (!state.configReading) refreshConfig() }, 1500)
    setInterval(refreshDiagnostics, 1500)
    if (!IS_ANDROID) setInterval(refreshCapture, 1500)
    setInterval(refreshAutoSwitch, 2000)
    checkAppUpdate()
    setInterval(checkAppUpdate, APP_UPDATE_INTERVAL)
  } else {
    showToast('Wails 运行时未加载', 'error')
    renderApp()
  }
})

// ---- 节点组（组合条件）----
function renderNodeGroupsCard() {
  const groups = state.config?.node_groups || []
  const usedBy = id => (state.config?.custom_rules || []).filter(rule => rule.action === 'group:' + id).length
  const health = id => state.nodeHealth?.[id]?.url
  const fastest = g => groupNodes(g).map(n => ({ n, h: health(n.id) })).filter(x => x.h?.ok).sort((a, b) => a.h.latency_ms - b.h.latency_ms)[0]
  const rows = groups.map(g => {
    const count = groupNodes(g).length, best = fastest(g), used = usedBy(g.id)
    return `<div class="group-row"><div class="group-main"><div class="group-title"><strong>${escapeHtml(g.name)}</strong><span class="muted">${count} 个节点${used ? ` · ${used} 条规则在用` : ''}</span></div><div class="group-conds">${groupConditions(g)}</div></div><div class="group-best">${best ? `<span class="muted">当前最快</span><span>${best.n.country ? regionFlag(best.n.country) + ' ' : ''}${escapeHtml(best.n.name)}</span><span class="latency">${best.h.latency_ms} ms</span>` : count ? '<span class="muted">还没测速</span>' : '<span class="rule-target missing">没有匹配的节点</span>'}</div><button class="btn btn-ghost btn-sm" data-edit-group="${escapeHtml(g.id)}">编辑</button></div>`
  }).join('')
  return `<section class="card"><div class="section-heading"><div><h2>节点组</h2><p class="muted">按地区、机场、协议、名称组合挑节点，规则可以直接走一个组，组内自动用最快的。</p></div><button class="btn btn-primary" id="newGroupBtn">新建节点组</button></div>${rows || '<p class="muted empty-hint">还没有节点组。</p>'}</section>`
}

function openGroupModal(id, fromRule = false) {
  const g = (state.config?.node_groups || []).find(item => item.id === id)
  state.groupModal = { id: g?.id || '', fromRule, name: g?.name || '', regions: [...(g?.regions || [])], subscriptions: [...(g?.subscriptions || [])], protocols: [...(g?.protocols || [])], keyword: g?.keyword || '', exclude: g?.exclude || '' }
  renderApp()
}

function renderGroupModal() {
  const m = state.groupModal
  if (!m) return ''
  const nodes = state.config?.nodes || []
  const chip = (field, value, label, count) => `<button class="filter-btn ${m[field].includes(value) ? 'active' : ''}" data-group-chip="${field}" data-value="${escapeHtml(value)}">${label} <span class="muted">${count}</span></button>`
  const subs = [...(state.config?.subscriptions || []).map(sub => sub.id), ...(nodes.some(n => !n.subscription) ? [''] : [])]
  const protocols = [...new Set(nodes.map(n => n.protocol))]
  const matched = groupNodes(m)
  return `<div class="modal" id="groupModal" role="dialog" aria-modal="true" aria-labelledby="groupModalTitle"><div class="modal-backdrop" data-close-group></div>
    <section class="modal-panel group-modal">
      <header class="modal-header"><h2 id="groupModalTitle">${m.id ? '编辑节点组' : '新建节点组'}</h2><button class="icon-btn" data-close-group aria-label="关闭" title="关闭">×</button></header>
      <div class="modal-body group-form">
        <label class="rule-field"><span>名称</span><input id="groupName" class="input" value="${escapeHtml(m.name)}" placeholder="例如：美国 hy2、流媒体解锁" /></label>
        <div class="group-dim"><span>机场</span><div class="filter-row">${subs.map(id => chip('subscriptions', id, escapeHtml(subscriptionName(id)), nodes.filter(n => (n.subscription || '') === id).length)).join('')}</div></div>
        <div class="group-dim"><span>地区</span><div class="filter-row">${nodeRegions().map(({ code, count }) => chip('regions', code, regionLabel(code), count)).join('')}</div></div>
        <div class="group-dim"><span>协议</span><div class="filter-row">${protocols.map(pr => chip('protocols', pr, escapeHtml(protocolLabel(pr)), nodes.filter(n => n.protocol === pr).length)).join('')}</div></div>
        <div class="group-keywords"><label class="rule-field"><span>名称包含</span><input id="groupKeyword" class="input" value="${escapeHtml(m.keyword)}" placeholder="IPLC 解锁（任一即可）" /></label><label class="rule-field"><span>名称排除</span><input id="groupExclude" class="input" value="${escapeHtml(m.exclude)}" placeholder="过期 剩余 官网" /></label></div>
        <p class="muted modal-hint">不选的项表示不限；同一项里选多个是“任一”，不同项之间是“同时满足”。订阅更新后组里的节点会自动增减。</p>
        <div class="group-preview"><div class="group-preview-head">匹配 <strong>${matched.length}</strong> 个节点</div>${matched.slice(0, 12).map(n => `<span class="group-node">${n.country ? regionFlag(n.country) + ' ' : ''}${escapeHtml(n.name)} <span class="muted">${escapeHtml(protocolLabel(n.protocol))}</span></span>`).join('')}${matched.length > 12 ? `<span class="muted">… 还有 ${matched.length - 12} 个</span>` : ''}</div>
      </div>
      <footer class="modal-footer">${m.id ? '<button class="btn btn-ghost danger" id="deleteGroupBtn">删除</button>' : ''}<button class="btn btn-ghost" data-close-group>取消</button><button class="btn btn-primary" id="saveGroupBtn" ${matched.length ? '' : 'disabled'}>${m.fromRule ? '保存并用于这条规则' : '保存'}</button></footer>
    </section></div>`
}

function bindGroupEvents() {
  document.querySelector('#newGroupBtn')?.addEventListener('click', () => openGroupModal(null))
  document.querySelectorAll('[data-edit-group]').forEach(btn => btn.addEventListener('click', () => openGroupModal(btn.dataset.editGroup)))
  const m = state.groupModal
  if (!m) return
  document.querySelectorAll('[data-close-group]').forEach(el => el.addEventListener('click', () => { state.groupModal = null; renderApp() }))
  document.querySelectorAll('[data-group-chip]').forEach(btn => btn.addEventListener('click', () => {
    const list = m[btn.dataset.groupChip], value = btn.dataset.value
    list.includes(value) ? list.splice(list.indexOf(value), 1) : list.push(value)
    renderApp()
  }))
  for (const [sel, field] of [['#groupName', 'name'], ['#groupKeyword', 'keyword'], ['#groupExclude', 'exclude']]) {
    const input = document.querySelector(sel)
    // 关键词改动要刷新预览：失焦时重绘，输入时只记值，避免打字时丢焦点
    input?.addEventListener('input', () => { m[field] = input.value })
    if (field !== 'name') input?.addEventListener('change', () => renderApp())
  }
  document.querySelector('#saveGroupBtn')?.addEventListener('click', () => runOnce('group', async () => {
    const name = m.name.trim()
    if (!name) throw new Error('请填写节点组名称')
    const id = m.id || 'g' + Date.now().toString(36)
    await updateConfig(cfg => {
      cfg.node_groups = cfg.node_groups || []
      if (cfg.node_groups.some(g => g.id !== id && g.name === name)) throw new Error('已有同名节点组')
      const group = { id, name, regions: m.regions, subscriptions: m.subscriptions, protocols: m.protocols, keyword: m.keyword.trim(), exclude: m.exclude.trim() }
      const i = cfg.node_groups.findIndex(g => g.id === id)
      i >= 0 ? cfg.node_groups.splice(i, 1, group) : cfg.node_groups.push(group)
    })
    if (m.fromRule) state.customRuleForm.action = 'group:' + id
    state.groupModal = null
    showToast('节点组已保存', 'success')
  }))
  document.querySelector('#deleteGroupBtn')?.addEventListener('click', () => runOnce('group', async () => {
    await updateConfig(cfg => { cfg.node_groups = (cfg.node_groups || []).filter(g => g.id !== m.id) })
    state.groupModal = null
    showToast('节点组已删除，用它的规则暂走代理', 'success')
  }))
}
