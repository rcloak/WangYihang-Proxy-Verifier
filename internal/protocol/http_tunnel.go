package protocol

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/WangYihang/Proxy-Verifier/internal/model"
	"github.com/WangYihang/Proxy-Verifier/internal/util"
)

func EstablishHttpTunnel(proxyHost string, proxyPort uint16, targetUri string, timeoutDuration time.Duration, result *model.Result) (net.Conn, error) {
	// Establish tunnel
	conn, err := net.DialTimeout(
		"tcp",
		fmt.Sprintf("%s:%d", proxyHost, proxyPort),
		timeoutDuration,
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

	// Read the CONNECT response without consuming any bytes the target server
	// may send afterwards (e.g. an SMTP greeting).
	firstLine, err := readConnectResponse(conn, timeoutDuration)
	result.RawClientProxyResponse = firstLine
	if err != nil {
		conn.Close()
		return nil, err
	}

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

func HttpViaHttpTunnel(task *model.Task) (result model.Result) {
	result = model.Result{Task: task}
	timeoutDuration := time.Second * time.Duration(task.Timeout)

	// Connect to the proxy
	conn, err := EstablishHttpTunnel(task.ProxyHost, task.ProxyPort, task.TargetUri, timeoutDuration, &result)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	// Send the verification request directly over the established tunnel.
	newUrl := util.BuildUrl(task.TargetUri, task.TaskId, task.ProxyProtocol, task.ProxyHost, task.ProxyPort)
	if err := util.SendVerificationHTTPRequestViaConn(conn, newUrl, timeoutDuration, task.Secret, &result); err != nil {
		result.Error = err.Error()
	}
	return result
}
