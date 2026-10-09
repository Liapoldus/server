package site

import (
	"context"
	"errors"
	"io"
	"time"

	"liapoldus.local/server-plugin/contracts"
	"liapoldus.local/server-plugin/internal/domain/interfaces"
	sitemodel "liapoldus.local/server-plugin/internal/domain/models/site"
)

var (
	ErrInvalidSitePublisher = sitemodel.ErrInvalidSitePublisher
	ErrInvalidSitePublish   = sitemodel.ErrInvalidSitePublish
	ErrSiteConflict         = sitemodel.ErrSiteConflict
	ErrSiteNotFound         = sitemodel.ErrSiteNotFound
	ErrOperationNotFound    = sitemodel.ErrOperationNotFound
	ErrDocumentNotFound     = sitemodel.ErrDocumentNotFound
)

// SitePublishInput is the application-facing artifact action input.
type SitePublishInput = sitemodel.SitePublishInput
type SiteOperation = sitemodel.SiteOperation
type SiteReleaseState = sitemodel.SiteReleaseState
type SiteSummary = sitemodel.SiteSummary
type SiteReleaseSummary = sitemodel.SiteReleaseSummary
type SiteOperationSummary = sitemodel.SiteOperationSummary
type SitePage[T any] = sitemodel.SitePage[T]

// SitePublisher owns the use-case boundary; archive parsing and durable state
// remain behind the domain repository port.
type SitePublisher struct {
	repository interfaces.SiteReleaseRepository
}

func NewSitePublisher(repository interfaces.SiteReleaseRepository) (*SitePublisher, error) {
	if repository == nil {
		return nil, ErrInvalidSitePublisher
	}
	return &SitePublisher{repository: repository}, nil
}

func (publisher *SitePublisher) Accept(ctx context.Context, input SitePublishInput) (SiteOperation, error) {
	if publisher == nil || publisher.repository == nil || ctx == nil || input.Body == nil ||
		input.ContentType == "" || input.IdempotencyKey == "" || len(input.Metadata) == 0 {
		return SiteOperation{}, ErrInvalidSitePublish
	}
	return publisher.repository.AcceptSitePublish(ctx, input)
}

func (publisher *SitePublisher) ProcessPending(ctx context.Context) error {
	if publisher == nil || publisher.repository == nil || ctx == nil {
		return ErrInvalidSitePublisher
	}
	return publisher.repository.ProcessPendingSitePublishes(ctx)
}

// RunWorker processes durable accepted site operations outside the artifact
// request. An operation is visible as accepted before archive activation runs;
// fatal storage/journal errors are returned to the composition root.
func (publisher *SitePublisher) RunWorker(ctx context.Context, interval time.Duration) error {
	if publisher == nil || publisher.repository == nil || ctx == nil || interval <= 0 {
		return ErrInvalidSitePublisher
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := publisher.ProcessPending(ctx); err != nil {
				return err
			}
		}
	}
}

func (publisher *SitePublisher) Rollback(ctx context.Context, input sitemodel.SiteRollbackInput) (SiteOperation, error) {
	if publisher == nil || publisher.repository == nil || ctx == nil || input.SiteID == "" ||
		input.ExpectedCurrentRevision == "" || input.TargetRevision == "" || input.IdempotencyKey == "" || input.Capability == "" {
		return SiteOperation{}, ErrInvalidSitePublish
	}
	return publisher.repository.RollbackSite(ctx, input)
}

func (publisher *SitePublisher) Operation(ctx context.Context, operationID string) (SiteOperation, error) {
	if publisher == nil || publisher.repository == nil || ctx == nil || operationID == "" {
		return SiteOperation{}, ErrInvalidSitePublisher
	}
	return publisher.repository.SiteOperation(ctx, operationID)
}

func (publisher *SitePublisher) State(ctx context.Context, siteID string) (SiteReleaseState, error) {
	if publisher == nil || publisher.repository == nil || ctx == nil || siteID == "" {
		return SiteReleaseState{}, ErrInvalidSitePublisher
	}
	return publisher.repository.SiteReleaseState(ctx, siteID)
}

func (publisher *SitePublisher) ListSites(ctx context.Context, limit int, cursor string) (SitePage[SiteSummary], error) {
	if publisher == nil || publisher.repository == nil || ctx == nil || limit < 1 || limit > 100 {
		return SitePage[SiteSummary]{}, ErrInvalidSitePublisher
	}
	return publisher.repository.ListSites(ctx, limit, cursor)
}

func (publisher *SitePublisher) ListReleases(ctx context.Context, siteID string, limit int, cursor string) (SitePage[SiteReleaseSummary], error) {
	if publisher == nil || publisher.repository == nil || ctx == nil || siteID == "" || limit < 1 || limit > 100 {
		return SitePage[SiteReleaseSummary]{}, ErrInvalidSitePublisher
	}
	return publisher.repository.ListSiteReleases(ctx, siteID, limit, cursor)
}

func (publisher *SitePublisher) ListOperations(ctx context.Context, state string, limit int, cursor string) (SitePage[SiteOperationSummary], error) {
	if publisher == nil || publisher.repository == nil || ctx == nil || limit < 1 || limit > 100 {
		return SitePage[SiteOperationSummary]{}, ErrInvalidSitePublisher
	}
	return publisher.repository.ListSiteOperations(ctx, state, limit, cursor)
}

func (publisher *SitePublisher) OpenSiteDocument(ctx context.Context, siteID, requestPath string) (io.ReadCloser, error) {
	if publisher == nil || publisher.repository == nil || ctx == nil || siteID == "" || requestPath == "" {
		return nil, ErrInvalidSitePublisher
	}
	return publisher.repository.OpenSiteDocument(ctx, siteID, requestPath)
}

// ErrorCode returns the stable contract error code without exposing internal
// messages or storage details.
func ErrorCode(err error) string {
	_, code := ErrorStatusAndCode(err)
	return code
}

// ErrorStatusAndCode maps a product failure to the status and code owned by the
// Admin Action contract. It never returns internal storage or parser details.
func ErrorStatusAndCode(err error) (int, string) {
	category := "unavailable"
	switch {
	case errors.Is(err, ErrInvalidSitePublish):
		category = "invalid_input"
	case errors.Is(err, ErrSiteConflict):
		category = "conflict"
	case errors.Is(err, ErrSiteNotFound), errors.Is(err, ErrOperationNotFound), errors.Is(err, ErrDocumentNotFound):
		category = "not_found"
	}
	status, code, lookupErr := contracts.SitePublishProblem(category)
	if lookupErr != nil {
		return 0, ""
	}
	return status, code
}
