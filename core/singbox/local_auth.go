package singbox

// LocalAuth protects the loopback services sbtun exposes while running: the
// clash API (Bearer secret) and the capture proxy pair (Basic credential).
// Without it any local program could read connections, switch nodes, or use
// those ports as an open proxy. A zero value disables authentication.
type LocalAuth struct {
	ClashSecret   string `json:"clash_secret"`
	ProxyUsername string `json:"proxy_username"`
	ProxyPassword string `json:"proxy_password"`
}

func (a LocalAuth) proxyEnabled() bool {
	return a.ProxyUsername != "" && a.ProxyPassword != ""
}
