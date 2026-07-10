package e2e

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const e2eSecret = "e2e-secret"

// buildBinary compiles a command from this module into dir and returns its path.
func buildBinary(t *testing.T, dir, importPath, name string) string {
	t.Helper()
	bin := filepath.Join(dir, name)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", bin, importPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building %s failed: %v\n%s", importPath, err, out)
	}
	return bin
}

// startEchoServer starts an in-process copy of the controlled echo server.
func startEchoServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		challenge := r.URL.Query().Get("challenge")
		sum := md5.Sum([]byte(challenge + e2eSecret))
		_ = json.NewEncoder(w).Encode(map[string]string{"response": hex.EncodeToString(sum[:])})
	}))
	srv.Config.SetKeepAlivesEnabled(false)
	t.Cleanup(srv.Close)
	return srv
}

// startForwardProxy starts an in-process HTTP forward proxy and returns its port.
func startForwardProxy(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
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
	return ln.Addr().(*net.TCPAddr).Port
}

func run(t *testing.T, name string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, out)
	}
}

// TestEndToEnd builds the real verifier and exporter binaries and runs the full
// pipeline: verifier probes a proxy (an in-process forward proxy) against an
// in-process echo server, and exporter distills the log into the list of
// working proxies.
func TestEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping end-to-end test in -short mode")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	dir := t.TempDir()
	verifier := buildBinary(t, dir, "github.com/WangYihang/Proxy-Verifier/cmd/verifier", "verifier")
	exporter := buildBinary(t, dir, "github.com/WangYihang/Proxy-Verifier/cmd/exporter", "exporter")

	echo := startEchoServer(t)
	proxyPort := startForwardProxy(t)

	inputCSV := filepath.Join(dir, "proxies.csv")
	if err := os.WriteFile(inputCSV, []byte(fmt.Sprintf("http,127.0.0.1,%d\n", proxyPort)), 0644); err != nil {
		t.Fatal(err)
	}
	verifierLog := filepath.Join(dir, "verifier.log")
	exportOut := filepath.Join(dir, "available.txt")

	run(t, verifier,
		"--input-file", inputCSV,
		"--output-file", verifierLog,
		"--url", echo.URL+"/",
		"--secret", e2eSecret,
		"--num-workers", "4",
		"--timeout", "5",
		"--monitor-interval", "1",
	)

	// The verifier log should contain at least one successful (error-free) result.
	logData, err := os.ReadFile(verifierLog)
	if err != nil {
		t.Fatalf("reading verifier log: %v", err)
	}
	if !strings.Contains(string(logData), "\"error\":\"\"") {
		t.Fatalf("expected at least one successful result in the log:\n%s", logData)
	}

	run(t, exporter, "--input-file", verifierLog, "--output-file", exportOut)

	exported, err := os.ReadFile(exportOut)
	if err != nil {
		t.Fatalf("reading exporter output: %v", err)
	}
	want := fmt.Sprintf("http://127.0.0.1:%d/", proxyPort)
	if !strings.Contains(string(exported), want) {
		t.Fatalf("exporter output %q does not contain the working proxy %q", string(exported), want)
	}
}
