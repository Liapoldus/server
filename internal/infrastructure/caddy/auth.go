package caddy

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"

	caddycore "github.com/caddyserver/caddy/v2"
	caddyhttp "github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

const authHandlerModule = "liapoldus_service_auth"

var processAuthMACKey struct {
	once sync.Once
	key  []byte
	err  error
}

func authSecretMAC(secret []byte) (string, error) {
	processAuthMACKey.once.Do(func() {
		processAuthMACKey.key = make([]byte, 32)
		_, processAuthMACKey.err = rand.Read(processAuthMACKey.key)
		if processAuthMACKey.err != nil {
			clear(processAuthMACKey.key)
			processAuthMACKey.key = nil
		}
	})
	if processAuthMACKey.err != nil || len(processAuthMACKey.key) != 32 || len(secret) == 0 {
		return "", errUnsupportedSettings
	}
	mac := hmac.New(sha256.New, processAuthMACKey.key)
	_, _ = mac.Write(secret)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func matchesAuthSecret(expected string, candidate []byte) bool {
	mac, err := authSecretMAC(candidate)
	if err != nil || len(expected) != sha256.Size*2 || len(mac) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(mac)) == 1
}

type serviceAuthHandler struct {
	Scheme     string `json:"scheme"`
	SecretMAC  string `json:"secretMac"`
	HeaderName string `json:"headerName,omitempty"`
	Username   string `json:"username,omitempty"`
	Realm      string `json:"realm,omitempty"`
}

func init() { caddycore.RegisterModule(serviceAuthHandler{}) }

func (serviceAuthHandler) CaddyModule() caddycore.ModuleInfo {
	return caddycore.ModuleInfo{ID: caddycore.ModuleID("http.handlers." + authHandlerModule), New: func() caddycore.Module { return new(serviceAuthHandler) }}
}

func (handler *serviceAuthHandler) Provision(caddycore.Context) error {
	if handler == nil || (handler.Scheme != "bearer" && handler.Scheme != "apiKey" && handler.Scheme != "basic") ||
		len(handler.SecretMAC) != sha256.Size*2 {
		return errUnsupportedSettings
	}
	if handler.Scheme == "apiKey" && (!validFeatureHeaderName(handler.HeaderName) || strings.EqualFold(handler.HeaderName, "authorization")) {
		return errUnsupportedSettings
	}
	if handler.Scheme == "basic" && (handler.Username == "" || len(handler.Username) > 128 || strings.ContainsAny(handler.Username, ":\r\n\x00") || len(handler.Realm) > 128 || strings.ContainsAny(handler.Realm, "\r\n\x00")) {
		return errUnsupportedSettings
	}
	return nil
}

func (handler *serviceAuthHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request, next caddyhttp.Handler) error {
	var authorized bool
	switch handler.Scheme {
	case "bearer":
		parts := strings.Fields(request.Header.Get("Authorization"))
		authorized = len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") && matchesAuthSecret(handler.SecretMAC, []byte(parts[1]))
	case "apiKey":
		values := request.Header.Values(handler.HeaderName)
		authorized = len(values) == 1 && matchesAuthSecret(handler.SecretMAC, []byte(values[0]))
	case "basic":
		parts := strings.Fields(request.Header.Get("Authorization"))
		if len(parts) == 2 && strings.EqualFold(parts[0], "Basic") {
			decoded, err := base64.StdEncoding.DecodeString(parts[1])
			if err == nil {
				username, password, found := strings.Cut(string(decoded), ":")
				usernameMatches := subtle.ConstantTimeCompare([]byte(username), []byte(handler.Username)) == 1
				authorized = found && usernameMatches && matchesAuthSecret(handler.SecretMAC, []byte(password))
				clear(decoded)
			}
		}
	}
	if !authorized {
		if handler.Scheme == "basic" {
			realm := handler.Realm
			if realm == "" {
				realm = "Restricted"
			}
			writer.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`", charset="UTF-8"`)
		} else if handler.Scheme == "bearer" {
			writer.Header().Set("WWW-Authenticate", "Bearer")
		}
		writer.WriteHeader(http.StatusUnauthorized)
		return nil
	}
	return next.ServeHTTP(writer, request)
}

var _ caddycore.Module = (*serviceAuthHandler)(nil)
var _ caddyhttp.MiddlewareHandler = (*serviceAuthHandler)(nil)
