package contracts

import (
	"encoding/json"
	"errors"
	"io/fs"
)

type HTTPDispatch struct {
	Module                string   `json:"module"`
	App                   string   `json:"app"`
	DefaultTimeoutMillis  int      `json:"defaultTimeoutMillis"`
	MaxRequestBytes       int64    `json:"maxRequestBytes"`
	UnavailableStatus     int      `json:"unavailableStatus"`
	InvalidResponseStatus int      `json:"invalidResponseStatus"`
	RequestTooLargeStatus int      `json:"requestTooLargeStatus"`
	InvalidRequestStatus  int      `json:"invalidRequestStatus"`
	RequestIDHeader       string   `json:"requestIDHeader"`
	CookieHeader          string   `json:"cookieHeader"`
	SetCookieHeader       string   `json:"setCookieHeader"`
	BlockedHeaders        []string `json:"blockedHeaders"`
}

var ErrInvalidDispatchAssets = errors.New("")

func LoadHTTPDispatch() (HTTPDispatch, error) {
	contents, err := fs.ReadFile(files, "v1/http-dispatch.json")
	if err != nil {
		return HTTPDispatch{}, ErrInvalidDispatchAssets
	}
	var contract HTTPDispatch
	if err := json.Unmarshal(contents, &contract); err != nil || contract.Module == "" || contract.App == "" || contract.DefaultTimeoutMillis < 1 || contract.MaxRequestBytes < 1 || contract.UnavailableStatus < 100 || contract.InvalidResponseStatus < 100 || contract.RequestTooLargeStatus < 100 || contract.InvalidRequestStatus < 100 || len(contract.BlockedHeaders) == 0 {
		return HTTPDispatch{}, ErrInvalidDispatchAssets
	}
	return contract, nil
}
