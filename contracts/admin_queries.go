package contracts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type AdminQuery struct {
	Resource   string
	Capability string
	Payload    []byte
}

func AdminQueryDefaultLimit() (int, error) {
	contents, err := fs.ReadFile(files, "v1/admin-query.json")
	if err != nil {
		return 0, ErrInvalidAssets
	}
	var catalog struct {
		DefaultLimit int `json:"defaultLimit"`
	}
	if json.Unmarshal(contents, &catalog) != nil || catalog.DefaultLimit < 1 || catalog.DefaultLimit > 100 {
		return 0, ErrInvalidAssets
	}
	return catalog.DefaultLimit, nil
}

func AdminQueryFallbackCapability() (string, error) {
	contents, err := fs.ReadFile(files, "v1/admin-query.json")
	if err != nil {
		return "", ErrInvalidAssets
	}
	var catalog struct {
		FallbackCapability string `json:"fallbackCapability"`
	}
	if json.Unmarshal(contents, &catalog) != nil || catalog.FallbackCapability == "" {
		return "", ErrInvalidAssets
	}
	return catalog.FallbackCapability, nil
}

func AdminQueryActionID() (string, error) {
	contents, err := fs.ReadFile(files, "v1/admin-query.json")
	if err != nil {
		return "", ErrInvalidAssets
	}
	var catalog struct {
		ActionID string `json:"actionId"`
	}
	if json.Unmarshal(contents, &catalog) != nil || catalog.ActionID == "" {
		return "", ErrInvalidAssets
	}
	return catalog.ActionID, nil
}

func CertificateStatusCapability() (string, error) {
	contents, err := fs.ReadFile(files, "v1/admin-query.json")
	if err != nil {
		return "", ErrInvalidAssets
	}
	var catalog struct {
		Capability string `json:"certificateStatusCapability"`
	}
	if json.Unmarshal(contents, &catalog) != nil || catalog.Capability == "" {
		return "", ErrInvalidAssets
	}
	return catalog.Capability, nil
}

func AdminActionSuccessStatus(capability string) (int, error) {
	contents, err := fs.ReadFile(files, "v1/admin-actions.json")
	if err != nil {
		return 0, ErrInvalidAssets
	}
	var catalog struct {
		Operations map[string]struct {
			HTTPStatus int `json:"httpStatus"`
		} `json:"operations"`
	}
	if json.Unmarshal(contents, &catalog) != nil {
		return 0, ErrInvalidAssets
	}
	operation, exists := catalog.Operations[capability]
	if !exists || operation.HTTPStatus < 200 || operation.HTTPStatus > 299 {
		return 0, ErrInvalidAssets
	}
	return operation.HTTPStatus, nil
}

func AdminProblem(capability, category string) (int, string, error) {
	contents, err := fs.ReadFile(files, "v1/admin-actions.json")
	if err != nil {
		return 0, "", ErrInvalidAssets
	}
	var catalog struct {
		Operations map[string]struct {
			Errors map[string]struct {
				HTTP int `json:"http"`
			} `json:"errors"`
		} `json:"operations"`
	}
	if json.Unmarshal(contents, &catalog) != nil {
		return 0, "", ErrInvalidAssets
	}
	operation, exists := catalog.Operations[capability]
	if !exists {
		return 0, "", ErrInvalidAssets
	}
	problem, exists := operation.Errors[category]
	if !exists || problem.HTTP < 400 || problem.HTTP > 599 {
		return 0, "", ErrInvalidAssets
	}
	return problem.HTTP, category, nil
}

// ResolveAdminQuery validates the plugin-owned query discriminator, resolves
// its capability from the catalog and validates the remaining fields against
// that capability's existing request schema.
func ResolveAdminQuery(pageID string, contents []byte) (AdminQuery, error) {
	if pageID == "" || len(contents) == 0 || !ValidJSONNoDuplicateKeys(contents) {
		return AdminQuery{}, ErrInvalidAssets
	}
	var input map[string]json.RawMessage
	if json.Unmarshal(contents, &input) != nil || input == nil {
		return AdminQuery{}, ErrInvalidAssets
	}
	resourceRaw, exists := input["resource"]
	if !exists {
		return AdminQuery{}, ErrInvalidAssets
	}
	var resource string
	if json.Unmarshal(resourceRaw, &resource) != nil || resource == "" {
		return AdminQuery{}, ErrInvalidAssets
	}
	querySchema, err := fs.ReadFile(files, "v1/admin-query.schema.json")
	if err != nil || validateAgainstSchema(querySchema, contents, "admin-query") != nil {
		return AdminQuery{}, ErrInvalidAssets
	}
	queryCatalog, err := fs.ReadFile(files, "v1/admin-query.json")
	if err != nil {
		return AdminQuery{}, ErrInvalidAssets
	}
	var catalog struct {
		Resources     map[string]string   `json:"resources"`
		PageResources map[string][]string `json:"pageResources"`
	}
	if json.Unmarshal(queryCatalog, &catalog) != nil {
		return AdminQuery{}, ErrInvalidAssets
	}
	capability, exists := catalog.Resources[resource]
	if !exists || capability == "" {
		return AdminQuery{}, ErrInvalidAssets
	}
	pageResources, exists := catalog.PageResources[pageID]
	if !exists || !containsResource(pageResources, resource) {
		return AdminQuery{}, ErrInvalidAssets
	}
	delete(input, "resource")
	payload, err := json.Marshal(input)
	if err != nil || !validAdminQueryPayload(capability, payload) {
		return AdminQuery{}, ErrInvalidAssets
	}
	return AdminQuery{Resource: resource, Capability: capability, Payload: payload}, nil
}

func containsResource(resources []string, target string) bool {
	for _, resource := range resources {
		if resource == target {
			return true
		}
	}
	return false
}

func AdminActionCapability(pageID, actionID string) (string, error) {
	contents, err := fs.ReadFile(files, "v1/admin-actions.json")
	if err != nil {
		return "", ErrInvalidAssets
	}
	var catalog struct {
		Operations map[string]struct {
			SurfaceBinding struct {
				PageID   string `json:"page"`
				ActionID string `json:"action"`
			} `json:"surfaceBinding"`
		} `json:"operations"`
	}
	if json.Unmarshal(contents, &catalog) != nil {
		return "", ErrInvalidAssets
	}
	for capability, operation := range catalog.Operations {
		if operation.SurfaceBinding.PageID == pageID && operation.SurfaceBinding.ActionID == actionID {
			return capability, nil
		}
	}
	return "", ErrInvalidAssets
}

func ValidateAdminRequest(capability string, contents []byte) error {
	if capability == "" || len(contents) == 0 || !ValidJSONNoDuplicateKeys(contents) {
		return ErrInvalidAssets
	}
	return validateCapabilityRequest(capability, contents)
}

func validAdminQueryPayload(capability string, payload []byte) bool {
	contents, err := fs.ReadFile(files, "v1/admin-actions.json")
	if err != nil {
		return false
	}
	var catalog struct {
		Operations map[string]struct {
			RequestSchema json.RawMessage `json:"requestSchema"`
		} `json:"operations"`
	}
	if json.Unmarshal(contents, &catalog) != nil {
		return false
	}
	operation, exists := catalog.Operations[capability]
	if !exists || len(operation.RequestSchema) == 0 {
		return false
	}
	return validateAgainstSchema(operation.RequestSchema, payload, capability) == nil
}

func validateCapabilityRequest(capability string, payload []byte) error {
	contents, err := fs.ReadFile(files, "v1/admin-actions.json")
	if err != nil {
		return ErrInvalidAssets
	}
	var catalog struct {
		Operations map[string]struct {
			RequestSchema json.RawMessage `json:"requestSchema"`
		} `json:"operations"`
	}
	if json.Unmarshal(contents, &catalog) != nil {
		return ErrInvalidAssets
	}
	operation, exists := catalog.Operations[capability]
	if !exists || len(operation.RequestSchema) == 0 {
		return ErrInvalidAssets
	}
	return validateAgainstSchema(operation.RequestSchema, payload, capability)
}

func validateAgainstSchema(schemaContents, value []byte, name string) error {
	var schemaDocument any
	var candidate any
	if json.Unmarshal(schemaContents, &schemaDocument) != nil || json.Unmarshal(value, &candidate) != nil {
		return ErrInvalidAssets
	}
	var metadata struct {
		ID string `json:"$id"`
	}
	if json.Unmarshal(schemaContents, &metadata) != nil || metadata.ID == "" {
		metadata.ID = fmt.Sprintf("https://liapoldus.github.io/plugins/server/v1/%s.schema.json", name)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource(metadata.ID, schemaDocument); err != nil {
		return ErrInvalidAssets
	}
	compiled, err := compiler.Compile(metadata.ID)
	if err != nil {
		return ErrInvalidAssets
	}
	if err := compiled.Validate(candidate); err != nil {
		return ErrInvalidAssets
	}
	return nil
}

func RemoveAdminQueryDiscriminator(contents []byte) ([]byte, error) {
	var value map[string]json.RawMessage
	if !ValidJSONNoDuplicateKeys(contents) || json.Unmarshal(contents, &value) != nil || value == nil {
		return nil, ErrInvalidAssets
	}
	delete(value, "resource")
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	if err := encoder.Encode(value); err != nil {
		return nil, ErrInvalidAssets
	}
	return bytes.TrimSpace(output.Bytes()), nil
}
