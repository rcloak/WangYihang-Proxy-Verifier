package protocol

import (
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/WangYihang/Proxy-Verifier/internal/model"
	"github.com/WangYihang/Proxy-Verifier/internal/util"
)

func EstablishHttpsTransparentProxy(proxyHost string, proxyPort uint16, timeoutDuration time.Duration) (net.Conn, error) {
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
	return conn, nil
}

func HttpViaHttpsTransparentProxy(task *model.Task) (result model.Result) {
	result = model.Result{Task: task}
	timeoutDuration := time.Second * time.Duration(task.Timeout)

	// Establish tunnel
	conn, err := EstablishHttpsTransparentProxy(task.ProxyHost, task.ProxyPort, timeoutDuration)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	// Send the verification request directly over the established connection.
	newUrl := util.BuildUrl(task.TargetUri, task.TaskId, task.ProxyProtocol, task.ProxyHost, task.ProxyPort)
	if err := util.SendVerificationHTTPRequestViaConn(conn, newUrl, timeoutDuration, task.Secret, &result); err != nil {
		result.Error = err.Error()
	}
	return result
}
