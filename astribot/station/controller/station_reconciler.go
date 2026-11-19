package controller

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// SequenceName is the namespace-local StationSequence singleton name.
	SequenceName = "sequence"
	// DefaultOfflineAfter is the heartbeat freshness window in the API contract.
	DefaultOfflineAfter = 90 * time.Second
	sequenceCASAttempts = 8
	// rebootMargin is how much later a reported boot time must be to count as a reboot.
	rebootMargin = 30 * time.Second
)

var (
	// ErrInvalidRegistration indicates that a namespace is not mapped to the supplied cluster.
	ErrInvalidRegistration = errors.New("invalid cluster registration")
	// ErrSequenceOwnerConflict prevents allocating IDs from another cluster's sequence.
	ErrSequenceOwnerConflict = errors.New("station sequence belongs to another cluster")
	// ErrStationNameCollision is returned when a deterministic ID is occupied by another binding.
	ErrStationNameCollision = errors.New("deterministic station ID collides with another binding")
)

// ClusterRegistration is the trusted mapping from the agent namespace to a stable cluster ID.
type ClusterRegistration struct {
	ClusterID string
	Namespace string
}

// ClusterResolver returns only trusted server-side registration data.
type ClusterResolver interface {
	ResolveNamespace(context.Context, string) (ClusterRegistration, error)
}

// NodeReadiness reads the RLark Node projection for the registered cluster.
type NodeReadiness interface {
	Ready(context.Context, string, string, string) (bool, error)
}

// StationReconciler turns the latest server-validated observation into durable Station state.
type StationReconciler struct {
	client        client.Client
	reader        client.Reader
	registrations ClusterResolver
	identityOps   IdentityOperationCoordinator
	nodeReadiness NodeReadiness
	offlineAfter  time.Duration
	now           func() time.Time
}

// NewStationReconciler constructs a reconciler with explicit writer and uncached reader dependencies.
func NewStationReconciler(writer client.Client, reader client.Reader, registrations ClusterResolver, identityOps IdentityOperationCoordinator, readiness NodeReadiness, offlineAfter time.Duration, now func() time.Time) *StationReconciler {
	return &StationReconciler{
		client: writer, reader: reader, registrations: registrations, identityOps: identityOps,
		nodeReadiness: readiness, offlineAfter: offlineAfter, now: now,
	}
}

// SetupWithManager watches observation changes and uses uncached reads for reconciliation.
func (r *StationReconciler) SetupWithManager(mgr manager.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&stationv1alpha1.StationObservation{}).
		Watches(&stationv1alpha1.Station{}, handler.EnqueueRequestsFromMapFunc(func(_ context.Context, object client.Object) []reconcile.Request {
			station, ok := object.(*stationv1alpha1.Station)
			if !ok || station.Spec.Binding.NodeName == "" {
				return nil
			}
			return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: station.Namespace, Name: station.Spec.Binding.NodeName}}}
		})).
		Complete(r)
}

// Compile-time assertion: StationObservation events are the reconciliation key;
// the handler always rereads the current object and related Stations.
var _ reconcile.Reconciler = (*StationReconciler)(nil)

// Reconcile rereads the current observation, registration and Station set before writing state.
func (r *StationReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	release, err := r.identityOps.Acquire(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("acquire Station identity operation lock: %w", err)
	}
	defer release()

	registration, err := r.registrations.ResolveNamespace(ctx, req.Namespace)
	if err != nil {
		return reconcile.Result{}, err
	}

	var observation stationv1alpha1.StationObservation
	err = r.reader.Get(ctx, req.NamespacedName, &observation)
	if apierrors.IsNotFound(err) {
		return reconcile.Result{}, r.markNodeOffline(ctx, req.Namespace, req.Name)
	}
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("read station observation: %w", err)
	}
	if observation.Status.NodeUID == "" || observation.Status.ReportedAt.IsZero() {
		return reconcile.Result{}, errors.New("observation is missing server-validated identity or receipt time")
	}

	var stations stationv1alpha1.StationList
	if err := r.reader.List(ctx, &stations, client.InNamespace(req.Namespace)); err != nil {
		return reconcile.Result{}, fmt.Errorf("list stations: %w", err)
	}
	byUID, byNodeName := make([]stationv1alpha1.Station, 0, 1), make([]stationv1alpha1.Station, 0, 1)
	var deterministic *stationv1alpha1.Station
	name := StationName(registration.ClusterID, observation.Status.NodeUID)
	for i := range stations.Items {
		station := &stations.Items[i]
		if station.Spec.Binding.NodeUID == observation.Status.NodeUID {
			byUID = append(byUID, *station.DeepCopy())
		}
		// A retired Station's node was removed: its name is free for the next machine.
		if station.Spec.Binding.NodeName == observation.Name && !station.Spec.Retired {
			byNodeName = append(byNodeName, *station.DeepCopy())
		}
		if station.Name == name {
			deterministic = station.DeepCopy()
		}
	}

	if len(byUID) > 1 || len(byNodeName) > 1 {
		conflicts := uniqueStations(append(byUID, byNodeName...))
		for i := range conflicts {
			if err := r.setConflict(ctx, &conflicts[i], "AmbiguousBinding", "multiple Stations claim this Node identity"); err != nil {
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, nil
	}
	if len(byUID) == 1 {
		station := &byUID[0]
		if station.Spec.Retired {
			return reconcile.Result{}, r.setPhase(ctx, station, stationv1alpha1.StationPhaseRetired)
		}
		if station.Spec.Binding.NodeName != observation.Name || len(byNodeName) != 1 {
			if err := r.setConflict(ctx, station, "NodeIdentityMismatch", "verified Node identity conflicts with the existing binding"); err != nil {
				return reconcile.Result{}, err
			}
			for i := range byNodeName {
				if byNodeName[i].UID != station.UID {
					if err := r.setConflict(ctx, &byNodeName[i], "NodeIdentityMismatch", "another Station is bound to this Node name"); err != nil {
						return reconcile.Result{}, err
					}
				}
			}
			return reconcile.Result{}, nil
		}
		return r.observeStation(ctx, registration, station, &observation)
	}
	if len(byNodeName) != 0 {
		return r.replacedNode(ctx, &byNodeName[0], &observation)
	}
	if deterministic != nil {
		if deterministic.Spec.Binding.NodeUID != observation.Status.NodeUID || deterministic.Spec.Binding.NodeName != observation.Name {
			return reconcile.Result{}, ErrStationNameCollision
		}
		return r.observeStation(ctx, registration, deterministic, &observation)
	}

	return r.createStation(ctx, registration, &observation)
}

// SelfHealedAnnotation is, on a NodeAdmission, what RLark found of the
// machine's earlier attempt or earlier node and put right by itself, as the
// console shows it. Station API writes it too.
const SelfHealedAnnotation = "station.rlark.io/self-healed"

// SelfHealRequester is who RLark's own rebinds are requested by.
const SelfHealRequester = "station-controller"

// replacedNode answers a Station bound to a Node of this name that is gone:
// station-api verified the name with another UID, and a name holds one Node.
// A machine an administrator admitted under the name since that node was
// last seen (its NodeAdmission was redeemed after) is the node the name
// stands for now. If it is the machine the Station was of (the same system
// UUID), the Station is rebound to it, through a StationRebind like an
// administrator's; if it is another machine, the Station of the gone node is
// retired and the new node gets one of its own. A node of this name that no
// admission let in is left to an administrator.
func (r *StationReconciler) replacedNode(ctx context.Context, old *stationv1alpha1.Station, observation *stationv1alpha1.StationObservation) (reconcile.Result, error) {
	admission := &stationv1alpha1.NodeAdmission{}
	err := r.reader.Get(ctx, types.NamespacedName{Namespace: old.Namespace, Name: observation.Name}, admission)
	if err != nil && !apierrors.IsNotFound(err) {
		return reconcile.Result{}, fmt.Errorf("read NodeAdmission %s: %w", observation.Name, err)
	}
	admitted := err == nil && admission.Spec.RedeemedAt != nil && (old.Status.LastSeenTime == nil || admission.Spec.RedeemedAt.After(old.Status.LastSeenTime.Time))
	if !admitted {
		return reconcile.Result{}, r.setConflict(ctx, old, "NodeUIDChanged", fmt.Sprintf("Node %s now has UID %s, and no join code or SSH join admitted a machine under the name since the node of this Station was last seen; remove the node and join it again, or rebind the Station", observation.Name, observation.Status.NodeUID))
	}
	var healed string
	if uuid := observation.Status.SystemUUID; uuid != "" && uuid == old.Spec.Binding.SystemUUID {
		digest := sha256.Sum256([]byte(string(old.UID) + "/" + observation.Status.NodeUID))
		id := "self-heal-" + hex.EncodeToString(digest[:10])
		rebind := &stationv1alpha1.StationRebind{
			ObjectMeta: metav1.ObjectMeta{Name: id, Namespace: old.Namespace},
			Spec: stationv1alpha1.StationRebindSpec{
				RequestID: id, StationRef: stationv1alpha1.StationReference{Name: old.Name, UID: string(old.UID)},
				ExpectedBindingVersion: old.Spec.Binding.Version, TargetNodeName: observation.Name, TargetNodeUID: observation.Status.NodeUID, TargetSystemUUID: uuid,
				Reason: fmt.Sprintf("the same machine (system UUID %s) joined again as %s with the admission redeemed at %s; Node UID %s is gone",
					uuid, observation.Name, admission.Spec.RedeemedAt.UTC().Format(time.RFC3339), old.Spec.Binding.NodeUID),
				RequestedBy: SelfHealRequester,
			},
		}
		if err := r.client.Create(ctx, rebind); apierrors.IsAlreadyExists(err) {
			// Asked for already: one that was refused leaves the Station to an administrator, with why.
			asked := &stationv1alpha1.StationRebind{}
			if err := r.reader.Get(ctx, client.ObjectKeyFromObject(rebind), asked); err != nil {
				return reconcile.Result{}, fmt.Errorf("read StationRebind %s: %w", id, err)
			}
			if asked.Status.Phase == stationv1alpha1.StationRebindPhaseRejected {
				why := "the rebind to the machine that joined again was refused"
				if condition := apimeta.FindStatusCondition(asked.Status.Conditions, "Completed"); condition != nil {
					why += ": " + condition.Message
				}
				return reconcile.Result{}, r.setConflict(ctx, old, "NodeUIDChanged", why+"; remove the node and join it again, or rebind the Station")
			}
		} else if err != nil {
			return reconcile.Result{}, fmt.Errorf("rebind Station %s to the machine that joined again: %w", old.Name, err)
		}
		healed = fmt.Sprintf("场站 %s 原绑在已不存在的节点上（UID %s）；这台机器（systemUUID 相同）重新入网，已自动重绑到新节点", old.Spec.DisplayName, old.Spec.Binding.NodeUID)
	} else {
		retired := old.DeepCopy()
		retired.Spec.Retired = true
		if err := r.client.Patch(ctx, retired, client.MergeFromWithOptions(old, client.MergeFromWithOptimisticLock{})); err != nil {
			return reconcile.Result{}, fmt.Errorf("retire Station %s of the node that is gone: %w", old.Name, err)
		}
		// No observation of its node comes any more to say so: what it showed last (Bound, a failed probe) would stay.
		if err := r.setPhase(ctx, retired, stationv1alpha1.StationPhaseRetired); err != nil {
			return reconcile.Result{}, err
		}
		// A machine that reports no system UUID, or whose Station holds none, cannot be told from another.
		whose := "属于另一台机器（systemUUID 不同）"
		if observation.Status.SystemUUID == "" || old.Spec.Binding.SystemUUID == "" {
			whose = "认不出是不是这台机器的（没有 systemUUID 可比）"
		}
		healed = fmt.Sprintf("名字 %s 原来的场站 %s %s，那个节点（UID %s）已不在集群：旧场站已自动退役，这台机器建新场站", observation.Name, old.Spec.DisplayName, whose, old.Spec.Binding.NodeUID)
	}
	if admission.Annotations[SelfHealedAnnotation] != healed {
		noted := admission.DeepCopy()
		if noted.Annotations == nil {
			noted.Annotations = map[string]string{}
		}
		noted.Annotations[SelfHealedAnnotation] = healed
		if err := r.client.Patch(ctx, noted, client.MergeFrom(admission)); err != nil {
			return reconcile.Result{}, fmt.Errorf("note on NodeAdmission %s what was put right: %w", admission.Name, err)
		}
	}
	// The Station's change (rebound, or retired) is what reconciles the name again.
	return reconcile.Result{}, nil
}

func (r *StationReconciler) observeStation(ctx context.Context, registration ClusterRegistration, station *stationv1alpha1.Station, observation *stationv1alpha1.StationObservation) (reconcile.Result, error) {
	now := r.now().UTC()
	age := now.Sub(observation.Status.ReportedAt.Time)
	fresh := age >= 0 && age < r.offlineAfter
	want := station.Status.DeepCopy()
	if fresh {
		want.Phase = stationv1alpha1.StationPhaseBound
	} else {
		want.Phase = stationv1alpha1.StationPhaseOffline
	}
	want.LastSeenTime = observation.Status.ReportedAt.DeepCopy()
	if observation.Status.BootTime != nil {
		// A restart shorter than the heartbeat window never shows as Offline;
		// the boot time is what records it. The margin absorbs the kernel
		// recomputing its boot time when the clock is stepped.
		if before := station.Status.BootTime; before != nil && observation.Status.BootTime.Time.After(before.Time.Add(rebootMargin)) {
			want.Reboots++
			setCondition(want, now, "Rebooted", metav1.ConditionTrue, "BootTimeChanged", fmt.Sprintf("booted at %s; the boot before was at %s",
				observation.Status.BootTime.UTC().Format(time.RFC3339), before.UTC().Format(time.RFC3339)))
		}
		want.BootTime = observation.Status.BootTime.DeepCopy()
	}
	observedStatus, observedReason, observedMessage := metav1.ConditionFalse, "HeartbeatExpired", "last authenticated observation is outside the freshness window"
	if fresh {
		observedStatus, observedReason, observedMessage = metav1.ConditionTrue, "HeartbeatFresh", "a fresh authenticated observation is available"
	}
	setCondition(want, now, "Observed", observedStatus, observedReason, observedMessage)
	setCondition(want, now, "IdentityMatched", metav1.ConditionTrue, "NodeUIDMatched", "observation Node UID matches the existing binding")
	if len(observation.Status.Probes) == 0 {
		setCondition(want, now, "NetworkVerified", metav1.ConditionUnknown, "NoProbeResults", "no network probes were reported")
	} else {
		var failed []string
		for _, probe := range observation.Status.Probes {
			if !probe.OK {
				failed = append(failed, probe.Target)
			}
		}
		unreachable, err := r.livePeers(ctx, failed)
		if err != nil {
			return reconcile.Result{}, err
		}
		reached := len(observation.Status.Probes) - len(failed)
		if len(unreachable) == 0 {
			setCondition(want, now, "NetworkVerified", metav1.ConditionTrue, "AllLivePeersReached",
				fmt.Sprintf("reached %d of %d peers through the tunnel; the other %d are offline themselves", reached, len(observation.Status.Probes), len(failed)))
		} else {
			setCondition(want, now, "NetworkVerified", metav1.ConditionFalse, "ProbeFailed",
				fmt.Sprintf("reached %d of %d peers through the tunnel; no reply from %d that are online: %s", reached, len(observation.Status.Probes), len(unreachable), strings.Join(unreachable[:min(5, len(unreachable))], ", ")))
		}
	}
	ready, err := r.nodeReadiness.Ready(ctx, registration.ClusterID, registration.Namespace, observation.Name)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("read RLark Node readiness: %w", err)
	}
	if ready {
		setCondition(want, now, "RLarkLinked", metav1.ConditionTrue, "NodeReady", "the corresponding RLark Node is Ready")
	} else {
		setCondition(want, now, "RLarkLinked", metav1.ConditionFalse, "NodeNotReady", "the corresponding RLark Node is missing or not Ready")
	}
	if err := r.patchStationStatus(ctx, station, *want); err != nil {
		return reconcile.Result{}, err
	}
	if fresh {
		return reconcile.Result{RequeueAfter: r.offlineAfter - age}, nil
	}
	return reconcile.Result{}, nil
}

func (r *StationReconciler) createStation(ctx context.Context, registration ClusterRegistration, observation *stationv1alpha1.StationObservation) (reconcile.Result, error) {
	sequence, err := r.nextSequence(ctx, registration)
	if err != nil {
		return reconcile.Result{}, err
	}
	station := &stationv1alpha1.Station{
		ObjectMeta: metav1.ObjectMeta{Name: StationName(registration.ClusterID, observation.Status.NodeUID), Namespace: registration.Namespace},
		Spec: stationv1alpha1.StationSpec{
			DisplayName: fmt.Sprintf("Station-%04d", sequence),
			Sequence:    sequence,
			Binding: stationv1alpha1.StationBinding{
				NodeName: observation.Name, NodeUID: observation.Status.NodeUID,
				SystemUUID: observation.Status.SystemUUID, Version: 1, BoundBy: "system",
			},
		},
	}
	err = r.client.Create(ctx, station)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return reconcile.Result{}, fmt.Errorf("create Station: %w", err)
	}
	if apierrors.IsAlreadyExists(err) {
		if getErr := r.reader.Get(ctx, types.NamespacedName{Namespace: registration.Namespace, Name: station.Name}, station); getErr != nil {
			return reconcile.Result{}, fmt.Errorf("read Station after create race: %w", getErr)
		}
		if station.Spec.Binding.NodeUID != observation.Status.NodeUID || station.Spec.Binding.NodeName != observation.Name {
			return reconcile.Result{}, ErrStationNameCollision
		}
	}
	return r.observeStation(ctx, registration, station, observation)
}

func (r *StationReconciler) nextSequence(ctx context.Context, registration ClusterRegistration) (int64, error) {
	key := types.NamespacedName{Namespace: registration.Namespace, Name: SequenceName}
	for attempt := 0; attempt < sequenceCASAttempts; attempt++ {
		var sequence stationv1alpha1.StationSequence
		err := r.reader.Get(ctx, key, &sequence)
		if apierrors.IsNotFound(err) {
			sequence = stationv1alpha1.StationSequence{ObjectMeta: metav1.ObjectMeta{Namespace: registration.Namespace, Name: SequenceName}, Spec: stationv1alpha1.StationSequenceSpec{ClusterID: registration.ClusterID}}
			if createErr := r.client.Create(ctx, &sequence); createErr != nil && !apierrors.IsAlreadyExists(createErr) {
				return 0, fmt.Errorf("create StationSequence: %w", createErr)
			}
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("read StationSequence: %w", err)
		}
		if sequence.Spec.ClusterID != registration.ClusterID {
			return 0, ErrSequenceOwnerConflict
		}
		base := sequence.DeepCopy()
		sequence.Status.LastAssigned++
		if err := r.client.Status().Patch(ctx, &sequence, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); apierrors.IsConflict(err) {
			continue
		} else if err != nil {
			return 0, fmt.Errorf("reserve Station sequence: %w", err)
		}
		return sequence.Status.LastAssigned, nil
	}
	return 0, errors.New("Station sequence remained contended after bounded retries")
}

func (r *StationReconciler) markNodeOffline(ctx context.Context, namespace, nodeName string) error {
	var stations stationv1alpha1.StationList
	if err := r.reader.List(ctx, &stations, client.InNamespace(namespace)); err != nil {
		return fmt.Errorf("list Stations for missing observation: %w", err)
	}
	bound := make([]*stationv1alpha1.Station, 0, 1)
	for i := range stations.Items {
		station := &stations.Items[i]
		if station.Spec.Binding.NodeName == nodeName && !station.Spec.Retired {
			bound = append(bound, station)
		}
	}
	if len(bound) > 1 {
		for _, station := range bound {
			if err := r.setConflict(ctx, station, "DuplicateNodeNameBinding", "multiple Stations are bound to this Node name"); err != nil {
				return err
			}
		}
		return nil
	}
	for _, station := range bound {
		want := station.Status.DeepCopy()
		want.Phase = stationv1alpha1.StationPhaseOffline
		now := r.now().UTC()
		setCondition(want, now, "Observed", metav1.ConditionFalse, "ObservationMissing", "no current observation exists for this Node name")
		setCondition(want, now, "NetworkVerified", metav1.ConditionUnknown, "NoObservation", "network state is unknown because the observation is missing")
		if err := r.patchStationStatus(ctx, station, *want); err != nil {
			return err
		}
	}
	return nil
}

func (r *StationReconciler) setConflict(ctx context.Context, station *stationv1alpha1.Station, reason, message string) error {
	want := station.Status.DeepCopy()
	want.Phase = stationv1alpha1.StationPhaseConflict
	setCondition(want, r.now().UTC(), "IdentityMatched", metav1.ConditionFalse, reason, message)
	return r.patchStationStatus(ctx, station, *want)
}

func (r *StationReconciler) setPhase(ctx context.Context, station *stationv1alpha1.Station, phase stationv1alpha1.StationPhase) error {
	want := station.Status.DeepCopy()
	want.Phase = phase
	return r.patchStationStatus(ctx, station, *want)
}

func (r *StationReconciler) patchStationStatus(ctx context.Context, original *stationv1alpha1.Station, desired stationv1alpha1.StationStatus) error {
	key := types.NamespacedName{Namespace: original.Namespace, Name: original.Name}
	for attempt := 0; attempt < sequenceCASAttempts; attempt++ {
		var current stationv1alpha1.Station
		if err := r.reader.Get(ctx, key, &current); err != nil {
			return fmt.Errorf("read Station status target: %w", err)
		}
		if reflect.DeepEqual(current.Status, desired) {
			return nil
		}
		base := current.DeepCopy()
		current.Status = *desired.DeepCopy()
		if err := r.client.Status().Patch(ctx, &current, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); apierrors.IsConflict(err) {
			continue
		} else if err != nil {
			return fmt.Errorf("patch Station status: %w", err)
		}
		return nil
	}
	return errors.New("Station status remained contended after bounded retries")
}

// StationName derives an idempotent business ID from trusted cluster and Node identities.
func StationName(clusterID, nodeUID string) string {
	digest := sha256.Sum256([]byte(clusterID + "\x00" + nodeUID))
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(digest[:16])
	return "stn-" + strings.ToLower(encoded)
}

func setCondition(status *stationv1alpha1.StationStatus, now time.Time, conditionType string, conditionStatus metav1.ConditionStatus, reason, message string) {
	apimeta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type: conditionType, Status: conditionStatus, Reason: reason,
		Message: message, LastTransitionTime: metav1.NewTime(now),
	})
}

func uniqueStations(stations []stationv1alpha1.Station) []stationv1alpha1.Station {
	seen := make(map[types.NamespacedName]struct{}, len(stations))
	result := make([]stationv1alpha1.Station, 0, len(stations))
	for i := range stations {
		key := types.NamespacedName{Namespace: stations[i].Namespace, Name: stations[i].Name}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, stations[i])
	}
	return result
}

// livePeers returns the probe targets that failed although the peer is online
// by its own heartbeat. A node is not unverified because a peer is switched
// off, or has not come up yet.
func (r *StationReconciler) livePeers(ctx context.Context, failed []string) ([]string, error) {
	if len(failed) == 0 {
		return nil, nil
	}
	var enrollments stationv1alpha1.NetworkEnrollmentList
	if err := r.client.List(ctx, &enrollments); err != nil {
		return nil, fmt.Errorf("list NetworkEnrollments for probe targets: %w", err)
	}
	var stations stationv1alpha1.StationList
	if err := r.client.List(ctx, &stations); err != nil {
		return nil, fmt.Errorf("list Stations for probe targets: %w", err)
	}
	bound := map[types.NamespacedName]bool{}
	for i := range stations.Items {
		station := &stations.Items[i]
		bound[types.NamespacedName{Namespace: station.Namespace, Name: station.Spec.Binding.NodeName}] = station.Status.Phase == stationv1alpha1.StationPhaseBound
	}
	// A peer is online when its Station is: a peer that is not known to be
	// (a machine whose admission put it in the tunnel before it enrolled, one
	// whose Station is offline) is not a failure of the node that probed it.
	online := map[string]bool{}
	for i := range enrollments.Items {
		enrollment := &enrollments.Items[i]
		if enrollment.Status.TunnelAddress != "" && bound[types.NamespacedName{Namespace: enrollment.Namespace, Name: enrollment.Name}] {
			online[enrollment.Status.TunnelAddress] = true
		}
	}
	var live []string
	for _, target := range failed {
		if online[target] {
			live = append(live, target)
		}
	}
	return live, nil
}
