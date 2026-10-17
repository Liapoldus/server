package site

import (
	"context"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sitemodel "liapoldus.local/server-service/internal/domain/models/site"
)

type workerRepository struct {
	processCalls atomic.Int32
}

func (repository *workerRepository) AcceptSitePublish(context.Context, sitemodel.SitePublishInput) (sitemodel.SiteOperation, error) {
	return sitemodel.SiteOperation{OperationID: "op-1"}, nil
}

func (repository *workerRepository) ProcessPendingSitePublishes(context.Context) error {
	repository.processCalls.Add(1)
	return nil
}

func (*workerRepository) RollbackSite(context.Context, sitemodel.SiteRollbackInput) (sitemodel.SiteOperation, error) {
	panic("unexpected RollbackSite call")
}

func (*workerRepository) SiteOperation(context.Context, string) (sitemodel.SiteOperation, error) {
	panic("unexpected SiteOperation call")
}

func (*workerRepository) SiteReleaseState(context.Context, string) (sitemodel.SiteReleaseState, error) {
	panic("unexpected SiteReleaseState call")
}

func (*workerRepository) ListSites(context.Context, int, string) (sitemodel.SitePage[sitemodel.SiteSummary], error) {
	panic("unexpected ListSites call")
}

func (*workerRepository) ListSiteReleases(context.Context, string, int, string) (sitemodel.SitePage[sitemodel.SiteReleaseSummary], error) {
	panic("unexpected ListSiteReleases call")
}

func (*workerRepository) ListSiteOperations(context.Context, string, int, string) (sitemodel.SitePage[sitemodel.SiteOperationSummary], error) {
	panic("unexpected ListSiteOperations call")
}

func (*workerRepository) OpenSiteDocument(context.Context, string, string) (io.ReadCloser, error) {
	panic("unexpected OpenSiteDocument call")
}

func TestRunWorkerProcessesOnlyAfterAcceptedPublish(t *testing.T) {
	repository := &workerRepository{}
	publisher, err := NewSitePublisher(repository)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	workerDone := make(chan error, 1)
	go func() { workerDone <- publisher.RunWorker(ctx) }()

	time.Sleep(25 * time.Millisecond)
	if calls := repository.processCalls.Load(); calls != 0 {
		t.Fatalf("worker polled durable storage without a pending publish: %d calls", calls)
	}

	_, err = publisher.Accept(ctx, SitePublishInput{
		Body: io.NopCloser(strings.NewReader("payload")), ContentType: "application/gzip", IdempotencyKey: "key", Metadata: []byte("{}"),
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.After(time.Second)
	for repository.processCalls.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("worker did not process accepted publish")
		case <-time.After(time.Millisecond):
		}
	}

	cancel()
	select {
	case err := <-workerDone:
		if err != nil {
			t.Fatalf("worker returned error after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not stop after cancellation")
	}
}
