package util

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/WangYihang/Proxy-Verifier/internal/config"
	"github.com/WangYihang/Proxy-Verifier/internal/model"
	"github.com/google/uuid"
)

func BuildUrl(urlString string, challenge, proxyProtocol, proxyHost string, proxyPort uint16) string {
	urlObject, err := url.Parse(urlString)
	if err != nil {
		fmt.Println(err)
	}
	urlObject.Path = fmt.Sprintf("/%s.php", uuid.New().String())
	q := urlObject.Query()
	q.Set(config.ChallengeParamName, challenge)
	q.Set("proxy_protocol", proxyProtocol)
	q.Set("proxy_host", proxyHost)
	q.Set("proxy_port", fmt.Sprintf("%d", proxyPort))
	urlObject.RawQuery = q.Encode()
	return urlObject.String()
}

func ExtractChallengeFromUrl(urlString string) string {
	urlObject, err := url.Parse(urlString)
	if err != nil {
		fmt.Println(err)
		return ""
	}
	return urlObject.Query().Get(config.ChallengeParamName)
}

func HandleChallenge(challenge string, secret string) string {
	hash := md5.Sum([]byte(challenge + secret))
	answer := hex.EncodeToString(hash[:])
	return answer
}

func VerifyChallenge(challenge string, answer string, secret string) bool {
	expectedAnswer := HandleChallenge(challenge, secret)
	return expectedAnswer == answer
}

func VerifyIntegrity(responseBody []byte, challenge string, secret string) bool {
	type EchoServerResponse struct {
		BackendAddress string              `json:"backend_address"`
		RequestUri     string              `json:"request_uri"`
		RequestHeaders map[string][]string `json:"request_headers"`
		RequestBody    string              `json:"request_body"`
		Challenge      string              `json:"challenge"`
		Response       string              `json:"response"`
	}
	echoServerResponse := EchoServerResponse{}
	err := json.Unmarshal(responseBody, &echoServerResponse)
	if err != nil {
		return false
	}
	return VerifyChallenge(challenge, echoServerResponse.Response, secret)
}

func SendVerificationHttpRequest(client *http.Client, url string, secret string, result *model.Result) error {
	// Create request
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	// Record the raw request in result
	rawRequest, err := httputil.DumpRequest(request, true)
	if err != nil {
		return err
	}
	result.RawProxyOriginRequest = string(rawRequest)

	// Send the request
	response, err := client.Do(request)
	if err != nil {
		return err
	}

	// Record the raw response in result
	rawResponse, err := httputil.DumpResponse(response, true)
	if err != nil {
		return err
	}

	// Record the raw response in result
	result.RawProxyOriginResponse = string(rawResponse)
	rawBody := rawResponse[bytes.Index(rawResponse, []byte("\r\n\r\n"))+4:]

	// Verify integrity
	challenge := ExtractChallengeFromUrl(url)
	if !VerifyIntegrity(rawBody, challenge, secret) {
		return fmt.Errorf("integrity verification failed")
	}

	// No error
	return nil
}

// SendVerificationHTTPRequestViaConn sends the verification request directly
// over an already-established connection (an HTTP/HTTPS tunnel or a transparent
// proxy connection) and verifies the integrity of the response.
//
// Unlike handing the connection to an http.Transport through a Dial closure that
// always returns the same connection, this writes the request and reads the
// response on the connection exactly once. That avoids the transport re-dialing
// and reusing a single-use connection, which could otherwise turn a working
// proxy into a false negative.
func SendVerificationHTTPRequestViaConn(conn net.Conn, url string, timeout time.Duration, secret string, result *model.Result) error {
	defer conn.Close()

	// Create request
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	// Record the raw request in result
	rawRequest, err := httputil.DumpRequest(request, true)
	if err != nil {
		return err
	}
	result.RawProxyOriginRequest = string(rawRequest)

	// Bound the whole exchange by the timeout
	_ = conn.SetDeadline(time.Now().Add(timeout))
	defer conn.SetDeadline(time.Time{})

	// Write the request onto the established connection
	if err := request.Write(conn); err != nil {
		return err
	}

	// Read the response back from the same connection
	response, err := http.ReadResponse(bufio.NewReader(conn), request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	// Record the raw response in result
	rawResponse, err := httputil.DumpResponse(response, true)
	if err != nil {
		return err
	}
	result.RawProxyOriginResponse = string(rawResponse)
	rawBody := rawResponse[bytes.Index(rawResponse, []byte("\r\n\r\n"))+4:]

	// Verify integrity
	challenge := ExtractChallengeFromUrl(url)
	if !VerifyIntegrity(rawBody, challenge, secret) {
		return fmt.Errorf("integrity verification failed")
	}

	return nil
}
