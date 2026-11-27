package stationidentity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
)

const (
	// DiscoveryScanLifetime bounds the time Hubs have to return a scan.
	DiscoveryScanLifetime         = 5 * time.Minute
	discoveryScanPrefix           = "discovery-scan.station.rlark.io/"
	discoveryScanResultAnnotation = "station.rlark.io/discovery-scan-result"
)

// DiscoveryScan is the latest one-shot scan for a site. Its target Hub UIDs
// are captured at acceptance so a replacement enrollment cannot complete it.
type DiscoveryScan struct {
	ID        string      `json:"id"`
	StartedAt time.Time   `json:"startedAt"`
	Hubs      []types.UID `json:"hubs"`
}

// DiscoveryScanFor reads the site's latest operation from SiteNetwork metadata.
func DiscoveryScanFor(network *unstructured.Unstructured, site string) (DiscoveryScan, error) {
	var scan DiscoveryScan
	if value := network.GetAnnotations()[discoveryScanPrefix+site]; value != "" {
		if err := json.Unmarshal([]byte(value), &scan); err != nil {
			return DiscoveryScan{}, err
		}
	}
	return scan, nil
}

// StoreDiscoveryScan sets the latest operation for a site on the object to be
// updated with its resource version.
func StoreDiscoveryScan(network *unstructured.Unstructured, site string, scan DiscoveryScan) error {
	value, err := json.Marshal(scan)
	if err != nil {
		return err
	}
	annotations := network.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[discoveryScanPrefix+site] = string(value)
	network.SetAnnotations(annotations)
	return nil
}

// DiscoveryScanHubSite returns the site of an enrolled Hub with a tunnel.
func DiscoveryScanHubSite(hub *unstructured.Unstructured) string {
	role, _, _ := unstructured.NestedString(hub.Object, "status", "role")
	phase, _, _ := unstructured.NestedString(hub.Object, "status", "phase")
	tunnel, _, _ := unstructured.NestedString(hub.Object, "status", "tunnelAddress")
	if role != "Hub" || phase != "Enrolled" || tunnel == "" || hub.GetUID() == "" {
		return ""
	}
	site, _, _ := unstructured.NestedString(hub.Object, "status", "site")
	return site
}

// DiscoveryScanHubResult is the result recorded by Station API for the given run.
func DiscoveryScanHubResult(hub *unstructured.Unstructured, id string) (done, failed bool) {
	value := hub.GetAnnotations()[discoveryScanResultAnnotation]
	return value == id+":ok" || value == id+":failed", value == id+":failed"
}

func (d *NetworkDocuments) pendingDiscoveryScan(ctx context.Context, key string) (DiscoveryScan, *unstructured.Unstructured, error) {
	index, err := d.Enrolled.Lookup(ctx, key)
	if err != nil || index[key] == nil {
		return DiscoveryScan{}, nil, err
	}
	indexed := index[key]
	hub, err := d.Client.Resource(networkEnrollmentGVR).Namespace(indexed.GetNamespace()).Get(ctx, indexed.GetName(), metav1.GetOptions{})
	if err != nil {
		return DiscoveryScan{}, nil, err
	}
	currentKey, _, _ := unstructured.NestedString(hub.Object, "status", "publicKey")
	site := DiscoveryScanHubSite(hub)
	if site == "" || hub.GetUID() != indexed.GetUID() || currentKey != key {
		return DiscoveryScan{}, nil, nil
	}
	network, err := d.Client.Resource(siteNetworkGVR).Namespace("station-system").Get(ctx, "default", metav1.GetOptions{})
	if err != nil {
		return DiscoveryScan{}, nil, err
	}
	sites, _, _ := unstructured.NestedSlice(network.Object, "spec", "sites")
	enabled := false
	for _, raw := range sites {
		if entry, ok := raw.(map[string]any); ok && entry["name"] == site {
			enabled, _, _ = unstructured.NestedBool(entry, "discovery", "enabled")
			break
		}
	}
	if !enabled {
		return DiscoveryScan{}, nil, nil
	}
	scan, err := DiscoveryScanFor(network, site)
	done, _ := DiscoveryScanHubResult(hub, scan.ID)
	if err != nil || scan.ID == "" || done || !slices.Contains(scan.Hubs, hub.GetUID()) || !clock(d.Now).Before(scan.StartedAt.Add(DiscoveryScanLifetime)) {
		return DiscoveryScan{}, nil, err
	}
	return scan, hub, nil
}

// ServeDiscoveryScan returns an outstanding scan only to a Hub that proves
// possession of the key of an enrollment targeted by that operation.
func (d *NetworkDocuments) ServeDiscoveryScan(w http.ResponseWriter, r *http.Request) {
	var request documentRequest
	if !decodeBody(w, r, &request) || !d.proven(w, request) {
		return
	}
	scan, _, err := d.pendingDiscoveryScan(r.Context(), request.PublicKey)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "discovery scan storage unavailable")
		return
	}
	if scan.ID == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		ID       string    `json:"id"`
		Deadline time.Time `json:"deadline"`
	}{scan.ID, scan.StartedAt.Add(DiscoveryScanLifetime)})
}

var errDiscoveryHubChanged = errors.New("the discovery Hub enrollment changed")

func (d *NetworkDocuments) finishDiscoveryScan(ctx context.Context, hub *unstructured.Unstructured, key, id string, failed bool) error {
	enrollments := d.Client.Resource(networkEnrollmentGVR).Namespace(hub.GetNamespace())
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current, err := enrollments.Get(ctx, hub.GetName(), metav1.GetOptions{})
		if err != nil {
			return err
		}
		currentKey, _, _ := unstructured.NestedString(current.Object, "status", "publicKey")
		if current.GetUID() != hub.GetUID() || currentKey != key || DiscoveryScanHubSite(current) != DiscoveryScanHubSite(hub) {
			return errDiscoveryHubChanged
		}
		if done, _ := DiscoveryScanHubResult(current, id); done {
			return nil
		}
		annotations := current.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}
		result := ":ok"
		if failed {
			result = ":failed"
		}
		annotations[discoveryScanResultAnnotation] = id + result
		current.SetAnnotations(annotations)
		_, err = enrollments.Update(ctx, current, metav1.UpdateOptions{})
		return err
	})
}
