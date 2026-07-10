package protocol

import (
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/WangYihang/Proxy-Verifier/internal/model"
	"github.com/WangYihang/Proxy-Verifier/internal/util"
)

func EstablishHttpsTunnel(proxyHost string, proxyPort uint16, targetUri string, timeoutDuration time.Duration, result *model.Result) (net.Conn, error) {
	// Establish tunnel.
	// InsecureSkipVerify is intentional: we are probing arbitrary third-party
	// proxies whose TLS certificates are routinely self-signed or mismatched,
	// and connectivity (not certificate trust) is what we are verifying here.
	conn, err := tls.Dial(
		"tcp",
		fmt.Sprintf("%s:%d", proxyHost, proxyPort),
		&tls.Config{
			InsecureSkipVerify: true,
		},
	)

	if err != nil {
		return nil, err
	}

	host, port, err := util.ExtractHostPortFromUri(targetUri)
	if err != nil {
		result.Error = err.Error()
		conn.Close()
		return nil, err
	}

	rawClientProxyRequest := fmt.Sprintf("CONNECT %s:%d HTTP/1.1\r\nHost: %s:%d\r\n\r\n", host, port, proxyHost, proxyPort)
	remainingBytes := []byte(rawClientProxyRequest)
	for len(remainingBytes) > 0 {
		_ = conn.SetDeadline(time.Now().Add(timeoutDuration))
		n, err := conn.Write(remainingBytes)
		_ = conn.SetDeadline(time.Time{})
		if err != nil {
			conn.Close()
			return nil, err
		}
		remainingBytes = remainingBytes[n:]
	}
	result.RawClientProxyRequest = rawClientProxyRequest

	// Read the first line of response
	buffer := make([]byte, 1024)
	_ = conn.SetDeadline(time.Now().Add(timeoutDuration))
	n, err := conn.Read(buffer)
	_ = conn.SetDeadline(time.Time{})
	if err != nil {
		conn.Close()
		return nil, err
	}

	// Check the first line of response
	firstLine := string(buffer[:n])
	result.RawClientProxyResponse = firstLine

	// HTTP/1.0 200 Connection established
	// HTTP/1.1 200 Connection established
	// HTTP/1.0 200 Connected
	// HTTP/1.1 200 Connected
	// HTTP/1.1 200 OK
	if strings.Contains(result.RawClientProxyResponse, " 200 ") {
		return conn, nil
	} else {
		conn.Close()
		return nil, fmt.Errorf("invalid Tunnel Server Response: %s", firstLine)
	}
}

func HttpViaHttpsTunnel(task *model.Task) (result model.Result) {
	result = model.Result{Task: task}
	timeoutDuration := time.Second * time.Duration(task.Timeout)

	// Connect to the proxy
	conn, err := EstablishHttpsTunnel(task.ProxyHost, task.ProxyPort, task.TargetUri, timeoutDuration, &result)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	// Send the verification request directly over the established tunnel.
	newUrl := util.BuildUrl(task.TargetUri, task.TaskId, task.ProxyProtocol, task.ProxyHost, task.ProxyPort)
	if err := util.SendVerificationHTTPRequestViaConn(conn, newUrl, timeoutDuration, &result); err != nil {
		result.Error = err.Error()
	}
	return result
}
