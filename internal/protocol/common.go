package protocol

import "github.com/WangYihang/Proxy-Verifier/internal/model"

var TargetProtocl2ProxyProtocol2SubProxyProtocol2Handler = map[string]map[string]map[string]func(task *model.Task) model.Result{
	// Access HTTP Server via Proxies
	"http": {
		"http": {
			"proxy":             HttpViaHttpProxy,
			"transparent_proxy": HttpViaHttpTransparentProxy,
			"tunnel":            HttpViaHttpTunnel,
		},
		"https": {
			"proxy":             HttpViaHttpsProxy,
			"transparent_proxy": HttpViaHttpsTransparentProxy,
			"tunnel":            HttpViaHttpsTunnel,
		},
		"socks4": {
			"tunnel": HttpViaSocksTunnel,
		},
		"socks4a": {
			"tunnel": HttpViaSocksTunnel,
		},
		"socks5": {
			"tunnel": HttpViaSocksTunnel,
		},
	},
	// Access an SMTP Server via Proxies. Only tunneling proxies (HTTP CONNECT
	// and SOCKS) can carry a raw SMTP session; plain and transparent HTTP
	// proxies operate at the HTTP application layer and do not apply.
	"smtp": {
		"http": {
			"tunnel": SmtpViaHttpTunnel,
		},
		"https": {
			"tunnel": SmtpViaHttpsTunnel,
		},
		"socks4": {
			"tunnel": SmtpViaSocksTunnel,
		},
		"socks4a": {
			"tunnel": SmtpViaSocksTunnel,
		},
		"socks5": {
			"tunnel": SmtpViaSocksTunnel,
		},
	},
}
