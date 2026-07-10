package protocol

import (
	"time"

	"github.com/WangYihang/Proxy-Verifier/internal/model"
	"github.com/WangYihang/Proxy-Verifier/internal/util"
)

func SmtpViaHttpTunnel(task *model.Task) (result model.Result) {
	result = model.Result{Task: task}
	timeoutDuration := time.Second * time.Duration(task.Timeout)

	// Connect to the SMTP server through an HTTP CONNECT tunnel.
	conn, err := EstablishHttpTunnel(task.ProxyHost, task.ProxyPort, task.TargetUri, timeoutDuration, &result)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	if err := util.SendVerificationSmtpRequest(conn, task.TaskId, task.Secret, timeoutDuration, &result); err != nil {
		result.Error = err.Error()
	}
	return result
}

func SmtpViaHttpsTunnel(task *model.Task) (result model.Result) {
	result = model.Result{Task: task}
	timeoutDuration := time.Second * time.Duration(task.Timeout)

	// Connect to the SMTP server through an HTTPS CONNECT tunnel.
	conn, err := EstablishHttpsTunnel(task.ProxyHost, task.ProxyPort, task.TargetUri, timeoutDuration, &result)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	if err := util.SendVerificationSmtpRequest(conn, task.TaskId, task.Secret, timeoutDuration, &result); err != nil {
		result.Error = err.Error()
	}
	return result
}
