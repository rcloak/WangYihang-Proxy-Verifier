package util

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/WangYihang/Proxy-Verifier/internal/model"
)

// SendVerificationSmtpRequest performs a challenge/response handshake with the
// controlled SMTP echo server over an already-established connection (an
// HTTP/HTTPS tunnel or a SOCKS connection).
//
// It reads the server greeting, sends `HELO <challenge>`, and checks that the
// reply carries md5(challenge+secret). Because the challenge is unique per
// probe and the answer depends on the shared secret, a successful verification
// proves the request actually reached the controlled server through the proxy
// (rather than a transparent interceptor that cannot compute the answer).
func SendVerificationSmtpRequest(conn net.Conn, challenge, secret string, timeout time.Duration, result *model.Result) error {
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))
	defer conn.SetDeadline(time.Time{})

	reader := bufio.NewReader(conn)

	// Read the server greeting.
	greeting, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	result.RawProxyOriginResponse = greeting
	if !strings.HasPrefix(greeting, "220") {
		return fmt.Errorf("unexpected SMTP greeting: %s", strings.TrimSpace(greeting))
	}

	// Send HELO carrying the challenge.
	helo := fmt.Sprintf("HELO %s\r\n", challenge)
	if _, err := conn.Write([]byte(helo)); err != nil {
		return err
	}
	result.RawProxyOriginRequest = helo

	// Read the reply to HELO.
	reply, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	result.RawProxyOriginResponse += reply

	// Politely close the session (best effort; the answer has already arrived).
	_, _ = conn.Write([]byte("QUIT\r\n"))

	if !strings.HasPrefix(reply, "250") {
		return fmt.Errorf("unexpected SMTP HELO reply: %s", strings.TrimSpace(reply))
	}
	answer := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(reply), "250"))
	if !VerifyChallenge(challenge, answer, secret) {
		return fmt.Errorf("integrity verification failed")
	}
	return nil
}
