package restplugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	sdkpresentation "github.com/Liapoldus/plugin-sdk/presentation"
	"liapoldus.local/server-plugin/contracts"
	"liapoldus.local/server-plugin/internal/application"
	"liapoldus.local/server-plugin/internal/domain/models"
)

type artifactReceipt struct {
	Version     int    `json:"version"`
	OperationID string `json:"operationId"`
	State       string `json:"state"`
}

type actionProblem struct {
	Code string `json:"code"`
}

type operationStatusRequest struct {
	OperationID string `json:"operationId"`
}

type adminQueryRequest struct {
	Resource string `json:"resource"`
	SiteID   string `json:"siteId"`
	Domain   string `json:"domain"`
	State    string `json:"state"`
	Cursor   string `json:"cursor"`
	Limit    int    `json:"limit"`
}

type siteRollbackRequest struct {
	SiteID                  string `json:"siteId"`
	ExpectedCurrentRevision string `json:"expectedCurrentRevision"`
	TargetRevision          string `json:"targetRevision"`
}

type certificateStatusRequest struct {
	Domain string `json:"domain"`
}

type operationStatusResponse struct {
	OperationID     string  `json:"operationId"`
	SiteID          *string `json:"siteId"`
	Kind            string  `json:"kind"`
	State           string  `json:"state"`
	ProgressPercent *int    `json:"progressPercent"`
	UpdatedAt       string  `json:"updatedAt"`
	ErrorCode       *string `json:"errorCode"`
}

func (adapter *Adapter) AcceptArtifact(ctx context.Context, input sdkpresentation.ArtifactInput) (sdkpresentation.ArtifactResponse, error) {
	if adapter == nil || adapter.publisher == nil || input.Invocation.Validate() != nil || input.Body == nil {
		return sdkpresentation.ArtifactResponse{}, errors.New("invalid artifact action")
	}
	contract, err := contracts.LoadSiteOperationContract()
	if err != nil || input.Invocation.PageID != contract.PageID || input.Invocation.ActionID != contract.ActionID {
		return adapter.artifactProblem("invalid_input")
	}
	metadata, err := contracts.ValidateSiteArtifactMetadata(input.Metadata)
	if err != nil || (input.Invocation.IfMatch != "" && (metadata.Payload.ExpectedCurrentRevision == nil || *metadata.Payload.ExpectedCurrentRevision != input.Invocation.IfMatch)) {
		return adapter.artifactProblem("invalid_input")
	}
	operation, err := adapter.publisher.Accept(ctx, application.SitePublishInput{
		Metadata: input.Metadata, ContentType: input.ContentType,
		IdempotencyKey: input.Invocation.IdempotencyKey, Body: input.Body,
	})
	if err != nil {
		return adapter.artifactFailure(err)
	}
	receipt, err := json.Marshal(artifactReceipt{Version: 1, OperationID: operation.OperationID, State: contract.AcceptedState})
	if err != nil {
		return sdkpresentation.ArtifactResponse{}, errors.New("invalid artifact receipt")
	}
	return sdkpresentation.ArtifactResponse{StatusCode: 202, Body: receipt}, nil
}

func (adapter *Adapter) artifactFailure(err error) (sdkpresentation.ArtifactResponse, error) {
	status, code := application.ErrorStatusAndCode(err)
	if status < 400 || code == "" {
		return sdkpresentation.ArtifactResponse{}, errors.New("invalid artifact error contract")
	}
	return problemResponse(status, code)
}

func (adapter *Adapter) artifactProblem(category string) (sdkpresentation.ArtifactResponse, error) {
	status, code, err := contracts.SitePublishProblem(category)
	if err != nil {
		return sdkpresentation.ArtifactResponse{}, errors.New("invalid artifact error contract")
	}
	return problemResponse(status, code)
}

func (adapter *Adapter) AdminSurface(context.Context) ([]byte, error) {
	if adapter == nil {
		return nil, errors.New("invalid Server admin surface")
	}
	return contracts.AdminSurface()
}

func (adapter *Adapter) HandleAdminAction(ctx context.Context, input sdkpresentation.AdminActionInput) (sdkpresentation.AdminActionResponse, error) {
	if adapter == nil || adapter.publisher == nil || input.Invocation.Validate() != nil {
		return sdkpresentation.AdminActionResponse{}, errors.New("invalid Server admin action")
	}
	operationContract, err := contracts.LoadSiteOperationContract()
	if err != nil {
		return sdkpresentation.AdminActionResponse{}, errors.New("invalid Server admin action contract")
	}
	queryActionID, err := contracts.AdminQueryActionID()
	if err != nil {
		return sdkpresentation.AdminActionResponse{}, errors.New("invalid Server query contract")
	}
	if input.Invocation.ActionID == queryActionID {
		return adapter.adminQuery(ctx, input.Invocation.PageID, input.Body)
	}
	if input.Invocation.PageID == operationContract.StatusPageID && input.Invocation.ActionID == operationContract.StatusActionID {
		return adapter.operationStatus(ctx, input.Body)
	}
	capability, err := contracts.AdminActionCapability(input.Invocation.PageID, input.Invocation.ActionID)
	if err != nil {
		return sdkpresentation.AdminActionResponse{}, errors.New("invalid Server admin action contract")
	}
	if input.Invocation.PageID == operationContract.PageID && input.Invocation.ActionID == operationContract.RollbackActionID {
		return adapter.rollback(ctx, capability, input)
	}
	certificateCapability, err := contracts.CertificateStatusCapability()
	if err != nil {
		return sdkpresentation.AdminActionResponse{}, errors.New("invalid Server certificate contract")
	}
	if capability == certificateCapability {
		return adapter.certificateStatus(ctx, capability, input.Body)
	}
	return adminActionProblem(capability, "unavailable")
}

func (adapter *Adapter) operationStatus(ctx context.Context, body []byte) (sdkpresentation.AdminActionResponse, error) {
	operationContract, err := contracts.LoadSiteOperationContract()
	if err != nil {
		return sdkpresentation.AdminActionResponse{}, errors.New("invalid Server admin action contract")
	}
	if err := contracts.ValidateAdminRequest(operationContract.StatusCapability, body); err != nil {
		return adminActionProblem(operationContract.StatusCapability, "invalid_input")
	}
	var request operationStatusRequest
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF || request.OperationID == "" {
		return adminActionProblem(operationContract.StatusCapability, "invalid_input")
	}
	operation, err := adapter.publisher.Operation(ctx, request.OperationID)
	if err != nil {
		return adminActionProblem(operationContract.StatusCapability, "not_found")
	}
	var siteID *string
	if operation.SiteID != "" {
		value := operation.SiteID
		siteID = &value
	}
	var errorCode *string
	if operation.ErrorCode != "" {
		value := operation.ErrorCode
		errorCode = &value
	}
	response, err := json.Marshal(operationStatusResponse{
		OperationID: operation.OperationID, SiteID: siteID, Kind: operation.Kind,
		State: operation.State, UpdatedAt: operation.UpdatedAt, ErrorCode: errorCode,
	})
	if err != nil {
		return sdkpresentation.AdminActionResponse{}, errors.New("invalid Server operation status")
	}
	status, err := contracts.AdminActionSuccessStatus(operationContract.StatusCapability)
	if err != nil {
		return sdkpresentation.AdminActionResponse{}, errors.New("invalid Server admin action contract")
	}
	return sdkpresentation.AdminActionResponse{StatusCode: status, Body: response}, nil
}

func (adapter *Adapter) adminQuery(ctx context.Context, pageID string, body []byte) (sdkpresentation.AdminActionResponse, error) {
	query, err := contracts.ResolveAdminQuery(pageID, body)
	if err != nil {
		capability, capabilityErr := contracts.AdminQueryFallbackCapability()
		if capabilityErr != nil {
			return sdkpresentation.AdminActionResponse{}, errors.New("invalid Server query contract")
		}
		return adminActionProblem(capability, "invalid_input")
	}
	var request adminQueryRequest
	if err := json.Unmarshal(query.Payload, &request); err != nil {
		return adminActionProblem(query.Capability, "invalid_input")
	}
	defaultLimit, err := contracts.AdminQueryDefaultLimit()
	if err != nil {
		return sdkpresentation.AdminActionResponse{}, errors.New("invalid Server query contract")
	}
	if request.Limit == 0 {
		request.Limit = defaultLimit
	}
	var result any
	switch query.Resource {
	case "sites":
		page, listErr := adapter.publisher.ListSites(ctx, request.Limit, request.Cursor)
		err = listErr
		result = page
	case "releases":
		page, listErr := adapter.publisher.ListReleases(ctx, request.SiteID, request.Limit, request.Cursor)
		err = listErr
		result = page
	case "operations":
		page, listErr := adapter.publisher.ListOperations(ctx, request.State, request.Limit, request.Cursor)
		err = listErr
		result = page
	case "certificates":
		page, listErr := adapter.configuration.ListCertificates(ctx, request.Domain, request.Limit, request.Cursor)
		err = listErr
		result = page
	default:
		return adminActionProblem(query.Capability, "invalid_input")
	}
	if err != nil {
		return adminActionProblem(query.Capability, siteErrorCategory(err))
	}
	response, err := json.Marshal(result)
	if err != nil {
		return sdkpresentation.AdminActionResponse{}, errors.New("invalid Server query response")
	}
	status, err := contracts.AdminActionSuccessStatus(query.Capability)
	if err != nil {
		return sdkpresentation.AdminActionResponse{}, errors.New("invalid Server query contract")
	}
	return sdkpresentation.AdminActionResponse{StatusCode: status, Body: response}, nil
}

func (adapter *Adapter) certificateStatus(ctx context.Context, capability string, body []byte) (sdkpresentation.AdminActionResponse, error) {
	if err := contracts.ValidateAdminRequest(capability, body); err != nil {
		return adminActionProblem(capability, "invalid_input")
	}
	var request certificateStatusRequest
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF || request.Domain == "" {
		return adminActionProblem(capability, "invalid_input")
	}
	status, err := adapter.configuration.CertificateStatus(ctx, request.Domain)
	if err != nil {
		return adminActionProblem(capability, certificateErrorCategory(err))
	}
	response, err := json.Marshal(status)
	if err != nil {
		return sdkpresentation.AdminActionResponse{}, errors.New("invalid Server certificate response")
	}
	statusCode, err := contracts.AdminActionSuccessStatus(capability)
	if err != nil {
		return sdkpresentation.AdminActionResponse{}, errors.New("invalid Server certificate contract")
	}
	return sdkpresentation.AdminActionResponse{StatusCode: statusCode, Body: response}, nil
}

func certificateErrorCategory(err error) string {
	switch {
	case errors.Is(err, models.ErrInvalidCertificateQuery):
		return "invalid_input"
	case errors.Is(err, models.ErrCertificateNotFound):
		return "not_found"
	default:
		return "unavailable"
	}
}

func (adapter *Adapter) rollback(ctx context.Context, capability string, input sdkpresentation.AdminActionInput) (sdkpresentation.AdminActionResponse, error) {
	if err := contracts.ValidateAdminRequest(capability, input.Body); err != nil {
		return adminActionProblem(capability, "invalid_input")
	}
	var request siteRollbackRequest
	decoder := json.NewDecoder(bytes.NewReader(input.Body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF || input.Invocation.IdempotencyKey == "" ||
		input.Invocation.IfMatch == "" || input.Invocation.IfMatch != request.ExpectedCurrentRevision {
		return adminActionProblem(capability, "invalid_input")
	}
	operation, err := adapter.publisher.Rollback(ctx, models.SiteRollbackInput{
		SiteID: request.SiteID, ExpectedCurrentRevision: request.ExpectedCurrentRevision,
		TargetRevision: request.TargetRevision, IdempotencyKey: input.Invocation.IdempotencyKey,
		Capability: capability,
	})
	if err != nil {
		return adminActionProblem(capability, siteErrorCategory(err))
	}
	if err := adapter.publisher.ProcessPending(ctx); err != nil {
		return adminActionProblem(capability, "unavailable")
	}
	var previous *string
	if operation.ExpectedCurrentRevision != "" {
		value := operation.ExpectedCurrentRevision
		previous = &value
	}
	response, err := json.Marshal(struct {
		SiteID           string  `json:"siteId"`
		CurrentRevision  string  `json:"currentRevision"`
		PreviousRevision *string `json:"previousRevision"`
	}{SiteID: operation.SiteID, CurrentRevision: operation.RevisionID, PreviousRevision: previous})
	if err != nil {
		return sdkpresentation.AdminActionResponse{}, errors.New("invalid Server rollback response")
	}
	status, err := contracts.AdminActionSuccessStatus(capability)
	if err != nil {
		return sdkpresentation.AdminActionResponse{}, errors.New("invalid Server rollback contract")
	}
	return sdkpresentation.AdminActionResponse{StatusCode: status, Body: response}, nil
}

func siteErrorCategory(err error) string {
	switch {
	case errors.Is(err, application.ErrInvalidSitePublish):
		return "invalid_input"
	case errors.Is(err, models.ErrInvalidCertificateQuery):
		return "invalid_input"
	case errors.Is(err, application.ErrSiteNotFound), errors.Is(err, application.ErrOperationNotFound):
		return "not_found"
	case errors.Is(err, application.ErrSiteConflict):
		return "conflict"
	default:
		return "unavailable"
	}
}

func adminActionProblem(capability, category string) (sdkpresentation.AdminActionResponse, error) {
	status, code, err := contracts.AdminProblem(capability, category)
	if err != nil {
		return sdkpresentation.AdminActionResponse{}, errors.New("invalid Server admin action contract")
	}
	body, err := json.Marshal(actionProblem{Code: code})
	if err != nil {
		return sdkpresentation.AdminActionResponse{}, errors.New("invalid Server admin action response")
	}
	return sdkpresentation.AdminActionResponse{StatusCode: status, Body: body}, nil
}

func problemResponse(status int, code string) (sdkpresentation.ArtifactResponse, error) {
	body, err := json.Marshal(actionProblem{Code: code})
	if err != nil {
		return sdkpresentation.ArtifactResponse{}, errors.New("invalid Server artifact response")
	}
	return sdkpresentation.ArtifactResponse{StatusCode: status, Body: body}, nil
}

var _ sdkpresentation.ArtifactAcceptor = (*Adapter)(nil)
var _ sdkpresentation.AdminSurfaceProvider = (*Adapter)(nil)
var _ sdkpresentation.AdminActionHandler = (*Adapter)(nil)
