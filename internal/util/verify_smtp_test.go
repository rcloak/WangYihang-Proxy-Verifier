package util_test

import (
	"bufio"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/WangYihang/Proxy-Verifier/internal/model"
	"github.com/WangYihang/Proxy-Verifier/internal/util"
)

// serveSmtpEcho mimics the controlled SMTP echo server: greet, answer HELO with
// md5(challenge+secret), then close.
func serveSmtpEcho(conn net.Conn, tamper bool) {
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
			answer := answerFor(challenge)
			if tamper {
				answer = "wrong-answer"
			}
			_, _ = io.WriteString(conn, "250 "+answer+"\r\n")
		case "QUIT":
			_, _ = io.WriteString(conn, "221 Bye\r\n")
			return
		default:
			_, _ = io.WriteString(conn, "250 OK\r\n")
		}
	}
}

func TestSendVerificationSmtpRequestSuccess(t *testing.T) {
	client, server := net.Pipe()
	go serveSmtpEcho(server, false)

	result := &model.Result{}
	if err := util.SendVerificationSmtpRequest(client, "challenge-abc", testSecret, 5*time.Second, result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RawProxyOriginRequest == "" || result.RawProxyOriginResponse == "" {
		t.Error("expected the raw SMTP exchange to be recorded")
	}
	if !strings.Contains(result.RawProxyOriginRequest, "HELO challenge-abc") {
		t.Errorf("recorded request = %q, want it to contain the HELO challenge", result.RawProxyOriginRequest)
	}
}

func TestSendVerificationSmtpRequestTampered(t *testing.T) {
	client, server := net.Pipe()
	go serveSmtpEcho(server, true)

	result := &model.Result{}
	err := util.SendVerificationSmtpRequest(client, "challenge-abc", testSecret, 5*time.Second, result)
	if err == nil {
		t.Fatal("expected an integrity verification failure, got nil")
	}
	if !strings.Contains(err.Error(), "integrity") {
		t.Errorf("error = %q, want it to mention integrity", err.Error())
	}
}

func TestSendVerificationSmtpRequestBadGreeting(t *testing.T) {
	client, server := net.Pipe()
	go func() {
		defer server.Close()
		_, _ = io.WriteString(server, "554 no service here\r\n")
	}()

	result := &model.Result{}
	if err := util.SendVerificationSmtpRequest(client, "challenge-abc", testSecret, 5*time.Second, result); err == nil {
		t.Fatal("expected an error for a non-220 greeting, got nil")
	}
}
