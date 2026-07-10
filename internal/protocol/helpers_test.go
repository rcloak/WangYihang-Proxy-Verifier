package protocol_test

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/WangYihang/Proxy-Verifier/internal/util"
)

const integrationSecret = "integration-secret"

var testTLSCert tls.Certificate

// TestMain installs a self-signed certificate as a trusted root so that the
// (certificate-verifying) HTTPS proxy handler can talk to our in-process TLS
// proxies.
func TestMain(m *testing.M) {
	cert, certPEM := mustGenerateCert()
	testTLSCert = cert

	f, err := os.CreateTemp("", "proxy-verifier-ca-*.pem")
	if err == nil {
		_, _ = f.Write(certPEM)
		_ = f.Close()
		// Consulted by crypto/x509 on Linux to build the system root pool.
		_ = os.Setenv("SSL_CERT_FILE", f.Name())
		defer os.Remove(f.Name())
	}

	os.Exit(m.Run())
}

func mustGenerateCert() (tls.Certificate, []byte) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:             time.Unix(0, 0),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.IPv6loopback},
		DNSNames:              []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		panic(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		panic(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return cert, certPEM
}

func hostPort(t *testing.T, addr string) (string, uint16) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("bad address %q: %v", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("bad port in %q: %v", addr, err)
	}
	return host, uint16(port)
}

// startEchoServer starts an in-process copy of the controlled echo server: it
// answers any request with md5(challenge+secret).
func startEchoServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		challenge := r.URL.Query().Get("challenge")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"response": util.HandleChallenge(challenge, integrationSecret),
		})
	}))
	// Close connections after each response so the proxies' pipes drain cleanly.
	srv.Config.SetKeepAlivesEnabled(false)
	t.Cleanup(srv.Close)
	return srv
}

func newListener(t *testing.T, tlsWrap bool) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	if tlsWrap {
		ln = tls.NewListener(ln, &tls.Config{Certificates: []tls.Certificate{testTLSCert}})
	}
	t.Cleanup(func() { _ = ln.Close() })
	return ln
}

func acceptLoop(ln net.Listener, handle func(net.Conn)) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go handle(conn)
	}
}

// startForwardProxy is an ordinary HTTP forward proxy: it forwards absolute-form
// requests to the origin. Used for both the HTTP and HTTPS proxy handlers.
func startForwardProxy(t *testing.T, tlsWrap bool) (string, uint16) {
	t.Helper()
	ln := newListener(t, tlsWrap)
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			outReq, err := http.NewRequest(r.Method, r.URL.String(), r.Body)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			outReq.Header = r.Header.Clone()
			resp, err := http.DefaultTransport.RoundTrip(outReq)
			if err != nil {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			defer resp.Body.Close()
			for k, vs := range resp.Header {
				for _, v := range vs {
					w.Header().Add(k, v)
				}
			}
			w.WriteHeader(resp.StatusCode)
			_, _ = io.Copy(w, resp.Body)
		}),
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return hostPort(t, ln.Addr().String())
}

// startTunnelProxy is an HTTP CONNECT tunnel proxy.
func startTunnelProxy(t *testing.T, tlsWrap bool) (string, uint16) {
	t.Helper()
	ln := newListener(t, tlsWrap)
	go acceptLoop(ln, func(conn net.Conn) {
		defer conn.Close()
		br := bufio.NewReader(conn)
		req, err := http.ReadRequest(br)
		if err != nil || req.Method != http.MethodConnect {
			return
		}
		target, err := net.Dial("tcp", req.Host)
		if err != nil {
			_, _ = io.WriteString(conn, "HTTP/1.1 502 Bad Gateway\r\n\r\n")
			return
		}
		defer target.Close()
		_, _ = io.WriteString(conn, "HTTP/1.1 200 Connection established\r\n\r\n")
		go func() { _, _ = io.Copy(target, br) }()
		_, _ = io.Copy(conn, target)
	})
	return hostPort(t, ln.Addr().String())
}

// startTransparentProxy reads an origin-form request and forwards it to the
// host named in the Host header.
func startTransparentProxy(t *testing.T, tlsWrap bool) (string, uint16) {
	t.Helper()
	ln := newListener(t, tlsWrap)
	go acceptLoop(ln, func(conn net.Conn) {
		defer conn.Close()
		br := bufio.NewReader(conn)
		req, err := http.ReadRequest(br)
		if err != nil {
			return
		}
		target, err := net.Dial("tcp", req.Host)
		if err != nil {
			return
		}
		defer target.Close()
		if err := req.Write(target); err != nil {
			return
		}
		_, _ = io.Copy(conn, target)
	})
	return hostPort(t, ln.Addr().String())
}

// startSocks5Proxy is a minimal no-auth SOCKS5 CONNECT proxy.
func startSocks5Proxy(t *testing.T) (string, uint16) {
	return startSocks5ProxyWithDelay(t, 0)
}

// startSocks5ProxyWithDelay is like startSocks5Proxy but waits relayDelay before
// relaying the target's data back to the client. This models the proxy->target
// round-trip latency that, in the real world, separates the SOCKS reply from a
// server-speaks-first greeting (e.g. SMTP) -- a separation h12.io/socks relies
// on, since it reads the CONNECT reply with a single fixed-size read.
func startSocks5ProxyWithDelay(t *testing.T, relayDelay time.Duration) (string, uint16) {
	t.Helper()
	ln := newListener(t, false)
	go acceptLoop(ln, socks5Handler(relayDelay))
	return hostPort(t, ln.Addr().String())
}

func socks5Handler(relayDelay time.Duration) func(net.Conn) {
	return func(conn net.Conn) {
		defer conn.Close()
		br := bufio.NewReader(conn)

		// Greeting: VER, NMETHODS, METHODS...
		ver, err := br.ReadByte()
		if err != nil || ver != 0x05 {
			return
		}
		nMethods, err := br.ReadByte()
		if err != nil {
			return
		}
		if _, err := io.CopyN(io.Discard, br, int64(nMethods)); err != nil {
			return
		}
		// Select "no authentication".
		if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
			return
		}

		// Request: VER, CMD, RSV, ATYP, DST.ADDR, DST.PORT
		header := make([]byte, 4)
		if _, err := io.ReadFull(br, header); err != nil {
			return
		}
		var host string
		switch header[3] {
		case 0x01: // IPv4
			buf := make([]byte, 4)
			if _, err := io.ReadFull(br, buf); err != nil {
				return
			}
			host = net.IP(buf).String()
		case 0x03: // domain
			length, err := br.ReadByte()
			if err != nil {
				return
			}
			buf := make([]byte, length)
			if _, err := io.ReadFull(br, buf); err != nil {
				return
			}
			host = string(buf)
		case 0x04: // IPv6
			buf := make([]byte, 16)
			if _, err := io.ReadFull(br, buf); err != nil {
				return
			}
			host = net.IP(buf).String()
		default:
			return
		}
		portBuf := make([]byte, 2)
		if _, err := io.ReadFull(br, portBuf); err != nil {
			return
		}
		target := net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(portBuf))))

		upstream, err := net.Dial("tcp", target)
		if err != nil {
			// Reply: general failure.
			_, _ = conn.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
			return
		}
		defer upstream.Close()
		// Reply: success, bound address 0.0.0.0:0.
		if _, err := conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
			return
		}
		if relayDelay > 0 {
			time.Sleep(relayDelay)
		}
		go func() { _, _ = io.Copy(upstream, br) }()
		_, _ = io.Copy(conn, upstream)
	}
}
