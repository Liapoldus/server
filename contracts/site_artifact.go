package contracts

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"sync"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// SitePublishErrorCode maps a classified application error to the existing
// Server-owned Admin Action error catalog.
func SitePublishErrorCode(code string) (string, error) {
	_, resolved, err := sitePublishProblem(code)
	return resolved, err
}

// SitePublishProblem returns the HTTP status and error code declared by the
// operation catalog. Runtime handlers must not duplicate public status values.
func SitePublishProblem(category string) (int, string, error) {
	status, code, err := sitePublishProblem(category)
	return status, code, err
}

func sitePublishProblem(code string) (int, string, error) {
	contents, readErr := fs.ReadFile(files, "v1/admin-actions.json")
	if readErr != nil {
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
	operation, exists := catalog.Operations["server.sites.publish"]
	if !exists {
		return 0, "", ErrInvalidAssets
	}
	problem, exists := operation.Errors[code]
	if !exists || problem.HTTP < 400 || problem.HTTP > 599 {
		return 0, "", ErrInvalidAssets
	}
	return problem.HTTP, code, nil
}

type SiteOperationContract struct {
	Capability         string
	RollbackCapability string
	ManifestEntry      string
	PageID             string
	SectionID          string
	ActionID           string
	StatusCapability   string
	StatusPageID       string
	StatusActionID     string
	WorkerPollInterval time.Duration
	RollbackActionID   string
	AcceptedState      string
	RunningState       string
	CompletedState     string
	FailedState        string
}

// LoadSiteOperationContract binds runtime status values to the Server-owned
// publish and operation contracts instead of duplicating product strings.
func LoadSiteOperationContract() (SiteOperationContract, error) {
	manifest, err := loadSitePublishManifestLink()
	if err != nil || manifest.PublishCapability == "" || manifest.RollbackCapability == "" {
		return SiteOperationContract{}, ErrInvalidAssets
	}
	contents, err := fs.ReadFile(files, "v1/admin-actions.json")
	if err != nil {
		return SiteOperationContract{}, ErrInvalidAssets
	}
	var document struct {
		Operations map[string]struct {
			SurfaceBinding struct {
				PageID    string `json:"page"`
				SectionID string `json:"section"`
				ActionID  string `json:"action"`
			} `json:"surfaceBinding"`
		} `json:"operations"`
		OperationStatus struct {
			PluginStatusCapability string   `json:"pluginStatusCapability"`
			PluginStates           []string `json:"pluginStates"`
			WorkerPollMilliseconds int      `json:"workerPollMilliseconds"`
		} `json:"operationStatus"`
	}
	if json.Unmarshal(contents, &document) != nil || len(document.OperationStatus.PluginStates) != 4 ||
		document.OperationStatus.WorkerPollMilliseconds < 25 || document.OperationStatus.WorkerPollMilliseconds > 60000 {
		return SiteOperationContract{}, ErrInvalidAssets
	}
	operation, exists := document.Operations[manifest.PublishCapability]
	if !exists || operation.SurfaceBinding.PageID == "" || operation.SurfaceBinding.SectionID == "" || operation.SurfaceBinding.ActionID == "" {
		return SiteOperationContract{}, ErrInvalidAssets
	}
	statusOperation, exists := document.Operations[document.OperationStatus.PluginStatusCapability]
	if !exists || statusOperation.SurfaceBinding.PageID == "" || statusOperation.SurfaceBinding.ActionID == "" {
		return SiteOperationContract{}, ErrInvalidAssets
	}
	rollbackOperation, exists := document.Operations[manifest.RollbackCapability]
	if !exists || rollbackOperation.SurfaceBinding.PageID == "" || rollbackOperation.SurfaceBinding.ActionID == "" {
		return SiteOperationContract{}, ErrInvalidAssets
	}
	states := document.OperationStatus.PluginStates
	return SiteOperationContract{Capability: manifest.PublishCapability, RollbackCapability: manifest.RollbackCapability, ManifestEntry: manifest.ArchiveEntry,
		PageID: operation.SurfaceBinding.PageID, SectionID: operation.SurfaceBinding.SectionID, ActionID: operation.SurfaceBinding.ActionID,
		StatusCapability: document.OperationStatus.PluginStatusCapability,
		StatusPageID:     statusOperation.SurfaceBinding.PageID, StatusActionID: statusOperation.SurfaceBinding.ActionID,
		RollbackActionID: rollbackOperation.SurfaceBinding.ActionID,
		WorkerPollInterval: time.Duration(document.OperationStatus.WorkerPollMilliseconds) * time.Millisecond,
		AcceptedState:    states[0], RunningState: states[1], CompletedState: states[2], FailedState: states[3]}, nil
}

// AdminActionProblem finds the operation bound to one page/action and resolves
// its declared error status. Unknown bindings fail closed.
func AdminActionProblem(pageID, actionID, category string) (int, string, error) {
	contents, err := fs.ReadFile(files, "v1/admin-actions.json")
	if err != nil {
		return 0, "", ErrInvalidAssets
	}
	var catalog struct {
		Operations map[string]struct {
			SurfaceBinding struct {
				PageID   string `json:"page"`
				ActionID string `json:"action"`
			} `json:"surfaceBinding"`
			Errors map[string]struct {
				HTTP int `json:"http"`
			} `json:"errors"`
		} `json:"operations"`
	}
	if json.Unmarshal(contents, &catalog) != nil {
		return 0, "", ErrInvalidAssets
	}
	for _, operation := range catalog.Operations {
		if operation.SurfaceBinding.PageID != pageID || operation.SurfaceBinding.ActionID != actionID {
			continue
		}
		problem, exists := operation.Errors[category]
		if !exists || problem.HTTP < 400 || problem.HTTP > 599 {
			return 0, "", ErrInvalidAssets
		}
		return problem.HTTP, category, nil
	}
	return 0, "", ErrInvalidAssets
}

// AdminSurface returns the plugin-owned action descriptor exactly as versioned.
func AdminSurface() ([]byte, error) {
	contents, err := fs.ReadFile(files, "v1/admin-surface.json")
	if err != nil || !ValidJSONNoDuplicateKeys(contents) {
		return nil, ErrInvalidAssets
	}
	return append([]byte(nil), contents...), nil
}

type SiteArtifactMetadata struct {
	Version  int `json:"version"`
	Artifact struct {
		MediaType  string `json:"mediaType"`
		ByteLength int64  `json:"byteLength"`
		SHA256     string `json:"sha256"`
	} `json:"artifact"`
	Payload struct {
		SiteID                  string  `json:"siteId"`
		ExpectedCurrentRevision *string `json:"expectedCurrentRevision,omitempty"`
	} `json:"payload"`
}

var (
	artifactSchemaOnce sync.Once
	artifactSchema     *jsonschema.Schema
	artifactSchemaErr  error
)

// ValidateSiteArtifactMetadata validates the exact product metadata document
// against the Server-owned schema and refuses duplicate keys before decoding.
func ValidateSiteArtifactMetadata(contents []byte) (SiteArtifactMetadata, error) {
	if len(contents) == 0 || !ValidJSONNoDuplicateKeys(contents) {
		return SiteArtifactMetadata{}, ErrInvalidAssets
	}
	schema, err := compiledArtifactSchema()
	if err != nil {
		return SiteArtifactMetadata{}, ErrInvalidAssets
	}
	var value any
	if err := json.Unmarshal(contents, &value); err != nil || schema.Validate(value) != nil {
		return SiteArtifactMetadata{}, ErrInvalidAssets
	}
	var metadata SiteArtifactMetadata
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&metadata); err != nil {
		return SiteArtifactMetadata{}, ErrInvalidAssets
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return SiteArtifactMetadata{}, ErrInvalidAssets
	}
	return metadata, nil
}

// ValidJSONNoDuplicateKeys accepts exactly one JSON value and rejects objects
// whose duplicate keys would otherwise have last-value-wins semantics.
func ValidJSONNoDuplicateKeys(contents []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()
	if consumeUniqueJSONValue(decoder) != nil {
		return false
	}
	_, err := decoder.Token()
	return errors.Is(err, io.EOF)
}

func consumeUniqueJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return nil
	}
	switch delimiter {
	case '{':
		keys := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return ErrInvalidAssets
			}
			if _, exists := keys[key]; exists {
				return ErrInvalidAssets
			}
			keys[key] = struct{}{}
			if err := consumeUniqueJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return ErrInvalidAssets
		}
	case '[':
		for decoder.More() {
			if err := consumeUniqueJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return ErrInvalidAssets
		}
	default:
		return ErrInvalidAssets
	}
	return nil
}

func compiledArtifactSchema() (*jsonschema.Schema, error) {
	artifactSchemaOnce.Do(func() {
		contents, err := fs.ReadFile(files, "v1/artifact-metadata.schema.json")
		if err != nil {
			artifactSchemaErr = ErrInvalidAssets
			return
		}
		var document any
		if err := json.Unmarshal(contents, &document); err != nil {
			artifactSchemaErr = ErrInvalidAssets
			return
		}
		var id struct {
			ID string `json:"$id"`
		}
		if err := json.Unmarshal(contents, &id); err != nil || id.ID == "" {
			artifactSchemaErr = ErrInvalidAssets
			return
		}
		compiler := jsonschema.NewCompiler()
		compiler.DefaultDraft(jsonschema.Draft2020)
		if err := compiler.AddResource(id.ID, document); err != nil {
			artifactSchemaErr = ErrInvalidAssets
			return
		}
		artifactSchema, artifactSchemaErr = compiler.Compile(id.ID)
	})
	return artifactSchema, artifactSchemaErr
}
