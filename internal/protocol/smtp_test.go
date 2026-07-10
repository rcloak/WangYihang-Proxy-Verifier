package protocol_test

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/WangYihang/Proxy-Verifier/internal/protocol"
)

func smtpAnswer(challenge string) string {
	sum := md5.Sum([]byte(challenge + integrationSecret))
	return hex.EncodeToString(sum[:])
}

// startSmtpEchoServer starts an in-process SMTP echo server that answers HELO
// with md5(challenge+secret), mirroring cmd/echo-server/smtp.
func startSmtpEchoServer(t *testing.T) (string, uint16) {
	t.Helper()
	ln := newListener(t, false)
	go acceptLoop(ln, func(conn net.Conn) {
		defer conn.Close()
		reader := bufio.NewReader(conn)
		_, _ = io.WriteString(conn, "220 test SMTP echo\r\n")
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			parts := strings.Fields(strings.TrimRight(line, "\r\n"))
			if len(parts) == 0 {
				continue
			}
			switch strings.ToUpper(parts[0]) {
			case "HELO", "EHLO":
				challenge := ""
				if len(parts) > 1 {
					challenge = parts[1]
				}
				_, _ = io.WriteString(conn, fmt.Sprintf("250 %s\r\n", smtpAnswer(challenge)))
			case "QUIT":
				_, _ = io.WriteString(conn, "221 Bye\r\n")
				return
			default:
				_, _ = io.WriteString(conn, "250 OK\r\n")
			}
		}
	})
	return hostPort(t, ln.Addr().String())
}

func smtpURL(host string, port uint16) string {
	return fmt.Sprintf("smtp://%s:%d", host, port)
}

func TestSmtpViaHttpTunnel(t *testing.T) {
	smtpHost, smtpPort := startSmtpEchoServer(t)
	host, port := startTunnelProxy(t, false)
	runHandler(t, "http", host, port, smtpURL(smtpHost, smtpPort), protocol.SmtpViaHttpTunnel)
}

func TestSmtpViaHttpsTunnel(t *testing.T) {
	smtpHost, smtpPort := startSmtpEchoServer(t)
	host, port := startTunnelProxy(t, true)
	runHandler(t, "https", host, port, smtpURL(smtpHost, smtpPort), protocol.SmtpViaHttpsTunnel)
}

func TestSmtpViaSocksTunnel(t *testing.T) {
	smtpHost, smtpPort := startSmtpEchoServer(t)
	// A relay delay separates the SOCKS reply from the SMTP greeting so
	// h12.io/socks' single-read reply parser is not confused by them coalescing
	// on the loopback interface (real remote proxies are separated by latency).
	host, port := startSocks5ProxyWithDelay(t, 200*time.Millisecond)
	runHandler(t, "socks5", host, port, smtpURL(smtpHost, smtpPort), protocol.SmtpViaSocksTunnel)
}

func TestSmtpViaHttpTunnelRejected(t *testing.T) {
	smtpHost, smtpPort := startSmtpEchoServer(t)
	host, port := startRejectingTunnelProxy(t, false)
	task := taskFor("http", host, port, smtpURL(smtpHost, smtpPort), protocol.SmtpViaHttpTunnel)
	result := protocol.SmtpViaHttpTunnel(&task)
	if result.Error == "" {
		t.Error("expected an error when the tunnel is rejected")
	}
}

func TestDispatchTableSmtp(t *testing.T) {
	smtp, ok := protocol.TargetProtocl2ProxyProtocol2SubProxyProtocol2Handler["smtp"]
	if !ok {
		t.Fatal("expected an 'smtp' target protocol entry")
	}
	for _, proxyProto := range []string{"http", "https", "socks4", "socks4a", "socks5"} {
		handlers, ok := smtp[proxyProto]
		if !ok {
			t.Errorf("missing proxy protocol %q for smtp", proxyProto)
			continue
		}
		if handlers["tunnel"] == nil {
			t.Errorf("missing handler for smtp/%s/tunnel", proxyProto)
		}
		// SMTP cannot be carried by plain/transparent HTTP proxies.
		if handlers["proxy"] != nil || handlers["transparent_proxy"] != nil {
			t.Errorf("smtp/%s should only support tunneling", proxyProto)
		}
	}
}
