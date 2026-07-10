package protocol_test

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/WangYihang/Proxy-Verifier/internal/model"
	"github.com/WangYihang/Proxy-Verifier/internal/protocol"
)

// startRejectingTunnelProxy accepts a CONNECT request but replies with a
// non-200 status, simulating a proxy that refuses to open the tunnel.
func startRejectingTunnelProxy(t *testing.T, tlsWrap bool) (string, uint16) {
	t.Helper()
	ln := newListener(t, tlsWrap)
	go acceptLoop(ln, func(conn net.Conn) {
		defer conn.Close()
		br := bufio.NewReader(conn)
		if _, err := http.ReadRequest(br); err != nil {
			return
		}
		_, _ = io.WriteString(conn, "HTTP/1.1 403 Forbidden\r\n\r\n")
	})
	return hostPort(t, ln.Addr().String())
}

func TestHttpViaHttpTunnelRejected(t *testing.T) {
	echo := startEchoServer(t)
	host, port := startRejectingTunnelProxy(t, false)
	task := model.NewTask(0, "integration", "http", host, port, echo.URL, 4, protocol.HttpViaHttpTunnel)
	result := protocol.HttpViaHttpTunnel(&task)
	if result.Error == "" {
		t.Error("expected an error when the tunnel is rejected")
	}
}

func TestHttpViaHttpsTunnelRejected(t *testing.T) {
	echo := startEchoServer(t)
	host, port := startRejectingTunnelProxy(t, true)
	task := model.NewTask(0, "integration", "https", host, port, echo.URL, 4, protocol.HttpViaHttpsTunnel)
	result := protocol.HttpViaHttpsTunnel(&task)
	if result.Error == "" {
		t.Error("expected an error when the TLS tunnel is rejected")
	}
}

// TestHttpTunnelInvalidTarget exercises the target-URI parsing failure branch:
// the proxy dial succeeds but the target URL cannot be resolved to host:port.
func TestHttpTunnelInvalidTarget(t *testing.T) {
	host, port := startTunnelProxy(t, false)
	task := model.NewTask(0, "integration", "http", host, port, "unknownscheme://host-without-port", 4, protocol.HttpViaHttpTunnel)
	result := protocol.HttpViaHttpTunnel(&task)
	if result.Error == "" {
		t.Error("expected an error for an unresolvable target URI")
	}
}
