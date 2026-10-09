package site

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
	"liapoldus.local/server-plugin/contracts"
)

var ErrInvalidArchive = errors.New("invalid site archive")

type ArchiveResult struct {
	Manifest        Manifest `json:"manifest"`
	Digest          string   `json:"digest"`
	CompressedBytes int64    `json:"compressedBytes"`
	ExpandedBytes   int64    `json:"expandedBytes"`
}

// ExtractAndValidateArchive extracts one gzip-compressed tar stream into an
// existing empty staging directory. It never changes a site's active release.
func ExtractAndValidateArchive(source io.Reader, stagingRoot, expectedSiteID string) (_ ArchiveResult, returnedErr error) {
	phase := "initialization"
	defer func() {
		if returnedErr != nil {
			returnedErr = fmt.Errorf("%s: %w", phase, returnedErr)
		}
	}()
	limits, err := contracts.LoadSiteArchiveLimits()
	if err != nil || source == nil || stagingRoot == "" || expectedSiteID == "" {
		return ArchiveResult{}, ErrInvalidArchive
	}
	rootInfo, err := os.Lstat(stagingRoot)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return ArchiveResult{}, ErrInvalidArchive
	}
	stage, err := os.OpenRoot(stagingRoot)
	if err != nil {
		return ArchiveResult{}, ErrInvalidArchive
	}
	defer stage.Close()
	stageDirectory, err := stage.Open(".")
	if err != nil {
		return ArchiveResult{}, ErrInvalidArchive
	}
	children, err := stageDirectory.ReadDir(-1)
	_ = stageDirectory.Close()
	if err != nil || len(children) != 0 {
		return ArchiveResult{}, ErrInvalidArchive
	}

	createdRoots := make(map[string]struct{})
	defer func() {
		if returnedErr == nil {
			return
		}
		for name := range createdRoots {
			_ = stage.RemoveAll(name)
		}
	}()

	hasher := sha256.New()
	compressed := &countingReader{reader: io.LimitReader(source, limits.ArtifactBytes+1)}
	gz, err := gzip.NewReader(io.TeeReader(compressed, hasher))
	if err != nil {
		return ArchiveResult{}, ErrInvalidArchive
	}
	// Go's default multistream behavior is intentional: all members are drained
	// and verified; concatenated members are part of this contract.
	expanded := &expandedLimitReader{reader: gz, limit: limits.ExpandedBytes}
	archive := tar.NewReader(expanded)
	paths := newArchivePaths()

	for {
		phase = "tar entries"
		before := expanded.count
		header, nextErr := archive.Next()
		if nextErr == io.EOF {
			if expanded.count-before < 1024 {
				return ArchiveResult{}, fmt.Errorf("terminator delta=%d", expanded.count-before)
			}
			break
		}
		if nextErr != nil || header == nil {
			return ArchiveResult{}, ErrInvalidArchive
		}
		if int64(paths.entries+1) > limits.MaxEntries {
			return ArchiveResult{}, ErrInvalidArchive
		}
		kind := header.Typeflag
		isDirectory := kind == tar.TypeDir
		if kind != tar.TypeReg && kind != tar.TypeRegA && !isDirectory {
			return ArchiveResult{}, ErrInvalidArchive
		}
		if header.Size < 0 || isDirectory && header.Size != 0 {
			return ArchiveResult{}, ErrInvalidArchive
		}
		for key := range header.PAXRecords {
			if strings.HasPrefix(key, "GNU.sparse.") {
				return ArchiveResult{}, ErrInvalidArchive
			}
		}
		relative, err := cleanArchivePath(header.Name, isDirectory)
		if err != nil {
			return ArchiveResult{}, ErrInvalidArchive
		}
		if relative == "." {
			if !isDirectory {
				return ArchiveResult{}, ErrInvalidArchive
			}
			paths.entries++
			continue
		}
		if err := paths.register(relative, isDirectory); err != nil {
			return ArchiveResult{}, ErrInvalidArchive
		}
		paths.entries++
		createdRoots[strings.Split(relative, "/")[0]] = struct{}{}
		if isDirectory {
			if err := makeArchiveDirectory(stage, relative, createdRoots); err != nil {
				return ArchiveResult{}, ErrInvalidArchive
			}
			continue
		}
		parentDirectory := filepath.ToSlash(filepath.Dir(filepath.FromSlash(relative)))
		if err := makeArchiveDirectory(stage, parentDirectory, createdRoots); err != nil {
			return ArchiveResult{}, ErrInvalidArchive
		}
		file, err := stage.OpenFile(relative, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return ArchiveResult{}, ErrInvalidArchive
		}
		if _, err = io.CopyN(file, archive, header.Size); err != nil {
			_ = file.Close()
			return ArchiveResult{}, ErrInvalidArchive
		}
		if err = file.Close(); err != nil {
			return ArchiveResult{}, ErrInvalidArchive
		}
	}

	// tar.Reader stops at the two-block terminator; only zero padding may follow.
	phase = "trailing data"
	buffer := make([]byte, 32*1024)
	for {
		count, readErr := expanded.Read(buffer)
		for _, value := range buffer[:count] {
			if value != 0 {
				return ArchiveResult{}, ErrInvalidArchive
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return ArchiveResult{}, ErrInvalidArchive
		}
	}
	if err := gz.Close(); err != nil {
		return ArchiveResult{}, ErrInvalidArchive
	}
	if compressed.count < limits.MinArtifactBytes || compressed.count > limits.ArtifactBytes || expanded.count > limits.ExpandedBytes {
		phase = "archive limits"
		return ArchiveResult{}, ErrInvalidArchive
	}
	if expanded.count > compressed.count*limits.MaxCompressionRatio {
		phase = "compression ratio"
		return ArchiveResult{}, ErrInvalidArchive
	}
	manifest, err := ValidateStagedManifest(stagingRoot, expectedSiteID)
	phase = "manifest"
	if err != nil {
		return ArchiveResult{}, ErrInvalidArchive
	}
	return ArchiveResult{
		Manifest: manifest, Digest: "sha256:" + hex.EncodeToString(hasher.Sum(nil)),
		CompressedBytes: compressed.count, ExpandedBytes: expanded.count,
	}, nil
}

type countingReader struct {
	reader io.Reader
	count  int64
}

func (r *countingReader) Read(destination []byte) (int, error) {
	n, err := r.reader.Read(destination)
	r.count += int64(n)
	return n, err
}

type expandedLimitReader struct {
	reader       io.Reader
	count, limit int64
}

func (r *expandedLimitReader) Read(destination []byte) (int, error) {
	if r.count > r.limit {
		return 0, ErrInvalidArchive
	}
	remaining := r.limit - r.count
	if int64(len(destination)) > remaining+1 {
		destination = destination[:remaining+1]
	}
	n, err := r.reader.Read(destination)
	r.count += int64(n)
	if r.count > r.limit {
		return n, ErrInvalidArchive
	}
	return n, err
}

type archivePathEntry struct {
	original  string
	directory bool
	explicit  bool
}
type archivePathSet struct {
	byFolded map[string]archivePathEntry
	entries  int
}

func newArchivePaths() *archivePathSet {
	return &archivePathSet{byFolded: make(map[string]archivePathEntry)}
}

func (paths *archivePathSet) register(value string, directory bool) error {
	parts := strings.Split(value, "/")
	originalPrefix := ""
	for index, part := range parts {
		if originalPrefix == "" {
			originalPrefix = part
		} else {
			originalPrefix += "/" + part
		}
		isLast := index == len(parts)-1
		folded := cases.Fold().String(norm.NFC.String(originalPrefix))
		entry, exists := paths.byFolded[folded]
		if exists && entry.original != originalPrefix {
			return ErrInvalidArchive
		}
		wantDirectory := !isLast || directory
		if exists && !entry.directory && wantDirectory {
			return ErrInvalidArchive
		}
		if exists && isLast && entry.explicit {
			return ErrInvalidArchive
		}
		if !exists {
			paths.byFolded[folded] = archivePathEntry{original: originalPrefix, directory: wantDirectory, explicit: isLast}
		}
		if exists && isLast {
			entry.explicit = true
			paths.byFolded[folded] = entry
		}
	}
	return nil
}

func cleanArchivePath(value string, directory bool) (string, error) {
	if value == "" || !utf8.ValidString(value) || strings.ContainsRune(value, '\\') || strings.HasPrefix(value, "/") || len(value) >= 2 && value[1] == ':' {
		return "", ErrInvalidArchive
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return "", ErrInvalidArchive
		}
	}
	if directory {
		value = strings.TrimSuffix(value, "/")
	}
	if path.IsAbs(value) {
		return "", ErrInvalidArchive
	}
	parts := strings.Split(value, "/")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part {
		case "":
			return "", ErrInvalidArchive
		case ".":
			continue
		case "..":
			return "", ErrInvalidArchive
		default:
			cleaned = append(cleaned, part)
		}
	}
	if len(cleaned) == 0 {
		if directory {
			return ".", nil
		}
		return "", ErrInvalidArchive
	}
	return strings.Join(cleaned, "/"), nil
}

func makeArchiveDirectory(root *os.Root, directory string, createdRoots map[string]struct{}) error {
	if directory == "" || directory == "." {
		return nil
	}
	relative := filepath.Clean(filepath.FromSlash(directory))
	if relative == ".." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return ErrInvalidArchive
	}
	current := ""
	for index, part := range strings.Split(relative, string(filepath.Separator)) {
		if current == "" {
			current = part
		} else {
			current = filepath.Join(current, part)
		}
		info, statErr := root.Lstat(current)
		if statErr == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return ErrInvalidArchive
			}
			continue
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return fmt.Errorf("inspect archive staging path: %w", statErr)
		}
		if err := root.Mkdir(current, 0o700); err != nil {
			return err
		}
		if index == 0 {
			createdRoots[part] = struct{}{}
		}
	}
	return nil
}
