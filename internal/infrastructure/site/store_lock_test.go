package site

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofrs/flock"
	sitemodel "liapoldus.local/server-service/internal/domain/models/site"
)

func TestMutationLockCoordinatesStoreInstancesAndHonorsCancellation(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "coordination"), 0o700); err != nil {
		t.Fatal(err)
	}

	lockPath := filepath.Join(root, "coordination", "release-mutations.lock")
	first := &ReleaseStore{mutationLock: flock.New(lockPath)}
	second := &ReleaseStore{mutationLock: flock.New(lockPath)}

	if err := first.acquireMutationLock(context.Background()); err != nil {
		t.Fatalf("first store acquire: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	started := time.Now()
	if err := second.acquireMutationLock(ctx); !errors.Is(err, sitemodel.ErrInvalidSitePublisher) {
		t.Fatalf("second store should fail when its context expires, got %v", err)
	}
	if time.Since(started) < 40*time.Millisecond {
		t.Fatal("second store returned before waiting for lock contention")
	}

	if err := first.releaseMutationLock(); err != nil {
		t.Fatalf("first store release: %v", err)
	}
	if err := second.acquireMutationLock(context.Background()); err != nil {
		t.Fatalf("second store acquire after release: %v", err)
	}
	if err := second.releaseMutationLock(); err != nil {
		t.Fatalf("second store release: %v", err)
	}
}

func TestMutationLockRapidReacquisitionKeepsStableLockFile(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "coordination"), 0o700); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(root, "coordination", "release-mutations.lock")
	store := &ReleaseStore{mutationLock: flock.New(lockPath)}

	for range 32 {
		if err := store.acquireMutationLock(context.Background()); err != nil {
			t.Fatalf("acquire: %v", err)
		}
		if err := store.releaseMutationLock(); err != nil {
			t.Fatalf("release: %v", err)
		}
	}
	info, err := os.Stat(lockPath)
	if err != nil {
		t.Fatalf("stable lock file missing: %v", err)
	}
	if info.IsDir() {
		t.Fatal("lock path is a directory")
	}
}
