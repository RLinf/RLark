package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	registryManifestLimit      = 4 << 20
	registryConfigLimit        = 4 << 20
	registryDetailsConcurrency = 4
	registryImageCacheEntries  = 4096
)

const (
	mediaTypeOCIIndex        = "application/vnd.oci.image.index.v1+json"
	mediaTypeOCIManifest     = "application/vnd.oci.image.manifest.v1+json"
	mediaTypeDockerList      = "application/vnd.docker.distribution.manifest.list.v2+json"
	mediaTypeDockerManifest  = "application/vnd.docker.distribution.manifest.v2+json"
	attestationReferenceType = "vnd.docker.reference.type"
	attestationManifestType  = "attestation-manifest"
)

// registryImageInfo is the immutable metadata derived from one manifest digest.
type registryImageInfo struct {
	Created   *time.Time
	Size      int64
	Platforms []string
}

type registryTagDetail struct {
	Tag       string   `json:"tag"`
	Digest    string   `json:"digest,omitempty"`
	Created   string   `json:"created,omitempty"`
	Size      int64    `json:"size,omitempty"`
	Platforms []string `json:"platforms,omitempty"`
	Error     string   `json:"error,omitempty"`
}

type registryDescriptor struct {
	MediaType   string            `json:"mediaType"`
	Digest      string            `json:"digest"`
	Size        int64             `json:"size"`
	Annotations map[string]string `json:"annotations"`
	Platform    *struct {
		OS           string `json:"os"`
		Architecture string `json:"architecture"`
		Variant      string `json:"variant"`
	} `json:"platform"`
}

type registryManifestDocument struct {
	MediaType string               `json:"mediaType"`
	Config    registryDescriptor   `json:"config"`
	Layers    []registryDescriptor `json:"layers"`
	Manifests []registryDescriptor `json:"manifests"`
}

var errRegistryImageUnavailable = errors.New("image metadata unavailable")

func (g *Gateway) handleRegistryTagDetails(c *gin.Context) {
	credential, ok := g.registryCredential(c)
	if !ok {
		return
	}
	repository, last := c.Query("repository"), c.Query("last")
	if !validRegistryRepository(repository) || (last != "" && !registryTagPattern.MatchString(last)) {
		respondError(c, http.StatusBadRequest, CodeInvalidQueryParameter, "invalid repository or tag cursor")
		return
	}
	browser := g.registryBrowser()
	tags, next, ok := listRegistryTags(c, browser, credential, repository, last)
	if !ok {
		return
	}
	details := make([]registryTagDetail, len(tags))
	work := make(chan int)
	var wg sync.WaitGroup
	for range min(registryDetailsConcurrency, len(tags)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				details[i] = g.describeRegistryTag(c.Request.Context(), browser, credential, repository, tags[i])
			}
		}()
	}
	for i := range tags {
		if c.Request.Context().Err() != nil {
			break
		}
		work <- i
	}
	close(work)
	wg.Wait()
	if err := c.Request.Context().Err(); err != nil {
		respondError(c, http.StatusBadGateway, CodeUpstreamUnavailable, "registry request canceled")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": details, "next": next})
}

func (g *Gateway) describeRegistryTag(ctx context.Context, browser *registryBrowser, credential storedImageRegistryCredential, repository, tag string) registryTagDetail {
	detail := registryTagDetail{Tag: tag}
	body, mediaType, digest, err := fetchRegistryManifest(ctx, browser, credential, repository, tag, "")
	if err != nil {
		detail.Error = "unavailable"
		return detail
	}
	detail.Digest = digest
	key := credential.Registry + "/" + repository + "@" + digest
	cached, found := g.registryImages.Get(key)
	info, _ := cached.(registryImageInfo)
	if !found {
		info, err = resolveRegistryImageInfo(ctx, browser, credential, repository, body, mediaType)
		if err != nil {
			detail.Error = "unavailable"
			return detail
		}
		g.registryImages.Add(key, info)
	}
	if info.Created != nil {
		detail.Created = info.Created.UTC().Format(time.RFC3339)
	}
	detail.Size = info.Size
	detail.Platforms = info.Platforms
	return detail
}

func resolveRegistryImageInfo(ctx context.Context, browser *registryBrowser, credential storedImageRegistryCredential, repository string, body []byte, mediaType string) (registryImageInfo, error) {
	var document registryManifestDocument
	if err := json.Unmarshal(body, &document); err != nil {
		return registryImageInfo{}, errRegistryImageUnavailable
	}
	if document.MediaType != "" {
		mediaType = document.MediaType
	}
	switch mediaType {
	case mediaTypeOCIIndex, mediaTypeDockerList:
		platforms, chosen := indexPlatforms(document.Manifests)
		if chosen == nil {
			return registryImageInfo{}, errRegistryImageUnavailable
		}
		childBody, _, _, err := fetchRegistryManifest(ctx, browser, credential, repository, chosen.Digest, chosen.Digest)
		if err != nil {
			return registryImageInfo{}, err
		}
		var child registryManifestDocument
		if err := json.Unmarshal(childBody, &child); err != nil || (child.MediaType != "" && child.MediaType != mediaTypeOCIManifest && child.MediaType != mediaTypeDockerManifest) {
			return registryImageInfo{}, errRegistryImageUnavailable
		}
		info, err := imageInfoFromManifest(ctx, browser, credential, repository, child)
		if err != nil {
			return registryImageInfo{}, err
		}
		info.Platforms = platforms
		return info, nil
	case mediaTypeOCIManifest, mediaTypeDockerManifest:
		return imageInfoFromManifest(ctx, browser, credential, repository, document)
	default:
		return registryImageInfo{}, errRegistryImageUnavailable
	}
}

// indexPlatforms lists real platforms in an index, skipping attestation
// entries, and picks linux/amd64 or else the first real platform.
func indexPlatforms(manifests []registryDescriptor) ([]string, *registryDescriptor) {
	var platforms []string
	var chosen *registryDescriptor
	for i := range manifests {
		entry := &manifests[i]
		if entry.Platform == nil || entry.Platform.OS == "unknown" || entry.Annotations[attestationReferenceType] == attestationManifestType || !registryDigestPattern.MatchString(entry.Digest) {
			continue
		}
		platforms = append(platforms, platformName(entry.Platform.OS, entry.Platform.Architecture, entry.Platform.Variant))
		if chosen == nil || (entry.Platform.OS == "linux" && entry.Platform.Architecture == "amd64" && !(chosen.Platform.OS == "linux" && chosen.Platform.Architecture == "amd64")) {
			chosen = entry
		}
	}
	return platforms, chosen
}

func platformName(os, architecture, variant string) string {
	name := os + "/" + architecture
	if variant != "" {
		name += "/" + variant
	}
	return name
}

func imageInfoFromManifest(ctx context.Context, browser *registryBrowser, credential storedImageRegistryCredential, repository string, manifest registryManifestDocument) (registryImageInfo, error) {
	if !registryDigestPattern.MatchString(manifest.Config.Digest) || manifest.Config.Size < 0 || manifest.Config.Size > registryConfigLimit {
		return registryImageInfo{}, errRegistryImageUnavailable
	}
	size := manifest.Config.Size
	for _, layer := range manifest.Layers {
		if layer.Size < 0 {
			return registryImageInfo{}, errRegistryImageUnavailable
		}
		size += layer.Size
	}
	response, err := browser.request(ctx, credential, http.MethodGet, "/v2/"+repository+"/blobs/"+manifest.Config.Digest, nil)
	if err != nil {
		return registryImageInfo{}, fmt.Errorf("fetch image config: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return registryImageInfo{}, errRegistryImageUnavailable
	}
	body, err := readLimited(response.Body, registryConfigLimit)
	if err != nil || contentDigest(body) != manifest.Config.Digest {
		return registryImageInfo{}, errRegistryImageUnavailable
	}
	var config struct {
		Created      *time.Time `json:"created"`
		OS           string     `json:"os"`
		Architecture string     `json:"architecture"`
		Variant      string     `json:"variant"`
	}
	if err := json.Unmarshal(body, &config); err != nil {
		return registryImageInfo{}, errRegistryImageUnavailable
	}
	info := registryImageInfo{Created: config.Created, Size: size}
	if config.OS != "" && config.Architecture != "" {
		info.Platforms = []string{platformName(config.OS, config.Architecture, config.Variant)}
	}
	return info, nil
}

// fetchRegistryManifest GETs a manifest by tag or digest and verifies that its
// content matches the digest reported by the registry (or the requested one).
func fetchRegistryManifest(ctx context.Context, browser *registryBrowser, credential storedImageRegistryCredential, repository, reference, expected string) ([]byte, string, string, error) {
	response, err := browser.request(ctx, credential, http.MethodGet, "/v2/"+repository+"/manifests/"+reference, nil)
	if err != nil {
		return nil, "", "", fmt.Errorf("fetch manifest: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, "", "", errRegistryImageUnavailable
	}
	body, err := readLimited(response.Body, registryManifestLimit)
	if err != nil {
		return nil, "", "", err
	}
	digest := contentDigest(body)
	if reported := response.Header.Get("Docker-Content-Digest"); reported != "" && reported != digest {
		return nil, "", "", errRegistryImageUnavailable
	}
	if expected != "" && expected != digest {
		return nil, "", "", errRegistryImageUnavailable
	}
	mediaType, _, _ := strings.Cut(response.Header.Get("Content-Type"), ";")
	return body, strings.TrimSpace(mediaType), digest, nil
}

func readLimited(reader io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read registry response: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, errRegistryImageUnavailable
	}
	return body, nil
}

func contentDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}
