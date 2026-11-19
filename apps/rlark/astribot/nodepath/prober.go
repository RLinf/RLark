package nodepath

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

const (
	// StorageIDLabel on a StorageClass names the storage it belongs to.
	StorageIDLabel = "storage.rlark.io/id"
	// NodeLabelPrefix + a storage's id is the node label the node plugin
	// sets to "true" while the node has the storage, and removes otherwise.
	// RLark's storage View selects nodes by it.
	NodeLabelPrefix = "storage.rlark.io/"

	reasonAvailable   = "StorageAvailable"
	reasonUnavailable = "StorageUnavailable"
	reasonRemounted   = "StorageRemounted"
)

// NodeLabel is the node label of a storage.
func NodeLabel(storageID string) string { return NodeLabelPrefix + storageID }

// Published is a target the node plugin bound, and what it bound.
type Published struct {
	Target       string `json:"target"`
	Root         string `json:"root"`
	FSType       string `json:"fsType,omitempty"`
	Device       string `json:"device"`
	PodNamespace string `json:"podNamespace,omitempty"`
	PodName      string `json:"podName,omitempty"`
	// Reported is set once the Pod was told its mount went stale.
	Reported bool `json:"reported,omitempty"`
}

// PublishedStore keeps the records in a directory that outlives the plugin
// (the plugin's directory on the node), one file per target.
type PublishedStore struct{ Dir string }

func (s *PublishedStore) file(target string) string {
	sum := sha256.Sum256([]byte(target))
	return filepath.Join(s.Dir, hex.EncodeToString(sum[:12])+".json")
}

func (s *PublishedStore) Put(record Published) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	temporary := s.file(record.Target) + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, s.file(record.Target))
}

func (s *PublishedStore) Delete(target string) error {
	if err := os.Remove(s.file(target)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *PublishedStore) List() ([]Published, error) {
	paths, err := filepath.Glob(filepath.Join(s.Dir, "*.json"))
	if err != nil {
		return nil, err
	}
	records := make([]Published, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var record Published
		if err := json.Unmarshal(data, &record); err != nil {
			return nil, fmt.Errorf("decode %s: %w", path, err)
		}
		records = append(records, record)
	}
	return records, nil
}

// Prober is the node plugin's report of the node's storages. Every interval
// it checks each host-path storage that has a StorageClass of this driver the
// way NodePublishVolume does, keeps the node's storage labels to what it
// found (so a node that mounts KPFS, also a new one, is chosen without any
// object changing, and one that lost it is not), records each change as a
// Node event, and tells a Pod whose bound root was remounted on the node
// (which a bind does not follow) with an event on the Pod and on the Node.
type Prober struct {
	Client    kubernetes.Interface
	Node      string
	HostRoot  string
	Mounts    MountTable
	Published *PublishedStore
	Now       func() time.Time

	// last is each storage's previous finding: "" when available, the
	// reason otherwise.
	last map[string]string
}

type probedStorage struct {
	id, root, fsType string
}

// Run probes until ctx ends. A failed probe is retried at the next tick.
func (p *Prober) Run(ctx context.Context, interval time.Duration, logf func(string, ...any)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := p.Probe(ctx); err != nil {
			logf("probe storages: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Probe checks the storages once.
func (p *Prober) Probe(ctx context.Context) error {
	classes, err := p.Client.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{LabelSelector: StorageIDLabel})
	if err != nil {
		return fmt.Errorf("list StorageClasses: %w", err)
	}
	storages := map[string]probedStorage{}
	for _, class := range classes.Items {
		if class.Provisioner != Name {
			continue
		}
		id := class.Labels[StorageIDLabel]
		storages[id] = probedStorage{id: id, root: class.Parameters[ParameterRoot], fsType: class.Parameters[ParameterFSType]}
	}
	mounts, err := p.Mounts()
	if err != nil {
		return fmt.Errorf("read mount table: %w", err)
	}
	findings := make(map[string]string, len(storages))
	for id, storage := range storages {
		findings[id] = ""
		if _, err := CheckRoot(mounts, p.HostRoot, p.Node, storage.root, storage.fsType); err != nil {
			findings[id] = err.Error()
		}
	}
	if err := p.label(ctx, findings); err != nil {
		return err
	}
	if p.last == nil {
		p.last = map[string]string{}
	}
	for _, id := range slices.Sorted(maps.Keys(findings)) {
		previous, seen := p.last[id]
		finding := findings[id]
		if seen && previous == finding || !seen && finding == "" {
			continue
		}
		if finding == "" {
			p.event(ctx, p.nodeRef(), corev1.EventTypeNormal, reasonAvailable, fmt.Sprintf("storage %s (%s) is available on node %s", id, storages[id].root, p.Node))
		} else {
			p.event(ctx, p.nodeRef(), corev1.EventTypeWarning, reasonUnavailable, fmt.Sprintf("storage %s: %s", id, finding))
		}
	}
	p.last = findings
	return p.reportStale(ctx, mounts)
}

// label sets the node's storage labels to the findings.
func (p *Prober) label(ctx context.Context, findings map[string]string) error {
	node, err := p.Client.CoreV1().Nodes().Get(ctx, p.Node, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get node %s: %w", p.Node, err)
	}
	changes := map[string]any{}
	for key := range node.Labels {
		id, found := strings.CutPrefix(key, NodeLabelPrefix)
		if !found {
			continue
		}
		// A storage that is gone or that this node does not have.
		if finding, known := findings[id]; !known || finding != "" {
			changes[key] = nil
		}
	}
	for id, finding := range findings {
		if finding == "" && node.Labels[NodeLabel(id)] != "true" {
			changes[NodeLabel(id)] = "true"
		}
	}
	if len(changes) == 0 {
		return nil
	}
	patch, err := json.Marshal(map[string]any{"metadata": map[string]any{"labels": changes}})
	if err != nil {
		return err
	}
	if _, err := p.Client.CoreV1().Nodes().Patch(ctx, p.Node, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("label node %s: %w", p.Node, err)
	}
	return nil
}

// reportStale tells each Pod whose bound root is no longer the node's mount.
func (p *Prober) reportStale(ctx context.Context, mounts []Mount) error {
	records, err := p.Published.List()
	if err != nil {
		return fmt.Errorf("list published volumes: %w", err)
	}
	for _, record := range records {
		if record.Reported {
			continue
		}
		current, err := CheckRoot(mounts, p.HostRoot, p.Node, record.Root, record.FSType)
		if err == nil && current.Device == record.Device {
			continue
		}
		reason := fmt.Sprintf("the node's mount of %s changed after this Pod bound it (device %s, now ", record.Root, record.Device)
		if err != nil {
			reason += "not mounted: " + err.Error()
		} else {
			reason += "device " + current.Device
		}
		reason += "); the Pod still sees the old mount: restart the Pod"
		if record.PodName != "" {
			p.event(ctx, corev1.ObjectReference{Kind: "Pod", APIVersion: "v1", Namespace: record.PodNamespace, Name: record.PodName}, corev1.EventTypeWarning, reasonRemounted, reason)
		}
		p.event(ctx, p.nodeRef(), corev1.EventTypeWarning, reasonRemounted, fmt.Sprintf("pod %s/%s: %s", record.PodNamespace, record.PodName, reason))
		record.Reported = true
		if err := p.Published.Put(record); err != nil {
			return err
		}
	}
	return nil
}

func (p *Prober) nodeRef() corev1.ObjectReference {
	return corev1.ObjectReference{Kind: "Node", APIVersion: "v1", Name: p.Node, UID: types.UID(p.Node)}
}

// event records an event; a failure only loses the event, never the probe.
func (p *Prober) event(ctx context.Context, object corev1.ObjectReference, eventType, reason, message string) {
	namespace := object.Namespace
	if namespace == "" {
		namespace = metav1.NamespaceDefault
	}
	at := p.Now()
	now := metav1.NewTime(at)
	_, _ = p.Client.CoreV1().Events(namespace).Create(ctx, &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: fmt.Sprintf("%s.%x", object.Name, at.UnixNano()), Namespace: namespace},
		InvolvedObject: object, Type: eventType, Reason: reason, Message: message,
		Source:         corev1.EventSource{Component: Name, Host: p.Node},
		FirstTimestamp: now, LastTimestamp: now, Count: 1,
	}, metav1.CreateOptions{})
}
