package interfaces

import (
	"context"
	"io"

	"liapoldus.local/server-plugin/internal/domain/models"
)

// SiteReleaseRepository persists accepted site actions, immutable releases and
// the current/previous generation pair.
type SiteReleaseRepository interface {
	AcceptSitePublish(context.Context, models.SitePublishInput) (models.SiteOperation, error)
	ProcessPendingSitePublishes(context.Context) error
	RollbackSite(context.Context, models.SiteRollbackInput) (models.SiteOperation, error)
	SiteOperation(context.Context, string) (models.SiteOperation, error)
	SiteReleaseState(context.Context, string) (models.SiteReleaseState, error)
	ListSites(context.Context, int, string) (models.SitePage[models.SiteSummary], error)
	ListSiteReleases(context.Context, string, int, string) (models.SitePage[models.SiteReleaseSummary], error)
	ListSiteOperations(context.Context, string, int, string) (models.SitePage[models.SiteOperationSummary], error)
	OpenSiteDocument(context.Context, string, string) (io.ReadCloser, error)
}
