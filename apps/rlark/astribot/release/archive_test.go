package release

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"slices"
	"testing"
)

// The installer's archive of a platform: an index by its digest with that platform's manifest only, a single image
// only when it is of the platform, each named as a Pod names it; every blob as the registry holds it.
func TestTheInstallersArchiveHoldsThePlatformsImagesByTheirNames(t *testing.T) {
	ctx := context.Background()
	held := &registry{blobs: map[string]bool{}, linked: map[string]bool{}, data: map[string][]byte{}, manifests: map[string]string{}}
	server := httptest.NewServer(held)
	defer server.Close()
	client, err := NewRegistryClient(ctx, "registry.rlark.internal:5443", RegistryAccess{Address: server.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	digest := func(data string) string { return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(data))) }
	image := func(repository, architecture string) string {
		config, layer := fmt.Sprintf(`{"architecture":%q,"os":"linux"}`, architecture), "a layer of "+repository+" "+architecture
		held.data[digest(config)], held.data[digest(layer)] = []byte(config), []byte(layer)
		manifest := fmt.Sprintf(`{"schemaVersion":2,"mediaType":%q,"config":{"digest":%q,"size":%d},"layers":[{"digest":%q,"size":%d}]}`,
			ociImage, digest(config), len(config), digest(layer), len(layer))
		held.manifests["/v2/"+repository+"/manifests/"+digest(manifest)] = manifest
		return manifest
	}
	amd64, arm64 := image("rlark/busybox", "amd64"), image("rlark/busybox", "arm64")
	index := fmt.Sprintf(`{"schemaVersion":2,"mediaType":%q,"manifests":[{"mediaType":%q,"digest":%q,"size":%d,"platform":{"architecture":"amd64","os":"linux"}},{"mediaType":%q,"digest":%q,"size":%d,"platform":{"architecture":"arm64","os":"linux","variant":"v8"}}]}`,
		ociIndex, ociImage, digest(amd64), len(amd64), ociImage, digest(arm64), len(arm64))
	held.manifests["/v2/rlark/busybox/manifests/"+digest(index)] = index
	single := image("rlark/registry", "amd64")
	busybox := "registry.rlark.internal:5443/rlark/busybox@" + digest(index)
	registryImage := "registry.rlark.internal:5443/rlark/registry@" + digest(single)

	for platform, want := range map[string][]string{"linux/amd64": {busybox, registryImage}, "linux/arm64": {busybox}} {
		var out bytes.Buffer
		got, err := client.Archive(ctx, &out, []string{busybox, registryImage}, platform)
		if err != nil || !slices.Equal(got, want) {
			t.Fatalf("%s: %v %v", platform, got, err)
		}
		files := map[string][]byte{}
		archive := tar.NewReader(&out)
		for {
			header, err := archive.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			files[header.Name], _ = io.ReadAll(archive)
		}
		var layout struct {
			Manifests []struct {
				Digest      string
				Annotations map[string]string
			}
		}
		if err := json.Unmarshal(files["index.json"], &layout); err != nil || string(files["oci-layout"]) != `{"imageLayoutVersion":"1.0.0"}` {
			t.Fatalf("%s: %s %v", platform, files["index.json"], err)
		}
		var names []string
		for _, entry := range layout.Manifests {
			names = append(names, entry.Annotations[ociImageName])
			if fmt.Sprintf("registry.rlark.internal:5443/rlark/%s@%s", map[string]string{digest(index): "busybox", digest(single): "registry"}[entry.Digest], entry.Digest) != entry.Annotations[ociImageName] {
				t.Errorf("%s: %s is named %s", platform, entry.Digest, entry.Annotations[ociImageName])
			}
		}
		if !slices.Equal(names, want) {
			t.Errorf("%s: the archive names %v", platform, names)
		}
		for name, data := range files {
			if name != "index.json" && name != "oci-layout" && name != "blobs/sha256/"+fmt.Sprintf("%x", sha256.Sum256(data)) {
				t.Errorf("%s: %s holds something else", platform, name)
			}
		}
		// The index and the platform's manifest; the other platform's is not there.
		mine, other := amd64, arm64
		if platform == "linux/arm64" {
			mine, other = arm64, amd64
		}
		for blob, there := range map[string]bool{index: true, mine: true, other: false} {
			if _, found := files["blobs/sha256/"+digest(blob)[len("sha256:"):]]; found != there {
				t.Errorf("%s: %s in the archive: %v", platform, digest(blob), found)
			}
		}
	}
}
