package protocol_test

import (
	"testing"

	"github.com/WangYihang/Proxy-Verifier/internal/model"
	"github.com/WangYihang/Proxy-Verifier/internal/protocol"
)

// runHandler drives one protocol handler against an in-process proxy + echo
// server and asserts the verification succeeded end to end.
func runHandler(t *testing.T, protocolName, host string, port uint16, targetURL string, handler func(*model.Task) model.Result) {
	t.Helper()
	task := model.NewTask(0, "integration", protocolName, host, port, targetURL, 8, handler)
	task.Secret = integrationSecret
	result := handler(&task)
	if result.Error != "" {
		t.Fatalf("handler returned error: %s", result.Error)
	}
	if result.RawProxyOriginResponse == "" {
		t.Error("expected the origin response to be recorded")
	}
}

func TestHttpViaHttpProxy(t *testing.T) {
	echo := startEchoServer(t)
	host, port := startForwardProxy(t, false)
	runHandler(t, "http", host, port, echo.URL, protocol.HttpViaHttpProxy)
}

func TestHttpViaHttpsProxy(t *testing.T) {
	echo := startEchoServer(t)
	host, port := startForwardProxy(t, true)
	runHandler(t, "https", host, port, echo.URL, protocol.HttpViaHttpsProxy)
}

func TestHttpViaHttpTunnel(t *testing.T) {
	echo := startEchoServer(t)
	host, port := startTunnelProxy(t, false)
	runHandler(t, "http", host, port, echo.URL, protocol.HttpViaHttpTunnel)
}

func TestHttpViaHttpsTunnel(t *testing.T) {
	echo := startEchoServer(t)
	host, port := startTunnelProxy(t, true)
	runHandler(t, "https", host, port, echo.URL, protocol.HttpViaHttpsTunnel)
}

func TestHttpViaHttpTransparentProxy(t *testing.T) {
	echo := startEchoServer(t)
	host, port := startTransparentProxy(t, false)
	runHandler(t, "http", host, port, echo.URL, protocol.HttpViaHttpTransparentProxy)
}

func TestHttpViaHttpsTransparentProxy(t *testing.T) {
	echo := startEchoServer(t)
	host, port := startTransparentProxy(t, true)
	runHandler(t, "https", host, port, echo.URL, protocol.HttpViaHttpsTransparentProxy)
}

func TestHttpViaSocksTunnel(t *testing.T) {
	echo := startEchoServer(t)
	host, port := startSocks5Proxy(t)
	runHandler(t, "socks5", host, port, echo.URL, protocol.HttpViaSocksTunnel)
}

// TestHandlerFailsWithoutProxy makes sure a handler reports an error (rather
// than a false success) when the proxy is unreachable.
func TestHandlerFailsWithoutProxy(t *testing.T) {
	echo := startEchoServer(t)
	// 127.0.0.1:1 is a port nothing listens on.
	task := model.NewTask(0, "integration", "http", "127.0.0.1", 1, echo.URL, 2, protocol.HttpViaHttpProxy)
	result := protocol.HttpViaHttpProxy(&task)
	if result.Error == "" {
		t.Error("expected an error when the proxy is unreachable")
	}
}

func TestDispatchTable(t *testing.T) {
	table := protocol.TargetProtocl2ProxyProtocol2SubProxyProtocol2Handler

	http, ok := table["http"]
	if !ok {
		t.Fatal("expected an 'http' target protocol entry")
	}

	wantSubProtocols := map[string][]string{
		"http":    {"proxy", "transparent_proxy", "tunnel"},
		"https":   {"proxy", "transparent_proxy", "tunnel"},
		"socks4":  {"tunnel"},
		"socks4a": {"tunnel"},
		"socks5":  {"tunnel"},
	}
	for proxyProto, subs := range wantSubProtocols {
		handlers, ok := http[proxyProto]
		if !ok {
			t.Errorf("missing proxy protocol %q", proxyProto)
			continue
		}
		for _, sub := range subs {
			if handlers[sub] == nil {
				t.Errorf("missing handler for http/%s/%s", proxyProto, sub)
			}
		}
	}
}
