package caddy

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	caddycore "github.com/caddyserver/caddy/v2"
	caddyhttp "github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"golang.org/x/net/idna"
)

const routeMatcherModule = "liapoldus_route"

var errInvalidRequestPath = errors.New("")

type requestPathContextKey struct{}

type settingsRouteMatch struct {
	Hosts   []string     `json:"hosts"`
	Methods []string     `json:"methods"`
	Path    *pathMatcher `json:"path"`
}

type pathMatcher struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type caddyRouteMatcher struct {
	Hosts   []string     `json:"hosts,omitempty"`
	Methods []string     `json:"methods,omitempty"`
	Path    *pathMatcher `json:"path,omitempty"`

	compiledHosts   []compiledHost
	compiledMethods map[string]struct{}
	compiledPath    *compiledPath
}

type compiledHost struct {
	name     string
	wildcard bool
}

type compiledPath struct {
	mode    string
	value   string
	pattern *regexp.Regexp
}

func init() {
	caddycore.RegisterModule(caddyRouteMatcher{})
}

func (caddyRouteMatcher) CaddyModule() caddycore.ModuleInfo {
	return caddycore.ModuleInfo{
		ID:  caddycore.ModuleID("http.matchers." + routeMatcherModule),
		New: func() caddycore.Module { return new(caddyRouteMatcher) },
	}
}

func (matcher *caddyRouteMatcher) Provision(caddycore.Context) error {
	compiled, _, err := canonicalMatcher(&settingsRouteMatch{Hosts: matcher.Hosts, Methods: matcher.Methods, Path: matcher.Path})
	if err != nil {
		return errUnsupportedSettings
	}
	matcher.compiledHosts = compiled.compiledHosts
	matcher.compiledMethods = compiled.compiledMethods
	matcher.compiledPath = compiled.compiledPath
	return nil
}

func (matcher *caddyRouteMatcher) Match(request *http.Request) bool {
	matches, _ := matcher.MatchWithError(request)
	return matches
}

func (matcher *caddyRouteMatcher) MatchWithError(request *http.Request) (bool, error) {
	if request == nil || request.URL == nil {
		return false, caddyhttp.Error(http.StatusBadRequest, errInvalidRequestPath)
	}
	canonical, err := canonicalRequestPath(request)
	if err != nil {
		return false, caddyhttp.Error(http.StatusBadRequest, errInvalidRequestPath)
	}
	if len(matcher.compiledMethods) != 0 {
		if _, exists := matcher.compiledMethods[request.Method]; !exists {
			return false, nil
		}
	}
	if len(matcher.compiledHosts) != 0 {
		host := canonicalRequestHost(request.Host)
		matched := false
		for _, candidate := range matcher.compiledHosts {
			if candidate.matches(host) {
				matched = true
				break
			}
		}
		if !matched {
			return false, nil
		}
	}
	if matcher.compiledPath != nil && !matcher.compiledPath.matches(canonical) {
		return false, nil
	}
	return true, nil
}

func canonicalMatcher(source *settingsRouteMatch) (*caddyRouteMatcher, string, error) {
	result := &caddyRouteMatcher{}
	if source != nil {
		result.Hosts = make([]string, 0, len(source.Hosts))
		seenHosts := make(map[string]struct{}, len(source.Hosts))
		for _, host := range source.Hosts {
			canonical, err := canonicalConfiguredHost(host)
			if err != nil {
				return nil, "", errUnsupportedSettings
			}
			if _, exists := seenHosts[canonical]; exists {
				return nil, "", errUnsupportedSettings
			}
			seenHosts[canonical] = struct{}{}
			result.Hosts = append(result.Hosts, canonical)
		}
		result.Methods = append([]string(nil), source.Methods...)
		seenMethods := make(map[string]struct{}, len(result.Methods))
		for _, method := range result.Methods {
			if method == "" || strings.TrimSpace(method) != method {
				return nil, "", errUnsupportedSettings
			}
			if _, exists := seenMethods[method]; exists {
				return nil, "", errUnsupportedSettings
			}
			seenMethods[method] = struct{}{}
		}
		if source.Path != nil {
			compiled, err := compilePathMatcher(*source.Path)
			if err != nil {
				return nil, "", err
			}
			pathConfig := *source.Path
			result.Path = &pathConfig
			result.compiledPath = compiled
		}
	}
	result.compiledHosts = make([]compiledHost, 0, len(result.Hosts))
	for _, host := range result.Hosts {
		compiled, err := parseCanonicalHost(host)
		if err != nil {
			return nil, "", errUnsupportedSettings
		}
		result.compiledHosts = append(result.compiledHosts, compiled)
	}
	if len(result.Methods) != 0 {
		result.compiledMethods = make(map[string]struct{}, len(result.Methods))
		for _, method := range result.Methods {
			result.compiledMethods[method] = struct{}{}
		}
	}
	sort.Strings(result.Hosts)
	sort.Strings(result.Methods)
	identity := struct {
		Hosts   []string     `json:"hosts,omitempty"`
		Methods []string     `json:"methods,omitempty"`
		Path    *pathMatcher `json:"path,omitempty"`
	}{Hosts: result.Hosts, Methods: result.Methods, Path: result.Path}
	encoded, err := json.Marshal(identity)
	if err != nil {
		return nil, "", errUnsupportedSettings
	}
	return result, string(encoded), nil
}

func compilePathMatcher(source pathMatcher) (*compiledPath, error) {
	if source.Value == "" || !utf8.ValidString(source.Value) || containsPathControl(source.Value) {
		return nil, errUnsupportedSettings
	}
	compiled := &compiledPath{mode: source.Type, value: source.Value}
	switch source.Type {
	case "exact", "prefix", "glob":
		if err := validateCanonicalConfiguredPath(source.Value); err != nil {
			return nil, errUnsupportedSettings
		}
		if source.Type == "glob" {
			pattern, err := compileGlob(source.Value)
			if err != nil {
				return nil, errUnsupportedSettings
			}
			compiled.pattern = pattern
		}
	case "regex":
		pattern, err := regexp.Compile(source.Value)
		if err != nil {
			return nil, errUnsupportedSettings
		}
		compiled.pattern = pattern
	default:
		return nil, errUnsupportedSettings
	}
	return compiled, nil
}

func validateCanonicalConfiguredPath(value string) error {
	if !strings.HasPrefix(value, "/") || strings.Contains(value, "//") {
		return errUnsupportedSettings
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "." || segment == ".." {
			return errUnsupportedSettings
		}
	}
	canonical := path.Clean(value)
	if strings.HasSuffix(value, "/") && canonical != "/" {
		canonical += "/"
	}
	if canonical != value {
		return errUnsupportedSettings
	}
	return nil
}

func compileGlob(source string) (*regexp.Regexp, error) {
	var expression strings.Builder
	expression.WriteByte('^')
	for index := 0; index < len(source); {
		runeValue, width := utf8.DecodeRuneInString(source[index:])
		switch runeValue {
		case '*':
			if index+width < len(source) && source[index+width] == '*' {
				expression.WriteString(".*")
				index += width + 1
			} else {
				expression.WriteString("[^/]*")
				index += width
			}
		case '?':
			expression.WriteString("[^/]")
			index += width
		default:
			expression.WriteString(regexp.QuoteMeta(string(runeValue)))
			index += width
		}
	}
	expression.WriteByte('$')
	return regexp.Compile(expression.String())
}

func (matcher *compiledPath) matches(value string) bool {
	if matcher == nil {
		return true
	}
	switch matcher.mode {
	case "exact":
		return value == matcher.value
	case "prefix":
		return strings.HasPrefix(value, matcher.value)
	case "glob":
		return matcher.pattern != nil && matcher.pattern.MatchString(value)
	case "regex":
		indices := matcher.pattern.FindStringIndex(value)
		return indices != nil && indices[0] == 0 && indices[1] == len(value)
	default:
		return false
	}
}

func canonicalRequestPath(request *http.Request) (string, error) {
	if canonical, ok := request.Context().Value(requestPathContextKey{}).(string); ok {
		return canonical, nil
	}
	target := request.RequestURI
	if target == "" {
		target = request.URL.RequestURI()
	}
	rawPath, query, _ := strings.Cut(target, "?")
	if rawPath == "" || !strings.HasPrefix(rawPath, "/") {
		return "", errInvalidRequestPath
	}
	decoded, err := url.PathUnescape(rawPath)
	if err != nil || !utf8.ValidString(decoded) || containsPathControl(decoded) || !strings.HasPrefix(decoded, "/") {
		return "", errInvalidRequestPath
	}
	trailingSlash := strings.HasSuffix(decoded, "/") || strings.HasSuffix(decoded, "/.") || strings.HasSuffix(decoded, "/..")
	canonical := path.Clean(decoded)
	if trailingSlash && canonical != "/" && !strings.HasSuffix(canonical, "/") {
		canonical += "/"
	}
	request.URL.Path = canonical
	request.URL.RawPath = ""
	request.URL.RawQuery = query
	requestContext := context.WithValue(request.Context(), requestPathContextKey{}, canonical)
	*request = *request.WithContext(requestContext)
	return canonical, nil
}

func containsPathControl(value string) bool {
	for _, character := range value {
		if character <= 0x1f || character == 0x7f {
			return true
		}
	}
	return false
}

func canonicalConfiguredHost(value string) (string, error) {
	wildcard := strings.HasPrefix(value, "*.")
	host := value
	if wildcard {
		host = strings.TrimPrefix(value, "*.")
	}
	if host == "" || strings.ContainsAny(host, "*:/[]?#") {
		return "", errUnsupportedSettings
	}
	ascii, err := idna.Lookup.ToASCII(host)
	if err != nil || ascii == "" || strings.HasSuffix(ascii, ".") {
		return "", errUnsupportedSettings
	}
	ascii = strings.ToLower(ascii)
	if wildcard {
		ascii = "*." + ascii
	}
	return ascii, nil
}

func parseCanonicalHost(value string) (compiledHost, error) {
	wildcard := strings.HasPrefix(value, "*.")
	host := strings.TrimPrefix(value, "*.")
	if host == "" {
		return compiledHost{}, errUnsupportedSettings
	}
	return compiledHost{name: host, wildcard: wildcard}, nil
}

func canonicalRequestHost(value string) string {
	host := value
	if strings.HasPrefix(value, "[") {
		parsed, _, err := net.SplitHostPort(value)
		if err != nil {
			return ""
		}
		host = parsed
	} else if strings.Count(value, ":") == 1 {
		parsed, _, err := net.SplitHostPort(value)
		if err != nil {
			return ""
		}
		host = parsed
	} else if strings.Contains(value, ":") {
		return ""
	}
	canonical, err := canonicalConfiguredHost(host)
	if err != nil || strings.HasPrefix(canonical, "*.") {
		return ""
	}
	return canonical
}

func (host compiledHost) matches(value string) bool {
	if !host.wildcard {
		return value == host.name
	}
	suffix := "." + host.name
	if !strings.HasSuffix(value, suffix) {
		return false
	}
	label := strings.TrimSuffix(value, suffix)
	return label != "" && !strings.Contains(label, ".")
}
