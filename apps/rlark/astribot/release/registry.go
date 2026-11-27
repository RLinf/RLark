package release

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// RegistryClient is a registry as a release reaches it: the images it copies, the installers it reads, the trees it
// pushes.
type RegistryClient struct {
	// Registry is the registry's name in image references; base where it is reached.
	Registry string
	HTTP     *http.Client
	base     string
	auth     string
}

// The kinds of manifest an image is, each copied as the registry holds it (by digest, so byte for byte).
const (
	ociIndex    = "application/vnd.oci.image.index.v1+json"
	dockerList  = "application/vnd.docker.distribution.manifest.list.v2+json"
	ociImage    = "application/vnd.oci.image.manifest.v1+json"
	dockerImage = "application/vnd.docker.distribution.manifest.v2+json"
)

var manifestTypes = strings.Join([]string{ociIndex, dockerList, ociImage, dockerImage}, ", ")

// NewRegistryClient reaches the registry name at access: its address, the CA it is trusted by, and its account, an
// image registry credential of the management plane (the Gateway keeps it as the Secret default/<id>-replication,
// credential.json), read there now by management (the release's identity) and kept in memory only.
func NewRegistryClient(ctx context.Context, name string, access RegistryAccess, management kubernetes.Interface) (*RegistryClient, error) {
	// A request takes as long as its body does (a tree of megabytes over a site's slow uplink); the registry is to
	// answer within a minute of it.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = time.Minute
	client := &RegistryClient{Registry: name, base: strings.TrimRight(access.Address, "/"), HTTP: &http.Client{Transport: transport}}
	if access.CA != "" {
		pool, err := x509.SystemCertPool()
		if err != nil {
			return nil, err
		}
		ca, err := os.ReadFile(access.CA)
		if err != nil {
			return nil, fmt.Errorf("the registry's CA: %w", err)
		}
		pool.AppendCertsFromPEM(ca)
		transport.TLSClientConfig = &tls.Config{RootCAs: pool}
	}
	if access.Credential != "" {
		secret, err := management.CoreV1().Secrets("default").Get(ctx, access.Credential+"-replication", metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("the registry's credential %s: %w", access.Credential, err)
		}
		var credential struct{ Username, Password string }
		if err := json.Unmarshal(secret.Data["credential.json"], &credential); err != nil {
			return nil, fmt.Errorf("the registry's credential %s: %w", access.Credential, err)
		}
		client.auth = base64.StdEncoding.EncodeToString([]byte(credential.Username + ":" + credential.Password))
	}
	return client, nil
}

// do sends a request with the registry's account to target, a path of the registry or a location it answered with.
// An answer of another status than want is an error, with the answer, closed. headers are name, value pairs.
func (r *RegistryClient) do(ctx context.Context, method, target string, body io.Reader, size int64, want int, headers ...string) (*http.Response, error) {
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		target = r.base + target
	}
	request, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	request.ContentLength = size
	for i := 0; i+1 < len(headers); i += 2 {
		request.Header.Set(headers[i], headers[i+1])
	}
	if r.auth != "" {
		request.Header.Set("Authorization", "Basic "+r.auth)
	}
	response, err := r.HTTP.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != want {
		response.Body.Close()
		return response, fmt.Errorf("registry %s: %s %s: %s", r.Registry, method, request.URL.Path, response.Status)
	}
	return response, nil
}

// has says whether the registry holds the manifest or blob at path (a HEAD answered 200, not 404).
func (r *RegistryClient) has(ctx context.Context, path string) (bool, error) {
	response, err := r.do(ctx, http.MethodHead, path, nil, 0, http.StatusOK, "Accept", manifestTypes)
	switch {
	case err == nil:
		response.Body.Close()
		return true, nil
	case response != nil && response.StatusCode == http.StatusNotFound:
		return false, nil
	}
	return false, err
}

// manifest is repository's manifest by reference as the registry holds it, and its media type; one asked for by its
// digest is that digest's.
func (r *RegistryClient) manifest(ctx context.Context, repository, reference string) ([]byte, string, error) {
	response, err := r.do(ctx, http.MethodGet, "/v2/"+repository+"/manifests/"+reference, nil, 0, http.StatusOK, "Accept", manifestTypes)
	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if digest := digestOf(data); err == nil && strings.HasPrefix(reference, "sha256:") && digest != reference {
		err = fmt.Errorf("%s@%s: the registry answered a manifest of %s", repository, reference, digest)
	}
	mediaType, _, _ := strings.Cut(response.Header.Get("Content-Type"), ";")
	return data, mediaType, err
}

// blob opens repository's blob.
func (r *RegistryClient) blob(ctx context.Context, repository, digest string) (io.ReadCloser, error) {
	response, err := r.do(ctx, http.MethodGet, "/v2/"+repository+"/blobs/"+digest, nil, 0, http.StatusOK)
	if err != nil {
		return nil, err
	}
	return response.Body, nil
}

// putManifest writes a manifest under a digest or a tag.
func (r *RegistryClient) putManifest(ctx context.Context, repository, reference, mediaType string, data []byte) error {
	response, err := r.do(ctx, http.MethodPut, "/v2/"+repository+"/manifests/"+reference, bytes.NewReader(data), int64(len(data)), http.StatusCreated, "Content-Type", mediaType)
	if err == nil {
		response.Body.Close()
	}
	return err
}

// startUpload is where an upload to repository goes.
func (r *RegistryClient) startUpload(ctx context.Context, repository string) (string, error) {
	response, err := r.do(ctx, http.MethodPost, "/v2/"+repository+"/blobs/uploads/", nil, 0, http.StatusAccepted)
	if err != nil {
		return "", err
	}
	response.Body.Close()
	return r.resolve(response.Header.Get("Location")), nil
}

// resolve is a location the registry answered with, against its address.
func (r *RegistryClient) resolve(location string) string {
	base, _ := url.Parse(r.base + "/")
	target, err := base.Parse(location)
	if err != nil {
		return location
	}
	return target.String()
}

// finish closes the upload at location as the blob of digest, with the rest of it as body.
func (r *RegistryClient) finish(ctx context.Context, location, digest string, body io.Reader, size int64) error {
	target, err := url.Parse(location)
	if err != nil {
		return err
	}
	query := target.Query()
	query.Set("digest", digest)
	target.RawQuery = query.Encode()
	response, err := r.do(ctx, http.MethodPut, target.String(), body, size, http.StatusCreated, "Content-Type", "application/octet-stream")
	if err == nil {
		response.Body.Close()
	}
	return err
}

// blobRetries is how many times a blob is sent again after a send that failed (a tunnel that dropped it).
const blobRetries = 3

// Copy copies the image from@reference of r into to of target, by digest: the manifests and blobs target lacks,
// streamed (an image of gigabytes is never held), an index with every manifest it lists. tag, if any, names it in
// target besides. It returns the image's digest, which target holds once Copy returns.
func (r *RegistryClient) Copy(ctx context.Context, target *RegistryClient, from, reference, to, tag string) (string, error) {
	if strings.HasPrefix(reference, "sha256:") && tag == "" {
		if held, err := target.has(ctx, "/v2/"+to+"/manifests/"+reference); err != nil || held {
			return reference, err
		}
	}
	data, mediaType, err := r.manifest(ctx, from, reference)
	if err != nil {
		return "", err
	}
	var parts struct {
		Manifests []struct{ Digest string } `json:"manifests"`
		Config    struct {
			Digest string
			Size   int64
		} `json:"config"`
		Layers []struct {
			Digest string
			Size   int64
		} `json:"layers"`
	}
	if err := json.Unmarshal(data, &parts); err != nil {
		return "", fmt.Errorf("%s@%s: %w", from, reference, err)
	}
	switch mediaType {
	case ociIndex, dockerList:
		for _, manifest := range parts.Manifests {
			if _, err := r.Copy(ctx, target, from, manifest.Digest, to, ""); err != nil {
				return "", err
			}
		}
	case ociImage, dockerImage:
		for _, blob := range append(parts.Layers, parts.Config) {
			held, err := target.has(ctx, "/v2/"+to+"/blobs/"+blob.Digest)
			if err != nil {
				return "", err
			}
			for attempt := 0; !held; attempt++ {
				err = r.sendBlob(ctx, target, from, to, blob.Digest, blob.Size)
				if held = err == nil; !held && (attempt == blobRetries || ctx.Err() != nil) {
					return "", err
				}
			}
		}
	default:
		return "", fmt.Errorf("%s@%s is a %q, not an image", from, reference, mediaType)
	}
	digest := digestOf(data)
	for _, name := range []string{digest, tag} {
		if name != "" {
			if err := target.putManifest(ctx, to, name, mediaType, data); err != nil {
				return "", err
			}
		}
	}
	return digest, nil
}

// sendBlob streams a blob of r into target, whole.
func (r *RegistryClient) sendBlob(ctx context.Context, target *RegistryClient, from, to, digest string, size int64) error {
	source, err := r.blob(ctx, from, digest)
	if err != nil {
		return err
	}
	defer source.Close()
	location, err := target.startUpload(ctx, to)
	if err != nil {
		return err
	}
	return target.finish(ctx, location, digest, source, size)
}

// uploadChunk is how much of a blob one request carries: a registry answers each within its minute even over a
// site's slow uplink (the office's management machine to the registry through the tunnel, ~30 KB/s on 11-16),
// where a blob of megabytes in one request does not reach it in that time.
const uploadChunk = 256 << 10

// uploadRetries is how many times in a row a chunk is asked about and sent again.
const uploadRetries = 5

// pushBlob uploads data in chunks (PATCH each, PUT with its digest last) unless the repository has it. A chunk that
// is not answered (the uplink busy with others' pushes) is asked about: the upload goes on from what the registry has
// of it, a few times.
func (r *RegistryClient) pushBlob(ctx context.Context, repository string, data []byte) (string, error) {
	blob := digestOf(data)
	if held, err := r.has(ctx, "/v2/"+repository+"/blobs/"+blob); err != nil || held {
		return blob, err
	}
	location, err := r.startUpload(ctx, repository)
	if err != nil {
		return "", err
	}
	for start, failures := 0, 0; start < len(data); {
		end := min(start+uploadChunk, len(data))
		response, err := r.do(ctx, http.MethodPatch, location, bytes.NewReader(data[start:end]), int64(end-start), http.StatusAccepted,
			"Content-Type", "application/octet-stream", "Content-Range", fmt.Sprintf("%d-%d", start, end-1))
		if err == nil {
			response.Body.Close()
			location, start, failures = r.resolve(response.Header.Get("Location")), end, 0
			continue
		}
		if failures++; failures > uploadRetries || ctx.Err() != nil {
			return "", err
		}
		// How far the upload has come: Range 0-<last byte it has>. A registry says 0-0 of an upload that has nothing
		// yet (distribution's), as of one byte: no chunk is one byte but a blob of one, so 0-0 is nothing.
		state, err := r.do(ctx, http.MethodGet, location, nil, 0, http.StatusNoContent)
		if err != nil {
			return "", err
		}
		state.Body.Close()
		_, last, _ := strings.Cut(state.Header.Get("Range"), "-")
		have, _ := strconv.Atoi(last)
		location, start = r.resolve(state.Header.Get("Location")), 0
		if have > 0 {
			start = have + 1
		}
	}
	return blob, r.finish(ctx, location, blob, nil, 0)
}

func digestOf(data []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(data)) }
