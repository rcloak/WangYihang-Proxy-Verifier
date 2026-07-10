package protocol

import (
	"net"
	"time"
)

// readConnectResponse reads a proxy's CONNECT reply up to and including the
// blank line that terminates the response headers, WITHOUT consuming any bytes
// the target server may send afterwards.
//
// This matters for server-speaks-first protocols such as SMTP: the target
// sends its greeting immediately, and a naive fixed-size read could swallow it
// together with the CONNECT response, leaving nothing for the verifier to read.
func readConnectResponse(conn net.Conn, timeout time.Duration) (string, error) {
	_ = conn.SetDeadline(time.Now().Add(timeout))
	defer conn.SetDeadline(time.Time{})

	var resp []byte
	buf := make([]byte, 1)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return string(resp), err
		}
		if n == 0 {
			continue
		}
		resp = append(resp, buf[0])
		if len(resp) >= 4 && string(resp[len(resp)-4:]) == "\r\n\r\n" {
			break
		}
		if len(resp) > 8192 {
			break
		}
	}
	return string(resp), nil
}
