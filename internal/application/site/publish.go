package site

import (
	"context"
	"errors"
	"io"

	"liapoldus.local/server-service/contracts"
	"liapoldus.local/server-service/internal/domain/interfaces"
	sitemodel "liapoldus.local/server-service/internal/domain/models/site"
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
	pending    chan struct{}
}

func NewSitePublisher(repository interfaces.SiteReleaseRepository) (*SitePublisher, error) {
	if repository == nil {
		return nil, ErrInvalidSitePublisher
	}
	return &SitePublisher{repository: repository, pending: make(chan struct{}, 1)}, nil
}

func (publisher *SitePublisher) Accept(ctx context.Context, input SitePublishInput) (SiteOperation, error) {
	if publisher == nil || publisher.repository == nil || ctx == nil || input.Body == nil ||
		input.ContentType == "" || input.IdempotencyKey == "" || len(input.Metadata) == 0 {
		return SiteOperation{}, ErrInvalidSitePublish
	}
	operation, err := publisher.repository.AcceptSitePublish(ctx, input)
	if err == nil {
		publisher.signalPending()
	}
	return operation, err
}

func (publisher *SitePublisher) signalPending() {
	select {
	case publisher.pending <- struct{}{}:
	default:
	}
}

func (publisher *SitePublisher) ProcessPending(ctx context.Context) error {
	if publisher == nil || publisher.repository == nil || ctx == nil {
		return ErrInvalidSitePublisher
	}
	return publisher.repository.ProcessPendingSitePublishes(ctx)
}

// RunWorker processes durable accepted site operations outside the artifact
// request. Accepted operations coalesce into one wake-up because each pass
// scans the complete durable journal. Startup recovery is run by the caller
// before the worker starts; fatal storage/journal errors return to composition.
func (publisher *SitePublisher) RunWorker(ctx context.Context) error {
	if publisher == nil || publisher.repository == nil || publisher.pending == nil || ctx == nil {
		return ErrInvalidSitePublisher
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-publisher.pending:
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

// ValidatePinnedRelease rejects a static route unless its revision is an
// immutable release owned by the referenced site. It never follows the
// mutable current pointer, so a later publication cannot change the route.
func (publisher *SitePublisher) ValidatePinnedRelease(ctx context.Context, siteID, revisionID string) error {
	if publisher == nil || publisher.repository == nil || ctx == nil || siteID == "" || revisionID == "" {
		return ErrInvalidSitePublish
	}
	cursor := ""
	seenCursors := map[string]bool{}
	for pageNumber := 0; pageNumber < 100; pageNumber++ {
		page, err := publisher.repository.ListSiteReleases(ctx, siteID, 100, cursor)
		if err != nil {
			return ErrInvalidSitePublish
		}
		for _, release := range page.Items {
			if release.SiteID == siteID && release.RevisionID == revisionID {
				return nil
			}
		}
		if page.NextCursor == nil {
			return ErrInvalidSitePublish
		}
		cursor = *page.NextCursor
		if cursor == "" || seenCursors[cursor] {
			return ErrInvalidSitePublish
		}
		seenCursors[cursor] = true
	}
	return ErrInvalidSitePublish
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
	state, err := publisher.repository.SiteReleaseState(ctx, siteID)
	if err != nil {
		return nil, err
	}
	return publisher.repository.OpenSiteDocument(ctx, siteID, state.CurrentRevision, requestPath)
}

// OpenPinnedSiteDocument reads only the immutable revision named by the
// effective route configuration. It never follows the mutable current pointer.
func (publisher *SitePublisher) OpenPinnedSiteDocument(ctx context.Context, siteID, releaseID, requestPath string) (io.ReadCloser, error) {
	if publisher == nil || publisher.repository == nil || ctx == nil || siteID == "" || releaseID == "" || requestPath == "" {
		return nil, ErrInvalidSitePublisher
	}
	return publisher.repository.OpenSiteDocument(ctx, siteID, releaseID, requestPath)
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
