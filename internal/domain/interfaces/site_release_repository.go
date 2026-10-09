package interfaces

import (
	"context"
	"io"

	sitemodel "liapoldus.local/server-plugin/internal/domain/models/site"
)

// SiteReleaseRepository persists accepted site actions, immutable releases and
// the current/previous generation pair.
type SiteReleaseRepository interface {
	AcceptSitePublish(context.Context, sitemodel.SitePublishInput) (sitemodel.SiteOperation, error)
	ProcessPendingSitePublishes(context.Context) error
	RollbackSite(context.Context, sitemodel.SiteRollbackInput) (sitemodel.SiteOperation, error)
	SiteOperation(context.Context, string) (sitemodel.SiteOperation, error)
	SiteReleaseState(context.Context, string) (sitemodel.SiteReleaseState, error)
	ListSites(context.Context, int, string) (sitemodel.SitePage[sitemodel.SiteSummary], error)
	ListSiteReleases(context.Context, string, int, string) (sitemodel.SitePage[sitemodel.SiteReleaseSummary], error)
	ListSiteOperations(context.Context, string, int, string) (sitemodel.SitePage[sitemodel.SiteOperationSummary], error)
	OpenSiteDocument(context.Context, string, string) (io.ReadCloser, error)
}
