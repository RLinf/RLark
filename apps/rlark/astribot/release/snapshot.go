// Package release holds what a release batch is checked against: a snapshot
// of the management plane, the checks that compare two snapshots, and the
// stages of a batch with the checks each one names. The checks are the
// readings a person took at each step of a rollout; a controller that walks
// the stages calls the same ones.
package release

import (
	"context"
	"fmt"
	"slices"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// ObservationFresh is how old a node's last report may be: station-agent
// reports every 30 s, so two minutes is four reports missed.
const ObservationFresh = 2 * time.Minute

var (
	clusterGVR     = schema.GroupVersionResource{Group: "rlinf.io", Version: "v1alpha1", Resource: "clusters"}
	nodeGVR        = schema.GroupVersionResource{Group: "rlinf.io", Version: "v1alpha1", Resource: "nodes"}
	addonGVR       = schema.GroupVersionResource{Group: "rlinf.io", Version: "v1alpha1", Resource: "addons"}
	siteNetworkGVR = schema.GroupVersionResource{Group: "station.rlark.io", Version: "v1alpha1", Resource: "sitenetworks"}
	enrollmentGVR  = schema.GroupVersionResource{Group: "station.rlark.io", Version: "v1alpha1", Resource: "networkenrollments"}
)

// Snapshot is everything the checks read. A site is never all green, so a
// snapshot taken before a batch is the baseline a later one is compared
// with. Objects of a cluster are keyed "<clusterID>/<name>", which for an
// enrollment is its member id. As JSON its keys are sorted, so two snapshots
// of the same state differ only in takenAt.
type Snapshot struct {
	TakenAt     time.Time   `json:"takenAt"`
	SiteNetwork SiteNetwork `json:"siteNetwork"`
	// Clusters and Nodes hold status.phase.
	Clusters     map[string]string      `json:"clusters"`
	Nodes        map[string]string      `json:"nodes"`
	Enrollments  map[string]Enrollment  `json:"enrollments"`
	Addons       map[string]Addon       `json:"addons"`
	Observations map[string]Observation `json:"observations"`
}

type SiteNetwork struct {
	Ready          bool     `json:"ready"`
	FullyConnected bool     `json:"fullyConnected"`
	Members        int32    `json:"members"`
	Unreachable    []string `json:"unreachable,omitempty"`
}

type Enrollment struct {
	Phase      string   `json:"phase"`
	ConfigHash string   `json:"configHash,omitempty"`
	Problems   []string `json:"problems,omitempty"`
	// Links is hub -> state.
	Links map[string]string `json:"links,omitempty"`
}

type Addon struct {
	Phase   string `json:"phase"`
	Version string `json:"version,omitempty"`
}

type Observation struct {
	ReportedAt time.Time `json:"reportedAt"`
	// Fresh is ReportedAt against the snapshot's own time, so that a
	// baseline says what was fresh when it was taken.
	Fresh bool `json:"fresh"`
	// GatewayLevel is the newest document the node's station-gateway
	// understands, 0 when it states none.
	GatewayLevel int32 `json:"gatewayLevel,omitempty"`
	// Pressures are the pressure conditions of the member Node: empty for a
	// Node under none, null when the report was stored without them.
	Pressures []string `json:"pressures"`
}

// The objects as far as they are read. Station's API types are in a module
// of their own that this one does not import.
type (
	clusterObject struct {
		Spec struct {
			AgentNamespace string `json:"agentNamespace"`
		} `json:"spec"`
		Status struct {
			Phase string `json:"phase"`
		} `json:"status"`
	}
	nodeObject struct {
		Status struct {
			Phase string `json:"phase"`
		} `json:"status"`
	}
	addonObject struct {
		Status Addon `json:"status"`
	}
	siteNetworkObject struct {
		Status struct {
			Members     int32              `json:"members"`
			Unreachable []string           `json:"unreachable"`
			Conditions  []metav1.Condition `json:"conditions"`
		} `json:"status"`
	}
	enrollmentObject struct {
		Status struct {
			Phase      string   `json:"phase"`
			ConfigHash string   `json:"configHash"`
			Problems   []string `json:"problems"`
			Links      []struct {
				Hub   string `json:"hub"`
				State string `json:"state"`
			} `json:"links"`
			// Observation is the node's last report; a node that never reported has none.
			Observation *struct {
				ReportedAt metav1.Time `json:"reportedAt"`
				Gateway    struct {
					DocumentLevel int32 `json:"documentLevel"`
				} `json:"gateway"`
				Pressures []string `json:"pressures"`
			} `json:"observation"`
		} `json:"status"`
	}
)

// Read takes a snapshot of the management plane. Nothing is read from a
// member cluster: what a cluster's agents report is already here.
func Read(ctx context.Context, client dynamic.Interface, now time.Time) (Snapshot, error) {
	snapshot := Snapshot{
		TakenAt:      now.UTC().Truncate(time.Second),
		Clusters:     map[string]string{},
		Nodes:        map[string]string{},
		Enrollments:  map[string]Enrollment{},
		Addons:       map[string]Addon{},
		Observations: map[string]Observation{},
	}

	object, err := client.Resource(siteNetworkGVR).Namespace("station-system").Get(ctx, "default", metav1.GetOptions{})
	if err != nil {
		return Snapshot{}, err
	}
	var network siteNetworkObject
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(object.Object, &network); err != nil {
		return Snapshot{}, fmt.Errorf("SiteNetwork station-system/default: %w", err)
	}
	slices.Sort(network.Status.Unreachable)
	snapshot.SiteNetwork = SiteNetwork{
		Ready:          meta.IsStatusConditionTrue(network.Status.Conditions, "Ready"),
		FullyConnected: meta.IsStatusConditionTrue(network.Status.Conditions, "FullyConnected"),
		Members:        network.Status.Members,
		Unreachable:    network.Status.Unreachable,
	}

	clusters, err := list[clusterObject](ctx, client.Resource(clusterGVR))
	if err != nil {
		return Snapshot{}, err
	}
	for clusterID, cluster := range clusters {
		snapshot.Clusters[clusterID] = cluster.Status.Phase
		namespace := cluster.Spec.AgentNamespace

		nodes, err := list[nodeObject](ctx, client.Resource(nodeGVR).Namespace(namespace))
		if err != nil {
			return Snapshot{}, err
		}
		for name, node := range nodes {
			snapshot.Nodes[clusterID+"/"+name] = node.Status.Phase
		}

		addons, err := list[addonObject](ctx, client.Resource(addonGVR).Namespace(namespace))
		if err != nil {
			return Snapshot{}, err
		}
		for name, addon := range addons {
			snapshot.Addons[clusterID+"/"+name] = addon.Status
		}

		enrollments, err := list[enrollmentObject](ctx, client.Resource(enrollmentGVR).Namespace(namespace))
		if err != nil {
			return Snapshot{}, err
		}
		for name, enrollment := range enrollments {
			status := enrollment.Status
			slices.Sort(status.Problems)
			entry := Enrollment{Phase: status.Phase, ConfigHash: status.ConfigHash, Problems: status.Problems}
			if len(status.Links) > 0 {
				entry.Links = map[string]string{}
			}
			for _, link := range status.Links {
				entry.Links[link.Hub] = link.State
			}
			snapshot.Enrollments[clusterID+"/"+name] = entry
			if observation := status.Observation; observation != nil {
				slices.Sort(observation.Pressures)
				snapshot.Observations[clusterID+"/"+name] = Observation{
					ReportedAt: observation.ReportedAt.UTC(), Fresh: now.Sub(observation.ReportedAt.Time) <= ObservationFresh,
					GatewayLevel: observation.Gateway.DocumentLevel, Pressures: observation.Pressures,
				}
			}
		}
	}
	return snapshot, nil
}

func list[T any](ctx context.Context, resource dynamic.ResourceInterface) (map[string]T, error) {
	objects, err := resource.List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	typed := make(map[string]T, len(objects.Items))
	for _, object := range objects.Items {
		var item T
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(object.Object, &item); err != nil {
			return nil, fmt.Errorf("%s %s/%s: %w", object.GetKind(), object.GetNamespace(), object.GetName(), err)
		}
		typed[object.GetName()] = item
	}
	return typed, nil
}
