package app

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"net/url"
	"strings"
)

// shadowrocketLink 是 Shadowrocket 导出的 vless/vmess 链接：
// vless://base64(加密方式:uuid@地址:端口)?remarks=名称&obfs=websocket&obfsParam=Host&path=/ws&tls=1&peer=SNI
type shadowrocketLink struct {
	method, uuid, server, port string
	transport, host, path      string
	query                      url.Values
	name                       string
}

// parseShadowrocketLink 识别 Shadowrocket 格式；标准链接（地址里有 @ 或端口）返回 false。
func parseShadowrocketLink(link string) (shadowrocketLink, bool) {
	scheme, rest, ok := strings.Cut(link, "://")
	if !ok {
		return shadowrocketLink{}, false
	}
	var fragment string
	rest, fragment, _ = strings.Cut(rest, "#")
	encoded, rawQuery, _ := strings.Cut(rest, "?")
	encoded = strings.TrimSuffix(encoded, "/")
	if encoded == "" || strings.ContainsAny(encoded, "@:") {
		return shadowrocketLink{}, false
	}
	if scheme == "vmess" && rawQuery == "" {
		return shadowrocketLink{}, false // 普通 vmess://base64(JSON)
	}
	decoded, err := tryBase64Decode(encoded)
	if err != nil {
		return shadowrocketLink{}, false
	}
	userinfo, hostport, ok := strings.Cut(decoded, "@")
	if !ok {
		return shadowrocketLink{}, false
	}
	method, uuid, ok := strings.Cut(userinfo, ":")
	if !ok {
		uuid, method = method, ""
	}
	server, port, err := net.SplitHostPort(strings.TrimSpace(hostport))
	if err != nil || uuid == "" {
		return shadowrocketLink{}, false
	}
	vals, _ := url.ParseQuery(rawQuery)
	r := shadowrocketLink{method: method, uuid: uuid, server: server, port: port, query: vals, path: vals.Get("path")}
	r.transport, r.host = shadowrocketTransport(vals)
	if name, err := url.PathUnescape(fragment); err == nil && name != "" {
		r.name = name
	} else {
		r.name = vals.Get("remarks")
	}
	return r, true
}

// shadowrocketTransport 把 obfs/obfsParam 换成传输方式和 Host。
func shadowrocketTransport(vals url.Values) (transport, host string) {
	switch strings.ToLower(vals.Get("obfs")) {
	case "websocket", "ws":
		transport = "ws"
	case "grpc":
		transport = "grpc"
	case "h2", "http2", "http":
		transport = "http"
	case "httpupgrade":
		transport = "httpupgrade"
	}
	host = vals.Get("obfsParam")
	if strings.HasPrefix(strings.TrimSpace(host), "{") {
		// 有的版本写成 {"Host":"example.com"}
		var headers map[string]string
		host = ""
		if json.Unmarshal([]byte(vals.Get("obfsParam")), &headers) == nil {
			host = firstNonEmpty(headers["Host"], headers["host"])
		}
	}
	return transport, host
}

// shadowrocketQuery 把标准链接里混用的 Shadowrocket 参数补成标准参数（标准参数优先）。
func shadowrocketQuery(vals url.Values) {
	transport, host := shadowrocketTransport(vals)
	fill := func(key, value string) {
		if vals.Get(key) == "" && value != "" {
			vals.Set(key, value)
		}
	}
	fill("sni", vals.Get("peer"))
	fill("type", transport)
	fill("host", host)
	if vals.Get("type") == "grpc" {
		fill("serviceName", vals.Get("path"))
		if vals.Get("serviceName") == vals.Get("path") {
			vals.Del("path")
		}
	}
}

// standardVLESS 把 Shadowrocket 格式转成标准 vless 链接。
func (r shadowrocketLink) standardVLESS() string {
	q := url.Values{}
	set := func(key, value string) {
		if value != "" {
			q.Set(key, value)
		}
	}
	set("type", r.transport)
	set("host", r.host)
	if r.transport == "grpc" {
		set("serviceName", r.path)
	} else {
		set("path", r.path)
	}
	if r.query.Get("tls") == "1" {
		q.Set("security", "tls")
	}
	if r.query.Get("pbk") != "" {
		q.Set("security", "reality")
	}
	set("sni", r.query.Get("peer"))
	for _, key := range []string{"alpn", "pbk", "sid", "fp", "flow", "packetEncoding"} {
		set(key, r.query.Get(key))
	}
	if r.query.Get("xtls") == "2" && q.Get("flow") == "" {
		q.Set("flow", "xtls-rprx-vision")
	}
	if queryEnabled(r.query, "allowInsecure", "insecure") {
		q.Set("allowInsecure", "1")
	}
	u := url.URL{Scheme: "vless", User: url.User(r.uuid), Host: net.JoinHostPort(r.server, r.port), RawQuery: q.Encode(), Fragment: r.name}
	return u.String()
}

// standardVMess 把 Shadowrocket 格式转成标准 vmess://base64(JSON) 链接。
func (r shadowrocketLink) standardVMess() string {
	cfg := map[string]string{"v": "2", "ps": r.name, "add": r.server, "port": r.port, "id": r.uuid, "aid": r.query.Get("alterId"), "scy": r.method, "net": r.transport, "host": r.host, "path": r.path, "sni": r.query.Get("peer"), "alpn": r.query.Get("alpn"), "fp": r.query.Get("fp")}
	if cfg["net"] == "" {
		cfg["net"] = "tcp"
	}
	if r.query.Get("tls") == "1" {
		cfg["tls"] = "tls"
	}
	if queryEnabled(r.query, "allowInsecure", "insecure") {
		cfg["allowInsecure"] = "1"
	}
	data, _ := json.Marshal(cfg)
	return "vmess://" + base64.StdEncoding.EncodeToString(data)
}
