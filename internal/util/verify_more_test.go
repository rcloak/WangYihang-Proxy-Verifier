package util_test

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/WangYihang/Proxy-Verifier/internal/model"
	"github.com/WangYihang/Proxy-Verifier/internal/util"
)

func md5hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestHandleChallengeAndVerify(t *testing.T) {
	answer := util.HandleChallenge("abc", testSecret)
	if want := md5hex("abc" + testSecret); answer != want {
		t.Errorf("HandleChallenge = %q, want %q", answer, want)
	}
	if !util.VerifyChallenge("abc", answer, testSecret) {
		t.Error("VerifyChallenge should accept the correct answer")
	}
	if util.VerifyChallenge("abc", "wrong-answer", testSecret) {
		t.Error("VerifyChallenge should reject a wrong answer")
	}
	if util.VerifyChallenge("abc", answer, "different-secret") {
		t.Error("VerifyChallenge should reject when the secret differs")
	}
}

func TestBuildUrlAndExtractChallenge(t *testing.T) {
	raw := util.BuildUrl("http://echo.example/", "chal-123", "socks5", "9.9.9.9", 1080)

	if got := util.ExtractChallengeFromUrl(raw); got != "chal-123" {
		t.Errorf("extracted challenge = %q, want chal-123", got)
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("BuildUrl produced an unparseable URL %q: %v", raw, err)
	}
	if !strings.HasSuffix(parsed.Path, ".php") {
		t.Errorf("path = %q, want a cache-busting .php path", parsed.Path)
	}
	q := parsed.Query()
	if q.Get("proxy_protocol") != "socks5" || q.Get("proxy_host") != "9.9.9.9" || q.Get("proxy_port") != "1080" {
		t.Errorf("proxy params not embedded correctly: %v", q)
	}
}

func TestExtractChallengeFromInvalidUrl(t *testing.T) {
	if got := util.ExtractChallengeFromUrl("http://%zz"); got != "" {
		t.Errorf("expected empty challenge for an invalid URL, got %q", got)
	}
}

func TestVerifyIntegrity(t *testing.T) {
	challenge := "xyz"
	good, _ := json.Marshal(map[string]string{"response": util.HandleChallenge(challenge, testSecret)})
	if !util.VerifyIntegrity(good, challenge, testSecret) {
		t.Error("VerifyIntegrity should accept a correctly signed body")
	}

	bad, _ := json.Marshal(map[string]string{"response": "not-the-answer"})
	if util.VerifyIntegrity(bad, challenge, testSecret) {
		t.Error("VerifyIntegrity should reject a wrong answer")
	}

	if util.VerifyIntegrity([]byte("this is not json"), challenge, testSecret) {
		t.Error("VerifyIntegrity should reject a non-JSON body")
	}
}

// newEchoServer returns an httptest server that mimics the controlled echo
// server: it answers any request with md5(challenge+secret).
func newEchoServer(tamper bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		challenge := r.URL.Query().Get("challenge")
		response := util.HandleChallenge(challenge, testSecret)
		if tamper {
			response = "tampered"
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"response": response})
	}))
}

func TestSendVerificationHttpRequestSuccess(t *testing.T) {
	srv := newEchoServer(false)
	defer srv.Close()

	result := &model.Result{}
	target := util.BuildUrl(srv.URL, "task-1", "http", "127.0.0.1", 8080)
	if err := util.SendVerificationHttpRequest(srv.Client(), target, testSecret, result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RawProxyOriginRequest == "" || result.RawProxyOriginResponse == "" {
		t.Error("expected the raw request/response to be recorded")
	}
}

func TestSendVerificationHttpRequestIntegrityFailure(t *testing.T) {
	srv := newEchoServer(true)
	defer srv.Close()

	result := &model.Result{}
	target := util.BuildUrl(srv.URL, "task-2", "http", "127.0.0.1", 8080)
	err := util.SendVerificationHttpRequest(srv.Client(), target, testSecret, result)
	if err == nil {
		t.Fatal("expected an integrity verification error, got nil")
	}
	if !strings.Contains(err.Error(), "integrity") {
		t.Errorf("error = %q, want it to mention integrity", err.Error())
	}
}

func TestSendVerificationHttpRequestConnectionError(t *testing.T) {
	srv := newEchoServer(false)
	target := util.BuildUrl(srv.URL, "task-3", "http", "127.0.0.1", 8080)
	srv.Close() // close before sending so the dial fails

	result := &model.Result{}
	if err := util.SendVerificationHttpRequest(srv.Client(), target, testSecret, result); err == nil {
		t.Error("expected a connection error after the server was closed")
	}
}
