package protocol

import (
	"net"
	"strconv"
	"time"

	"github.com/WangYihang/Proxy-Verifier/internal/model"
	"github.com/WangYihang/Proxy-Verifier/internal/util"
	"h12.io/socks"
)

func SmtpViaSocksTunnel(task *model.Task) (result model.Result) {
	result = model.Result{Task: task}
	timeoutDuration := time.Second * time.Duration(task.Timeout)

	// Resolve the SMTP server address from the target URI.
	host, port, err := util.ExtractHostPortFromUri(task.TargetUri)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	// Connect to the SMTP server through the SOCKS proxy.
	dial := socks.Dial(task.ProxyUri)
	conn, err := dial("tcp", net.JoinHostPort(host, strconv.Itoa(int(port))))
	if err != nil {
		result.Error = err.Error()
		return result
	}

	if err := util.SendVerificationSmtpRequest(conn, task.TaskId, task.Secret, timeoutDuration, &result); err != nil {
		result.Error = err.Error()
	}
	return result
}
