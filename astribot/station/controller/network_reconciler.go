package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	"github.com/rlinf/rlark/astribot/station/network"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// NetworkReconciler answers every NetworkEnrollment from the one SiteNetwork:
// it allocates addresses, computes each node's peers and routes, and writes
// the result to the enrollment's status, where station-api serves it.
// The whole network is one reconcile key, so allocations never race.
//
// It also says what the Hubs see of every Leaf (status.links). Hubs report
// every 30 s; rather than compute the network on each report, the reconcile
// repeats every linkInterval and reads the reports the Station reconciler's
// cache already holds. A Leaf's status is written when a link changes state
// and, while a link is Unanswered, once per Hub report to keep the counter
// the next report is compared with; a Connected or Silent link writes nothing.
type NetworkReconciler struct {
	client  client.Client
	reader  client.Reader
	network types.NamespacedName
	now     func() time.Time
	// MemberCredentialNamespaces hold the Secrets the management plane
	// reaches the member clusters' APIs with.
	MemberCredentialNamespaces []string
}

const (
	linkInterval = 30 * time.Second
	// A session that is alive has a handshake at most two minutes old; a
	// Leaf whose handshakes fail gives up the old session after three.
	linkHandshakeAge = 3 * time.Minute
	// A Hub that has not reported for this long says nothing about its Leaves.
	linkReportAge = 2 * time.Minute
)

func NewNetworkReconciler(writer client.Client, reader client.Reader, siteNetwork types.NamespacedName, now func() time.Time) *NetworkReconciler {
	return &NetworkReconciler{client: writer, reader: reader, network: siteNetwork, now: now}
}

func (r *NetworkReconciler) SetupWithManager(mgr manager.Manager) error {
	whole := handler.EnqueueRequestsFromMapFunc(func(context.Context, client.Object) []reconcile.Request {
		return []reconcile.Request{{NamespacedName: r.network}}
	})
	return ctrl.NewControllerManagedBy(mgr).Named("site-network").
		For(&stationv1alpha1.SiteNetwork{}).
		Watches(&stationv1alpha1.NetworkEnrollment{}, whole).
		Watches(&stationv1alpha1.NodeAdmission{}, whole).
		Watches(&stationv1alpha1.NodeClass{}, whole).
		Watches(&stationv1alpha1.Machine{}, whole, builder.WithPredicates(predicate.Funcs{
			// Every sighting touches a Machine's status; only the key its
			// entry device shows alters what the hubs look for.
			UpdateFunc: func(e event.UpdateEvent) bool {
				before, okBefore := e.ObjectOld.(*stationv1alpha1.Machine)
				after, okAfter := e.ObjectNew.(*stationv1alpha1.Machine)
				return !okBefore || !okAfter || before.Status.Entry.HostKeyFingerprint != after.Status.Entry.HostKeyFingerprint
			},
		})).
		Watches(&stationv1alpha1.Station{}, whole, builder.WithPredicates(predicate.Funcs{
			// Heartbeats touch every Station's status; only a binding,
			// retirement or phase change alters the network.
			UpdateFunc: func(e event.UpdateEvent) bool {
				before, okBefore := e.ObjectOld.(*stationv1alpha1.Station)
				after, okAfter := e.ObjectNew.(*stationv1alpha1.Station)
				return !okBefore || !okAfter || before.Generation != after.Generation || before.Status.Phase != after.Status.Phase
			},
		})).
		Complete(r)
}

// Reconcile answers every enrollment. station-api writes enrollments while this
// runs; losing such a race is not an error, the network is computed again.
func (r *NetworkReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	if req.NamespacedName != r.network {
		return reconcile.Result{}, nil
	}
	result, err := r.reconcile(ctx)
	if apierrors.IsConflict(err) {
		return reconcile.Result{Requeue: true}, nil
	}
	return result, err
}

func (r *NetworkReconciler) reconcile(ctx context.Context) (reconcile.Result, error) {
	var siteNetwork stationv1alpha1.SiteNetwork
	if err := r.reader.Get(ctx, r.network, &siteNetwork); apierrors.IsNotFound(err) {
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, fmt.Errorf("read SiteNetwork: %w", err)
	}
	var enrollments stationv1alpha1.NetworkEnrollmentList
	if err := r.reader.List(ctx, &enrollments); err != nil {
		return reconcile.Result{}, fmt.Errorf("list NetworkEnrollments: %w", err)
	}
	var stations stationv1alpha1.StationList
	if err := r.reader.List(ctx, &stations); err != nil {
		return reconcile.Result{}, fmt.Errorf("list Stations: %w", err)
	}
	var admissionList stationv1alpha1.NodeAdmissionList
	if err := r.reader.List(ctx, &admissionList); err != nil {
		return reconcile.Result{}, fmt.Errorf("list NodeAdmissions: %w", err)
	}
	sort.Slice(admissionList.Items, func(i, j int) bool {
		a, b := &admissionList.Items[i], &admissionList.Items[j]
		return a.Spec.ClusterID+"/"+a.Name < b.Spec.ClusterID+"/"+b.Name
	})
	sort.Slice(enrollments.Items, func(i, j int) bool {
		a, b := &enrollments.Items[i], &enrollments.Items[j]
		return a.Spec.ClusterID+"/"+a.Name < b.Spec.ClusterID+"/"+b.Name
	})

	spec := siteNetwork.Spec
	if err := r.keepMemberCredentials(ctx, spec); err != nil {
		return reconcile.Result{}, err
	}
	// What the hubs of a site with discovery look for: the factory host keys
	// of every NodeClass, at the SSH ports of their entry devices, and at each
	// site the keys its own machines' entry devices are known by. A machine
	// that joined was given a key of its own; looked for by it, it is seen at
	// whatever address its lease gives it next.
	var classes stationv1alpha1.NodeClassList
	if err := r.reader.List(ctx, &classes); err != nil {
		return reconcile.Result{}, fmt.Errorf("list NodeClasses: %w", err)
	}
	looked := network.Discovery{RelayPort: network.DiscoveryRelayPort}
	for _, class := range classes.Items {
		if class.Spec.Discovery == nil || class.Spec.Access == nil {
			continue
		}
		looked.HostKeys = append(looked.HostKeys, class.Spec.Discovery.HostKeyFingerprints...)
		if port := int(class.Spec.Access.Entry.Port); !slices.Contains(looked.Ports, port) {
			looked.Ports = append(looked.Ports, port)
		}
	}
	slices.Sort(looked.Ports)
	var machines stationv1alpha1.MachineList
	if err := r.reader.List(ctx, &machines, client.InNamespace(r.network.Namespace)); err != nil {
		return reconcile.Result{}, fmt.Errorf("list Machines: %w", err)
	}
	machineKeys := map[string][]string{}
	for _, machine := range machines.Items {
		if key := machine.Status.Entry.HostKeyFingerprint; key != "" {
			machineKeys[machine.Spec.Site] = append(machineKeys[machine.Spec.Site], key)
		}
	}
	siteOfCluster, siteDeclared := map[string]string{}, map[string]bool{}
	topology := network.Topology{Pool: spec.Pool, MTU: int(spec.MTU), Interface: spec.Interface, ListenPort: int(spec.ListenPort), KeepaliveSeconds: int(spec.KeepaliveSeconds), Names: hostNames(spec.Names)}
	for _, site := range spec.Sites {
		names := hostNames(site.Names)
		topology.Sites = append(topology.Sites, network.Site{Name: site.Name, Mesh: site.Mesh, Reaches: site.Reaches, LeavesViaRelay: site.LeavesViaRelay, Relay: site.Relay, Names: names, DNS: site.DNS})
		if site.LeafUplink != nil {
			topology.Sites[len(topology.Sites)-1].LeafUplink = &network.Uplink{Gateway: site.LeafUplink.Gateway}
		}
		if site.Discovery != nil && site.Discovery.Enabled && len(looked.HostKeys) > 0 {
			discovery := looked
			discovery.HostKeys = append(slices.Clone(looked.HostKeys), machineKeys[site.Name]...)
			slices.Sort(discovery.HostKeys)
			discovery.HostKeys = slices.Compact(discovery.HostKeys)
			discovery.Ranges, discovery.IntervalSeconds = site.Discovery.Ranges, int(site.Discovery.IntervalSeconds)
			topology.Sites[len(topology.Sites)-1].DiscoveryFrom = site.Discovery.From.ClusterID + "/" + site.Discovery.From.NodeName
			topology.Sites[len(topology.Sites)-1].Discovery = &discovery
		}
		siteDeclared[site.Name] = true
		for _, cluster := range site.Clusters {
			siteOfCluster[cluster] = site.Name
		}
	}
	declared := map[string]stationv1alpha1.SiteNetworkNode{}
	for _, node := range spec.Nodes {
		declared[node.ClusterID+"/"+node.NodeName] = node
	}
	stationByUID := map[types.UID]*stationv1alpha1.Station{}
	for i := range stations.Items {
		stationByUID[stations.Items[i].UID] = &stations.Items[i]
	}

	// Keep the allocations of identities that still exist; a retired or
	// deleted Station gives its address back.
	allocated := map[string]stationv1alpha1.SiteNetworkAllocation{}
	var taken []string
	for _, allocation := range siteNetwork.Status.Allocations {
		if station := stationByUID[types.UID(allocation.StationUID)]; station != nil && !station.Spec.Retired {
			allocated[allocation.StationUID] = allocation
			taken = append(taken, allocation.Address)
		}
	}
	for _, node := range spec.Nodes {
		if node.Address != "" {
			taken = append(taken, node.Address)
		}
	}

	// An admission is the declaration of a node that joins by a one-time
	// code. Its address is durable in its own status before anything uses it.
	admissions := map[string]*stationv1alpha1.NodeAdmission{}
	// holderOf is the node an address is another node's by: an admission's or an allocation's. A node the
	// SiteNetwork declares at such an address is left out, never let take it from the node that has it.
	holderOf := map[string]string{}
	for _, allocation := range allocated {
		holderOf[allocation.Address] = allocation.ClusterID + "/" + allocation.NodeName
	}
	for i := range admissionList.Items {
		admission := &admissionList.Items[i]
		holder := admission.Spec.ClusterID + "/" + admission.Name
		if admission.Spec.Address != "" {
			taken = append(taken, admission.Spec.Address)
			holderOf[admission.Spec.Address] = holder
		} else if admission.Status.Address != "" {
			taken = append(taken, admission.Status.Address)
			holderOf[admission.Status.Address] = holder
		}
	}
	for i := range admissionList.Items {
		admission := &admissionList.Items[i]
		id := admission.Spec.ClusterID + "/" + admission.Name
		if spec.Mode == stationv1alpha1.SiteNetworkModeRouted {
			continue
		}
		// A node the SiteNetwork declares stays as declared there: its
		// admission only lets a machine in under that declaration.
		node, inSpec := declared[id]
		address := admission.Spec.Address
		if address == "" {
			address = admission.Status.Address
		}
		if inSpec && node.Address != "" {
			address = node.Address
		}
		// A machine that replaces a node keeps the node's address.
		for _, allocation := range allocated {
			if address == "" && allocation.ClusterID == admission.Spec.ClusterID && allocation.NodeName == admission.Name {
				address = allocation.Address
			}
		}
		if address == "" {
			free, err := network.Allocate(spec.Pool, taken)
			if err != nil {
				continue
			}
			address = free
			taken = append(taken, free)
		}
		if admission.Status.Address != address {
			admission.Status.Address = address
			if err := r.client.Status().Update(ctx, admission); err != nil {
				return reconcile.Result{}, fmt.Errorf("record the address of NodeAdmission %s/%s: %w", admission.Namespace, admission.Name, err)
			}
		}
		admissions[id] = admission
		if inSpec {
			continue
		}
		declared[id] = stationv1alpha1.SiteNetworkNode{
			ClusterID: admission.Spec.ClusterID, NodeName: admission.Name, Role: string(network.RoleLeaf),
			Address: address, Uplink: admission.Spec.Uplink, Site: admission.Spec.Site,
		}
	}
	// A node is in the site it names (the SiteNetwork's entry before its
	// admission), or else in its cluster's.
	siteOf := func(clusterID, nodeName string) string {
		id := clusterID + "/" + nodeName
		if site := declared[id].Site; site != "" {
			return site
		}
		if admission := admissions[id]; admission != nil && admission.Spec.Site != "" {
			return admission.Spec.Site
		}
		return siteOfCluster[clusterID]
	}

	admitted := map[string]stationv1alpha1.NodeAdmissionStatus{}
	movedTo := map[string]string{}
	for id, admission := range admissions {
		if admission.Spec.PublicKey != "" && admission.Status.Phase != stationv1alpha1.NodeAdmissionJoined {
			movedTo[admission.Spec.PublicKey] = id
		}
	}
	answers := make(map[string]*stationv1alpha1.NetworkEnrollmentStatus, len(enrollments.Items))
	enrollmentOf := map[string]*stationv1alpha1.NetworkEnrollment{}
	enrolled, declaredOnly := map[string]bool{}, map[string]bool{}
	var members []network.Member
	for i := range enrollments.Items {
		enrollment := &enrollments.Items[i]
		id := enrollment.Spec.ClusterID + "/" + enrollment.Name
		answer := &stationv1alpha1.NetworkEnrollmentStatus{ObservedGeneration: enrollment.Generation, Phase: stationv1alpha1.NetworkEnrollmentRejected, PublicKey: enrollment.Spec.PublicKey}
		answers[id], enrollmentOf[id] = answer, enrollment
		station := stationByUID[types.UID(enrollment.Spec.StationUID)]
		if station == nil || station.Namespace != enrollment.Namespace || station.Spec.Retired || station.Status.Phase == stationv1alpha1.StationPhaseConflict ||
			station.Spec.Binding.NodeUID != enrollment.Spec.NodeUID || station.Spec.Binding.Version != enrollment.Spec.BindingVersion {
			answer.Reason = "the enrollment does not match a current, unretired Station binding"
			continue
		}
		answer.Site = siteOf(enrollment.Spec.ClusterID, enrollment.Name)
		if answer.Site == "" {
			answer.Reason = "cluster " + enrollment.Spec.ClusterID + " is not in any site of the SiteNetwork"
			continue
		}
		if !siteDeclared[answer.Site] {
			answer.Reason = "the node names the site " + answer.Site + ", which the SiteNetwork does not have"
			answer.Site = ""
			continue
		}
		// A machine that redeemed an admission elsewhere has moved: the
		// enrollment it left behind, under the same key, no longer speaks for it.
		if moved := movedTo[enrollment.Spec.PublicKey]; moved != "" && moved != id && enrollment.Spec.PublicKey != "" {
			answer.Reason = "the machine moved to " + moved
			continue
		}
		// A redeemed admission names the machine that now holds this node
		// name; an enrollment with another key is the machine it replaced.
		if admission := admissions[id]; admission != nil && admission.Status.Phase != stationv1alpha1.NodeAdmissionJoined &&
			admission.Spec.PublicKey != "" && admission.Spec.PublicKey != enrollment.Spec.PublicKey {
			answer.Reason = "superseded by the machine admitted under this node name"
			continue
		}
		node := declared[id]
		answer.Role = node.Role
		if answer.Role == "" {
			answer.Role = string(network.RoleLeaf)
		}
		if spec.Mode == stationv1alpha1.SiteNetworkModeRouted {
			answer.Address = node.NodeAddress
			if answer.Address == "" && len(enrollment.Spec.Addresses) > 0 {
				answer.Address = enrollment.Spec.Addresses[0]
			}
			answer.Phase, answer.Reason = stationv1alpha1.NetworkEnrollmentEnrolled, ""
			if answer.Address == "" {
				answer.Phase, answer.Reason = stationv1alpha1.NetworkEnrollmentPending, "the node has not reported an address"
			}
			continue
		}
		if enrollment.Spec.PublicKey == "" {
			answer.Phase, answer.Reason = stationv1alpha1.NetworkEnrollmentPending, "the node has not reported a public key"
			continue
		}
		address := node.Address
		if address == "" {
			allocation, ok := allocated[enrollment.Spec.StationUID]
			if !ok {
				free, err := network.Allocate(spec.Pool, taken)
				if err != nil {
					answer.Phase, answer.Reason = stationv1alpha1.NetworkEnrollmentPending, err.Error()
					continue
				}
				allocation = stationv1alpha1.SiteNetworkAllocation{StationUID: enrollment.Spec.StationUID, Address: free}
				taken = append(taken, free)
			}
			allocation.ClusterID, allocation.NodeName = enrollment.Spec.ClusterID, enrollment.Name
			allocated[enrollment.Spec.StationUID] = allocation
			address = allocation.Address
		}
		member := network.Member{
			ID: id, Site: answer.Site, Role: network.Role(answer.Role), Relay: node.Relay, PublicKey: enrollment.Spec.PublicKey,
			Address: address, NodeAddress: node.NodeAddress, Interface: node.Interface, ListenPort: int(node.ListenPort),
		}
		for _, endpoint := range node.Endpoints {
			if endpoint.Address == "" && len(enrollment.Spec.Addresses) > 0 {
				port := int(node.ListenPort)
				if port == 0 {
					port = int(spec.ListenPort)
				}
				endpoint.Address = net.JoinHostPort(enrollment.Spec.Addresses[0], strconv.Itoa(port))
			}
			member.Endpoints = append(member.Endpoints, network.Endpoint{Address: endpoint.Address, Scope: network.EndpointScope(endpoint.Scope)})
		}
		for _, advertisement := range node.Advertise {
			member.Advertise = append(member.Advertise, network.Advertisement{CIDR: advertisement.CIDR, Sites: advertisement.Sites})
		}
		if node.Uplink != nil {
			member.Uplink = &network.Uplink{Gateway: node.Uplink.Gateway}
		}
		members = append(members, member)
		enrolled[id] = true
	}

	// A declared node with a key takes part before it can enroll: its peers
	// must know it for its tunnel, and with it its path to Station API, to exist.
	// Its own document is published in the SiteNetwork's status (Declared).
	keyDeclared := map[string]bool{}
	var addressTaken []string
	for _, node := range spec.Nodes {
		id := node.ClusterID + "/" + node.NodeName
		if node.PublicKey == "" || node.Address == "" || spec.Mode == stationv1alpha1.SiteNetworkModeRouted || enrolled[id] {
			continue
		}
		if holder := holderOf[node.Address]; holder != "" && holder != id {
			addressTaken = append(addressTaken, fmt.Sprintf("%s is declared at %s, which is %s's", id, node.Address, holder))
			continue
		}
		declaredOnly[id], keyDeclared[id] = true, true
		member := network.Member{
			ID: id, Site: siteOf(node.ClusterID, node.NodeName), Role: network.Role(node.Role), Relay: node.Relay, PublicKey: node.PublicKey,
			Address: node.Address, NodeAddress: node.NodeAddress, Interface: node.Interface, ListenPort: int(node.ListenPort),
		}
		for _, endpoint := range node.Endpoints {
			member.Endpoints = append(member.Endpoints, network.Endpoint{Address: endpoint.Address, Scope: network.EndpointScope(endpoint.Scope)})
		}
		for _, advertisement := range node.Advertise {
			member.Advertise = append(member.Advertise, network.Advertisement{CIDR: advertisement.CIDR, Sites: advertisement.Sites})
		}
		members = append(members, member)
	}

	// A redeemed admission takes part the same way until the node has enrolled
	// under its own identity; the admission then only declares the node.
	for id, admission := range admissions {
		site := siteOf(admission.Spec.ClusterID, admission.Name)
		if admission.Spec.PublicKey == "" || enrolled[id] || declaredOnly[id] || !siteDeclared[site] {
			continue
		}
		node := declared[id]
		declaredOnly[id] = true
		member := network.Member{ID: id, Site: site, Role: network.RoleLeaf, PublicKey: admission.Spec.PublicKey, Address: admission.Status.Address, ListenPort: int(node.ListenPort)}
		if node.Uplink != nil {
			member.Uplink = &network.Uplink{Gateway: node.Uplink.Gateway}
		}
		members = append(members, member)
	}
	sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })

	// Addresses are durable before any node is told to use one.
	status := *siteNetwork.Status.DeepCopy()
	status.Allocations = status.Allocations[:0]
	for _, allocation := range allocated {
		status.Allocations = append(status.Allocations, allocation)
	}
	sort.Slice(status.Allocations, func(i, j int) bool { return status.Allocations[i].StationUID < status.Allocations[j].StationUID })
	if !reflect.DeepEqual(status.Allocations, siteNetwork.Status.Allocations) {
		siteNetwork.Status.Allocations = status.Allocations
		if err := r.client.Status().Update(ctx, &siteNetwork); err != nil {
			return reconcile.Result{}, fmt.Errorf("record address allocations: %w", err)
		}
	}

	ready := metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Computed", Message: "every enrollment is answered from the current topology"}
	if len(addressTaken) > 0 {
		ready = metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse, Reason: "AddressTaken", Message: strings.Join(addressTaken, "; ") + ": left out"}
	}
	connected := metav1.Condition{Type: "FullyConnected", Status: metav1.ConditionTrue, Reason: "AllPairsReachable", Message: "every pair of members has a path"}
	status.Members, status.Unreachable, status.Declared = 0, nil, nil
	var again time.Duration
	var observationList stationv1alpha1.StationObservationList
	if err := r.client.List(ctx, &observationList); err != nil {
		return reconcile.Result{}, fmt.Errorf("list StationObservations: %w", err)
	}
	observations := map[string]*stationv1alpha1.StationObservation{}
	for i := range observationList.Items {
		observation := &observationList.Items[i]
		observations[observation.Annotations["station.rlark.io/cluster-id"]+"/"+observation.Name] = observation
	}
	if spec.Mode != stationv1alpha1.SiteNetworkModeRouted {
		again = linkInterval
		memberOfKey := map[string]string{}
		for _, member := range members {
			memberOfKey[member.PublicKey] = member.ID
		}
		now := r.now()
		result, err := network.Compute(topology, members)
		if err != nil {
			ready = metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse, Reason: "InvalidTopology", Message: err.Error()}
			for _, member := range members {
				if !declaredOnly[member.ID] {
					answers[member.ID].Phase, answers[member.ID].Reason = stationv1alpha1.NetworkEnrollmentPending, "the SiteNetwork is invalid: "+err.Error()
				}
			}
		}
		for _, member := range members {
			answer := answers[member.ID]
			config, ok := result.Configs[member.ID]
			if declaredOnly[member.ID] {
				// Declared ahead of its enrollment: it is in its peers'
				// configurations and has no enrollment to answer.
				if ok {
					status.Members++
				}
				data, hash := document(config)
				if keyDeclared[member.ID] && ok {
					clusterID, nodeName, _ := strings.Cut(member.ID, "/")
					status.Declared = append(status.Declared, stationv1alpha1.SiteNetworkDeclaredConfig{ClusterID: clusterID, NodeName: nodeName, Config: data, ConfigHash: hash})
				}
				if admission := admissions[member.ID]; admission != nil && ok {
					admitted[member.ID] = stationv1alpha1.NodeAdmissionStatus{Config: data, ConfigHash: hash, PublicKey: member.PublicKey}
				}
				continue
			}
			answer.Problems = result.Problems[member.ID]
			answer.Relay = result.Relays[member.ID]
			if !ok {
				if err == nil {
					answer.Reason = "the node is not configurable: " + fmt.Sprint(answer.Problems)
				}
				continue
			}
			answer.Phase, answer.Reason = stationv1alpha1.NetworkEnrollmentEnrolled, ""
			answer.Config, answer.ConfigHash = document(config)
			answer.TunnelAddress, answer.Address = member.Address, member.Address
			if member.NodeAddress != "" {
				answer.Address = member.NodeAddress
			}
			status.Members++
			if member.Role != network.RoleLeaf {
				continue
			}
			// The Hubs a Leaf dials are the peers its configuration gives an
			// endpoint. Each one's latest report says what it sees of the Leaf.
			var unanswered, silent, unknown []string
			var silentSince time.Time
			for _, peer := range config.Peers {
				if peer.Endpoint == "" {
					continue
				}
				link := stationv1alpha1.NetworkEnrollmentLink{Hub: memberOfKey[peer.PublicKey], State: stationv1alpha1.NetworkLinkUnknown}
				var previous stationv1alpha1.NetworkEnrollmentLink
				for _, stored := range enrollmentOf[member.ID].Status.Links {
					if stored.Hub == link.Hub {
						previous = stored
					}
				}
				// What is kept of a report's time is kept to the second, as
				// the API server stores it.
				var seen *stationv1alpha1.WireguardPeerObservation
				var reported metav1.Time
				if observation := observations[link.Hub]; observation != nil && observation.Status.Wireguard != nil && now.Sub(observation.Status.ReportedAt.Time) < linkReportAge {
					reported = observation.Status.ReportedAt.Rfc3339Copy()
					for i := range observation.Status.Wireguard.Peers {
						if observation.Status.Wireguard.Peers[i].Name == member.ID {
							seen = &observation.Status.Wireguard.Peers[i]
						}
					}
				}
				stale := link
				if seen != nil {
					stale.LatestHandshake, stale.ReceivedBytes, stale.ObservedAt = seen.LatestHandshake, seen.ReceivedBytes, &reported
				}
				switch {
				case seen == nil:
				case seen.LatestHandshake != nil && now.Sub(seen.LatestHandshake.Time) < linkHandshakeAge:
					link.State = stationv1alpha1.NetworkLinkConnected
				case previous.ObservedAt == nil || seen.ReceivedBytes < previous.ReceivedBytes:
					// The first report with a stale handshake, or with a
					// counter the hub started again, only gives the counter
					// to compare the next one with: until then the link
					// keeps the state it had.
					link = stale
					if previous.State != "" {
						link.State = previous.State
					}
				case !reported.After(previous.ObservedAt.Time):
					link = previous
				case seen.ReceivedBytes > previous.ReceivedBytes:
					link = stale
					link.State = stationv1alpha1.NetworkLinkUnanswered
				case seen.ReceivedBytes == previous.ReceivedBytes && previous.State == stationv1alpha1.NetworkLinkSilent:
					// Nothing moved: the stored counter still serves, and the
					// status is not written again for a newer report time.
					link = previous
				default:
					link = stale
					link.State = stationv1alpha1.NetworkLinkSilent
				}
				link.Since = previous.Since
				if link.State != previous.State {
					link.Since = metav1.NewTime(now)
				}
				switch link.State {
				case stationv1alpha1.NetworkLinkUnanswered:
					unanswered = append(unanswered, link.Hub)
				case stationv1alpha1.NetworkLinkSilent:
					silent = append(silent, link.Hub)
					if link.Since.After(silentSince) {
						silentSince = link.Since.Time
					}
				case stationv1alpha1.NetworkLinkUnknown:
					unknown = append(unknown, link.Hub)
				}
				answer.Links = append(answer.Links, link)
			}
			// The summary names hubs and, at most, the time the last one went
			// silent, so it changes only when a link changes state.
			var lines []string
			if len(unanswered) > 0 {
				lines = append(lines, "this node's requests arrive at "+strings.Join(unanswered, ", ")+" and no session forms: the answers do not get back to it")
			}
			if len(silent) > 0 && len(silent) == len(answer.Links) {
				lines = append(lines, "nothing arrives from this node at any hub it dials since "+silentSince.UTC().Format(time.RFC3339))
			} else if len(silent) > 0 {
				lines = append(lines, "nothing arrives from this node at "+strings.Join(silent, ", "))
			}
			if len(unknown) > 0 {
				lines = append(lines, "no current report of this node from "+strings.Join(unknown, ", "))
			}
			answer.LinkSummary = strings.Join(lines, "; ")
		}
		if len(result.Unreachable) > 0 {
			status.Unreachable = result.Unreachable[:min(32, len(result.Unreachable))]
			connected = metav1.Condition{Type: "FullyConnected", Status: metav1.ConditionFalse, Reason: "UnreachablePairs", Message: strconv.Itoa(len(result.Unreachable)) + " ordered pairs of members have no path"}
		}
	} else {
		for _, answer := range answers {
			if answer.Phase == stationv1alpha1.NetworkEnrollmentEnrolled {
				status.Members++
			}
		}
	}

	for i := range enrollments.Items {
		enrollment := &enrollments.Items[i]
		answer := answers[enrollment.Spec.ClusterID+"/"+enrollment.Name]
		if reflect.DeepEqual(*answer, enrollment.Status) {
			continue
		}
		enrollment.Status = *answer
		if err := r.client.Status().Update(ctx, enrollment); err != nil {
			return reconcile.Result{}, fmt.Errorf("answer NetworkEnrollment %s/%s: %w", enrollment.Namespace, enrollment.Name, err)
		}
	}

	for id, admission := range admissions {
		answer := admitted[id]
		answer.ObservedGeneration, answer.Address = admission.Generation, admission.Status.Address
		key, previous := admission.Spec.PublicKey, admission.Status
		// A machine that redeems a code as the node it already is, with the
		// key it is enrolled with, joins again: its cluster has a new control
		// plane. Its admission answers with the configuration it has until
		// the node has enrolled from there, under a new Node UID; until then
		// the admission also keeps the node among its peers while its Station
		// is rebound. Whether it is that case is seen once, when the
		// redemption is first answered.
		enrollment := enrollmentOf[id]
		sameKey := key != "" && enrollment != nil && enrollment.Spec.PublicKey == key
		rejoins := previous.RejoinsNodeUID
		if key != "" && !admission.Spec.RedeemedAt.Equal(previous.RedeemedAt) {
			rejoins = ""
			if sameKey {
				rejoins = enrollment.Spec.NodeUID
			}
		}
		if key != "" {
			answer.RedeemedAt = admission.Spec.RedeemedAt
		}
		switch {
		case key != "" && rejoins != "" && !(enrolled[id] && sameKey && enrollment.Spec.NodeUID != rejoins):
			answer.Phase, answer.RejoinsNodeUID = stationv1alpha1.NodeAdmissionRedeemed, rejoins
			if enrolled[id] {
				answer.Config, answer.ConfigHash, answer.PublicKey = answers[id].Config, answers[id].ConfigHash, key
			}
		case key != "" && (enrolled[id] || previous.Phase == stationv1alpha1.NodeAdmissionJoined):
			answer = stationv1alpha1.NodeAdmissionStatus{ObservedGeneration: admission.Generation, Address: admission.Status.Address, Phase: stationv1alpha1.NodeAdmissionJoined, RedeemedAt: answer.RedeemedAt}
		case answer.Config != "":
			answer.Phase = stationv1alpha1.NodeAdmissionRedeemed
		case admission.Spec.PublicKey != "":
			answer.Phase, answer.Reason = stationv1alpha1.NodeAdmissionRedeemed, "the node is not configurable in the current SiteNetwork"
		case !r.now().Before(admission.Spec.ExpiresAt.Time):
			answer.Phase = stationv1alpha1.NodeAdmissionExpired
		default:
			answer.Phase = stationv1alpha1.NodeAdmissionPending
		}
		if answer.Phase == stationv1alpha1.NodeAdmissionRedeemed && answer.Reason == "" {
			answer.Reason = joinStep(admission, observations[id], stations.Items, enrollmentOf[id])
		}
		// A machine admitted into a site nothing declares never gets as far
		// as an enrollment that could be rejected: its admission says it.
		if site := siteOf(admission.Spec.ClusterID, admission.Name); site != "" && !siteDeclared[site] && answer.Phase != stationv1alpha1.NodeAdmissionJoined {
			answer.Reason = "the node names the site " + site + ", which the SiteNetwork does not have"
		}
		if reflect.DeepEqual(answer, admission.Status) {
			continue
		}
		admission.Status = answer
		if err := r.client.Status().Update(ctx, admission); err != nil {
			return reconcile.Result{}, fmt.Errorf("answer NodeAdmission %s/%s: %w", admission.Namespace, admission.Name, err)
		}
	}

	// A hub of a site with discovery that has stopped reporting what it finds:
	// the list of machines that have not joined is no longer that site's of now.
	stale := metav1.Condition{Type: "DiscoveryStale", Status: metav1.ConditionFalse, Reason: "HubsReport", Message: "every hub of a site with discovery reports what it finds"}
	looking := false
	for _, site := range topology.Sites {
		if site.Discovery == nil {
			continue
		}
		interval := time.Duration(site.Discovery.IntervalSeconds) * time.Second
		if again == 0 || interval < again {
			again = interval
		}
		for i := range enrollments.Items {
			hub := &enrollments.Items[i]
			if hub.Status.Site != site.Name || hub.Status.Role != string(network.RoleHub) || hub.Status.Phase != stationv1alpha1.NetworkEnrollmentEnrolled {
				continue
			}
			looking = true
			reported, err := time.Parse(time.RFC3339, hub.Annotations[DiscoveryReportedAnnotation])
			last := "never"
			if err == nil {
				last = "last at " + reported.UTC().Format(time.RFC3339)
			}
			if err != nil || r.now().Sub(reported) > 3*interval {
				message := fmt.Sprintf("site %s: hub %s/%s has not reported what it finds for three intervals (%s)", site.Name, hub.Spec.ClusterID, hub.Name, last)
				if stale.Status == metav1.ConditionTrue {
					message = stale.Message + "; " + message
				}
				stale.Status, stale.Reason, stale.Message = metav1.ConditionTrue, "HubSilent", message
			}
		}
	}
	conditions := []metav1.Condition{ready, connected}
	if looking {
		conditions = append(conditions, stale)
	} else {
		apimeta.RemoveStatusCondition(&status.Conditions, "DiscoveryStale")
	}
	status.ObservedGeneration = siteNetwork.Generation
	for _, condition := range conditions {
		condition.ObservedGeneration = siteNetwork.Generation
		condition.LastTransitionTime = metav1.NewTime(r.now())
		apimeta.SetStatusCondition(&status.Conditions, condition)
	}
	if !reflect.DeepEqual(status, siteNetwork.Status) {
		siteNetwork.Status = status
		if err := r.client.Status().Update(ctx, &siteNetwork); err != nil {
			return reconcile.Result{}, fmt.Errorf("update SiteNetwork status: %w", err)
		}
	}
	return reconcile.Result{RequeueAfter: again}, nil
}

// JoinTokenIssuedAnnotation is, on a NodeAdmission, when Station API last gave
// the machine that redeemed it a token to join its cluster with.
const JoinTokenIssuedAnnotation = "station.rlark.io/join-token-issued-at"

// joinStep says where the join of a redeemed admission that has its tunnel
// stands, from what the management plane holds of it: the console shows it,
// and offers to clear the name and start again.
func joinStep(admission *stationv1alpha1.NodeAdmission, observation *stationv1alpha1.StationObservation, stations []stationv1alpha1.Station, enrollment *stationv1alpha1.NetworkEnrollment) string {
	token, _ := time.Parse(time.RFC3339, admission.Annotations[JoinTokenIssuedAnnotation])
	var step string
	switch {
	case token.IsZero() || admission.Spec.RedeemedAt != nil && token.Before(admission.Spec.RedeemedAt.Time):
		step = "机器拿到了隧道配置，还没回来取入网令牌：机器上的安装程序没在跑，或连不上 Station API"
	case observation == nil || observation.Status.ReportedAt.Time.Before(token):
		step = "已发入网令牌（" + token.UTC().Format(time.RFC3339) + "），这个名字还没有节点上报：kubeadm join 没成，或节点上的 station-agent 没起来（看安装输出，SSH 入网的在机器的 /var/log/rlark-join.log）"
	default:
		var bound, named *stationv1alpha1.Station
		for i := range stations {
			station := &stations[i]
			if station.Namespace != admission.Namespace || station.Spec.Retired {
				continue
			}
			if station.Spec.Binding.NodeUID == observation.Status.NodeUID {
				bound = station
			} else if station.Spec.Binding.NodeName == admission.Name {
				named = station
			}
		}
		switch {
		case bound == nil && named != nil:
			step = "节点已上报（UID " + observation.Status.NodeUID + "），场站 " + named.Spec.DisplayName + " 还绑在这个名字以前的节点上"
			if condition := apimeta.FindStatusCondition(named.Status.Conditions, "IdentityMatched"); condition != nil && condition.Status == metav1.ConditionFalse {
				step += "：" + condition.Message
			}
		case bound == nil:
			step = "节点已上报（UID " + observation.Status.NodeUID + "），场站身份还没建成"
		case enrollment != nil && enrollment.Status.Phase != stationv1alpha1.NetworkEnrollmentEnrolled && enrollment.Status.Reason != "":
			step = "节点和场站 " + bound.Spec.DisplayName + " 都有了，隧道登记还没通过：" + enrollment.Status.Reason
		default:
			step = "节点和场站 " + bound.Spec.DisplayName + " 都有了，等节点以自己的身份登记隧道"
		}
	}
	return step
}

// DiscoveryReportedAnnotation is, on the NetworkEnrollment of a hub, when the
// hub last reported what it found on its site's network (Station API writes
// it; the enrollment's status is this controller's answer and is written whole).
const DiscoveryReportedAnnotation = "station.rlark.io/discovery-reported-at"

// document is a node's configuration as it is served, and its SHA-256.
func document(config network.NodeConfig) (string, string) {
	data, _ := json.Marshal(config)
	digest := sha256.Sum256(data)
	return string(data), hex.EncodeToString(digest[:])
}

func hostNames(declared []stationv1alpha1.SiteNetworkName) []network.HostName {
	var names []network.HostName
	for _, name := range declared {
		names = append(names, network.HostName(name))
	}
	return names
}
