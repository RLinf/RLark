package release

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"maps"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

func writeTar(t *testing.T, commit string, files map[string]string) string {
	path := filepath.Join(t.TempDir(), "tree.tar")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	writer := tar.NewWriter(f)
	if commit != "" {
		// What git archive writes first: the commit, in a pax global header.
		if err := writer.WriteHeader(&tar.Header{Typeflag: tar.TypeXGlobalHeader, Name: "pax_global_header", PAXRecords: map[string]string{"comment": commit}}); err != nil {
			t.Fatal(err)
		}
	}
	for name, content := range files {
		if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(writer, content)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadTree(t *testing.T) {
	files := map[string]string{
		"api/config/crd/bases/rlinf.io_clusters.yaml": "apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\nmetadata:\n  name: clusters.rlinf.io\nspec:\n  group: rlinf.io\n",
		"astribot/station/deploy/management-rbac.yaml": "---\napiVersion: rbac.authorization.k8s.io/v1\nkind: ClusterRole\nmetadata:\n  name: station-api\n---\n" +
			"apiVersion: rbac.authorization.k8s.io/v1\nkind: RoleBinding\nmetadata:\n  name: station-api\n  namespace: station-system\n",
		"astribot/station/deploy/station-agent.yaml": "kind: DaemonSet\n",
		"astribot/station/deploy/member-rbac.yaml":   "apiVersion: rbac.authorization.k8s.io/v1\nkind: Role\nmetadata:\n  name: station-api-join-token\n  namespace: kube-system\n",
		"astribot/station/deploy/node-operation-member-rbac.yaml": "apiVersion: v1\nkind: ServiceAccount\nmetadata:\n  name: station-node-operator\n  namespace: station-system\n---\n" +
			"apiVersion: rbac.authorization.k8s.io/v1\nkind: Role\nmetadata:\n  name: station-node-reset\n  namespace: rlark-system\n",
		"README.md": "not a definition",
		"apps/rlark/pkg/addons/catalog/s1u-robot/addon.yaml":     "name: s1u-robot\nversion: v1.4.0\n",
		"apps/rlark/pkg/addons/catalog/site-registry/addon.yaml": "name: site-registry\nversion: v0.2.1\n",
		"astribot/release/environments/live.yaml":                "name: live\n",
		"astribot/release/environments/live.registry-ca.crt":     "a CA\n",
		"apps/rlark/pkg/addons/catalog/nvidia-gpu/addon.yaml":    "name: nvidia-gpu\nversion: v0.1.2\n",
		"astribot/release/pinned.yaml": "images:\n  cluster-check: registry.rlark.internal:5443/mirror/ubuntu@sha256:45e9\n  relay: registry.rlark.internal:5443/rlark/agent@sha256:0734\n" +
			"addons:\n  site-registry: {values: {relayImage: \"{{images.relay}}\"}}\n  nvidia-gpu: {values: {toolkitImage: \"{{images.cluster-check}}\"}}\n",
	}
	// What the release may do in a member cluster, as shipped.
	releaseRBAC, err := os.ReadFile("../../../../" + releaseMemberRBAC)
	if err != nil {
		t.Fatal(err)
	}
	files[releaseMemberRBAC] = string(releaseRBAC)
	tree, err := ReadTree(writeTar(t, "0123456789012345678901234567890123456789", files))
	if err != nil {
		t.Fatal(err)
	}
	releaseDefinitions, err := ParseDefinitions(releaseMemberRBAC, releaseRBAC)
	if err != nil {
		t.Fatal(err)
	}
	ofTheRelease := map[string]bool{}
	for _, definition := range releaseDefinitions {
		ofTheRelease[definitionName(definition)] = true
	}
	var names []string
	for _, definition := range tree.Definitions {
		names = append(names, definitionName(definition))
	}
	var members []string
	for _, definition := range tree.MemberDefinitions {
		if !ofTheRelease[definitionName(definition)] {
			members = append(members, definitionName(definition))
		}
	}
	if !slices.Equal(slices.Sorted(slices.Values(members)), []string{"Role kube-system/station-api-join-token", "Role rlark-system/station-node-reset", "ServiceAccount station-system/station-node-operator"}) {
		t.Errorf("member definitions %v", tree.MemberDefinitions)
	}
	// The tree's three, then each of RLark's components' role and binding in kcp, as this rlarkadm states them.
	if tree.Commit != "0123456789012345678901234567890123456789" || len(names) != 3+7 || tree.AddonVersions["s1u-robot"] != "v1.4.0" || !strings.Contains(strings.Join(names, ","), "RoleBinding station-system/station-api") ||
		!slices.Contains(names, "ClusterRole /rlark-gateway") || !slices.Contains(names, "ClusterRoleBinding /rlark-server") {
		t.Errorf("commit %s, definitions %v", tree.Commit, names)
	}
	if len(releaseDefinitions) != 14 || len(tree.MemberDefinitions) != 17 || tree.Files["release-member-rbac.yaml"] != string(releaseRBAC) || tree.Files["station-agent.yaml"] != "kind: DaemonSet\n" ||
		!slices.Equal(tree.Environments, []string{"live"}) {
		t.Errorf("the release's member definitions %d, files %v", len(releaseDefinitions), slices.Sorted(maps.Keys(tree.Files)))
	}
	if _, err := ReadTree(writeTar(t, "", files)); err == nil || !strings.Contains(err.Error(), "no commit") {
		t.Errorf("a tar without its commit: %v", err)
	}

	// The candidate of the tree: the base with the tree's images, Addon versions and generation.
	_, environment, base := writeFiles(t)
	record, err := CurrentRelease(environment, base, tree)
	if err != nil {
		t.Fatal(err)
	}
	if record["release"] != base.Name || record["commit"] != tree.Commit || record["registry"] != "registry.rlark.internal:5443" ||
		!strings.Contains(record["addons.json"], `"s1u-robot":{"version":"v1.4.0","values":{"deviceImage":"`+device+`"`) || !strings.Contains(record["addons.json"], `"site-registry":{"version":"v0.2.1","values":{`) ||
		!strings.Contains(record["images.json"], base.Images["cluster-agent"]) || record["node-operation-member-rbac.yaml"] != files["astribot/station/deploy/node-operation-member-rbac.yaml"] {
		t.Errorf("the record %v", record)
	}
	candidate, err := Candidate(base, tree, "ci-0123456", "fleetcommit", map[string]string{"rlark-ui": newUI, "s1u-robot": newRobot})
	if err != nil {
		t.Fatal(err)
	}
	path, err := candidate.Write()
	if err != nil {
		t.Fatal(err)
	}
	written, err := LoadRelease(path)
	if err != nil {
		t.Fatal(err)
	}
	if written.Name != "ci-0123456" || written.Source.RLark != tree.Commit || written.Source.Fleet != "fleetcommit" ||
		written.Images["s1u-robot"] != newRobot || written.Images["rlark-server"] != base.Images["rlark-server"] ||
		written.Addons["s1u-robot"].Version != "v1.4.0" || written.Addons["s1u-robot"].After[0] != "site-registry" {
		t.Errorf("candidate: %+v", written)
	}
	// What the tree pins: its images, its values of the base's Addons and of Addons the base has not.
	if written.Images["cluster-check"] != "registry.rlark.internal:5443/mirror/ubuntu@sha256:45e9" || written.Addons["site-registry"].Values["relayImage"] != "{{images.relay}}" ||
		written.Addons["site-registry"].Values["prewarm"] == "" || written.Addons["nvidia-gpu"].Version != "v0.1.2" {
		t.Errorf("pinned: %+v", written)
	}
	if base.Addons["site-registry"].Values["relayImage"] != "" {
		t.Errorf("the base was changed: %+v", base.Addons["site-registry"])
	}
	// An image built of the tree that the base has not (an environment's installer) is the candidate's.
	if built, err := Candidate(base, tree, "x", "", map[string]string{"installer-live": newUI}); err != nil || built.Images["installer-live"] != newUI {
		t.Errorf("an installer built of the tree: %v %v", built.Images, err)
	}
	// An image of the base that nothing names any more (no component, nothing pinned or built, no environment of the
	// tree, no Addon of the candidate) is not the candidate's; the installer of an environment of the tree, not built
	// again, and the images its Addons name are.
	base.Images["release-ci"], base.Images["installer-staging"], base.Images["installer-live"] = newUI, newUI, newUI
	if built, err := Candidate(base, tree, "x", "", nil); err != nil || built.Images["release-ci"] != "" || built.Images["installer-staging"] != "" ||
		built.Images["installer-live"] != newUI || built.Images["fleet-robot"] != device || built.Images["tools"] != oldRLark {
		t.Errorf("the base's images the candidate carries: %v %v", built.Images, err)
	}
	delete(tree.AddonVersions, "site-registry")
	if _, err := Candidate(base, tree, "x", "", nil); err == nil || !strings.Contains(err.Error(), "site-registry") {
		t.Errorf("an Addon the tree's catalog lacks: %v", err)
	}
}

// registry is as much of a registry as copies, installers and the trees pushed need.
type registry struct {
	mu     sync.Mutex
	upload []byte
	// lose is how many chunks are not answered: the first not taken, later ones taken (an uplink that drops them).
	lose int
	// blobs are the blobs the registry has, by digest; linked those each repository has, by repository@digest;
	// uploaded the blobs that came up (not had).
	blobs     map[string]bool
	linked    map[string]bool
	uploaded  []string
	data      map[string][]byte
	manifests map[string]string
}

func (r *registry) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	path := request.URL.Path
	switch {
	case request.Method == http.MethodPost && strings.HasSuffix(path, "/blobs/uploads/"):
		repository := strings.TrimSuffix(strings.TrimPrefix(path, "/v2/"), "/blobs/uploads/")
		w.Header().Set("Location", "/v2/upload/1?repository="+repository)
		w.WriteHeader(http.StatusAccepted)
	case request.Method == http.MethodPatch && strings.HasPrefix(path, "/v2/upload/"):
		// A chunk, at the offset the upload has reached.
		body, _ := io.ReadAll(request.Body)
		if request.Header.Get("Content-Range") != fmt.Sprintf("%d-%d", len(r.upload), len(r.upload)+len(body)-1) {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		// A lost chunk: from the second on, taken without an answer; the first, never taken (nor answered).
		if r.lose > 0 && len(r.upload) == 0 {
			r.lose--
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		r.upload = append(r.upload, body...)
		if r.lose > 0 {
			r.lose--
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Location", "/v2/upload/1?repository="+request.URL.Query().Get("repository"))
		w.WriteHeader(http.StatusAccepted)
	case request.Method == http.MethodGet && strings.HasPrefix(path, "/v2/upload/"):
		w.Header().Set("Location", "/v2/upload/1?repository="+request.URL.Query().Get("repository"))
		// As distribution says it: 0-0 of an upload that has nothing yet.
		w.Header().Set("Range", fmt.Sprintf("0-%d", max(len(r.upload)-1, 0)))
		w.WriteHeader(http.StatusNoContent)
	case request.Method == http.MethodPut && strings.HasPrefix(path, "/v2/upload/"):
		chunk, _ := io.ReadAll(request.Body)
		body := append(r.upload, chunk...)
		r.upload = nil
		if sum := sha256.Sum256(body); request.URL.Query().Get("digest") != "sha256:"+hex.EncodeToString(sum[:]) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		digest := request.URL.Query().Get("digest")
		r.blobs[digest] = true
		r.linked[request.URL.Query().Get("repository")+"@"+digest] = true
		r.uploaded = append(r.uploaded, digest)
		if r.data != nil {
			r.data[digest] = body
		}
		w.WriteHeader(http.StatusCreated)
	case request.Method == http.MethodPut && strings.Contains(path, "/manifests/"):
		body, _ := io.ReadAll(request.Body)
		r.manifests[path] = string(body)
		w.WriteHeader(http.StatusCreated)
	case (request.Method == http.MethodHead || request.Method == http.MethodGet) && strings.Contains(path, "/manifests/"):
		// As the site's registry answers: an OCI manifest is found only for a client that accepts one.
		manifest, found := r.manifests[path]
		if !found || !strings.Contains(request.Header.Get("Accept"), "application/vnd.oci.image.manifest.v1+json") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var kind struct{ MediaType string }
		_ = json.Unmarshal([]byte(manifest), &kind)
		w.Header().Set("Content-Type", kind.MediaType)
		if request.Method == http.MethodGet {
			_, _ = io.WriteString(w, manifest)
		}
	case request.Method == http.MethodHead && strings.Contains(path, "/blobs/"):
		repository := path[len("/v2/"):strings.Index(path, "/blobs/")]
		if !r.linked[repository+"@"+path[strings.LastIndex(path, "/")+1:]] {
			w.WriteHeader(http.StatusNotFound)
		}
	case request.Method == http.MethodGet && strings.Contains(path, "/blobs/"):
		data, found := r.data[path[strings.LastIndex(path, "/")+1:]]
		if !found {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(data)
	default:
		w.WriteHeader(http.StatusBadRequest)
	}
}

// The registry is reached where the environment says, by the CA it names, or by an address that asks for none (an
// http:// entry on the site's tunnel).
func TestNewRecordsTakesTheEnvironmentsRegistry(t *testing.T) {
	served := &registry{blobs: map[string]bool{}, linked: map[string]bool{}, manifests: map[string]string{}}
	server := httptest.NewTLSServer(served)
	defer server.Close()
	dir := t.TempDir()
	ca := filepath.Join(dir, "registry-ca.crt")
	_ = os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o644)
	client, err := NewRegistryClient(context.Background(), "registry.rlark.internal:5443", RegistryAccess{Address: server.URL, CA: ca}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if held, err := client.has(context.Background(), "/v2/rlark/agent/manifests/sha256:0"); err != nil || held {
		t.Errorf("%v %v", held, err)
	}

	plain := httptest.NewServer(&registry{blobs: map[string]bool{}, linked: map[string]bool{}, manifests: map[string]string{}})
	defer plain.Close()
	open, err := NewRegistryClient(context.Background(), "registry.rlark.internal:5443", RegistryAccess{Address: plain.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	commit := strings.Repeat("1", 40)
	data, _ := os.ReadFile(writeTar(t, commit, map[string]string{"README.md": commit}))
	if _, err := open.PushSource(context.Background(), "fleet/source", "main", data); err != nil {
		t.Fatal(err)
	}
	if held, err := open.has(context.Background(), "/v2/fleet/source/manifests/"+commit); err != nil || !held {
		t.Errorf("by the tunnel's entry: %v %v", held, err)
	}
}

// A commit is pushed as its tree, tagged by the commit and annotated with it and its branch; what is no git archive
// is not pushed.
func TestSourcePush(t *testing.T) {
	served := &registry{blobs: map[string]bool{}, linked: map[string]bool{}, data: map[string][]byte{}, manifests: map[string]string{}}
	server := httptest.NewServer(served)
	defer server.Close()
	client, err := NewRegistryClient(context.Background(), "", RegistryAccess{Address: server.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	commit := strings.Repeat("1", 40)
	data, _ := os.ReadFile(writeTar(t, commit, map[string]string{"README.md": commit}))
	if source, err := client.PushSource(ctx, "fleet/source", "main", data); err != nil || source.Commit != commit || source.Branch != "main" {
		t.Fatalf("%v %v", source, err)
	}
	manifest := served.manifests["/v2/fleet/source/manifests/"+commit]
	if !strings.Contains(manifest, `"rlark.io/branch":"main"`) || !strings.Contains(manifest, `"rlark.io/commit":"`+commit+`"`) || !strings.Contains(manifest, sourceArtifact) {
		t.Errorf("the tree's manifest %s", manifest)
	}
	if _, err := client.PushSource(ctx, "fleet/source", "main", []byte("not an archive")); err == nil {
		t.Error("pushed what is no git archive")
	}
}

// A blob of megabytes goes up in chunks, each its own request, and arrives whole under its digest.
func TestABlobGoesUpInChunks(t *testing.T) {
	served := &registry{blobs: map[string]bool{}, linked: map[string]bool{}, data: map[string][]byte{}, manifests: map[string]string{}}
	server := httptest.NewServer(served)
	defer server.Close()
	client, err := NewRegistryClient(context.Background(), "registry.rlark.internal:5443", RegistryAccess{Address: server.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	data := bytes.Repeat([]byte("0123456789abcdef"), (5*uploadChunk+uploadChunk/2)/16)
	blob, err := client.pushBlob(context.Background(), "fleet/source", data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(served.data[blob], data) {
		t.Errorf("%s arrived as %d bytes of %d", blob, len(served.data[blob]), len(data))
	}
	// Chunks the registry took without its answer reaching the client: the upload goes on from what it has.
	served.lose = 3
	other := bytes.Repeat([]byte("fedcba9876543210"), (3*uploadChunk)/16)
	if blob, err := client.pushBlob(context.Background(), "fleet/source", other); err != nil || !bytes.Equal(served.data[blob], other) {
		t.Errorf("after lost answers: %v, %d bytes of %d", err, len(served.data[blob]), len(other))
	}
	// One the registry has is not uploaded again.
	served.data[blob] = nil
	if again, err := client.pushBlob(context.Background(), "fleet/source", data); err != nil || again != blob || served.data[blob] != nil {
		t.Errorf("uploaded again: %v", err)
	}
}

// A tree goes up in layers; a commit that changes a file sends the layer it is in, and its layers hold the tree whole.
func TestATreeGoesUpInLayers(t *testing.T) {
	served := &registry{blobs: map[string]bool{}, linked: map[string]bool{}, data: map[string][]byte{}, manifests: map[string]string{}}
	server := httptest.NewServer(served)
	defer server.Close()
	client, err := NewRegistryClient(context.Background(), "", RegistryAccess{Address: server.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	random := rand.New(rand.NewPCG(1, 2))
	content := func(size int) string {
		data := make([]byte, size)
		for i := range data {
			data[i] = byte(random.IntN(256))
		}
		return string(data)
	}
	files := map[string]string{"go.work": "go 1.26\n", "README.md": "tree\n"}
	for _, directory := range []string{"apps/rlark/astribot/release", "apps/rlark/pkg/gateway", "apps/rlark-ui/src", "astribot/station/controller", "astribot/release"} {
		for i := range 4 {
			files[fmt.Sprintf("%s/file-%d", directory, i)] = content(200 << 10)
		}
	}
	first, second := strings.Repeat("a", 40), strings.Repeat("b", 40)
	push := func(commit string) int {
		before := len(served.uploaded)
		data, _ := os.ReadFile(writeTar(t, commit, files))
		if _, err := client.PushSource(ctx, "fleet/source", "main", data); err != nil {
			t.Fatal(err)
		}
		return len(served.uploaded) - before
	}
	if sent := push(first); sent < 5 {
		t.Fatalf("the tree went up in %d blobs", sent)
	}
	files["apps/rlark/astribot/release/file-2"] = content(200 << 10)
	if sent := push(second); sent != 1 {
		t.Errorf("a change of one file sent %d layers", sent)
	}

	var manifest struct {
		Layers []struct{ Digest, MediaType string }
	}
	if err := json.Unmarshal([]byte(served.manifests["/v2/fleet/source/manifests/"+second]), &manifest); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, layer := range manifest.Layers {
		uncompressed, err := gzip.NewReader(bytes.NewReader(served.data[layer.Digest]))
		if err != nil || layer.MediaType != sourceLayer {
			t.Fatalf("layer %s (%s): %v", layer.Digest, layer.MediaType, err)
		}
		reader := tar.NewReader(uncompressed)
		for {
			header, err := reader.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			data, _ := io.ReadAll(reader)
			got[header.Name] = string(data)
		}
	}
	if !maps.Equal(got, files) {
		t.Errorf("the layers hold %d files of %d", len(got), len(files))
	}
}

// An image is copied between two registries by digest: an index with the manifest it lists, its blobs streamed,
// nothing the target holds sent again; an image the release does not name of the registry is no business of the
// site's registry.
func TestImagesAreCopiedIntoTheSitesRegistry(t *testing.T) {
	ctx := context.Background()
	registries := map[string]*registry{}
	reach := map[string]*RegistryClient{}
	for _, name := range []string{"source", "site"} {
		registries[name] = &registry{blobs: map[string]bool{}, linked: map[string]bool{}, data: map[string][]byte{}, manifests: map[string]string{}}
		server := httptest.NewServer(registries[name])
		defer server.Close()
		client, err := NewRegistryClient(ctx, "registry.rlark.internal:5443", RegistryAccess{Address: server.URL}, nil)
		if err != nil {
			t.Fatal(err)
		}
		reach[name] = client
	}
	source := registries["source"]
	digest := func(data []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(data)) }
	config, layer := []byte(`{"architecture":"amd64"}`), []byte("a layer of the image")
	for _, blob := range [][]byte{config, layer} {
		source.data[digest(blob)] = blob
	}
	image := fmt.Sprintf(`{"schemaVersion":2,"mediaType":%q,"config":{"mediaType":"application/vnd.oci.image.config.v1+json","digest":%q,"size":%d},"layers":[{"mediaType":"application/vnd.oci.image.layer.v1.tar","digest":%q,"size":%d}]}`,
		ociImage, digest(config), len(config), digest(layer), len(layer))
	index := fmt.Sprintf(`{"schemaVersion":2,"mediaType":%q,"manifests":[{"mediaType":%q,"digest":%q,"size":%d,"platform":{"architecture":"amd64","os":"linux"}}]}`,
		ociIndex, ociImage, digest([]byte(image)), len(image))
	source.manifests["/v2/rlark/agent/manifests/"+digest([]byte(image))] = image
	source.manifests["/v2/rlark/agent/manifests/"+digest([]byte(index))] = index

	run := &Run{Site: &Site{Registry: reach["source"], SiteRegistry: reach["site"]}, log: logTo{out: io.Discard},
		Images: []string{"registry.rlark.internal:5443/rlark/agent@" + digest([]byte(index)), "docker.io/library/busybox@sha256:" + strings.Repeat("0", 64)}}
	changes, err := run.changes(ctx, "images", State{})
	if err != nil || len(changes) != 1 || changes[0].What != "site registry rlark/agent" {
		t.Fatalf("changes %v %v", changes, err)
	}
	if err := run.act(ctx, "images", State{}); err != nil {
		t.Fatal(err)
	}
	site := registries["site"]
	for _, path := range []string{"/v2/rlark/agent/manifests/" + digest([]byte(index)), "/v2/rlark/agent/manifests/" + digest([]byte(image))} {
		if site.manifests[path] != source.manifests[path] {
			t.Errorf("the site's registry holds %q at %s", site.manifests[path], path)
		}
	}
	if !bytes.Equal(site.data[digest(layer)], layer) || !bytes.Equal(site.data[digest(config)], config) {
		t.Errorf("the site's registry lacks the blobs: %v", slices.Collect(maps.Keys(site.data)))
	}
	if changes, err := run.changes(ctx, "images", State{}); err != nil || len(changes) != 0 {
		t.Fatalf("after the copy: %v %v", changes, err)
	}
	uploaded := len(site.uploaded)
	if _, err := reach["source"].Copy(ctx, reach["site"], "rlark/agent", digest([]byte(image)), "rlark/agent", "1.0"); err != nil || len(site.uploaded) != uploaded {
		t.Errorf("copied again: %v, %d uploads more", err, len(site.uploaded)-uploaded)
	}
	if site.manifests["/v2/rlark/agent/manifests/1.0"] != image {
		t.Errorf("the tag names %q", site.manifests["/v2/rlark/agent/manifests/1.0"])
	}
}
