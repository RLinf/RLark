package controller

import (
	"cmp"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// MachineLabel is on the NodeOperation that joins a Machine and names it.
	MachineLabel = "station.rlark.io/machine"
	// machineRequestAnnotation is, on that NodeOperation, the generation of
	// the Machine it was made for: a request made anew gets a new operation.
	machineRequestAnnotation = "station.rlark.io/machine-generation"
	// machineLabelsAnnotation lists, on a NodeAdmission, the labels its
	// machine has values of its own for; the class does not put them back.
	machineLabelsAnnotation = "station.rlark.io/machine-labels"
	// MachineNodeJoinLabel is written by the Gateway on a Server NodeJoin.
	MachineNodeJoinLabel = "station.rlark.io/machine"
	// A machine is on the network now if it was seen this recently.
	machinePresent = 5 * time.Minute
	// A machine that waits to be admitted is looked at again this often.
	machineInspectEvery = 5 * time.Minute
	machineCandidateTTL = 7 * 24 * time.Hour
)

// MachineInspection is what a login says a machine is.
type MachineInspection struct {
	// Serial is read on the entry device.
	Serial string
	// NodeHostKey is the key the computer behind it showed; NodeName the
	// name it is a node under, empty if it is none.
	NodeHostKey, NodeName string
}

// MachineInspector logs in to a machine and reads what it is. The serial is
// returned even when the computer behind the entry device does not answer.
type MachineInspector interface {
	Inspect(ctx context.Context, class string, at MachineAddress) (MachineInspection, error)
}

// SSHMachineInspector inspects with the accounts of the machine's class.
type SSHMachineInspector struct{ Access NodeClassAccess }

func (i SSHMachineInspector) Inspect(ctx context.Context, name string, at MachineAddress) (MachineInspection, error) {
	class, accounts, err := i.Access.class(ctx, name)
	if err != nil {
		return MachineInspection{}, err
	}
	defer accounts.clear()
	entry, _, err := dialEntry(ctx, class, accounts, at)
	if err != nil {
		return MachineInspection{}, err
	}
	serial, err := RunNodeSSHStream(ctx, entry, "tr -d '\\000' < "+shellQuote(class.Spec.Access.Entry.SerialFile), nil)
	_ = entry.Close()
	if err != nil {
		return MachineInspection{}, fmt.Errorf("read the machine's serial on its entry device: %w", err)
	}
	seen := MachineInspection{Serial: strings.TrimSpace(string(serial))}
	if seen.Serial == "" {
		return seen, errors.New("the entry device's serial file is empty")
	}
	node, sudo, hostKey, err := dialNode(ctx, class, accounts, at)
	if err != nil {
		return seen, err
	}
	defer node.Close()
	output, err := RunNodeSudoScript(ctx, node, installObservationScript, sudo)
	if err != nil {
		return seen, fmt.Errorf("read whether the machine is a node: %w", err)
	}
	seen.NodeHostKey, seen.NodeName = hostKey, parseInstallObservation(output).Node
	return seen, nil
}

// ClusterNamespaces maps a registered cluster to its management namespace.
type ClusterNamespaces interface {
	Namespace(ctx context.Context, clusterID string) (string, error)
}

// MachineReconciler takes a Machine from being seen on its site's network to
// being a node: it reads what the machine is, and once an administrator has
// admitted it gives it the next name of its class, a NodeAdmission and the
// NodeOperation that joins it, and says on the Machine how that goes.
type MachineReconciler struct {
	Client    client.Client
	Reader    client.Reader
	Inspector MachineInspector
	Clusters  ClusterNamespaces
	// Namespace is where the Machines are.
	Namespace string
	Now       func() time.Time
}

func (r *MachineReconciler) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *MachineReconciler) SetupWithManager(mgr manager.Manager) error {
	if err := registerMachineNodeJoinKinds(mgr.GetScheme()); err != nil {
		return fmt.Errorf("register NodeJoin observation kind: %w", err)
	}
	ofMachine := handler.EnqueueRequestsFromMapFunc(func(_ context.Context, object client.Object) []reconcile.Request {
		if name := object.GetLabels()[MachineNodeJoinLabel]; name != "" {
			return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: r.Namespace, Name: name}}}
		}
		if name := object.GetLabels()[MachineLabel]; name != "" {
			return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: r.Namespace, Name: name}}}
		}
		return nil
	})
	return ctrl.NewControllerManagedBy(mgr).Named("machine").
		For(&stationv1alpha1.Machine{}).
		Watches(&stationv1alpha1.NodeOperation{}, ofMachine).
		Watches(&machineNodeJoinResource{TypeMeta: metav1.TypeMeta{APIVersion: machineNodeJoinGV, Kind: "NodeJoin"}}, ofMachine).
		// A class changes when an administrator enters its passwords: its machines are looked at anew.
		Watches(&stationv1alpha1.NodeClass{}, handler.EnqueueRequestsFromMapFunc(r.ofClass)).
		Watches(&stationv1alpha1.RobotModel{}, handler.EnqueueRequestsFromMapFunc(r.ofRobotModel)).
		// A node's removal marks its admission: its machine can be admitted again.
		Watches(&stationv1alpha1.NodeAdmission{}, handler.EnqueueRequestsFromMapFunc(r.ofAdmission)).
		Watches(&stationv1alpha1.NetworkEnrollment{}, handler.EnqueueRequestsFromMapFunc(r.ofEnrollment)).
		WithOptions(controller.Options{MaxConcurrentReconciles: 10}).
		Complete(r)
}

func (r *MachineReconciler) ofClass(ctx context.Context, class client.Object) []reconcile.Request {
	return r.machinesWhere(ctx, func(machine *stationv1alpha1.Machine) bool {
		return machineClassName(machine) == class.GetName()
	})
}

func (r *MachineReconciler) ofRobotModel(ctx context.Context, object client.Object) []reconcile.Request {
	classes := &stationv1alpha1.NodeClassList{}
	if err := r.Reader.List(ctx, classes); err != nil {
		return nil
	}
	classNames := map[string]struct{}{}
	for i := range classes.Items {
		_, model := effectiveNodeClassIdentity(&classes.Items[i])
		if model == object.GetName() {
			classNames[classes.Items[i].Name] = struct{}{}
		}
	}
	return r.machinesWhere(ctx, func(machine *stationv1alpha1.Machine) bool {
		_, referenced := classNames[machineClassName(machine)]
		return referenced
	})
}

func machineClassName(machine *stationv1alpha1.Machine) string {
	if machine.Spec.Admit != nil && machine.Spec.Admit.NodeClass != "" {
		return machine.Spec.Admit.NodeClass
	}
	return machine.Spec.NodeClass
}

func (r *MachineReconciler) ofAdmission(ctx context.Context, object client.Object) []reconcile.Request {
	admission := object.(*stationv1alpha1.NodeAdmission)
	return r.machinesWhere(ctx, func(machine *stationv1alpha1.Machine) bool {
		return admission.Spec.Serial != "" && machine.Status.Serial == admission.Spec.Serial ||
			machine.Status.ClusterID == admission.Spec.ClusterID && machine.Status.NodeName == admission.Name
	})
}

func (r *MachineReconciler) ofEnrollment(ctx context.Context, object client.Object) []reconcile.Request {
	enrollment := object.(*stationv1alpha1.NetworkEnrollment)
	return r.machinesWhere(ctx, func(machine *stationv1alpha1.Machine) bool {
		return machine.Status.ClusterID == enrollment.Spec.ClusterID && machine.Status.NodeName == enrollment.Name
	})
}

func (r *MachineReconciler) machinesWhere(ctx context.Context, match func(*stationv1alpha1.Machine) bool) []reconcile.Request {
	machines := &stationv1alpha1.MachineList{}
	if err := r.Reader.List(ctx, machines, client.InNamespace(r.Namespace)); err != nil {
		return nil
	}
	var requests []reconcile.Request
	for i := range machines.Items {
		if match(&machines.Items[i]) {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&machines.Items[i])})
		}
	}
	return requests
}

func (r *MachineReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	machine := &stationv1alpha1.Machine{}
	if err := r.Reader.Get(ctx, req.NamespacedName, machine); apierrors.IsNotFound(err) {
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, fmt.Errorf("read Machine: %w", err)
	}
	if machine.Spec.Source == "Discovery" && machine.Spec.NodeClass == "" && machine.Spec.Admit == nil &&
		machine.Status.NodeJoin == nil && machine.Status.Operation == "" && machine.Status.NodeName == "" &&
		(machine.Status.Phase == "" || machine.Status.Phase == stationv1alpha1.MachineDiscovered) {
		expires := machine.CreationTimestamp.Add(machineCandidateTTL)
		if machine.Status.Entry.LastSeen != nil {
			expires = machine.Status.Entry.LastSeen.Add(machineCandidateTTL)
		}
		if !machine.CreationTimestamp.IsZero() && !r.now().Before(expires) {
			return r.collectCandidate(ctx, machine)
		}
	}
	before := machine.DeepCopy()
	result, err := r.reconcile(ctx, machine)
	if !reflect.DeepEqual(before.Status, machine.Status) {
		if patchErr := r.Client.Status().Patch(ctx, machine, client.MergeFromWithOptions(before, client.MergeFromWithOptimisticLock{})); patchErr != nil {
			return reconcile.Result{}, fmt.Errorf("write Machine status: %w", patchErr)
		}
	}
	return result, err
}

func candidateRequeue(machine *stationv1alpha1.Machine, now time.Time) time.Duration {
	base := machine.CreationTimestamp.Time
	if machine.Status.Entry.LastSeen != nil {
		base = machine.Status.Entry.LastSeen.Time
	}
	if base.IsZero() {
		return machineCandidateTTL
	}
	remaining := base.Add(machineCandidateTTL).Sub(now)
	if remaining < time.Minute {
		return time.Minute
	}
	return remaining
}

func (r *MachineReconciler) reconcile(ctx context.Context, machine *stationv1alpha1.Machine) (reconcile.Result, error) {
	status := &machine.Status
	if err := r.observeMachineIdentity(ctx, machine); err != nil {
		return reconcile.Result{}, err
	}
	if machine.Spec.Admit != nil && machine.Spec.Admit.Type == stationv1alpha1.MachineTypeServer || status.NodeJoin != nil {
		_, err := r.projectNodeJoin(ctx, machine)
		return reconcile.Result{RequeueAfter: machineInspectEvery}, err
	}
	className := machineClassName(machine)
	if className == "" {
		// An unclassified candidate is only a sighting. It must never cause
		// station-controller to try shared robot credentials against it.
		if status.Phase == "" {
			status.Phase = stationv1alpha1.MachineDiscovered
		}
		status.Problem, status.Message = "", ""
		return reconcile.Result{RequeueAfter: machineCandidateTTL}, nil
	}
	class := &stationv1alpha1.NodeClass{}
	if err := r.Reader.Get(ctx, types.NamespacedName{Name: className}, class); err != nil {
		if apierrors.IsNotFound(err) {
			status.Problem, status.Message = stationv1alpha1.MachineClassIncomplete, "NodeClass "+className+" does not exist"
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("read NodeClass %s: %w", className, err)
	}
	if err := validateNodeClassModel(ctx, r.Reader, class); err != nil {
		if errors.Is(err, errRobotModelNotFound) {
			status.Problem, status.Message = stationv1alpha1.MachineUnknownModel, className+" refers to an unregistered RobotModel"
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("validate RobotModel for NodeClass %s: %w", className, err)
	}
	classKind, _ := effectiveNodeClassIdentity(class)
	if classKind == string(stationv1alpha1.NodeClassKindServer) {
		status.Problem, status.Message = stationv1alpha1.MachineClassIncomplete, "Server classes are not joined through Machine admission"
		return reconcile.Result{}, nil
	}
	if machine.Spec.Admit != nil {
		if machine.Spec.Admit.Type == stationv1alpha1.MachineTypeRobot && classKind != string(stationv1alpha1.NodeClassKindRobot) {
			status.Problem, status.Message = stationv1alpha1.MachineClassIncomplete, "the selected NodeClass is not a Robot class"
			return reconcile.Result{}, nil
		}
		classModel := effectiveModel(class)
		if machine.Spec.Admit.Model != "" && classModel != "" && machine.Spec.Admit.Model != classModel {
			status.Problem, status.Message = stationv1alpha1.MachineUnknownModel, "the selected model does not match the NodeClass model"
			return reconcile.Result{}, nil
		}
	}
	if class.Spec.Access == nil || class.Spec.Discovery == nil {
		status.Problem, status.Message = stationv1alpha1.MachineClassIncomplete, "NodeClass "+machine.Spec.NodeClass+" declares no access or no discovery"
		return reconcile.Result{}, nil
	}
	// Station API creates a Machine and then says where it saw it.
	if status.Entry.Address == "" {
		return reconcile.Result{}, nil
	}
	// A machine whose entry device was given keys of its own when it joined is
	// known by the key its NodeAdmission records, whatever key was last
	// written of it; one turned away for showing a key is looked at by that
	// one again. A joined machine written with its class's factory key is
	// looked at by that key: set up anew, it shows it.
	factory := class.Spec.Discovery.HostKeyFingerprints
	ownKey := ""
	if status.NodeName != "" {
		namespace, err := r.Clusters.Namespace(ctx, status.ClusterID)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("resolve cluster %s: %w", status.ClusterID, err)
		}
		admission := &stationv1alpha1.NodeAdmission{}
		if err := r.Reader.Get(ctx, types.NamespacedName{Namespace: namespace, Name: status.NodeName}, admission); err != nil && !apierrors.IsNotFound(err) {
			return reconcile.Result{}, fmt.Errorf("read NodeAdmission %s: %w", status.NodeName, err)
		}
		if !apierrors.IsNotFound(err) {
			ownKey = admission.Annotations[EntryHostKeyAnnotation]
		}
	}
	if ownKey != "" && ownKey != status.Entry.HostKeyFingerprint &&
		(status.Phase == stationv1alpha1.MachineJoined && !slices.Contains(factory, status.Entry.HostKeyFingerprint) || status.Problem == stationv1alpha1.MachineHostKeyChanged) {
		status.Entry.HostKeyFingerprint, status.InspectedAt = ownKey, nil
	}
	factoryKey := slices.Contains(factory, status.Entry.HostKeyFingerprint)
	switch status.Phase {
	case stationv1alpha1.MachineAdmitted:
		return r.follow(ctx, machine, class)
	case stationv1alpha1.MachineJoined:
		removed, err := r.nodeRemoved(ctx, status.ClusterID, status.NodeName)
		if err != nil {
			return reconcile.Result{}, err
		}
		if removed {
			// Its node was removed: the machine is looked at anew, and can be admitted again.
			status.Phase, status.ClusterID, status.NodeName, status.Operation, status.InspectedAt = stationv1alpha1.MachineDiscovered, "", "", "", nil
			break
		}
		if !factoryKey {
			return reconcile.Result{}, nil
		}
		// A joined machine that shows the factory's key again was set up anew.
		status.InspectedAt = nil
	}

	// A machine with a problem is looked at again when it is asked for anew, not at every sighting; one
	// that waits for another machine with its serial to go, for its class's passwords, or for a fault
	// that is not its own to pass, at every sighting.
	if status.InspectedAt == nil || r.now().Sub(status.InspectedAt.Time) >= machineInspectEvery ||
		slices.Contains([]string{stationv1alpha1.MachineDuplicateSerial, stationv1alpha1.MachineAccessUnconfigured, stationv1alpha1.MachineHubUnreachable, stationv1alpha1.MachineInspectFailed}, status.Problem) ||
		machine.Spec.Admit != nil && status.Problem != "" && status.InspectedAt.Before(&machine.Spec.Admit.RequestedAt) {
		seen, err := r.Inspector.Inspect(ctx, class.Name, MachineAddress{Entry: status.Entry.Address, EntryHostKey: status.Entry.HostKeyFingerprint, Via: status.Entry.Via, NodeHostKey: status.NodeHostKeyFingerprint})
		if seen.Serial != "" {
			status.Serial = seen.Serial
		}
		if err != nil {
			problem, message := machineProblem(err), fmt.Sprintf("%v; a hub last saw the machine at %s", err, status.Entry.LastSeen.UTC().Format(time.RFC3339))
			// A fault that is not the machine's is said and tried again; the machine has not been looked at. It is
			// said when it starts: a write queues the Machine again at once, past the backoff, and the text of a
			// dial's fault differs at each try.
			if problem == "" || problem == stationv1alpha1.MachineHubUnreachable {
				if problem = cmp.Or(problem, stationv1alpha1.MachineInspectFailed); status.Problem != problem {
					status.Problem, status.Message = problem, message
				}
				return reconcile.Result{}, fmt.Errorf("inspect Machine %s: %w", machine.Name, err)
			}
			// A machine of a node that shows the key its admission records, or
			// its class's factory key (set up anew), is that machine: it is
			// known by the key it shows and looked at again.
			var mismatch *SSHHostKeyMismatchError
			if problem == stationv1alpha1.MachineHostKeyChanged && status.NodeName != "" && errors.As(err, &mismatch) && (mismatch.Received == ownKey || slices.Contains(factory, mismatch.Received)) {
				status.Entry.HostKeyFingerprint, status.InspectedAt, status.Problem, status.Message = mismatch.Received, nil, "", ""
				return reconcile.Result{Requeue: true}, nil
			}
			now := metav1.NewTime(r.now())
			status.Phase, status.Problem, status.Message, status.InspectedAt = stationv1alpha1.MachineDiscovered, problem, message, &now
			return reconcile.Result{RequeueAfter: machineInspectEvery}, nil
		}
		now := metav1.NewTime(r.now())
		status.InspectedAt = &now
		status.NodeHostKeyFingerprint, status.Problem, status.Message = seen.NodeHostKey, "", ""
		if done, result, err := r.settle(ctx, machine, class, seen, factoryKey); done {
			return result, err
		}
	}
	if status.Problem != "" || machine.Spec.Admit == nil {
		return reconcile.Result{RequeueAfter: machineInspectEvery}, nil
	}
	return r.admit(ctx, machine, class)
}

// settle places a machine that has just been inspected among the others:
// one machine per serial, and a machine that is a node already is that node.
func (r *MachineReconciler) settle(ctx context.Context, machine *stationv1alpha1.Machine, class *stationv1alpha1.NodeClass, seen MachineInspection, factoryKey bool) (bool, reconcile.Result, error) {
	status := &machine.Status
	others := &stationv1alpha1.MachineList{}
	if err := r.Reader.List(ctx, others, client.InNamespace(machine.Namespace)); err != nil {
		return true, reconcile.Result{}, fmt.Errorf("list Machines: %w", err)
	}
	for i := range others.Items {
		other := &others.Items[i]
		if other.Name == machine.Name || other.Status.Serial != status.Serial {
			continue
		}
		if other.Status.Entry.LastSeen != nil && r.now().Sub(other.Status.Entry.LastSeen.Time) < machinePresent {
			if other.CreationTimestamp.Before(&machine.CreationTimestamp) || other.CreationTimestamp.Equal(&machine.CreationTimestamp) && other.Name < machine.Name {
				status.Phase, status.Problem, status.Message = stationv1alpha1.MachineDiscovered, stationv1alpha1.MachineDuplicateSerial, "Machine "+other.Name+" reports the same serial"
				return true, reconcile.Result{RequeueAfter: machineInspectEvery}, nil
			}
			continue
		}
		// The same machine, seen before through another device or address.
		if err := r.Client.Delete(ctx, other); err != nil && !apierrors.IsNotFound(err) {
			return true, reconcile.Result{}, fmt.Errorf("remove Machine %s, which %s is now: %w", other.Name, machine.Name, err)
		}
	}
	if seen.NodeName == "" {
		status.Phase, status.ClusterID, status.NodeName, status.Operation = stationv1alpha1.MachineDiscovered, "", "", ""
		return false, reconcile.Result{}, nil
	}
	admissions := &stationv1alpha1.NodeAdmissionList{}
	if err := r.Reader.List(ctx, admissions); err != nil {
		return true, reconcile.Result{}, fmt.Errorf("list NodeAdmissions: %w", err)
	}
	for i := range admissions.Items {
		admission := &admissions.Items[i]
		if admission.Name != seen.NodeName || admission.Spec.Serial != "" && admission.Spec.Serial != status.Serial {
			continue
		}
		// The node was removed while the machine could not be reached: what
		// it kept of the node goes when it is admitted again.
		if AdmissionNodeRemoved(admission) {
			status.Phase, status.ClusterID, status.NodeName, status.Operation = stationv1alpha1.MachineDiscovered, "", "", ""
			return false, reconcile.Result{}, nil
		}
		if admission.Spec.Serial == "" {
			admission.Spec.Serial = status.Serial
			if err := r.Client.Update(ctx, admission); err != nil {
				return true, reconcile.Result{}, fmt.Errorf("bind NodeAdmission %s to serial %s: %w", admission.Name, status.Serial, err)
			}
		}
		status.ClusterID, status.NodeName = admission.Spec.ClusterID, admission.Name
		if !factoryKey || !class.Spec.Access.Entry.RotateHostKey {
			status.Phase = stationv1alpha1.MachineJoined
			return true, reconcile.Result{}, nil
		}
		// A node whose entry device still shows the factory's key: the join
		// finds it joined and gives the device keys of its own.
		err := r.startJoin(ctx, machine, class, admission.Namespace, "station-controller")
		return true, reconcile.Result{}, err
	}
	status.Phase, status.Problem, status.Message = stationv1alpha1.MachineDiscovered, stationv1alpha1.MachineAlreadyNode, "the machine is node "+seen.NodeName+", which has no admission for this serial"
	return true, reconcile.Result{RequeueAfter: machineInspectEvery}, nil
}

// admit gives the machine its node name and starts its join. The name bound
// to the machine's serial is its name again; a new machine takes the first
// number of its class no admission of the cluster has. Creating the
// NodeAdmission is what takes the number: of two machines that reach for the
// same one, one is told it exists and takes the next.
func (r *MachineReconciler) admit(ctx context.Context, machine *stationv1alpha1.Machine, class *stationv1alpha1.NodeClass) (reconcile.Result, error) {
	status, admit := &machine.Status, machine.Spec.Admit
	if class.Spec.Naming == nil {
		status.Problem, status.Message = stationv1alpha1.MachineClassIncomplete, "NodeClass "+class.Name+" declares no naming"
		return reconcile.Result{}, nil
	}
	namespace, err := r.Clusters.Namespace(ctx, admit.ClusterID)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("resolve cluster %s: %w", admit.ClusterID, err)
	}
	admissions := &stationv1alpha1.NodeAdmissionList{}
	if err := r.Reader.List(ctx, admissions, client.InNamespace(namespace)); err != nil {
		return reconcile.Result{}, fmt.Errorf("list NodeAdmissions: %w", err)
	}
	name := ""
	for i := range admissions.Items {
		if admissions.Items[i].Spec.Serial == status.Serial {
			name = admissions.Items[i].Name
		}
	}
	for number := 1; name == ""; number++ {
		candidate := fmt.Sprintf(class.Spec.Naming.NodeName, number)
		if slices.ContainsFunc(admissions.Items, func(admission stationv1alpha1.NodeAdmission) bool { return admission.Name == candidate }) {
			continue
		}
		labels, own := map[string]string{}, []string{}
		for key, value := range class.Spec.Labels {
			labels[key] = value
			if replaced, ok := admit.Labels[key]; ok {
				labels[key] = replaced
				own = append(own, key)
			}
		}
		for key, value := range admit.Labels {
			if _, declared := class.Spec.Labels[key]; !declared {
				labels[key] = value
				own = append(own, key)
			}
		}
		for key, value := range class.Spec.Naming.Labels {
			labels[key] = fmt.Sprintf(value, number)
			own = append(own, key)
		}
		slices.Sort(own)
		// Nobody holds the code of this hash: the join gives the admission its code when it runs the installer.
		unredeemable := make([]byte, 32)
		if _, err := rand.Read(unredeemable); err != nil {
			return reconcile.Result{}, err
		}
		digest := sha256.Sum256(unredeemable)
		admission := &stationv1alpha1.NodeAdmission{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: candidate, Annotations: map[string]string{machineLabelsAnnotation: strings.Join(own, ",")}},
			Spec: stationv1alpha1.NodeAdmissionSpec{ClusterID: admit.ClusterID, Site: machine.Spec.Site, CodeHash: hex.EncodeToString(digest[:]), ExpiresAt: metav1.NewTime(r.now()),
				Labels: labels, Taints: class.Spec.Taints, NodeClass: class.Name, Serial: status.Serial, IssuedBy: admit.RequestedBy,
				Machine: &stationv1alpha1.NodeMachine{Category: stationv1alpha1.MachineCategoryRobot, Model: effectiveModel(class), Body: admit.Labels["rlark.io/robot-body"]}},
		}
		if err := r.Client.Create(ctx, admission); apierrors.IsAlreadyExists(err) {
			continue
		} else if err != nil {
			return reconcile.Result{}, fmt.Errorf("admit Machine %s as %s: %w", machine.Name, candidate, err)
		}
		name = candidate
	}
	status.ClusterID, status.NodeName = admit.ClusterID, name
	return reconcile.Result{}, r.startJoin(ctx, machine, class, namespace, admit.RequestedBy)
}

func (r *MachineReconciler) startJoin(ctx context.Context, machine *stationv1alpha1.Machine, class *stationv1alpha1.NodeClass, namespace, requestedBy string) error {
	status, access := &machine.Status, class.Spec.Access
	generation := strconv.FormatInt(machine.Generation, 10)
	operation := &stationv1alpha1.NodeOperation{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "join-" + status.NodeName + "-" + generation,
			Labels: map[string]string{MachineLabel: machine.Name}, Annotations: map[string]string{machineRequestAnnotation: generation}},
		Spec: stationv1alpha1.NodeOperationSpec{RequestID: machine.Name + "-" + generation, Action: "Join", ClusterID: status.ClusterID, NodeName: status.NodeName,
			Admission: status.NodeName, NodeClass: class.Name, RequestedBy: requestedBy,
			SSH: &stationv1alpha1.NodeSSHConnection{Address: access.Node.Address, Port: access.Node.Port, User: access.Node.User, HostKeyFingerprint: status.NodeHostKeyFingerprint,
				Jump: &stationv1alpha1.NodeSSHHop{Address: status.Entry.Address, Port: access.Entry.Port, User: access.Entry.User, HostKeyFingerprint: status.Entry.HostKeyFingerprint, Via: status.Entry.Via}}},
	}
	if err := r.Client.Create(ctx, operation); apierrors.IsAlreadyExists(err) {
		// This request joined the machine already, as a node removed since:
		// it is spent, and the machine waits to be admitted again.
		joined := &stationv1alpha1.NodeOperation{}
		if err := r.Reader.Get(ctx, client.ObjectKeyFromObject(operation), joined); err != nil {
			return fmt.Errorf("read the join of Machine %s: %w", machine.Name, err)
		}
		admission := &stationv1alpha1.NodeAdmission{}
		if err := r.Reader.Get(ctx, types.NamespacedName{Namespace: namespace, Name: status.NodeName}, admission); err != nil {
			return fmt.Errorf("read NodeAdmission %s: %w", status.NodeName, err)
		}
		if joined.Status.Phase == "Succeeded" && AdmissionNodeRemoved(admission) {
			status.Phase, status.ClusterID, status.NodeName, status.Operation = stationv1alpha1.MachineDiscovered, "", "", ""
			status.Problem, status.Message = "", "its node was removed; admit it again to join"
			return nil
		}
	} else if err != nil {
		return fmt.Errorf("start the join of Machine %s: %w", machine.Name, err)
	}
	status.Phase, status.Operation, status.Problem, status.Message = stationv1alpha1.MachineAdmitted, operation.Name, "", ""
	return nil
}

// follow says on the Machine how its join goes.
func (r *MachineReconciler) follow(ctx context.Context, machine *stationv1alpha1.Machine, class *stationv1alpha1.NodeClass) (reconcile.Result, error) {
	status := &machine.Status
	namespace, err := r.Clusters.Namespace(ctx, status.ClusterID)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("resolve cluster %s: %w", status.ClusterID, err)
	}
	operation := &stationv1alpha1.NodeOperation{}
	if err := r.Reader.Get(ctx, types.NamespacedName{Namespace: namespace, Name: status.Operation}, operation); apierrors.IsNotFound(err) {
		// Its join was taken away; the machine is looked at anew.
		status.Phase, status.Operation, status.InspectedAt = stationv1alpha1.MachineDiscovered, "", nil
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		return reconcile.Result{}, fmt.Errorf("read NodeOperation %s: %w", status.Operation, err)
	}
	switch operation.Status.Phase {
	case "Succeeded":
		admission := &stationv1alpha1.NodeAdmission{}
		if err := r.Reader.Get(ctx, types.NamespacedName{Namespace: namespace, Name: status.NodeName}, admission); err != nil {
			return reconcile.Result{}, fmt.Errorf("read NodeAdmission %s: %w", status.NodeName, err)
		}
		if key := admission.Annotations[EntryHostKeyAnnotation]; key != "" {
			status.Entry.HostKeyFingerprint = key
		}
		status.Phase, status.Problem, status.Message = stationv1alpha1.MachineJoined, "", ""
	case "Blocked", "Failed":
		// Asked for again since: the stopped join gives way to a new one.
		if machine.Spec.Admit != nil && operation.Annotations[machineRequestAnnotation] != strconv.FormatInt(machine.Generation, 10) {
			if err := r.Client.Delete(ctx, operation); err != nil && !apierrors.IsNotFound(err) {
				return reconcile.Result{}, fmt.Errorf("remove the stopped NodeOperation %s: %w", operation.Name, err)
			}
			return reconcile.Result{}, r.startJoin(ctx, machine, class, namespace, machine.Spec.Admit.RequestedBy)
		}
		status.Problem, status.Message = stationv1alpha1.MachineJoinBlocked, "the join stopped"
		for _, condition := range operation.Status.Conditions {
			if condition.Type != "Progressing" {
				continue
			}
			status.Message = condition.Message
			if slices.Contains([]string{stationv1alpha1.MachineCredentialRejected, stationv1alpha1.MachineHostKeyChanged, stationv1alpha1.MachineNodeCredentialRejected, stationv1alpha1.MachineNodeHostKeyChanged, stationv1alpha1.MachineEntryUnreachable, stationv1alpha1.MachineNodeUnreachable, stationv1alpha1.MachineHubUnreachable}, condition.Reason) {
				status.Problem = condition.Reason
			}
		}
	default:
		status.Problem, status.Message = "", ""
	}
	return reconcile.Result{}, nil
}

// nodeRemoved says whether the node a joined machine is was removed
// (StationTunnelRelease).
func (r *MachineReconciler) nodeRemoved(ctx context.Context, clusterID, nodeName string) (bool, error) {
	namespace, err := r.Clusters.Namespace(ctx, clusterID)
	if err != nil {
		return false, fmt.Errorf("resolve cluster %s: %w", clusterID, err)
	}
	admission := &stationv1alpha1.NodeAdmission{}
	if err := r.Reader.Get(ctx, types.NamespacedName{Namespace: namespace, Name: nodeName}, admission); apierrors.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("read NodeAdmission %s: %w", nodeName, err)
	}
	return AdmissionNodeRemoved(admission), nil
}
