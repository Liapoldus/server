package main

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	caddycore "github.com/caddyserver/caddy/v2"
	_ "github.com/caddyserver/caddy/v2/modules/standard"
	"liapoldus.local/server-plugin/contracts"
	_ "liapoldus.local/server-plugin/internal/infrastructure/caddy"
	"liapoldus.local/server-plugin/tests/fixtures/shared"
)

func main() {
	_, cleanup, err := shared.IsolateCaddyDataHome()
	check(err)
	defer cleanup()
	contract, err := contracts.LoadHTTPDispatch()
	check(err)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	check(err)
	address := listener.Addr().String()
	check(listener.Close())
	configuration, err := json.Marshal(map[string]any{
		"admin": map[string]any{"disabled": true, "config": map[string]any{"persist": false}},
		"apps": map[string]any{"http": map[string]any{"servers": map[string]any{
			"header-limit": map[string]any{
				"listen": []string{address}, "max_header_bytes": contract.MaxRequestHeaderBytes,
				"automatic_https": map[string]any{"disable": true, "disable_redirects": true},
				"routes": []any{
					map[string]any{
						"handle":   []any{map[string]any{"handler": "liapoldus_request_header_limit", "maxBytes": contract.MaxRequestHeaderBytes, "status": contract.RequestHeaderTooLargeStatus}},
						"terminal": false,
					},
					map[string]any{"handle": []any{map[string]any{"handler": "static_response", "status_code": 200, "body": "ok"}}, "terminal": true},
				},
			},
		}}},
	})
	check(err)
	check(caddycore.Load(configuration, true))
	defer func() { _ = caddycore.Stop() }()
	time.Sleep(100 * time.Millisecond)
	atLimit := requestStatus(address, contract.MaxRequestHeaderBytes)
	overLimit := requestStatus(address, contract.MaxRequestHeaderBytes+1)
	check(json.NewEncoder(os.Stdout).Encode(map[string]any{
		"maxRequestHeaderBytes": contract.MaxRequestHeaderBytes,
		"atLimitStatus":         atLimit,
		"overLimitStatus":       overLimit,
	}))
}

func requestStatus(address string, totalBytes int) int {
	connection, err := net.DialTimeout("tcp", address, time.Second)
	check(err)
	defer connection.Close()
	requestLine := "GET / HTTP/1.1\r\n"
	prefix := "Host: localhost\r\nX-Pad: "
	suffix := "\r\n"
	paddingBytes := totalBytes - len(prefix) - len(suffix)
	if paddingBytes < 0 {
		panic("header boundary is too small")
	}
	_, err = connection.Write([]byte(requestLine + prefix + strings.Repeat("a", paddingBytes) + suffix + "\r\n"))
	check(err)
	_ = connection.SetReadDeadline(time.Now().Add(3 * time.Second))
	statusLine, err := bufio.NewReader(connection).ReadString('\n')
	check(err)
	parts := strings.Fields(statusLine)
	if len(parts) < 2 {
		panic("invalid HTTP status line")
	}
	status, err := strconv.Atoi(parts[1])
	check(err)
	return status
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
