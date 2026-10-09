package site

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/caddyserver/certmagic"
	"liapoldus.local/server-plugin/contracts"
	sitemodel "liapoldus.local/server-plugin/internal/domain/models/site"
)

const releaseMutationLockName = "server-site-release-mutations"

// ReleaseStore keeps immutable site releases and their durable operation journal
// on one filesystem so a generation pointer rename is atomic.
type ReleaseStore struct {
	locks    *certmagic.FileStorage
	contract contracts.SiteOperationContract
	root     string
	mu       sync.Mutex
}

type persistedOperation struct {
	OperationID        string `json:"operationId"`
	SiteID             string `json:"siteId"`
	Kind               string `json:"kind"`
	State              string `json:"state"`
	RevisionID         string `json:"revisionId"`
	ExpectedCurrent    string `json:"expectedCurrentRevision,omitempty"`
	IdempotencyKeyHash string `json:"idempotencyKeyHash"`
	InputFingerprint   string `json:"inputFingerprint"`
	ErrorCode          string `json:"errorCode,omitempty"`
	UpdatedAt          string `json:"updatedAt"`
	HasExpectedCurrent bool   `json:"hasExpectedCurrentRevision"`
}

type persistedGeneration struct {
	GenerationID string `json:"generationId"`
	Current      string `json:"currentRevision"`
	Previous     string `json:"previousRevision,omitempty"`
}

// NewReleaseStore validates and creates the private on-disk layout. Opening an
// existing store does not activate a pending operation; callers explicitly run
// ProcessPendingSitePublishes after their product dependencies are ready.
func NewReleaseStore(root string) (*ReleaseStore, error) {
	if root == "" || !filepath.IsAbs(root) {
		return nil, sitemodel.ErrInvalidSitePublisher
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, sitemodel.ErrInvalidSitePublisher
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, sitemodel.ErrInvalidSitePublisher
	}
	for _, directory := range []string{"sites", "operations", "coordination"} {
		if err := makePrivateDirectory(filepath.Join(root, directory)); err != nil {
			return nil, sitemodel.ErrInvalidSitePublisher
		}
	}
	contract, err := contracts.LoadSiteOperationContract()
	if err != nil {
		return nil, sitemodel.ErrInvalidSitePublisher
	}
	return &ReleaseStore{
		root: root, contract: contract,
		locks: &certmagic.FileStorage{Path: filepath.Join(root, "coordination")},
	}, nil
}

func (store *ReleaseStore) AcceptSitePublish(ctx context.Context, input sitemodel.SitePublishInput) (sitemodel.SiteOperation, error) {
	if store == nil || ctx == nil || input.Body == nil {
		return sitemodel.SiteOperation{}, sitemodel.ErrInvalidSitePublish
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.acquireMutationLock(ctx); err != nil {
		store.drain(input.Body)
		return sitemodel.SiteOperation{}, sitemodel.ErrInvalidSitePublisher
	}
	operation, operationErr := store.acceptSitePublishLocked(ctx, input)
	if err := store.releaseMutationLock(); err != nil {
		return sitemodel.SiteOperation{}, sitemodel.ErrInvalidSitePublisher
	}
	return operation, operationErr
}

func (store *ReleaseStore) acceptSitePublishLocked(ctx context.Context, input sitemodel.SitePublishInput) (sitemodel.SiteOperation, error) {
	if store == nil || ctx == nil || input.Body == nil || input.ContentType != "application/gzip" || input.IdempotencyKey == "" {
		return sitemodel.SiteOperation{}, sitemodel.ErrInvalidSitePublish
	}
	metadata, err := contracts.ValidateSiteArtifactMetadata(input.Metadata)
	if err != nil || !validSiteIdentifier(metadata.Payload.SiteID) || metadata.Artifact.MediaType != input.ContentType {
		store.drain(input.Body)
		return sitemodel.SiteOperation{}, sitemodel.ErrInvalidSitePublish
	}
	limits, err := contracts.LoadSiteArchiveLimits()
	if err != nil || int64(len(input.Metadata)) > limits.MetadataBytes || metadata.Artifact.ByteLength > limits.ArtifactBytes {
		store.drain(input.Body)
		return sitemodel.SiteOperation{}, sitemodel.ErrInvalidSitePublish
	}

	if err := ctx.Err(); err != nil {
		store.drain(input.Body)
		return sitemodel.SiteOperation{}, sitemodel.ErrInvalidSitePublish
	}
	siteRoot := filepath.Join(store.root, "sites", metadata.Payload.SiteID)
	if err := ensureSiteLayout(siteRoot); err != nil {
		store.drain(input.Body)
		return sitemodel.SiteOperation{}, sitemodel.ErrInvalidSitePublisher
	}
	keyHash := sha256Hex([]byte(store.contract.Capability + "\x00" + input.IdempotencyKey))
	fingerprint := publishFingerprint(input.Metadata, metadata.Artifact.SHA256)
	if previous, exists, lookupErr := store.operationForKey(keyHash); lookupErr != nil {
		store.drain(input.Body)
		return sitemodel.SiteOperation{}, sitemodel.ErrInvalidSitePublisher
	} else if exists {
		actualLength, actualDigest, drainErr := hashBounded(input.Body, limits.ArtifactBytes)
		if drainErr != nil || actualLength != metadata.Artifact.ByteLength || "sha256:"+actualDigest != metadata.Artifact.SHA256 {
			return sitemodel.SiteOperation{}, sitemodel.ErrInvalidSitePublish
		}
		if previous.InputFingerprint != fingerprint {
			return sitemodel.SiteOperation{}, sitemodel.ErrSiteConflict
		}
		return publicOperation(previous), nil
	}

	current, stateErr := currentRevision(siteRoot)
	if stateErr != nil && !errors.Is(stateErr, os.ErrNotExist) {
		store.drain(input.Body)
		return sitemodel.SiteOperation{}, sitemodel.ErrInvalidSitePublisher
	}
	if !revisionPreconditionMatches(current, metadata.Payload.ExpectedCurrentRevision) || hasPendingOperation(filepath.Join(store.root, "operations"), metadata.Payload.SiteID, store.contract.AcceptedState, store.contract.RunningState) {
		store.drain(input.Body)
		return sitemodel.SiteOperation{}, sitemodel.ErrSiteConflict
	}

	operationID, err := randomID()
	if err != nil {
		store.drain(input.Body)
		return sitemodel.SiteOperation{}, sitemodel.ErrInvalidSitePublisher
	}
	stagingRoot := filepath.Join(siteRoot, "staging", operationID)
	if err := makePrivateDirectory(stagingRoot); err != nil {
		store.drain(input.Body)
		return sitemodel.SiteOperation{}, sitemodel.ErrInvalidSitePublisher
	}
	compressedBytes, digest, archiveErr := store.persistArtifact(ctx, input.Body, filepath.Join(stagingRoot, "artifact.gz"), limits.ArtifactBytes)
	if archiveErr != nil {
		_ = os.RemoveAll(stagingRoot)
		if errors.Is(archiveErr, sitemodel.ErrInvalidSitePublisher) {
			return sitemodel.SiteOperation{}, sitemodel.ErrInvalidSitePublisher
		}
		return sitemodel.SiteOperation{}, sitemodel.ErrInvalidSitePublish
	}
	if compressedBytes != metadata.Artifact.ByteLength || "sha256:"+digest != metadata.Artifact.SHA256 {
		_ = os.RemoveAll(stagingRoot)
		return sitemodel.SiteOperation{}, sitemodel.ErrInvalidSitePublish
	}
	if err := ctx.Err(); err != nil {
		_ = os.RemoveAll(stagingRoot)
		return sitemodel.SiteOperation{}, sitemodel.ErrInvalidSitePublish
	}
	operation := persistedOperation{
		OperationID: operationID, SiteID: metadata.Payload.SiteID,
		Kind: store.contract.Capability, State: store.contract.AcceptedState, RevisionID: metadata.Artifact.SHA256,
		IdempotencyKeyHash: keyHash, InputFingerprint: fingerprint,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if metadata.Payload.ExpectedCurrentRevision != nil {
		operation.ExpectedCurrent = *metadata.Payload.ExpectedCurrentRevision
		operation.HasExpectedCurrent = true
	}
	if err := store.writeOperation(operation); err != nil {
		_ = os.RemoveAll(stagingRoot)
		return sitemodel.SiteOperation{}, sitemodel.ErrInvalidSitePublisher
	}
	return publicOperation(operation), nil
}

func (store *ReleaseStore) ProcessPendingSitePublishes(ctx context.Context) error {
	if store == nil || ctx == nil {
		return sitemodel.ErrInvalidSitePublisher
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.acquireMutationLock(ctx); err != nil {
		return sitemodel.ErrInvalidSitePublisher
	}
	processErr := store.processPendingSitePublishesLocked(ctx)
	if err := store.releaseMutationLock(); err != nil {
		return sitemodel.ErrInvalidSitePublisher
	}
	return processErr
}

func (store *ReleaseStore) processPendingSitePublishesLocked(ctx context.Context) error {
	if err := store.removeOrphanStaging(); err != nil {
		return sitemodel.ErrInvalidSitePublisher
	}
	entries, err := os.ReadDir(filepath.Join(store.root, "operations"))
	if err != nil {
		return sitemodel.ErrInvalidSitePublisher
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return sitemodel.ErrInvalidSitePublisher
		}
		operation, err := store.readOperation(entry.Name())
		if err != nil {
			return sitemodel.ErrInvalidSitePublisher
		}
		if operation.State != store.contract.AcceptedState && operation.State != store.contract.RunningState {
			continue
		}
		if operation.State == store.contract.AcceptedState {
			operation.State = store.contract.RunningState
			operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			if err := store.writeOperation(operation); err != nil {
				return sitemodel.ErrInvalidSitePublisher
			}
		}
		if current, currentErr := currentRevision(filepath.Join(store.root, "sites", operation.SiteID)); currentErr == nil && current == operation.RevisionID {
			if err := store.activate(operation); err != nil {
				return sitemodel.ErrInvalidSitePublisher
			}
			continue
		}
		if err := store.prepareAcceptedArtifact(ctx, operation); err != nil {
			operation.State = store.contract.FailedState
			operation.ErrorCode, _ = contracts.SitePublishErrorCode("operation_failed")
			operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			if writeErr := store.writeOperation(operation); writeErr != nil {
				return sitemodel.ErrInvalidSitePublisher
			}
			_ = os.RemoveAll(filepath.Join(store.root, "sites", operation.SiteID, "staging", operation.OperationID))
			continue
		}
		if err := store.activate(operation); err != nil {
			operation.State = store.contract.FailedState
			operation.ErrorCode, _ = contracts.SitePublishErrorCode("operation_failed")
			operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			if writeErr := store.writeOperation(operation); writeErr != nil {
				return sitemodel.ErrInvalidSitePublisher
			}
			_ = os.RemoveAll(filepath.Join(store.root, "sites", operation.SiteID, "staging", operation.OperationID))
			continue
		}
	}
	return nil
}

func (store *ReleaseStore) persistArtifact(ctx context.Context, source io.Reader, destination string, maximum int64) (int64, string, error) {
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return 0, "", sitemodel.ErrInvalidSitePublisher
	}
	hasher := sha256.New()
	count, copyErr := io.Copy(io.MultiWriter(file, hasher), io.LimitReader(contextReader{ctx: ctx, reader: source}, maximum+1))
	if copyErr == nil && (count == 0 || count > maximum) {
		copyErr = sitemodel.ErrInvalidSitePublish
	}
	if copyErr == nil {
		copyErr = file.Sync()
	}
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(destination)
		return count, "", sitemodel.ErrInvalidSitePublish
	}
	if err := syncDirectory(filepath.Dir(destination)); err != nil {
		_ = os.Remove(destination)
		return count, "", sitemodel.ErrInvalidSitePublisher
	}
	return count, hex.EncodeToString(hasher.Sum(nil)), nil
}

func (store *ReleaseStore) prepareAcceptedArtifact(ctx context.Context, operation persistedOperation) error {
	if !validSiteIdentifier(operation.SiteID) || !validRevisionID(operation.RevisionID) || operation.OperationID == "" {
		return sitemodel.ErrInvalidSitePublisher
	}
	siteRoot := filepath.Join(store.root, "sites", operation.SiteID)
	releaseRoot := filepath.Join(siteRoot, "releases", strings.TrimPrefix(operation.RevisionID, "sha256:"))
	if _, err := ValidateStagedManifest(releaseRoot, operation.SiteID); err == nil {
		return nil
	}
	stagingRoot := filepath.Join(siteRoot, "staging", operation.OperationID)
	contentRoot := filepath.Join(stagingRoot, "content")
	markerPath := filepath.Join(stagingRoot, "prepared.json")
	if marker, err := os.ReadFile(markerPath); err == nil && contracts.ValidJSONNoDuplicateKeys(marker) {
		var prepared struct {
			OperationID string `json:"operationId"`
			RevisionID  string `json:"revisionId"`
		}
		if json.Unmarshal(marker, &prepared) == nil && prepared.OperationID == operation.OperationID && prepared.RevisionID == operation.RevisionID {
			if _, err := ValidateStagedManifest(contentRoot, operation.SiteID); err == nil {
				return nil
			}
		}
	}
	if err := os.RemoveAll(contentRoot); err != nil {
		return err
	}
	archiveFile, err := os.Open(filepath.Join(stagingRoot, "artifact.gz"))
	if err != nil {
		return err
	}
	if err := makePrivateDirectory(contentRoot); err != nil {
		_ = archiveFile.Close()
		return err
	}
	archiveResult, archiveErr := ExtractAndValidateArchive(contextReader{ctx: ctx, reader: archiveFile}, contentRoot, operation.SiteID)
	closeErr := archiveFile.Close()
	if archiveErr != nil || closeErr != nil || archiveResult.Digest != operation.RevisionID {
		_ = os.RemoveAll(contentRoot)
		return sitemodel.ErrInvalidSitePublish
	}
	if err := syncTree(contentRoot); err != nil || syncDirectory(stagingRoot) != nil {
		_ = os.RemoveAll(contentRoot)
		return sitemodel.ErrInvalidSitePublisher
	}
	if err := writeJSONAtomic(stagingRoot, "prepared.json", struct {
		OperationID string `json:"operationId"`
		RevisionID  string `json:"revisionId"`
	}{operation.OperationID, operation.RevisionID}); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(stagingRoot, "artifact.gz")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncDirectory(stagingRoot)
}

func (store *ReleaseStore) SiteOperation(_ context.Context, operationID string) (sitemodel.SiteOperation, error) {
	if store == nil || operationID == "" {
		return sitemodel.SiteOperation{}, sitemodel.ErrInvalidSitePublisher
	}
	operation, err := store.readOperation(operationID + ".json")
	if err != nil {
		return sitemodel.SiteOperation{}, sitemodel.ErrOperationNotFound
	}
	return publicOperation(operation), nil
}

func (store *ReleaseStore) SiteReleaseState(_ context.Context, siteID string) (sitemodel.SiteReleaseState, error) {
	if store == nil || !validSiteIdentifier(siteID) {
		return sitemodel.SiteReleaseState{}, sitemodel.ErrSiteNotFound
	}
	siteRoot := filepath.Join(store.root, "sites", siteID)
	generation, err := store.readActiveGeneration(siteRoot)
	if err != nil {
		return sitemodel.SiteReleaseState{}, sitemodel.ErrSiteNotFound
	}
	state := sitemodel.SiteReleaseState{CurrentRevision: generation.Current}
	if generation.Previous != "" {
		state.PreviousRevision = &generation.Previous
	}
	return state, nil
}

func (store *ReleaseStore) RollbackSite(ctx context.Context, input sitemodel.SiteRollbackInput) (sitemodel.SiteOperation, error) {
	if store == nil || ctx == nil || !validSiteIdentifier(input.SiteID) || !validRevisionID(input.ExpectedCurrentRevision) ||
		!validRevisionID(input.TargetRevision) || input.TargetRevision == input.ExpectedCurrentRevision ||
		input.IdempotencyKey == "" || input.Capability == "" {
		return sitemodel.SiteOperation{}, sitemodel.ErrInvalidSitePublish
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.acquireMutationLock(ctx); err != nil {
		return sitemodel.SiteOperation{}, sitemodel.ErrInvalidSitePublisher
	}
	operation, operationErr := store.rollbackSiteLocked(ctx, input)
	if err := store.releaseMutationLock(); err != nil {
		return sitemodel.SiteOperation{}, sitemodel.ErrInvalidSitePublisher
	}
	return operation, operationErr
}

func (store *ReleaseStore) rollbackSiteLocked(ctx context.Context, input sitemodel.SiteRollbackInput) (sitemodel.SiteOperation, error) {
	if err := ctx.Err(); err != nil {
		return sitemodel.SiteOperation{}, sitemodel.ErrInvalidSitePublish
	}
	siteRoot := filepath.Join(store.root, "sites", input.SiteID)
	if err := ensureSiteLayout(siteRoot); err != nil {
		return sitemodel.SiteOperation{}, sitemodel.ErrInvalidSitePublisher
	}
	keyHash := sha256Hex([]byte(input.Capability + "\x00" + input.IdempotencyKey))
	current, err := currentRevision(siteRoot)
	if err != nil || current != input.ExpectedCurrentRevision || hasPendingOperation(filepath.Join(store.root, "operations"), input.SiteID, store.contract.AcceptedState, store.contract.RunningState) {
		return sitemodel.SiteOperation{}, sitemodel.ErrSiteConflict
	}
	targetRoot := filepath.Join(siteRoot, "releases", strings.TrimPrefix(input.TargetRevision, "sha256:"))
	if _, err := ValidateStagedManifest(targetRoot, input.SiteID); err != nil {
		return sitemodel.SiteOperation{}, sitemodel.ErrSiteNotFound
	}
	operationID, err := randomID()
	if err != nil {
		return sitemodel.SiteOperation{}, sitemodel.ErrInvalidSitePublisher
	}
	operation := persistedOperation{
		OperationID: operationID, SiteID: input.SiteID, Kind: input.Capability,
		State: store.contract.AcceptedState, RevisionID: input.TargetRevision,
		ExpectedCurrent: input.ExpectedCurrentRevision, HasExpectedCurrent: true,
		IdempotencyKeyHash: keyHash,
		UpdatedAt:          time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := store.writeOperation(operation); err != nil {
		return sitemodel.SiteOperation{}, sitemodel.ErrInvalidSitePublisher
	}
	return publicOperation(operation), nil
}

func (store *ReleaseStore) acquireMutationLock(ctx context.Context) error {
	if store == nil || store.locks == nil || ctx == nil {
		return sitemodel.ErrInvalidSitePublisher
	}
	if err := store.locks.Lock(ctx, releaseMutationLockName); err != nil {
		return sitemodel.ErrInvalidSitePublisher
	}
	return nil
}

func (store *ReleaseStore) releaseMutationLock() error {
	if store == nil || store.locks == nil {
		return sitemodel.ErrInvalidSitePublisher
	}
	if err := store.locks.Unlock(context.Background(), releaseMutationLockName); err != nil {
		return sitemodel.ErrInvalidSitePublisher
	}
	return nil
}

func (store *ReleaseStore) ListSites(ctx context.Context, limit int, cursor string) (sitemodel.SitePage[sitemodel.SiteSummary], error) {
	if store == nil || ctx == nil || limit < 1 || limit > 100 {
		return sitemodel.SitePage[sitemodel.SiteSummary]{}, sitemodel.ErrInvalidSitePublisher
	}
	if err := ctx.Err(); err != nil {
		return sitemodel.SitePage[sitemodel.SiteSummary]{}, sitemodel.ErrInvalidSitePublisher
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	entries, err := os.ReadDir(filepath.Join(store.root, "sites"))
	if err != nil {
		return sitemodel.SitePage[sitemodel.SiteSummary]{}, sitemodel.ErrInvalidSitePublisher
	}
	items := make([]sitemodel.SiteSummary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !validSiteIdentifier(entry.Name()) {
			continue
		}
		generation, readErr := store.readActiveGeneration(filepath.Join(store.root, "sites", entry.Name()))
		if errors.Is(readErr, sitemodel.ErrSiteNotFound) {
			continue
		}
		if readErr != nil {
			return sitemodel.SitePage[sitemodel.SiteSummary]{}, sitemodel.ErrInvalidSitePublisher
		}
		current := generation.Current
		var previous *string
		if generation.Previous != "" {
			value := generation.Previous
			previous = &value
		}
		generationName, _ := os.Readlink(filepath.Join(store.root, "sites", entry.Name(), "active"))
		info, statErr := os.Stat(filepath.Join(store.root, "sites", entry.Name(), generationName, "generation.json"))
		if statErr != nil {
			return sitemodel.SitePage[sitemodel.SiteSummary]{}, sitemodel.ErrInvalidSitePublisher
		}
		items = append(items, sitemodel.SiteSummary{SiteID: entry.Name(), CurrentRevision: &current, PreviousRevision: previous, UpdatedAt: info.ModTime().UTC().Format(time.RFC3339Nano)})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].SiteID < items[j].SiteID })
	return paginate(items, limit, cursor, func(item sitemodel.SiteSummary) string { return item.SiteID })
}

func (store *ReleaseStore) ListSiteReleases(ctx context.Context, siteID string, limit int, cursor string) (sitemodel.SitePage[sitemodel.SiteReleaseSummary], error) {
	if store == nil || ctx == nil || !validSiteIdentifier(siteID) || limit < 1 || limit > 100 {
		return sitemodel.SitePage[sitemodel.SiteReleaseSummary]{}, sitemodel.ErrInvalidSitePublisher
	}
	if err := ctx.Err(); err != nil {
		return sitemodel.SitePage[sitemodel.SiteReleaseSummary]{}, sitemodel.ErrInvalidSitePublisher
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	siteRoot := filepath.Join(store.root, "sites", siteID)
	active, err := store.readActiveGeneration(siteRoot)
	if err != nil {
		return sitemodel.SitePage[sitemodel.SiteReleaseSummary]{}, sitemodel.ErrSiteNotFound
	}
	entries, err := os.ReadDir(filepath.Join(siteRoot, "releases"))
	if err != nil {
		return sitemodel.SitePage[sitemodel.SiteReleaseSummary]{}, sitemodel.ErrSiteNotFound
	}
	items := make([]sitemodel.SiteReleaseSummary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !validRevisionID("sha256:"+entry.Name()) {
			continue
		}
		revision := "sha256:" + entry.Name()
		if _, err := ValidateStagedManifest(filepath.Join(siteRoot, "releases", entry.Name()), siteID); err != nil {
			return sitemodel.SitePage[sitemodel.SiteReleaseSummary]{}, sitemodel.ErrInvalidSitePublisher
		}
		state := "available"
		switch revision {
		case active.Current:
			state = "current"
		case active.Previous:
			state = "previous"
		}
		info, err := entry.Info()
		if err != nil {
			return sitemodel.SitePage[sitemodel.SiteReleaseSummary]{}, sitemodel.ErrInvalidSitePublisher
		}
		items = append(items, sitemodel.SiteReleaseSummary{SiteID: siteID, RevisionID: revision, State: state, SHA256: revision, CreatedAt: info.ModTime().UTC().Format(time.RFC3339Nano)})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].RevisionID < items[j].RevisionID })
	return paginate(items, limit, cursor, func(item sitemodel.SiteReleaseSummary) string { return item.RevisionID })
}

func (store *ReleaseStore) ListSiteOperations(ctx context.Context, state string, limit int, cursor string) (sitemodel.SitePage[sitemodel.SiteOperationSummary], error) {
	if store == nil || ctx == nil || limit < 1 || limit > 100 {
		return sitemodel.SitePage[sitemodel.SiteOperationSummary]{}, sitemodel.ErrInvalidSitePublisher
	}
	if err := ctx.Err(); err != nil {
		return sitemodel.SitePage[sitemodel.SiteOperationSummary]{}, sitemodel.ErrInvalidSitePublisher
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	entries, err := os.ReadDir(filepath.Join(store.root, "operations"))
	if err != nil {
		return sitemodel.SitePage[sitemodel.SiteOperationSummary]{}, sitemodel.ErrInvalidSitePublisher
	}
	items := make([]sitemodel.SiteOperationSummary, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		operation, err := store.readOperation(entry.Name())
		if err != nil {
			return sitemodel.SitePage[sitemodel.SiteOperationSummary]{}, sitemodel.ErrInvalidSitePublisher
		}
		if state != "" && operation.State != state {
			continue
		}
		siteID := operation.SiteID
		items = append(items, sitemodel.SiteOperationSummary{OperationID: operation.OperationID, SiteID: &siteID, Kind: operation.Kind, State: operation.State, UpdatedAt: operation.UpdatedAt})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].OperationID < items[j].OperationID })
	return paginate(items, limit, cursor, func(item sitemodel.SiteOperationSummary) string { return item.OperationID })
}

func (store *ReleaseStore) OpenSiteDocument(_ context.Context, siteID, requestPath string) (io.ReadCloser, error) {
	if store == nil || !validSiteIdentifier(siteID) {
		return nil, sitemodel.ErrDocumentNotFound
	}
	siteRoot := filepath.Join(store.root, "sites", siteID)
	generation, err := store.readActiveGeneration(siteRoot)
	if err != nil || !validRevisionID(generation.Current) {
		return nil, sitemodel.ErrDocumentNotFound
	}
	releaseRoot := filepath.Join(siteRoot, "releases", strings.TrimPrefix(generation.Current, "sha256:"))
	manifest, err := ValidateStagedManifest(releaseRoot, siteID)
	if err != nil {
		return nil, sitemodel.ErrDocumentNotFound
	}
	relative, err := documentRelativePath(requestPath, manifest, store.contract.ManifestEntry)
	if err != nil {
		return nil, sitemodel.ErrDocumentNotFound
	}
	documentRoot := filepath.Join(releaseRoot, filepath.FromSlash(manifest.DocumentRoot))
	root, err := os.OpenRoot(documentRoot)
	if err != nil {
		return nil, sitemodel.ErrDocumentNotFound
	}
	file, err := root.Open(filepath.FromSlash(relative))
	if err == nil {
		info, statErr := file.Stat()
		if statErr != nil {
			_ = file.Close()
			_ = root.Close()
			return nil, sitemodel.ErrDocumentNotFound
		}
		if info.IsDir() {
			_ = file.Close()
			file, err = root.Open(filepath.Join(filepath.FromSlash(relative), filepath.FromSlash(manifest.IndexDocument)))
		}
	}
	_ = root.Close()
	if err != nil {
		return nil, sitemodel.ErrDocumentNotFound
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, sitemodel.ErrDocumentNotFound
	}
	return file, nil
}

func (store *ReleaseStore) activate(operation persistedOperation) error {
	siteRoot := filepath.Join(store.root, "sites", operation.SiteID)
	if err := makePrivateDirectory(filepath.Join(siteRoot, "releases")); err != nil {
		return err
	}
	if err := makePrivateDirectory(filepath.Join(siteRoot, "generations")); err != nil {
		return err
	}
	if !validSiteIdentifier(operation.SiteID) || !validRevisionID(operation.RevisionID) || operation.OperationID == "" {
		return sitemodel.ErrInvalidSitePublisher
	}
	revisionName := strings.TrimPrefix(operation.RevisionID, "sha256:")
	releasePath := filepath.Join(siteRoot, "releases", revisionName)
	stagingPath := filepath.Join(siteRoot, "staging", operation.OperationID)
	contentPath := filepath.Join(stagingPath, "content")
	current, err := currentRevision(siteRoot)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if current == operation.RevisionID {
		operation.State = store.contract.CompletedState
		operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		return store.writeOperation(operation)
	}
	if operation.HasExpectedCurrent {
		if current != operation.ExpectedCurrent {
			return sitemodel.ErrSiteConflict
		}
	} else if current != "" {
		return sitemodel.ErrSiteConflict
	}
	if _, err := os.Lstat(releasePath); errors.Is(err, os.ErrNotExist) {
		if _, stageErr := os.Lstat(contentPath); stageErr != nil {
			return stageErr
		}
		if err := os.Rename(contentPath, releasePath); err != nil {
			return err
		}
		if err := os.RemoveAll(stagingPath); err != nil {
			return err
		}
		if err := syncDirectory(filepath.Join(siteRoot, "releases")); err != nil {
			return err
		}
		if err := syncDirectory(filepath.Join(siteRoot, "staging")); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if err := os.RemoveAll(stagingPath); err != nil {
		return err
	}
	manifest, err := ValidateStagedManifest(releasePath, operation.SiteID)
	if err != nil {
		return err
	}
	previous := current
	generation := persistedGeneration{GenerationID: operation.OperationID, Current: operation.RevisionID, Previous: previous}
	generationPath := filepath.Join(siteRoot, "generations", operation.OperationID)
	if _, err := os.Lstat(generationPath); err == nil {
		if err := os.RemoveAll(generationPath); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := makePrivateDirectory(generationPath); err != nil {
		return err
	}
	currentTarget := filepath.Join("..", "..", "releases", revisionName, filepath.FromSlash(manifest.DocumentRoot))
	if err := os.Symlink(currentTarget, filepath.Join(generationPath, "current")); err != nil {
		return err
	}
	if previous != "" {
		previousRelease := filepath.Join(siteRoot, "releases", strings.TrimPrefix(previous, "sha256:"))
		previousManifest, manifestErr := ValidateStagedManifest(previousRelease, operation.SiteID)
		if manifestErr != nil {
			return manifestErr
		}
		previousTarget := filepath.Join("..", "..", "releases", strings.TrimPrefix(previous, "sha256:"), filepath.FromSlash(previousManifest.DocumentRoot))
		if err := os.Symlink(previousTarget, filepath.Join(generationPath, "previous")); err != nil {
			return err
		}
	}
	if err := writeJSONAtomic(generationPath, "generation.json", generation); err != nil {
		return err
	}
	if err := syncDirectory(filepath.Join(siteRoot, "generations")); err != nil {
		return err
	}
	temporaryLink := filepath.Join(siteRoot, "active-next-"+operation.OperationID)
	_ = os.Remove(temporaryLink)
	if err := os.Symlink(filepath.Join("generations", operation.OperationID), temporaryLink); err != nil {
		return err
	}
	if err := os.Rename(temporaryLink, filepath.Join(siteRoot, "active")); err != nil {
		_ = os.Remove(temporaryLink)
		return err
	}
	if err := syncDirectory(siteRoot); err != nil {
		return err
	}
	operation.State = store.contract.CompletedState
	operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return store.writeOperation(operation)
}

// removeOrphanStaging deletes only staging directories that have no durable
// accepted/running operation. A journal entry is written after the extracted
// tree is fsynced, so an unreferenced directory can never be the sole copy of
// an accepted release.
func (store *ReleaseStore) removeOrphanStaging() error {
	operationsRoot := filepath.Join(store.root, "operations")
	operations, err := os.ReadDir(operationsRoot)
	if err != nil {
		return err
	}
	pending := make(map[string]struct{})
	for _, entry := range operations {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		operation, err := store.readOperation(entry.Name())
		if err != nil {
			return err
		}
		if operation.State == store.contract.AcceptedState || operation.State == store.contract.RunningState {
			pending[operation.SiteID+"/"+operation.OperationID] = struct{}{}
		}
	}
	sitesRoot := filepath.Join(store.root, "sites")
	sites, err := os.ReadDir(sitesRoot)
	if err != nil {
		return err
	}
	for _, siteEntry := range sites {
		if !siteEntry.IsDir() || !validSiteIdentifier(siteEntry.Name()) {
			continue
		}
		stagingRoot := filepath.Join(sitesRoot, siteEntry.Name(), "staging")
		stagingInfo, err := os.Lstat(stagingRoot)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !stagingInfo.IsDir() || stagingInfo.Mode()&os.ModeSymlink != 0 {
			return sitemodel.ErrInvalidSitePublisher
		}
		stagingEntries, err := os.ReadDir(stagingRoot)
		if err != nil {
			return err
		}
		removed := false
		for _, stagingEntry := range stagingEntries {
			if _, exists := pending[siteEntry.Name()+"/"+stagingEntry.Name()]; exists {
				continue
			}
			if err := os.RemoveAll(filepath.Join(stagingRoot, stagingEntry.Name())); err != nil {
				return err
			}
			removed = true
		}
		if removed {
			if err := syncDirectory(stagingRoot); err != nil {
				return err
			}
		}
	}
	return nil
}

func (store *ReleaseStore) writeOperation(operation persistedOperation) error {
	return writeJSONAtomic(filepath.Join(store.root, "operations"), operation.OperationID+".json", operation)
}

func (store *ReleaseStore) readOperation(name string) (persistedOperation, error) {
	if filepath.Base(name) != name {
		return persistedOperation{}, sitemodel.ErrOperationNotFound
	}
	contents, err := os.ReadFile(filepath.Join(store.root, "operations", name))
	if err != nil || !contracts.ValidJSONNoDuplicateKeys(contents) {
		return persistedOperation{}, sitemodel.ErrOperationNotFound
	}
	var operation persistedOperation
	if err := json.Unmarshal(contents, &operation); err != nil || operation.OperationID == "" {
		return persistedOperation{}, sitemodel.ErrOperationNotFound
	}
	return operation, nil
}

func (store *ReleaseStore) operationForKey(keyHash string) (persistedOperation, bool, error) {
	entries, err := os.ReadDir(filepath.Join(store.root, "operations"))
	if err != nil {
		return persistedOperation{}, false, err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		operation, err := store.readOperation(entry.Name())
		if err != nil {
			return persistedOperation{}, false, err
		}
		if operation.IdempotencyKeyHash == keyHash {
			return operation, true, nil
		}
	}
	return persistedOperation{}, false, nil
}

func (store *ReleaseStore) drain(source io.Reader) {
	limits, err := contracts.LoadSiteArchiveLimits()
	if err != nil || source == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(source, limits.ArtifactBytes+1))
}

func paginate[T any](items []T, limit int, cursor string, key func(T) string) (sitemodel.SitePage[T], error) {
	start := 0
	if cursor != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || len(decoded) == 0 || len(decoded) > 768 {
			return sitemodel.SitePage[T]{}, sitemodel.ErrInvalidSitePublish
		}
		cursorValue := string(decoded)
		for start < len(items) && key(items[start]) <= cursorValue {
			start++
		}
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	page := sitemodel.SitePage[T]{Items: append([]T(nil), items[start:end]...)}
	if end < len(items) && end > start {
		next := base64.RawURLEncoding.EncodeToString([]byte(key(items[end-1])))
		page.NextCursor = &next
	}
	return page, nil
}

func ensureSiteLayout(root string) error {
	for _, child := range []string{"staging", "releases", "generations"} {
		if err := makePrivateDirectory(filepath.Join(root, child)); err != nil {
			return err
		}
	}
	return nil
}

func makePrivateDirectory(directory string) error {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return sitemodel.ErrInvalidSitePublisher
	}
	return os.Chmod(directory, 0o700)
}

func currentRevision(siteRoot string) (string, error) {
	active := filepath.Join(siteRoot, "active")
	generationName, err := os.Readlink(active)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(generationName) || filepath.Clean(generationName) != generationName || strings.Contains(generationName, "..") {
		return "", sitemodel.ErrInvalidSitePublisher
	}
	contents, err := os.ReadFile(filepath.Join(siteRoot, generationName, "generation.json"))
	if err != nil || !contracts.ValidJSONNoDuplicateKeys(contents) {
		return "", sitemodel.ErrInvalidSitePublisher
	}
	var generation persistedGeneration
	if err := json.Unmarshal(contents, &generation); err != nil || generation.GenerationID != filepath.Base(generationName) || !validRevisionID(generation.Current) || generation.Previous != "" && !validRevisionID(generation.Previous) {
		return "", sitemodel.ErrInvalidSitePublisher
	}
	return generation.Current, nil
}

func (store *ReleaseStore) readActiveGeneration(siteRoot string) (persistedGeneration, error) {
	generationName, err := os.Readlink(filepath.Join(siteRoot, "active"))
	if err != nil || filepath.IsAbs(generationName) || filepath.Clean(generationName) != generationName || strings.Contains(generationName, "..") {
		return persistedGeneration{}, sitemodel.ErrSiteNotFound
	}
	contents, err := os.ReadFile(filepath.Join(siteRoot, generationName, "generation.json"))
	if err != nil || !contracts.ValidJSONNoDuplicateKeys(contents) {
		return persistedGeneration{}, sitemodel.ErrSiteNotFound
	}
	var generation persistedGeneration
	if err := json.Unmarshal(contents, &generation); err != nil || generation.GenerationID != filepath.Base(generationName) || !validRevisionID(generation.Current) || generation.Previous != "" && !validRevisionID(generation.Previous) {
		return persistedGeneration{}, sitemodel.ErrSiteNotFound
	}
	return generation, nil
}

func hasPendingOperation(operationsRoot, siteID, acceptedState, runningState string) bool {
	entries, err := os.ReadDir(operationsRoot)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		contents, err := os.ReadFile(filepath.Join(operationsRoot, entry.Name()))
		if err != nil {
			continue
		}
		var operation persistedOperation
		if contracts.ValidJSONNoDuplicateKeys(contents) && json.Unmarshal(contents, &operation) == nil && operation.SiteID == siteID && (operation.State == acceptedState || operation.State == runningState) {
			return true
		}
	}
	return false
}

func revisionPreconditionMatches(current string, expected *string) bool {
	if current == "" {
		return expected == nil
	}
	return expected != nil && *expected == current
}

func validSiteIdentifier(value string) bool {
	if value == "" || len(value) > 128 || !utf8.ValidString(value) || value == "." || value == ".." ||
		strings.ContainsAny(value, `/\\`) {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func validRevisionID(value string) bool {
	if len(value) != len("sha256:")+sha256.Size*2 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil && len(decoded) == sha256.Size
}

func sha256Hex(contents []byte) string {
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:])
}

func publishFingerprint(metadata []byte, digest string) string {
	hasher := sha256.New()
	_, _ = hasher.Write(metadata)
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(digest))
	return hex.EncodeToString(hasher.Sum(nil))
}

func hashBounded(source io.Reader, maximum int64) (int64, string, error) {
	hasher := sha256.New()
	count, err := io.Copy(hasher, io.LimitReader(source, maximum+1))
	if err != nil || count == 0 || count > maximum {
		return count, "", sitemodel.ErrInvalidSitePublish
	}
	return count, hex.EncodeToString(hasher.Sum(nil)), nil
}

func randomID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader contextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}

func publicOperation(operation persistedOperation) sitemodel.SiteOperation {
	return sitemodel.SiteOperation{OperationID: operation.OperationID, SiteID: operation.SiteID,
		Kind: operation.Kind, State: operation.State, RevisionID: operation.RevisionID,
		ExpectedCurrentRevision: operation.ExpectedCurrent,
		ErrorCode:               operation.ErrorCode, UpdatedAt: operation.UpdatedAt}
}

func documentRelativePath(requestPath string, manifest Manifest, manifestName string) (string, error) {
	if !strings.HasPrefix(requestPath, "/") || strings.ContainsAny(requestPath, `\\`) || strings.ContainsRune(requestPath, 0) {
		return "", sitemodel.ErrDocumentNotFound
	}
	for _, character := range requestPath {
		if character < 0x20 || character == 0x7f {
			return "", sitemodel.ErrDocumentNotFound
		}
	}
	trimmed := strings.TrimPrefix(requestPath, "/")
	if manifest.DocumentRoot == "." && trimmed == manifestName {
		return "", sitemodel.ErrDocumentNotFound
	}
	if trimmed == "" {
		return manifest.IndexDocument, nil
	}
	clean := path.Clean(trimmed)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != trimmed {
		return "", sitemodel.ErrDocumentNotFound
	}
	if strings.HasSuffix(requestPath, "/") {
		clean = path.Join(clean, manifest.IndexDocument)
	}
	return clean, nil
}

func writeJSONAtomic(directory, name string, value any) error {
	contents, err := json.Marshal(value)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".write-")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer func() { _ = os.Remove(temporaryName) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, filepath.Join(directory, name)); err != nil {
		return err
	}
	return syncDirectory(directory)
}

func syncDirectory(directory string) error {
	file, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

func syncTree(root string) error {
	directories := make([]string, 0)
	err := filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return sitemodel.ErrInvalidSitePublisher
		}
		if entry.IsDir() {
			directories = append(directories, current)
			return nil
		}
		if !info.Mode().IsRegular() {
			return sitemodel.ErrInvalidSitePublisher
		}
		file, err := os.Open(current)
		if err != nil {
			return err
		}
		syncErr := file.Sync()
		closeErr := file.Close()
		if syncErr != nil {
			return syncErr
		}
		return closeErr
	})
	if err != nil {
		return err
	}
	for index := len(directories) - 1; index >= 0; index-- {
		if err := syncDirectory(directories[index]); err != nil {
			return err
		}
	}
	return nil
}
