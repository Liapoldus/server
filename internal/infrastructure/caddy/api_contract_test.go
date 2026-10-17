package caddy

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func validAPIContractForTest() apiContract {
	minLength, maxLength := 1, 32
	return apiContract{
		Version: serverHTTPContractVersion,
		Method:  "POST",
		Path:    "/v1/{name}",
		PathParameters: []apiParameter{{
			Name: "name", Type: "string", Repeated: "single", Required: true, MinLength: &minLength, MaxLength: &maxLength,
		}},
		QueryParameters: []apiParameter{{
			Name: "text", Type: "string", Repeated: "single", Required: true, MinLength: &minLength, MaxLength: &maxLength,
		}, {
			Name: "tag", Type: "string[]", Repeated: "repeat", Required: false, MinItems: intPointer(1), MaxItems: intPointer(4),
		}},
		HeaderParameters: []apiParameter{{
			Name: "X-Mode", Type: "string", Repeated: "single", Required: true, Enum: []string{"safe", "strict"},
		}},
		AllowUnknownQuery:  boolPointer(false),
		AllowUnknownHeader: boolPointer(false),
		Body: &apiRequestBody{
			Required: true,
			MaxBytes: 512,
			Schema:   json.RawMessage(`{"type":"object","additionalProperties":false,"required":["message"],"properties":{"message":{"type":"string","minLength":1,"maxLength":100}}}`),
		},
		Responses: []apiResponseDefinition{{
			Status: 200, ContentType: "application/json",
			Schema:  json.RawMessage(`{"type":"object","additionalProperties":false,"required":["accepted"],"properties":{"accepted":{"type":"boolean"}}}`),
			Example: json.RawMessage(`{"accepted":true}`),
		}},
	}
}

func TestAPIContractValidatesAndNormalizesRequestBeforeDispatch(t *testing.T) {
	compiled, err := compileAPIContract(validAPIContractForTest(), "echo-route", "call")
	if err != nil {
		t.Fatalf("compile contract: %v", err)
	}
	request := httptest.NewRequest("POST", "/v1/alice?text=hello&tag=one&tag=two", strings.NewReader(`{"message":"hello"}`))
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	request.Header.Set("X-Mode", "strict")
	parameters, status := compiled.validateRequest(request, []byte(`{"message":"hello"}`))
	if status != 0 {
		t.Fatalf("valid request rejected with status %d", status)
	}
	if parameters.Version != serverHTTPContractVersion || parameters.Path["name"] != "alice" || parameters.Query["text"] != "hello" || parameters.Headers["X-Mode"] != "strict" {
		t.Fatalf("unexpected normalized values: %#v", parameters)
	}
	tags, ok := parameters.Query["tag"].([]any)
	if !ok || !reflect.DeepEqual(tags, []any{"one", "two"}) {
		t.Fatalf("repeated query parameter was not preserved as an array: %#v", parameters.Query["tag"])
	}
	if string(parameters.Body) != `{"message":"hello"}` {
		t.Fatalf("body bytes changed: %s", parameters.Body)
	}
}

func TestAPIContractAcceptsDistinctResponseStatuses(t *testing.T) {
	contract := validAPIContractForTest()
	for _, status := range []int{201, 202} {
		contract.Responses = append(contract.Responses, apiResponseDefinition{
			Status: status, ContentType: "application/json",
			Schema: json.RawMessage(`{"type":"object","additionalProperties":true}`),
			Example: json.RawMessage(`{"accepted":true}`),
		})
	}
	if _, err := compileAPIContract(contract, "echo-route", "call"); err != nil {
		t.Fatalf("compile contract with distinct response statuses: %v", err)
	}
}

func TestAPIContractAllowsUnknownValuesByDefaultAndDecodesTypedArrays(t *testing.T) {
	contract := validAPIContractForTest()
	contract.AllowUnknownQuery = nil
	contract.AllowUnknownHeader = nil
	contract.Body = nil
	contract.QueryParameters = append(contract.QueryParameters, apiParameter{
		Name: "ids", Type: "integer[]", Repeated: "repeat", Required: false, MinItems: intPointer(1), MaxItems: intPointer(4),
	}, apiParameter{
		Name: "labels", Type: "string[]", Repeated: "commaDelimited", Required: false, MaxLength: intPointer(20),
	}, apiParameter{
		Name: "ratio", Type: "number", Repeated: "single", Required: false, Minimum: floatPointer(0), Maximum: floatPointer(10),
	}, apiParameter{
		Name: "enabled", Type: "boolean", Repeated: "single", Required: false,
	})
	compiled, err := compileAPIContract(contract, "echo-route", "call")
	if err != nil {
		t.Fatalf("compile contract: %v", err)
	}
	request := httptest.NewRequest("POST", "/v1/alice?text=hello&ids=2&ids=9&labels=one,two&ratio=1.25&enabled=true&extra=ignored", nil)
	request.Header.Set("X-Mode", "safe")
	request.Header.Set("X-Trace", "request-correlation")
	parameters, status := compiled.validateRequest(request, nil)
	if status != 0 {
		t.Fatalf("unknown values should be allowed by default; got %d", status)
	}
	if !reflect.DeepEqual(parameters.Query["ids"], []any{int64(2), int64(9)}) || !reflect.DeepEqual(parameters.Query["labels"], []any{"one", "two"}) || parameters.Query["ratio"] != 1.25 || parameters.Query["enabled"] != true {
		t.Fatalf("typed query values were not normalized: %#v", parameters.Query)
	}
}

func TestAPIContractRejectsUndeclaredUnaryBodyAndOversizedQuery(t *testing.T) {
	contract := validAPIContractForTest()
	contract.Body = nil
	compiled, err := compileAPIContract(contract, "echo-route", "call")
	if err != nil {
		t.Fatalf("compile contract: %v", err)
	}
	request := httptest.NewRequest("POST", "/v1/alice?text=hello", nil)
	request.Header.Set("X-Mode", "safe")
	if _, status := compiled.validateRequest(request, []byte(`{"unexpected":true}`)); status != 400 {
		t.Fatalf("undeclared unary body must be rejected, got %d", status)
	}
	request = httptest.NewRequest("POST", "/v1/alice?text="+strings.Repeat("x", maxAPIQueryBytes+1), nil)
	request.Header.Set("X-Mode", "safe")
	if _, status := compiled.validateRequest(request, nil); status != 400 {
		t.Fatalf("oversized query must be rejected, got %d", status)
	}
}

func TestAPIContractRejectsInvalidRequestsBeforeDispatch(t *testing.T) {
	compiled, err := compileAPIContract(validAPIContractForTest(), "echo-route", "call")
	if err != nil {
		t.Fatalf("compile contract: %v", err)
	}
	tests := []struct {
		name   string
		method string
		target string
		header string
		body   string
	}{
		{name: "wrong method", method: "GET", target: "/v1/alice?text=hello", header: "strict", body: `{"message":"hello"}`},
		{name: "missing required query", method: "POST", target: "/v1/alice", header: "strict", body: `{"message":"hello"}`},
		{name: "duplicate scalar query", method: "POST", target: "/v1/alice?text=one&text=two", header: "strict", body: `{"message":"hello"}`},
		{name: "unknown query", method: "POST", target: "/v1/alice?text=hello&unknown=x", header: "strict", body: `{"message":"hello"}`},
		{name: "missing declared header", method: "POST", target: "/v1/alice?text=hello", body: `{"message":"hello"}`},
		{name: "invalid enum header", method: "POST", target: "/v1/alice?text=hello", header: "loose", body: `{"message":"hello"}`},
		{name: "unknown header", method: "POST", target: "/v1/alice?text=hello", header: "strict", body: `{"message":"hello"}`},
		{name: "invalid body schema", method: "POST", target: "/v1/alice?text=hello", header: "strict", body: `{"message":""}`},
		{name: "body content type", method: "POST", target: "/v1/alice?text=hello", header: "strict", body: `{"message":"hello"}`},
		{name: "malformed query", method: "POST", target: "/v1/alice?text=%zz", header: "strict", body: `{"message":"hello"}`},
		{name: "invalid path value", method: "POST", target: "/v1/%00?text=hello", header: "strict", body: `{"message":"hello"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.target, strings.NewReader(test.body))
			if test.body != "" && test.name != "body content type" {
				request.Header.Set("Content-Type", "application/json")
			}
			if test.header != "" {
				request.Header.Set("X-Mode", test.header)
			}
			if test.name == "unknown header" {
				request.Header.Set("X-Unknown", "unexpected")
			}
			_, status := compiled.validateRequest(request, []byte(test.body))
			if status != 400 {
				t.Fatalf("expected generic 400 before dispatch, got %d", status)
			}
		})
	}
}

func TestAPIContractRejectsUnsafeOrInconsistentDefinitions(t *testing.T) {
	for _, mutate := range []struct {
		name string
		edit func(*apiContract)
	}{
		{"path placeholder mismatch", func(contract *apiContract) { contract.PathParameters = nil }},
		{"duplicate path placeholder", func(contract *apiContract) { contract.Path = "/v1/{name}/{name}" }},
		{"sensitive header", func(contract *apiContract) { contract.HeaderParameters[0].Name = "Authorization" }},
		{"stream body", func(contract *apiContract) {}},
		{"invalid response example", func(contract *apiContract) { contract.Responses[0].Example = json.RawMessage(`{"accepted":"yes"}`) }},
		{"duplicate response status", func(contract *apiContract) { contract.Responses = append(contract.Responses, contract.Responses[0]) }},
		{"external schema reference", func(contract *apiContract) {
			contract.Body.Schema = json.RawMessage(`{"$ref":"https://example.invalid/schema.json"}`)
		}},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			candidate := validAPIContractForTest()
			mutate.edit(&candidate)
			mode := "call"
			if mutate.name == "stream body" {
				mode = "sse"
			}
			if _, err := compileAPIContract(candidate, "echo-route", mode); err == nil {
				t.Fatal("unsafe or inconsistent definition accepted")
			}
		})
	}
}

func intPointer(value int) *int           { return &value }
func boolPointer(value bool) *bool        { return &value }
func floatPointer(value float64) *float64 { return &value }
