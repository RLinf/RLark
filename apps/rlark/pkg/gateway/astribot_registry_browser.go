package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/rlinf/rlark/apps/rlark/pkg/common"
)

var registryRepositoryPattern = regexp.MustCompile(`^[a-z0-9]+(?:(?:[._]|__|[-]+)[a-z0-9]+)*(?:/[a-z0-9]+(?:(?:[._]|__|[-]+)[a-z0-9]+)*)*$`)
var registryTagPattern = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_.-]{0,127}$`)
var registryDigestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

const registryBrowsePageSize = "50"

type registryBrowser struct {
	allowed []string
	client  *http.Client
}

func (g *Gateway) registryCredential(c *gin.Context) (storedImageRegistryCredential, bool) {
	id := c.Param("id")
	if err := validateImageRegistryID(id); err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidQueryParameter, "invalid registry ID")
		return storedImageRegistryCredential{}, false
	}
	secret, err := getImageRegistrySecret(c.Request.Context(), g.rawClient, g.managementNamespace(), id)
	if err != nil {
		if apierrors.IsNotFound(err) {
			respondError(c, http.StatusNotFound, CodeResourceNotFound, "image registry not found")
		} else {
			respondError(c, http.StatusInternalServerError, CodeInternalError, "cannot read image registry")
		}
		return storedImageRegistryCredential{}, false
	}
	var credential storedImageRegistryCredential
	if err := json.Unmarshal(secret.Data[common.ImageRegistryCredentialDataKey], &credential); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, "invalid image registry data")
		return storedImageRegistryCredential{}, false
	}
	return credential, true
}

func (g *Gateway) registryBrowser() *registryBrowser {
	client := g.registryHTTPClient
	if client == nil {
		client = &http.Client{
			Transport:     &http.Transport{Proxy: nil, DisableKeepAlives: true},
			Timeout:       15 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	return &registryBrowser{allowed: g.config.RegistryBrowseAllowedHosts, client: client}
}

func (browser *registryBrowser) request(ctx context.Context, credential storedImageRegistryCredential, method, path string, query url.Values) (*http.Response, error) {
	registry := credential.Registry
	parsed, err := url.Parse("https://" + registry)
	if err != nil || parsed.Host != registry || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Hostname() == "" || !registryHostAllowed(browser.allowed, registry, parsed.Hostname()) {
		return nil, fmt.Errorf("registry browsing is not enabled for this address")
	}
	parsed.Path = path
	parsed.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, method, parsed.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build registry request: %w", err)
	}
	request.SetBasicAuth(credential.Username, credential.Password)
	request.Header.Set("Accept", "application/vnd.oci.image.index.v1+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.docker.distribution.manifest.v2+json")
	return browser.client.Do(request)
}

// registryHostAllowed accepts exact authorities and IP literals in explicitly configured CIDRs.
// It never resolves hostnames for CIDR matching.
func registryHostAllowed(allowed []string, authority, hostname string) bool {
	if slices.Contains(allowed, authority) {
		return true
	}
	address, err := netip.ParseAddr(hostname)
	if err != nil || address.Zone() != "" {
		return false
	}
	for _, item := range allowed {
		prefix, err := netip.ParsePrefix(item)
		if err == nil && prefix.Contains(address) {
			return true
		}
	}
	return false
}

func registryBrowseResponse(c *gin.Context, response *http.Response, err error) bool {
	if err != nil {
		respondError(c, http.StatusBadGateway, CodeUpstreamUnavailable, "registry unavailable or browsing not enabled; check Gateway allowlist, network and TLS trust")
		return false
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		message := "registry request failed or operation is unsupported"
		if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
			message = "registry authentication or permission denied; token authentication is not supported"
		}
		respondError(c, http.StatusBadGateway, CodeUpstreamUnavailable, message)
		return false
	}
	return true
}

func validRegistryRepository(value string) bool {
	return len(value) <= 255 && registryRepositoryPattern.MatchString(value)
}

func (g *Gateway) handleRegistryRepositories(c *gin.Context) {
	credential, ok := g.registryCredential(c)
	if !ok {
		return
	}
	last := c.Query("last")
	if last != "" && !validRegistryRepository(last) {
		respondError(c, http.StatusBadRequest, CodeInvalidQueryParameter, "invalid repository cursor")
		return
	}
	query := url.Values{"n": {registryBrowsePageSize}}
	if last != "" {
		query.Set("last", last)
	}
	response, err := g.registryBrowser().request(c.Request.Context(), credential, http.MethodGet, "/v2/_catalog", query)
	if !registryBrowseResponse(c, response, err) {
		return
	}
	defer response.Body.Close()
	var data struct {
		Repositories *[]string `json:"repositories"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&data); err != nil || data.Repositories == nil || len(*data.Repositories) > 50 {
		respondError(c, http.StatusBadGateway, CodeUpstreamUnavailable, "invalid registry catalog response")
		return
	}
	for _, repository := range *data.Repositories {
		if !validRegistryRepository(repository) {
			respondError(c, http.StatusBadGateway, CodeUpstreamUnavailable, "invalid registry catalog response")
			return
		}
	}
	next := ""
	if strings.Contains(response.Header.Get("Link"), `rel="next"`) {
		if len(*data.Repositories) == 0 || (*data.Repositories)[len(*data.Repositories)-1] == last {
			respondError(c, http.StatusBadGateway, CodeUpstreamUnavailable, "invalid registry catalog cursor")
			return
		}
		next = (*data.Repositories)[len(*data.Repositories)-1]
	}
	c.JSON(http.StatusOK, gin.H{"items": *data.Repositories, "next": next})
}

func (g *Gateway) handleRegistryTags(c *gin.Context) {
	credential, ok := g.registryCredential(c)
	if !ok {
		return
	}
	repository, last := c.Query("repository"), c.Query("last")
	if !validRegistryRepository(repository) || (last != "" && !registryTagPattern.MatchString(last)) {
		respondError(c, http.StatusBadRequest, CodeInvalidQueryParameter, "invalid repository or tag cursor")
		return
	}
	tags, next, ok := listRegistryTags(c, g.registryBrowser(), credential, repository, last)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": tags, "next": next})
}

// listRegistryTags reads one validated tag page. On failure it writes the
// error response and returns false.
func listRegistryTags(c *gin.Context, browser *registryBrowser, credential storedImageRegistryCredential, repository, last string) ([]string, string, bool) {
	query := url.Values{"n": {registryBrowsePageSize}}
	if last != "" {
		query.Set("last", last)
	}
	response, err := browser.request(c.Request.Context(), credential, http.MethodGet, "/v2/"+repository+"/tags/list", query)
	// A repository the catalog lists whose images were only pushed by digest has no tags: the registry answers 404.
	if err == nil && response.StatusCode == http.StatusNotFound {
		response.Body.Close()
		return []string{}, "", true
	}
	if !registryBrowseResponse(c, response, err) {
		return nil, "", false
	}
	defer response.Body.Close()
	var data struct {
		Tags json.RawMessage `json:"tags"`
	}
	var tags []string
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&data); err != nil || len(data.Tags) == 0 || json.Unmarshal(data.Tags, &tags) != nil || len(tags) > 50 {
		respondError(c, http.StatusBadGateway, CodeUpstreamUnavailable, "invalid registry tags response")
		return nil, "", false
	}
	for _, tag := range tags {
		if !registryTagPattern.MatchString(tag) {
			respondError(c, http.StatusBadGateway, CodeUpstreamUnavailable, "invalid registry tags response")
			return nil, "", false
		}
	}
	next := ""
	if strings.Contains(response.Header.Get("Link"), `rel="next"`) {
		if len(tags) == 0 || tags[len(tags)-1] == last {
			respondError(c, http.StatusBadGateway, CodeUpstreamUnavailable, "invalid registry tags cursor")
			return nil, "", false
		}
		next = tags[len(tags)-1]
	}
	if tags == nil {
		tags = []string{}
	}
	return tags, next, true
}

func (g *Gateway) handleRegistryManifest(c *gin.Context) {
	credential, ok := g.registryCredential(c)
	if !ok {
		return
	}
	repository, tag := c.Query("repository"), c.Query("tag")
	if !validRegistryRepository(repository) || !registryTagPattern.MatchString(tag) {
		respondError(c, http.StatusBadRequest, CodeInvalidQueryParameter, "invalid repository or tag")
		return
	}
	response, err := g.registryBrowser().request(c.Request.Context(), credential, http.MethodHead, "/v2/"+repository+"/manifests/"+tag, nil)
	if !registryBrowseResponse(c, response, err) {
		return
	}
	defer response.Body.Close()
	digest := response.Header.Get("Docker-Content-Digest")
	if !registryDigestPattern.MatchString(digest) {
		respondError(c, http.StatusBadGateway, CodeUpstreamUnavailable, "registry did not return a supported manifest digest")
		return
	}
	c.JSON(http.StatusOK, gin.H{"digest": digest})
}
