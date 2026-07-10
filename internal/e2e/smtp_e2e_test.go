package e2e

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

func waitForPort(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s to accept connections", addr)
}

// startTunnelProxy starts an in-process HTTP CONNECT tunnel proxy (the kind that
// can carry a raw SMTP session) and returns its port.
func startTunnelProxy(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				br := bufio.NewReader(c)
				req, err := http.ReadRequest(br)
				if err != nil || req.Method != http.MethodConnect {
					return
				}
				target, err := net.Dial("tcp", req.Host)
				if err != nil {
					_, _ = io.WriteString(c, "HTTP/1.1 502 Bad Gateway\r\n\r\n")
					return
				}
				defer target.Close()
				_, _ = io.WriteString(c, "HTTP/1.1 200 Connection established\r\n\r\n")
				go func() { _, _ = io.Copy(target, br) }()
				_, _ = io.Copy(c, target)
			}(conn)
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return ln.Addr().(*net.TCPAddr).Port
}

// TestEndToEndSmtp builds the real smtp-echo-server, verifier and exporter
// binaries and runs the full SMTP pipeline: the verifier probes an in-process
// CONNECT tunnel proxy against the real SMTP echo server and exporter distills
// the log into the list of working proxies.
func TestEndToEndSmtp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping end-to-end test in -short mode")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	dir := t.TempDir()
	smtpServer := buildBinary(t, dir, "github.com/WangYihang/Proxy-Verifier/cmd/echo-server/smtp", "smtp-echo-server")
	verifier := buildBinary(t, dir, "github.com/WangYihang/Proxy-Verifier/cmd/verifier", "verifier")
	exporter := buildBinary(t, dir, "github.com/WangYihang/Proxy-Verifier/cmd/exporter", "exporter")

	const secret = "e2e-smtp-secret"
	smtpPort := freePort(t)
	smtpAddr := fmt.Sprintf("127.0.0.1:%d", smtpPort)

	ctx, cancel := context.WithCancel(context.Background())
	echoCmd := exec.CommandContext(ctx, smtpServer,
		"-b", "127.0.0.1", "-p", fmt.Sprint(smtpPort),
		"-l", filepath.Join(dir, "smtp.log"), "-s", secret)
	if err := echoCmd.Start(); err != nil {
		cancel()
		t.Fatalf("starting smtp echo server: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		_ = echoCmd.Wait()
	})
	waitForPort(t, smtpAddr, 10*time.Second)

	tunnelPort := startTunnelProxy(t)

	inputCSV := filepath.Join(dir, "proxies.csv")
	if err := os.WriteFile(inputCSV, []byte(fmt.Sprintf("http,127.0.0.1,%d\n", tunnelPort)), 0644); err != nil {
		t.Fatal(err)
	}
	verifierLog := filepath.Join(dir, "verifier.log")
	exportOut := filepath.Join(dir, "available.txt")

	run(t, verifier,
		"--input-file", inputCSV,
		"--output-file", verifierLog,
		"--url", fmt.Sprintf("smtp://%s", smtpAddr),
		"--secret", secret,
		"--num-workers", "4",
		"--timeout", "5",
		"--monitor-interval", "1",
	)

	logData, err := os.ReadFile(verifierLog)
	if err != nil {
		t.Fatalf("reading verifier log: %v", err)
	}
	if !strings.Contains(string(logData), "\"error\":\"\"") {
		t.Fatalf("expected at least one successful SMTP verification in the log:\n%s", logData)
	}

	run(t, exporter, "--input-file", verifierLog, "--output-file", exportOut)

	exported, err := os.ReadFile(exportOut)
	if err != nil {
		t.Fatalf("reading exporter output: %v", err)
	}
	want := fmt.Sprintf("http://127.0.0.1:%d/", tunnelPort)
	if !strings.Contains(string(exported), want) {
		t.Fatalf("exporter output %q does not contain the working proxy %q", string(exported), want)
	}
}
