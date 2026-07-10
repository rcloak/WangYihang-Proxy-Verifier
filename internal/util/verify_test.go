package util_test

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/WangYihang/Proxy-Verifier/internal"
	"github.com/WangYihang/Proxy-Verifier/internal/model"
	"github.com/WangYihang/Proxy-Verifier/internal/util"
)

const testSecret = "test-secret"

func answerFor(challenge string) string {
	sum := md5.Sum([]byte(challenge + testSecret))
	return hex.EncodeToString(sum[:])
}

// serveOneEchoResponse plays the role of the controlled echo server on the far
// end of an already-established connection: it reads a single HTTP request and
// replies with the md5(challenge+secret) answer, mirroring the real server.
func serveOneEchoResponse(conn net.Conn) {
	defer conn.Close()
	request, err := http.ReadRequest(bufio.NewReader(conn))
	if err != nil {
		return
	}
	challenge := request.URL.Query().Get("challenge")
	body, err := json.Marshal(map[string]string{"response": answerFor(challenge)})
	if err != nil {
		return
	}
	fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
}

func TestSendVerificationHTTPRequestViaConnSuccess(t *testing.T) {
	internal.Options = &model.MainOptions{}
	internal.Options.Secret = testSecret

	client, server := net.Pipe()
	go serveOneEchoResponse(server)

	result := &model.Result{}
	url := "http://127.0.0.1/proxy-verifier.php?challenge=deadbeef"
	if err := util.SendVerificationHTTPRequestViaConn(client, url, 5*time.Second, result); err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if result.RawProxyOriginRequest == "" {
		t.Errorf("expected the raw request to be recorded")
	}
	if result.RawProxyOriginResponse == "" {
		t.Errorf("expected the raw response to be recorded")
	}
}

func TestSendVerificationHTTPRequestViaConnTampered(t *testing.T) {
	internal.Options = &model.MainOptions{}
	internal.Options.Secret = testSecret

	client, server := net.Pipe()
	go func() {
		defer server.Close()
		if _, err := http.ReadRequest(bufio.NewReader(server)); err != nil {
			return
		}
		// Reply with a wrong answer to simulate a caching/intercepting hop.
		body := `{"response":"wrong-answer"}`
		fmt.Fprintf(server, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
	}()

	result := &model.Result{}
	url := "http://127.0.0.1/proxy-verifier.php?challenge=deadbeef"
	err := util.SendVerificationHTTPRequestViaConn(client, url, 5*time.Second, result)
	if err == nil {
		t.Fatalf("expected an integrity verification failure, got nil")
	}
}
