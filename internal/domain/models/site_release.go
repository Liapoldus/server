package models

import (
	"errors"
	"io"
)

var (
	ErrInvalidSitePublisher = errors.New("invalid site publisher")
	ErrInvalidSitePublish   = errors.New("invalid site publish request")
	ErrSiteConflict         = errors.New("site publish conflict")
	ErrSiteNotFound         = errors.New("site release not found")
	ErrOperationNotFound    = errors.New("site operation not found")
	ErrDocumentNotFound     = errors.New("site document not found")
)

// SitePublishInput is the product metadata and stream accepted by the site
// publish action. Core identity and idempotency context is supplied separately
// by the trusted Plugin SDK invocation.
type SitePublishInput struct {
	Metadata       []byte
	ContentType    string
	IdempotencyKey string
	Body           io.Reader
}

// SiteOperation is the durable, product-owned result of accepting a site action.
type SiteOperation struct {
	OperationID             string
	SiteID                  string
	Kind                    string
	State                   string
	RevisionID              string
	ExpectedCurrentRevision string
	ErrorCode               string
	UpdatedAt               string
}

// SiteReleaseState is the active pair that can be switched atomically through
// one generation pointer.
type SiteReleaseState struct {
	CurrentRevision  string
	PreviousRevision *string
}

type SiteRollbackInput struct {
	SiteID                  string
	ExpectedCurrentRevision string
	TargetRevision          string
	IdempotencyKey          string
	Capability              string
}

type SiteSummary struct {
	SiteID           string  `json:"siteId"`
	CurrentRevision  *string `json:"currentRevision"`
	PreviousRevision *string `json:"previousRevision"`
	UpdatedAt        string  `json:"updatedAt"`
}

type SiteReleaseSummary struct {
	SiteID     string `json:"siteId"`
	RevisionID string `json:"revisionId"`
	State      string `json:"state"`
	SHA256     string `json:"sha256"`
	CreatedAt  string `json:"createdAt"`
}

type SiteOperationSummary struct {
	OperationID     string  `json:"operationId"`
	SiteID          *string `json:"siteId"`
	Kind            string  `json:"kind"`
	State           string  `json:"state"`
	ProgressPercent *int    `json:"progressPercent"`
	UpdatedAt       string  `json:"updatedAt"`
}

type SitePage[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"nextCursor"`
}
