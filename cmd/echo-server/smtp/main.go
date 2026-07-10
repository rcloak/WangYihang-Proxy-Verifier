package main

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/WangYihang/Proxy-Verifier/internal/model"
	flags "github.com/jessevdk/go-flags"
)

var options *model.SMTPEchoServerOptions
var logFd *os.File

// handleChallenge answers a challenge the same way the HTTP echo server does:
// md5(challenge + secret). Both the verifier and this server must share the
// same secret for the challenge/response verification to succeed.
func handleChallenge(challenge string) string {
	hash := md5.Sum([]byte(challenge + options.Secret))
	return hex.EncodeToString(hash[:])
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	// Greeting message
	writer.WriteString("220 Proxy-Verifier SMTP echo server\r\n")
	writer.Flush()

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		parts := strings.Fields(strings.TrimRight(line, "\r\n"))
		command := ""
		if len(parts) > 0 {
			command = strings.ToUpper(parts[0])
		}

		switch command {
		case "HELO", "EHLO":
			// The client passes the challenge as the HELO/EHLO argument; reply
			// with md5(challenge+secret) so the verifier can confirm the SMTP
			// session actually reached this controlled server.
			challenge := ""
			if len(parts) > 1 {
				challenge = parts[1]
			}
			answer := handleChallenge(challenge)
			writer.WriteString(fmt.Sprintf("250 %s\r\n", answer))
			writer.Flush()
			if logFd != nil {
				fmt.Fprintf(logFd, "remote=%s challenge=%s answer=%s\n", conn.RemoteAddr(), challenge, answer)
			}
		case "QUIT":
			writer.WriteString("221 Bye\r\n")
			writer.Flush()
			return
		default:
			writer.WriteString("250 OK\r\n")
			writer.Flush()
		}
	}
}

func main() {
	options = &model.SMTPEchoServerOptions{}
	if _, err := flags.Parse(options); err != nil {
		os.Exit(1)
	}

	var err error
	logFd, err = os.OpenFile(options.LogFilename, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		os.Exit(1)
	}
	defer logFd.Close()

	addr := fmt.Sprintf("%s:%d", options.BindHost, options.BindPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Println("Error listening:", err)
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Printf("SMTP echo server listening on %s\n", addr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}
		go handleConnection(conn)
	}
}
