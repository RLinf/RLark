package release

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"slices"
	"strings"
	"time"
)

// The trees Fleet's build Jobs build are not fetched from GitHub, which the cloud's build node does not reach: Fleet's
// build pushes the tree of the commit to the registry (fleet/source), tagged by the commit and annotated with it and
// the branch, and the Job reads it from there.
//
// A tree goes up in layers, its directories' (sourceLayerSize each at most, a directory larger split by the next
// level), each a reproducible tar (its entries by name, dated 0, owned by 0) gzipped: a layer the repository holds
// is not uploaded again, so a commit that changes a few files sends the layers they are in (the office's uplink is
// ~150 KB/s).
const (
	sourceArtifact  = "application/vnd.rlark.source.v1"
	sourceLayer     = "application/vnd.rlark.source.tree.v1.tar+gzip"
	branchNote      = "rlark.io/branch"
	commitNote      = "rlark.io/commit"
	sourceLayerSize = 1 << 20
)

type treeEntry struct {
	header tar.Header
	data   []byte
}

// PushSource writes the tree of a branch's head (a `git archive` of the commit, which its header names) under
// repository:<commit>, and returns the commit.
func (r *RegistryClient) PushSource(ctx context.Context, repository, branch string, archive []byte) (string, error) {
	reader := tar.NewReader(bytes.NewReader(archive))
	var commit string
	var entries []treeEntry
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
		if header.Typeflag == tar.TypeXGlobalHeader {
			commit = header.PAXRecords["comment"]
			continue
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			return "", err
		}
		entries = append(entries, treeEntry{header: *header, data: data})
	}
	if commit == "" {
		return "", errors.New("the archive names no commit in its header: it is not what `git archive <commit>` writes")
	}
	// By name, whatever order the archive has them in: the same files are the same layers.
	slices.SortFunc(entries, func(a, b treeEntry) int { return strings.Compare(a.header.Name, b.header.Name) })
	var groups [][]treeEntry
	splitTree(entries, 0, &groups)
	// An OCI artifact of the layers, with its time and the commit and branch it is of.
	config := []byte("{}")
	configDigest, err := r.pushBlob(ctx, repository, config)
	if err != nil {
		return "", err
	}
	var layers []map[string]any
	for _, group := range groups {
		data, err := treeLayer(group)
		if err != nil {
			return "", err
		}
		digest, err := r.pushBlob(ctx, repository, data)
		if err != nil {
			return "", err
		}
		layers = append(layers, map[string]any{"mediaType": sourceLayer, "digest": digest, "size": len(data)})
	}
	manifest, err := json.Marshal(map[string]any{
		"schemaVersion": 2,
		"mediaType":     ociImage,
		"artifactType":  sourceArtifact,
		"config":        map[string]any{"mediaType": "application/vnd.oci.empty.v1+json", "digest": configDigest, "size": len(config)},
		"layers":        layers,
		"annotations":   map[string]string{"org.opencontainers.image.created": time.Now().UTC().Format(time.RFC3339), branchNote: branch, commitNote: commit},
	})
	if err != nil {
		return "", err
	}
	return commit, r.putManifest(ctx, repository, commit, ociImage, manifest)
}

// splitTree groups the entries below depth: all of them when they are small enough, each directory's otherwise, and
// the entries of this level in runs of sourceLayerSize.
func splitTree(entries []treeEntry, depth int, groups *[][]treeEntry) {
	size := 0
	for _, entry := range entries {
		size += len(entry.data)
	}
	if size <= sourceLayerSize {
		if len(entries) > 0 {
			*groups = append(*groups, entries)
		}
		return
	}
	var here []treeEntry
	below := map[string][]treeEntry{}
	for _, entry := range entries {
		parts := strings.Split(strings.TrimSuffix(entry.header.Name, "/"), "/")
		if len(parts) > depth+1 {
			below[parts[depth]] = append(below[parts[depth]], entry)
		} else {
			here = append(here, entry)
		}
	}
	run, runSize := []treeEntry(nil), 0
	for _, entry := range here {
		if runSize > 0 && runSize+len(entry.data) > sourceLayerSize {
			*groups, run, runSize = append(*groups, run), nil, 0
		}
		run, runSize = append(run, entry), runSize+len(entry.data)
	}
	if len(run) > 0 {
		*groups = append(*groups, run)
	}
	for _, name := range slices.Sorted(maps.Keys(below)) {
		splitTree(below[name], depth+1, groups)
	}
}

// treeLayer is the entries as a tar dated 0 and owned by 0, gzipped: the same entries are the same bytes.
func treeLayer(entries []treeEntry) ([]byte, error) {
	var buffer bytes.Buffer
	compressed, err := gzip.NewWriterLevel(&buffer, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	archive := tar.NewWriter(compressed)
	for _, entry := range entries {
		header := tar.Header{Typeflag: entry.header.Typeflag, Name: entry.header.Name, Linkname: entry.header.Linkname, Size: entry.header.Size,
			Mode: entry.header.Mode, ModTime: time.Unix(0, 0)}
		if err := archive.WriteHeader(&header); err != nil {
			return nil, err
		}
		if _, err := archive.Write(entry.data); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	if err := compressed.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
