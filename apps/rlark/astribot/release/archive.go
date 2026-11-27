package release

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
)

// ociImageName is the annotation of an OCI layout's index entry that containerd imports the image by (ctr images
// import): its whole name, here the registry's name, repository and digest a Pod names it by.
const ociImageName = "io.containerd.image.name"

// Archive writes the images (names by digest, of the registry r is) to w as an OCI image layout in a tar, of one
// platform (os/architecture): an index with its manifest of the platform alone, a single image only when it is of
// the platform. Each is named by its whole name, so a node that imports the archive has it as a Pod names it and
// pulls nothing. It returns the images of the platform, which the archive holds; the others are not in it.
func (r *RegistryClient) Archive(ctx context.Context, w io.Writer, images []string, platform string) ([]string, error) {
	out := tar.NewWriter(w)
	written := map[string]bool{}
	write := func(name string, size int64, from io.Reader) error {
		if err := out.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: size, Typeflag: tar.TypeReg, Format: tar.FormatPAX}); err != nil {
			return err
		}
		_, err := io.Copy(out, from)
		return err
	}
	// A blob (of the registry, or a manifest's data) once, under its digest.
	blob := func(repository, digest string, size int64, data []byte) error {
		if written[digest] {
			return nil
		}
		written[digest] = true
		if data != nil {
			return write("blobs/sha256/"+strings.TrimPrefix(digest, "sha256:"), size, bytes.NewReader(data))
		}
		body, err := r.blob(ctx, repository, digest)
		if err != nil {
			return err
		}
		defer body.Close()
		sum := sha256.New()
		if err := write("blobs/sha256/"+strings.TrimPrefix(digest, "sha256:"), size, io.TeeReader(body, sum)); err != nil {
			return err
		}
		if got := fmt.Sprintf("sha256:%x", sum.Sum(nil)); got != digest {
			return fmt.Errorf("registry %s: %s@%s: the blob is %s", r.Registry, repository, digest, got)
		}
		return nil
	}
	layout := `{"imageLayoutVersion":"1.0.0"}`
	if err := write("oci-layout", int64(len(layout)), strings.NewReader(layout)); err != nil {
		return nil, err
	}
	type descriptor struct {
		MediaType   string            `json:"mediaType"`
		Digest      string            `json:"digest"`
		Size        int64             `json:"size"`
		Annotations map[string]string `json:"annotations,omitempty"`
	}
	index := struct {
		SchemaVersion int          `json:"schemaVersion"`
		MediaType     string       `json:"mediaType"`
		Manifests     []descriptor `json:"manifests"`
	}{SchemaVersion: 2, MediaType: ociIndex, Manifests: []descriptor{}}
	var held []string
	for _, image := range images {
		name, digest, _ := strings.Cut(image, "@")
		_, repository, _ := strings.Cut(name, "/")
		top, topType, err := r.manifest(ctx, repository, digest)
		if err != nil {
			return nil, err
		}
		// data is the manifest of the platform: top itself, or, of an index, its manifest of the platform (of).
		data, of := top, ""
		type entry struct {
			Digest   string
			Platform struct{ OS, Architecture string }
		}
		var parts struct {
			Manifests []entry
			Config    struct {
				Digest string
				Size   int64
			}
			Layers []struct {
				Digest string
				Size   int64
			}
		}
		if err := json.Unmarshal(top, &parts); err != nil {
			return nil, fmt.Errorf("%s: %w", image, err)
		}
		if topType == ociIndex || topType == dockerList {
			at := slices.IndexFunc(parts.Manifests, func(m entry) bool { return m.Platform.OS+"/"+m.Platform.Architecture == platform })
			if at < 0 {
				continue
			}
			of = parts.Manifests[at].Digest
			if data, _, err = r.manifest(ctx, repository, of); err != nil {
				return nil, err
			}
			if err := json.Unmarshal(data, &parts); err != nil {
				return nil, fmt.Errorf("%s: %w", image, err)
			}
		} else {
			// A single image says its platform in its config.
			config, err := r.blob(ctx, repository, parts.Config.Digest)
			if err != nil {
				return nil, err
			}
			var is struct{ OS, Architecture string }
			err = json.NewDecoder(config).Decode(&is)
			config.Close()
			if err != nil {
				return nil, fmt.Errorf("%s: its config: %w", image, err)
			}
			if is.OS+"/"+is.Architecture != platform {
				continue
			}
		}
		for _, part := range append(parts.Layers, parts.Config) {
			if err := blob(repository, part.Digest, part.Size, nil); err != nil {
				return nil, err
			}
		}
		if of != "" {
			if err := blob(repository, of, int64(len(data)), data); err != nil {
				return nil, err
			}
		}
		if err := blob(repository, digest, int64(len(top)), top); err != nil {
			return nil, err
		}
		index.Manifests = append(index.Manifests, descriptor{MediaType: topType, Digest: digest, Size: int64(len(top)), Annotations: map[string]string{ociImageName: image}})
		held = append(held, image)
	}
	data, _ := json.Marshal(index)
	if err := write("index.json", int64(len(data)), bytes.NewReader(data)); err != nil {
		return nil, err
	}
	return held, out.Close()
}
