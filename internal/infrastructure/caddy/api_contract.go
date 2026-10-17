package caddy

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"mime"
	"net/http"
	"net/textproto"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"liapoldus.local/server-service/contracts"
)

const (
	serverHTTPContractVersion = "server-http/v1"
	maxAPIParameters          = 64
	maxAPISchemaBytes         = 64 * 1024
	maxAPISchemaNodes         = 4096
	maxAPISchemaDepth         = 32
	maxAPIParameterBytes      = 8192
	maxAPIQueryBytes          = 8192
	maxAPIQueryValues         = 256
)

var (
	errInvalidAPIRequest = errors.New("")
	apiParameterNameRE   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,63}$`)
	apiPathParameterRE   = regexp.MustCompile(`^\{([A-Za-z][A-Za-z0-9_]{0,63})\}$`)
	apiNumberRE          = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`)
)

type apiContract struct {
	Version            string                  `json:"version"`
	Method             string                  `json:"method"`
	Path               string                  `json:"path"`
	PathParameters     []apiParameter          `json:"pathParameters,omitempty"`
	QueryParameters    []apiParameter          `json:"queryParameters,omitempty"`
	HeaderParameters   []apiParameter          `json:"headerParameters,omitempty"`
	AllowUnknownQuery  *bool                   `json:"allowUnknownQuery,omitempty"`
	AllowUnknownHeader *bool                   `json:"allowUnknownHeaders,omitempty"`
	Body               *apiRequestBody         `json:"body,omitempty"`
	Responses          []apiResponseDefinition `json:"responses"`
}

type apiParameter struct {
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Repeated  string   `json:"repeated"`
	Required  bool     `json:"required"`
	Enum      []string `json:"enum,omitempty"`
	MinLength *int     `json:"minLength,omitempty"`
	MaxLength *int     `json:"maxLength,omitempty"`
	Minimum   *float64 `json:"minimum,omitempty"`
	Maximum   *float64 `json:"maximum,omitempty"`
	MinItems  *int     `json:"minItems,omitempty"`
	MaxItems  *int     `json:"maxItems,omitempty"`
}

type apiRequestBody struct {
	Required bool            `json:"required"`
	MaxBytes int64           `json:"maxBytes,omitempty"`
	Schema   json.RawMessage `json:"schema"`
}

type apiResponseDefinition struct {
	Status      int             `json:"status"`
	ContentType string          `json:"contentType"`
	Schema      json.RawMessage `json:"schema"`
	Example     json.RawMessage `json:"example"`
}

type compiledAPIContract struct {
	definition        apiContract
	pathExpression    *regexp.Regexp
	pathNames         []string
	pathParams        map[string]apiParameter
	queryParams       map[string]apiParameter
	headerParams      map[string]apiParameter
	requestBodySchema *jsonschema.Schema
	requestBodyBytes  int64
	allowUnknownQuery bool
	allowUnknownHeads bool
}

type typedRequestParameters struct {
	Version string          `json:"version"`
	Path    map[string]any  `json:"path,omitempty"`
	Query   map[string]any  `json:"query,omitempty"`
	Headers map[string]any  `json:"headers,omitempty"`
	Body    json.RawMessage `json:"body,omitempty"`
}

func compileAPIContract(contract apiContract, routeID string, mode string) (*compiledAPIContract, error) {
	if contract.Version != serverHTTPContractVersion || !validHTTPMethod(contract.Method) || routeID == "" {
		return nil, errUnsupportedSettings
	}
	expression, names, err := compileAPIPath(contract.Path)
	if err != nil {
		return nil, errUnsupportedSettings
	}
	compiled := &compiledAPIContract{
		definition: contract, pathExpression: expression, pathNames: names,
		pathParams:        make(map[string]apiParameter, len(contract.PathParameters)),
		queryParams:       make(map[string]apiParameter, len(contract.QueryParameters)),
		headerParams:      make(map[string]apiParameter, len(contract.HeaderParameters)),
		allowUnknownQuery: contract.AllowUnknownQuery == nil || *contract.AllowUnknownQuery,
		allowUnknownHeads: contract.AllowUnknownHeader == nil || *contract.AllowUnknownHeader,
	}
	if len(contract.PathParameters)+len(contract.QueryParameters)+len(contract.HeaderParameters) > maxAPIParameters {
		return nil, errUnsupportedSettings
	}
	for _, parameter := range contract.PathParameters {
		if !validAPIParameter(parameter) || parameter.Repeated != "single" || !parameter.Required {
			return nil, errUnsupportedSettings
		}
		if _, exists := compiled.pathParams[parameter.Name]; exists {
			return nil, errUnsupportedSettings
		}
		compiled.pathParams[parameter.Name] = parameter
	}
	if len(compiled.pathParams) != len(names) {
		return nil, errUnsupportedSettings
	}
	for _, name := range names {
		if _, exists := compiled.pathParams[name]; !exists {
			return nil, errUnsupportedSettings
		}
	}
	for _, parameter := range contract.QueryParameters {
		if !validAPIParameter(parameter) {
			return nil, errUnsupportedSettings
		}
		if _, exists := compiled.queryParams[parameter.Name]; exists {
			return nil, errUnsupportedSettings
		}
		compiled.queryParams[parameter.Name] = parameter
	}
	for _, parameter := range contract.HeaderParameters {
		if !validAPIParameter(parameter) || !validAPIHeaderName(parameter.Name) {
			return nil, errUnsupportedSettings
		}
		canonical := textproto.CanonicalMIMEHeaderKey(parameter.Name)
		if _, blocked := blockedAPIHeaders[canonical]; blocked {
			return nil, errUnsupportedSettings
		}
		if _, exists := compiled.headerParams[canonical]; exists {
			return nil, errUnsupportedSettings
		}
		compiled.headerParams[canonical] = parameter
	}
	if mode != "call" && contract.Body != nil {
		return nil, errUnsupportedSettings
	}
	if contract.Body != nil {
		if len(contract.Body.Schema) == 0 || len(contract.Body.Schema) > maxAPISchemaBytes {
			return nil, errUnsupportedSettings
		}
		maxBytes := contract.Body.MaxBytes
		dispatchLimits, err := contracts.LoadHTTPDispatch()
		if err != nil {
			return nil, errUnsupportedSettings
		}
		if maxBytes == 0 {
			maxBytes = int64(dispatchLimits.MaxRequestBytes)
		}
		if maxBytes < 1 || maxBytes > int64(dispatchLimits.MaxRequestBytes) {
			return nil, errUnsupportedSettings
		}
		bodySchema, err := compileEmbeddedSchema(contract.Body.Schema, "request-body", routeID)
		if err != nil {
			return nil, errUnsupportedSettings
		}
		compiled.requestBodySchema = bodySchema
		compiled.requestBodyBytes = maxBytes
	}
	if len(contract.Responses) == 0 || len(contract.Responses) > 32 {
		return nil, errUnsupportedSettings
	}
	seenStatuses := make(map[int]struct{}, len(contract.Responses))
	for _, response := range contract.Responses {
		if response.Status < 200 || response.Status > 599 || len(response.Schema) == 0 || len(response.Schema) > maxAPISchemaBytes || len(response.Example) == 0 {
			return nil, errUnsupportedSettings
		}
		if _, exists := seenStatuses[response.Status]; exists {
			return nil, errUnsupportedSettings
		}
		seenStatuses[response.Status] = struct{}{}
		mediaType, _, err := mimeParseMediaType(response.ContentType)
		if err != nil || mediaType != response.ContentType || response.ContentType == "" {
			return nil, errUnsupportedSettings
		}
		responseSchema, err := compileEmbeddedSchema(response.Schema, "response", routeID)
		if err != nil {
			return nil, errUnsupportedSettings
		}
		var example any
		if !json.Valid(response.Example) || json.Unmarshal(response.Example, &example) != nil || responseSchema.Validate(example) != nil {
			return nil, errUnsupportedSettings
		}
	}
	return compiled, nil
}

func compileAPIPath(value string) (*regexp.Regexp, []string, error) {
	if value == "" || len(value) > 2048 || !utf8.ValidString(value) || !strings.HasPrefix(value, "/") || strings.ContainsAny(value, "?#\\") || strings.Contains(value, "//") {
		return nil, nil, errUnsupportedSettings
	}
	if value == "/" {
		compiled, err := regexp.Compile(`^/$`)
		return compiled, nil, err
	}
	var expression strings.Builder
	expression.WriteString("^/")
	var names []string
	seen := make(map[string]struct{})
	segments := strings.Split(value, "/")
	for index, segment := range segments {
		if index == 0 {
			continue
		}
		if index > 1 {
			expression.WriteByte('/')
		}
		if segment == "" {
			if index != len(segments)-1 {
				return nil, nil, errUnsupportedSettings
			}
			continue
		}
		if segment == "." || segment == ".." || containsPathControl(segment) {
			return nil, nil, errUnsupportedSettings
		}
		if matches := apiPathParameterRE.FindStringSubmatch(segment); matches != nil {
			name := matches[1]
			if _, duplicate := seen[name]; duplicate {
				return nil, nil, errUnsupportedSettings
			}
			seen[name] = struct{}{}
			names = append(names, name)
			expression.WriteString("([^/]+)")
			continue
		}
		if strings.ContainsAny(segment, "{}") {
			return nil, nil, errUnsupportedSettings
		}
		decoded, err := url.PathUnescape(segment)
		if err != nil || decoded != segment || decoded == "." || decoded == ".." {
			return nil, nil, errUnsupportedSettings
		}
		expression.WriteString(regexp.QuoteMeta(segment))
	}
	expression.WriteByte('$')
	compiled, err := regexp.Compile(expression.String())
	return compiled, names, err
}

func (contract *compiledAPIContract) validateRequest(request *http.Request, body []byte) (typedRequestParameters, int) {
	if contract == nil || request == nil || request.URL == nil || request.Method != contract.definition.Method {
		return typedRequestParameters{}, http.StatusBadRequest
	}
	canonicalPath, err := canonicalRequestPath(request)
	if err != nil || len(canonicalPath) > 2048 || len(request.URL.RawQuery) > maxAPIQueryBytes {
		return typedRequestParameters{}, http.StatusBadRequest
	}
	indices := contract.pathExpression.FindStringSubmatch(canonicalPath)
	if indices == nil || len(indices) != len(contract.pathNames)+1 {
		return typedRequestParameters{}, http.StatusBadRequest
	}
	parameters := typedRequestParameters{Version: serverHTTPContractVersion}
	pathValues := make(map[string][]string, len(contract.pathNames))
	for index, name := range contract.pathNames {
		pathValues[name] = []string{indices[index+1]}
	}
	parameters.Path, err = validateAPILocation(pathValues, contract.pathParams, true)
	if err != nil {
		return typedRequestParameters{}, http.StatusBadRequest
	}
	query, err := url.ParseQuery(request.URL.RawQuery)
	queryValueCount := 0
	for _, values := range query {
		queryValueCount += len(values)
	}
	if err != nil || queryValueCount > maxAPIQueryValues || (!contract.allowUnknownQuery && hasUnknownQuery(query, contract.queryParams)) {
		return typedRequestParameters{}, http.StatusBadRequest
	}
	parameters.Query, err = validateAPILocation(query, contract.queryParams, false)
	if err != nil {
		return typedRequestParameters{}, http.StatusBadRequest
	}
	headerValues := make(map[string][]string, len(request.Header))
	for name, values := range request.Header {
		canonical := textproto.CanonicalMIMEHeaderKey(name)
		if _, blocked := blockedAPIHeaders[canonical]; blocked {
			continue
		}
		headerValues[canonical] = append([]string(nil), values...)
	}
	if !contract.allowUnknownHeads && hasUnknownHeaders(headerValues, contract.headerParams) {
		return typedRequestParameters{}, http.StatusBadRequest
	}
	parameters.Headers, err = validateAPILocation(headerValues, contract.headerParams, false)
	if err != nil {
		return typedRequestParameters{}, http.StatusBadRequest
	}
	if contract.requestBodySchema != nil {
		if len(body) == 0 {
			if contract.definition.Body.Required {
				return typedRequestParameters{}, http.StatusBadRequest
			}
		} else {
			if int64(len(body)) > contract.requestBodyBytes || !isJSONContentType(request.Header.Get("Content-Type")) || !contracts.ValidJSONNoDuplicateKeys(body) {
				return typedRequestParameters{}, http.StatusBadRequest
			}
			var candidate any
			decoder := json.NewDecoder(bytes.NewReader(body))
			decoder.UseNumber()
			if decoder.Decode(&candidate) != nil || contract.requestBodySchema.Validate(candidate) != nil {
				return typedRequestParameters{}, http.StatusBadRequest
			}
			parameters.Body = append(json.RawMessage(nil), body...)
		}
	} else if len(body) != 0 {
		return typedRequestParameters{}, http.StatusBadRequest
	}
	return parameters, 0
}

func validateAPILocation(values map[string][]string, definitions map[string]apiParameter, pathValues bool) (map[string]any, error) {
	result := make(map[string]any, len(definitions))
	for name, definition := range definitions {
		items, exists := values[name]
		if !exists || len(items) == 0 {
			if definition.Required {
				return nil, errInvalidAPIRequest
			}
			continue
		}
		if pathValues && len(items) != 1 {
			return nil, errInvalidAPIRequest
		}
		decoded, err := decodeAPIParameter(definition, items)
		if err != nil {
			return nil, errInvalidAPIRequest
		}
		result[name] = decoded
	}
	return result, nil
}

func decodeAPIParameter(definition apiParameter, values []string) (any, error) {
	var valuesToDecode []string
	switch definition.Repeated {
	case "single":
		if len(values) != 1 {
			return nil, errInvalidAPIRequest
		}
		valuesToDecode = values
	case "first":
		if definition.Type != "string" && definition.Type != "integer" && definition.Type != "number" && definition.Type != "boolean" {
			return nil, errInvalidAPIRequest
		}
		valuesToDecode = values[:1]
	case "last":
		if definition.Type != "string" && definition.Type != "integer" && definition.Type != "number" && definition.Type != "boolean" {
			return nil, errInvalidAPIRequest
		}
		valuesToDecode = values[len(values)-1:]
	case "repeat":
		if !strings.HasSuffix(definition.Type, "[]") {
			return nil, errInvalidAPIRequest
		}
		valuesToDecode = values
	case "commaDelimited":
		if !strings.HasSuffix(definition.Type, "[]") || len(values) != 1 {
			return nil, errInvalidAPIRequest
		}
		valuesToDecode = strings.Split(values[0], ",")
	default:
		return nil, errInvalidAPIRequest
	}
	array := strings.HasSuffix(definition.Type, "[]")
	baseType := strings.TrimSuffix(definition.Type, "[]")
	if array && len(valuesToDecode) > maxAPIParameters {
		return nil, errInvalidAPIRequest
	}
	decoded := make([]any, 0, len(valuesToDecode))
	for _, value := range valuesToDecode {
		if len(value) > maxAPIParameterBytes || !utf8.ValidString(value) {
			return nil, errInvalidAPIRequest
		}
		var parsed any
		switch baseType {
		case "string":
			length := utf8.RuneCountInString(value)
			if definition.MinLength != nil && length < *definition.MinLength || definition.MaxLength != nil && length > *definition.MaxLength {
				return nil, errInvalidAPIRequest
			}
			if len(definition.Enum) != 0 && !containsString(definition.Enum, value) {
				return nil, errInvalidAPIRequest
			}
			parsed = value
		case "integer":
			integer, err := strconv.ParseInt(value, 10, 64)
			if err != nil || strings.HasPrefix(value, "+") || (len(value) > 1 && value[0] == '0') || (len(value) > 2 && strings.HasPrefix(value, "-0")) {
				return nil, errInvalidAPIRequest
			}
			number := float64(integer)
			if !withinAPIBounds(number, definition.Minimum, definition.Maximum) {
				return nil, errInvalidAPIRequest
			}
			parsed = integer
		case "number":
			number, err := strconv.ParseFloat(value, 64)
			if err != nil || !apiNumberRE.MatchString(value) || math.IsNaN(number) || math.IsInf(number, 0) || !withinAPIBounds(number, definition.Minimum, definition.Maximum) {
				return nil, errInvalidAPIRequest
			}
			parsed = number
		case "boolean":
			if value != "true" && value != "false" {
				return nil, errInvalidAPIRequest
			}
			parsed = value == "true"
		default:
			return nil, errInvalidAPIRequest
		}
		decoded = append(decoded, parsed)
	}
	if array {
		if definition.MinItems != nil && len(decoded) < *definition.MinItems || definition.MaxItems != nil && len(decoded) > *definition.MaxItems {
			return nil, errInvalidAPIRequest
		}
		return decoded, nil
	}
	if len(decoded) != 1 {
		return nil, errInvalidAPIRequest
	}
	return decoded[0], nil
}

func validAPIParameter(parameter apiParameter) bool {
	if !apiParameterNameRE.MatchString(parameter.Name) {
		return false
	}
	baseType := strings.TrimSuffix(parameter.Type, "[]")
	array := strings.HasSuffix(parameter.Type, "[]")
	if baseType != "string" && baseType != "integer" && baseType != "number" && baseType != "boolean" {
		return false
	}
	switch parameter.Repeated {
	case "single", "first", "last":
		if array {
			return false
		}
	case "repeat", "commaDelimited":
		if !array {
			return false
		}
	default:
		return false
	}
	if parameter.MinLength != nil && (*parameter.MinLength < 0 || *parameter.MinLength > maxAPIParameterBytes) || parameter.MaxLength != nil && (*parameter.MaxLength < 0 || *parameter.MaxLength > maxAPIParameterBytes) {
		return false
	}
	if parameter.MinLength != nil && parameter.MaxLength != nil && *parameter.MinLength > *parameter.MaxLength {
		return false
	}
	if parameter.Minimum != nil && (math.IsNaN(*parameter.Minimum) || math.IsInf(*parameter.Minimum, 0)) || parameter.Maximum != nil && (math.IsNaN(*parameter.Maximum) || math.IsInf(*parameter.Maximum, 0)) {
		return false
	}
	if parameter.Minimum != nil && parameter.Maximum != nil && *parameter.Minimum > *parameter.Maximum {
		return false
	}
	if len(parameter.Enum) != 0 && baseType != "string" {
		return false
	}
	if len(parameter.Enum) > maxAPIParameters {
		return false
	}
	seenEnum := make(map[string]struct{}, len(parameter.Enum))
	for _, value := range parameter.Enum {
		if len(value) > maxAPIParameterBytes || !utf8.ValidString(value) {
			return false
		}
		if _, exists := seenEnum[value]; exists {
			return false
		}
		seenEnum[value] = struct{}{}
	}
	if (parameter.MinLength != nil || parameter.MaxLength != nil || len(parameter.Enum) != 0) && baseType != "string" {
		return false
	}
	if (parameter.Minimum != nil || parameter.Maximum != nil) && baseType != "integer" && baseType != "number" {
		return false
	}
	if (parameter.MinItems != nil || parameter.MaxItems != nil) && !array {
		return false
	}
	if parameter.MinItems != nil && (*parameter.MinItems < 0 || *parameter.MinItems > maxAPIParameters) || parameter.MaxItems != nil && (*parameter.MaxItems < 0 || *parameter.MaxItems > maxAPIParameters) {
		return false
	}
	if parameter.MinItems != nil && parameter.MaxItems != nil && *parameter.MinItems > *parameter.MaxItems {
		return false
	}
	return true
}

func validHTTPMethod(value string) bool {
	if value == "" || len(value) > 32 {
		return false
	}
	return httpTokenRE.MatchString(value)
}

var httpTokenRE = regexp.MustCompile(`^[!#$%&'*+.^_` + "`" + `|~0-9A-Za-z-]+$`)

var blockedAPIHeaders = map[string]struct{}{
	"Authorization": {}, "Connection": {}, "Keep-Alive": {}, "Proxy-Authenticate": {},
	"Proxy-Authorization": {}, "Te": {}, "Trailer": {}, "Transfer-Encoding": {}, "Upgrade": {},
	"Cookie": {}, "Content-Type": {},
}

func validAPIHeaderName(value string) bool {
	canonical := textproto.CanonicalMIMEHeaderKey(value)
	return canonical != "" && canonical == value && httpTokenRE.MatchString(value)
}

func hasUnknownQuery(values url.Values, definitions map[string]apiParameter) bool {
	for name := range values {
		if _, exists := definitions[name]; !exists {
			return true
		}
	}
	return false
}

func hasUnknownHeaders(values map[string][]string, definitions map[string]apiParameter) bool {
	for name := range values {
		if _, exists := definitions[name]; !exists {
			return true
		}
	}
	return false
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func withinAPIBounds(value float64, minimum, maximum *float64) bool {
	return (minimum == nil || value >= *minimum) && (maximum == nil || value <= *maximum)
}

func isJSONContentType(value string) bool {
	mediaType, parameters, err := mimeParseMediaType(value)
	if err != nil || mediaType != "application/json" {
		return false
	}
	charset := parameters["charset"]
	return charset == "" || strings.EqualFold(charset, "utf-8")
}

func compileEmbeddedSchema(raw json.RawMessage, kind, routeID string) (*jsonschema.Schema, error) {
	if len(raw) == 0 || len(raw) > maxAPISchemaBytes || !contracts.ValidJSONNoDuplicateKeys(raw) {
		return nil, errUnsupportedSettings
	}
	var document any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&document) != nil {
		return nil, errUnsupportedSettings
	}
	nodes := 0
	if !withinSchemaLimits(document, 0, &nodes) || nodes > maxAPISchemaNodes {
		return nil, errUnsupportedSettings
	}
	resourceID := "https://liapoldus.github.io/services/server/http-contract/v1/" + kind + "/" + url.PathEscape(routeID)
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource(resourceID, document); err != nil {
		return nil, err
	}
	return compiler.Compile(resourceID)
}

func withinSchemaLimits(value any, depth int, nodes *int) bool {
	if depth > maxAPISchemaDepth {
		return false
	}
	(*nodes)++
	switch item := value.(type) {
	case map[string]any:
		if len(item) > maxAPIParameters {
			return false
		}
		for _, child := range item {
			if !withinSchemaLimits(child, depth+1, nodes) {
				return false
			}
		}
	case []any:
		if len(item) > maxAPIParameters {
			return false
		}
		for _, child := range item {
			if !withinSchemaLimits(child, depth+1, nodes) {
				return false
			}
		}
	}
	return *nodes <= maxAPISchemaNodes
}

func mimeParseMediaType(value string) (string, map[string]string, error) {
	return mime.ParseMediaType(value)
}
