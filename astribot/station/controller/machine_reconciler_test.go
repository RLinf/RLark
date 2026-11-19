package controller

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// machineInspections answers an inspection by the entry address it is made at.
type machineInspections struct {
	mu      sync.Mutex
	seen    map[string]MachineInspection
	refused map[string]error
	calls   int
}

func (m *machineInspections) Inspect(_ context.Context, _ string, at MachineAddress) (MachineInspection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	return m.seen[at.Entry], m.refused[at.Entry]
}

type machineClusters map[string]string

func (m machineClusters) Namespace(_ context.Context, clusterID string) (string, error) {
	return m[clusterID], nil
}

const factoryKey = "SHA256:fffffffffffffffffffffffffffffffffffffffffff"

func machineFixture(t *testing.T, objects ...client.Object) (*MachineReconciler, client.Client, *machineInspections) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := stationv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	_ = clientscheme.AddToScheme(scheme)
	class := &stationv1alpha1.NodeClass{ObjectMeta: metav1.ObjectMeta{Name: "s1u"}, Spec: stationv1alpha1.NodeClassSpec{
		Labels: map[string]string{"rlark.io/robot-model": "s1u", "rlark.io/robot-body": "s1u"}, Taints: []string{"rlark.io/node-category=robot:NoSchedule"},
		Naming:    &stationv1alpha1.NodeClassNaming{NodeName: "s1u-%02d-x86", Labels: map[string]string{"rlark.io/robot-id": "s1u-%02d"}},
		Discovery: &stationv1alpha1.NodeClassDiscovery{HostKeyFingerprints: []string{factoryKey}},
		Access: &stationv1alpha1.NodeClassAccess{
			Entry: stationv1alpha1.NodeClassEntry{Port: 22, User: "orin", SerialFile: "/proc/device-tree/serial-number", RotateHostKey: true, Password: stationv1alpha1.SecretValue{Secret: "s1u-access", Key: "password"}},
			Node:  stationv1alpha1.NodeClassHost{Address: "192.168.0.10", Port: 22, User: "robot", Password: stationv1alpha1.SecretValue{Secret: "s1u-access", Key: "x86_password"}}}}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&stationv1alpha1.Machine{}, &stationv1alpha1.NodeOperation{}).WithObjects(append(objects, class)...).Build()
	inspections := &machineInspections{seen: map[string]MachineInspection{}, refused: map[string]error{}}
	return &MachineReconciler{Client: c, Reader: c, Inspector: inspections, Clusters: machineClusters{"robots": "agents-robots"}, Namespace: "station-system"}, c, inspections
}

func discoveredMachine(number int, admitted bool) *stationv1alpha1.Machine {
	seen := metav1.Now()
	machine := &stationv1alpha1.Machine{
		ObjectMeta: metav1.ObjectMeta{Namespace: "station-system", Name: fmt.Sprintf("lab-48b02d5c9a%02d", number), Generation: 1, CreationTimestamp: metav1.NewTime(time.Unix(int64(1_800_000_000+number), 0))},
		Spec:       stationv1alpha1.MachineSpec{Site: "lab", NodeClass: "s1u", Source: "Discovery"},
		Status:     stationv1alpha1.MachineStatus{Entry: stationv1alpha1.MachineEntry{Address: fmt.Sprintf("192.168.31.%d", 100+number), MAC: fmt.Sprintf("48:b0:2d:5c:9a:%02d", number), HostKeyFingerprint: factoryKey, LastSeen: &seen}},
	}
	if admitted {
		machine.Spec.Admit = &stationv1alpha1.MachineAdmit{ClusterID: "robots", RequestedBy: "alice", RequestedAt: seen}
	}
	return machine
}

func machineRequest(machine *stationv1alpha1.Machine) reconcile.Request {
	return reconcile.Request{NamespacedName: client.ObjectKeyFromObject(machine)}
}

func TestTenMachinesAdmittedAtOnceGetTenNamesAndTenJoins(t *testing.T) {
	ctx := context.Background()
	objects := []client.Object{
		// Two robots are nodes already; the numbers go on from theirs.
		&stationv1alpha1.NodeAdmission{ObjectMeta: metav1.ObjectMeta{Namespace: "agents-robots", Name: "s1u-01-x86"}, Spec: stationv1alpha1.NodeAdmissionSpec{ClusterID: "robots"}},
		&stationv1alpha1.NodeAdmission{ObjectMeta: metav1.ObjectMeta{Namespace: "agents-robots", Name: "s1u-02-x86"}, Spec: stationv1alpha1.NodeAdmissionSpec{ClusterID: "robots"}},
	}
	machines := make([]*stationv1alpha1.Machine, 10)
	for i := range machines {
		machines[i] = discoveredMachine(i, true)
		objects = append(objects, machines[i])
	}
	r, c, inspections := machineFixture(t, objects...)
	for i, machine := range machines {
		inspections.seen[machine.Status.Entry.Address] = MachineInspection{Serial: fmt.Sprintf("17949250306%02d", i), NodeHostKey: fmt.Sprintf("SHA256:node%02d", i)}
	}
	var group sync.WaitGroup
	failures := make(chan error, len(machines))
	for _, machine := range machines {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := r.Reconcile(ctx, machineRequest(machine)); err != nil {
				failures <- err
			}
		}()
	}
	group.Wait()
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}

	names := map[string]string{}
	for _, machine := range machines {
		stored := &stationv1alpha1.Machine{}
		if err := c.Get(ctx, client.ObjectKeyFromObject(machine), stored); err != nil {
			t.Fatal(err)
		}
		if stored.Status.Phase != stationv1alpha1.MachineAdmitted || stored.Status.Problem != "" || names[stored.Status.NodeName] != "" {
			t.Fatalf("machine %s: %+v (name taken by %q)", machine.Name, stored.Status, names[stored.Status.NodeName])
		}
		names[stored.Status.NodeName] = machine.Name
		admission := &stationv1alpha1.NodeAdmission{}
		if err := c.Get(ctx, types.NamespacedName{Namespace: "agents-robots", Name: stored.Status.NodeName}, admission); err != nil {
			t.Fatal(err)
		}
		number := strings.TrimSuffix(strings.TrimPrefix(admission.Name, "s1u-"), "-x86")
		if admission.Spec.Serial != stored.Status.Serial || admission.Spec.NodeClass != "s1u" || admission.Spec.Labels["rlark.io/robot-id"] != "s1u-"+number ||
			admission.Spec.Labels["rlark.io/robot-model"] != "s1u" || admission.Spec.Site != "lab" || len(admission.Spec.Taints) != 1 {
			t.Fatalf("admission %s: %+v", admission.Name, admission.Spec)
		}
		operation := &stationv1alpha1.NodeOperation{}
		if err := c.Get(ctx, types.NamespacedName{Namespace: "agents-robots", Name: stored.Status.Operation}, operation); err != nil {
			t.Fatal(err)
		}
		if operation.Spec.Admission != admission.Name || operation.Spec.NodeClass != "s1u" || operation.Spec.SSH.Jump.Address != machine.Status.Entry.Address ||
			operation.Spec.SSH.Jump.HostKeyFingerprint != factoryKey || operation.Spec.SSH.HostKeyFingerprint != stored.Status.NodeHostKeyFingerprint || operation.Labels[MachineLabel] != machine.Name {
			t.Fatalf("operation %s: %+v", operation.Name, operation.Spec)
		}
	}
	for number := 3; number <= 12; number++ {
		if names[fmt.Sprintf("s1u-%02d-x86", number)] == "" {
			t.Fatalf("the ten machines took %v, not s1u-03-x86 to s1u-12-x86", names)
		}
	}
}

func TestAMachineAdmittedAgainIsTheNodeItWas(t *testing.T) {
	ctx := context.Background()
	machine := discoveredMachine(1, true)
	r, c, inspections := machineFixture(t, machine,
		&stationv1alpha1.NodeAdmission{ObjectMeta: metav1.ObjectMeta{Namespace: "agents-robots", Name: "s1u-07-x86"}, Spec: stationv1alpha1.NodeAdmissionSpec{ClusterID: "robots", Serial: "1794925030610"}})
	inspections.seen[machine.Status.Entry.Address] = MachineInspection{Serial: "1794925030610", NodeHostKey: "SHA256:node"}
	if _, err := r.Reconcile(ctx, machineRequest(machine)); err != nil {
		t.Fatal(err)
	}
	stored := &stationv1alpha1.Machine{}
	_ = c.Get(ctx, client.ObjectKeyFromObject(machine), stored)
	admissions := &stationv1alpha1.NodeAdmissionList{}
	_ = c.List(ctx, admissions)
	if stored.Status.NodeName != "s1u-07-x86" || stored.Status.Operation != "join-s1u-07-x86-1" || len(admissions.Items) != 1 {
		t.Fatalf("the name bound to the serial: %+v, %d admissions", stored.Status, len(admissions.Items))
	}
}

func TestAMachineWhoseNodeWasRemovedCanBeAdmittedAgain(t *testing.T) {
	ctx := context.Background()
	machine := discoveredMachine(1, false)
	inspected := metav1.Now()
	machine.Status.Phase, machine.Status.Serial, machine.Status.ClusterID, machine.Status.NodeName, machine.Status.Operation, machine.Status.InspectedAt =
		stationv1alpha1.MachineJoined, "1794925030610", "robots", "s1u-07-x86", "join-s1u-07-x86-1", &inspected
	// The Remove operation forgot the machine's key on the admission, as StationTunnelRelease does.
	admission := &stationv1alpha1.NodeAdmission{ObjectMeta: metav1.ObjectMeta{Namespace: "agents-robots", Name: "s1u-07-x86", Annotations: map[string]string{RemovedNodeAnnotation: "node-uid"}},
		Spec: stationv1alpha1.NodeAdmissionSpec{ClusterID: "robots", Serial: "1794925030610"}}
	r, c, inspections := machineFixture(t, machine, admission, discoveredMachine(2, false))
	inspections.seen[machine.Status.Entry.Address] = MachineInspection{Serial: "1794925030610", NodeHostKey: "SHA256:node"}

	requests := r.ofAdmission(ctx, admission)
	if len(requests) != 1 || requests[0] != machineRequest(machine) {
		t.Fatalf("the removed node's admission wakes %v, not its machine", requests)
	}
	if _, err := r.Reconcile(ctx, requests[0]); err != nil {
		t.Fatal(err)
	}
	stored := &stationv1alpha1.Machine{}
	_ = c.Get(ctx, client.ObjectKeyFromObject(machine), stored)
	if got := stored.Status; got.Phase != stationv1alpha1.MachineDiscovered || got.Problem != "" || got.ClusterID != "" || got.NodeName != "" || got.Operation != "" {
		t.Fatalf("a machine whose node was removed: %+v", got)
	}
}

func TestTwoMachinesWithOneSerial(t *testing.T) {
	ctx := context.Background()
	first, second := discoveredMachine(1, false), discoveredMachine(2, true)
	r, c, inspections := machineFixture(t, first, second)
	inspections.seen[first.Status.Entry.Address] = MachineInspection{Serial: "1794925030610", NodeHostKey: "SHA256:a"}
	inspections.seen[second.Status.Entry.Address] = MachineInspection{Serial: "1794925030610", NodeHostKey: "SHA256:b"}
	for _, machine := range []*stationv1alpha1.Machine{first, second} {
		if _, err := r.Reconcile(ctx, machineRequest(machine)); err != nil {
			t.Fatal(err)
		}
	}
	stored := &stationv1alpha1.Machine{}
	_ = c.Get(ctx, client.ObjectKeyFromObject(second), stored)
	operations := &stationv1alpha1.NodeOperationList{}
	_ = c.List(ctx, operations)
	if stored.Status.Problem != stationv1alpha1.MachineDuplicateSerial || stored.Status.Phase != stationv1alpha1.MachineDiscovered || len(operations.Items) != 0 {
		t.Fatalf("the later of two machines on the network with one serial is not admitted: %+v, %d operations", stored.Status, len(operations.Items))
	}
	_ = c.Get(ctx, client.ObjectKeyFromObject(first), stored)
	if stored.Status.Problem != "" || stored.Status.Serial != "1794925030610" {
		t.Fatalf("the earlier one: %+v", stored.Status)
	}

	// The same machine seen through another network adapter, the first one gone: it is that machine.
	_ = c.Get(ctx, client.ObjectKeyFromObject(first), stored)
	long := metav1.NewTime(time.Now().Add(-time.Hour))
	stored.Status.Entry.LastSeen = &long
	_ = c.Status().Update(ctx, stored)
	if _, err := r.Reconcile(ctx, machineRequest(second)); err != nil {
		t.Fatal(err)
	}
	_ = c.Get(ctx, client.ObjectKeyFromObject(second), stored)
	if err := c.Get(ctx, client.ObjectKeyFromObject(first), &stationv1alpha1.Machine{}); err == nil || stored.Status.Phase != stationv1alpha1.MachineAdmitted || stored.Status.NodeName != "s1u-01-x86" {
		t.Fatalf("the machine takes the place of its earlier sighting: %+v, earlier one %v", stored.Status, err)
	}
}

func TestAMachineSaysHowItsJoinGoesAndWhatIsWrongWithIt(t *testing.T) {
	ctx := context.Background()
	machine, refusing, joined := discoveredMachine(1, true), discoveredMachine(2, false), discoveredMachine(3, false)
	r, c, inspections := machineFixture(t, machine, refusing, joined,
		&stationv1alpha1.NodeAdmission{ObjectMeta: metav1.ObjectMeta{Namespace: "agents-robots", Name: "s1u-02-x86"}, Spec: stationv1alpha1.NodeAdmissionSpec{ClusterID: "robots"}})
	inspections.seen[machine.Status.Entry.Address] = MachineInspection{Serial: "A", NodeHostKey: "SHA256:a"}
	inspections.refused[refusing.Status.Entry.Address] = fmt.Errorf("jump host: %w", ErrSSHCredentialRejected)
	// The third is a node already, from before machines were discovered; its entry device shows the factory's key.
	inspections.seen[joined.Status.Entry.Address] = MachineInspection{Serial: "C", NodeHostKey: "SHA256:c", NodeName: "s1u-02-x86"}
	status := func(of *stationv1alpha1.Machine) stationv1alpha1.MachineStatus {
		t.Helper()
		if _, err := r.Reconcile(ctx, machineRequest(of)); err != nil {
			t.Fatal(err)
		}
		stored := &stationv1alpha1.Machine{}
		if err := c.Get(ctx, client.ObjectKeyFromObject(of), stored); err != nil {
			t.Fatal(err)
		}
		return stored.Status
	}

	if got := status(refusing); got.Phase != stationv1alpha1.MachineDiscovered || got.Problem != stationv1alpha1.MachineCredentialRejected {
		t.Fatalf("a machine that refuses its class's account: %+v", got)
	}
	// It is not asked again at every sighting.
	before := inspections.calls
	if status(refusing); inspections.calls != before {
		t.Fatal("a refused account was tried again within the same five minutes")
	}

	adopted := status(joined)
	bound := &stationv1alpha1.NodeAdmission{}
	_ = c.Get(ctx, types.NamespacedName{Namespace: "agents-robots", Name: "s1u-02-x86"}, bound)
	if adopted.Phase != stationv1alpha1.MachineAdmitted || adopted.NodeName != "s1u-02-x86" || adopted.Operation != "join-s1u-02-x86-1" || bound.Spec.Serial != "C" {
		t.Fatalf("a node whose entry device still shows the factory's key gets a join that rotates it: %+v, admission serial %q", adopted, bound.Spec.Serial)
	}

	admitted := status(machine)
	operation := &stationv1alpha1.NodeOperation{}
	key := types.NamespacedName{Namespace: "agents-robots", Name: admitted.Operation}
	if err := c.Get(ctx, key, operation); err != nil {
		t.Fatal(err)
	}
	operation.Status.Phase = "Blocked"
	operation.Status.Conditions = []metav1.Condition{{Type: "Progressing", Status: metav1.ConditionFalse, Reason: stationv1alpha1.MachineCredentialRejected, Message: "Node operation stage Preflight could not log in to the machine", LastTransitionTime: metav1.Now()}}
	_ = c.Status().Update(ctx, operation)
	if got := status(machine); got.Phase != stationv1alpha1.MachineAdmitted || got.Problem != stationv1alpha1.MachineCredentialRejected || !strings.Contains(got.Message, "Preflight") {
		t.Fatalf("a join that stopped at a refused account: %+v", got)
	}
	// Admitted again: the stopped join gives way to a new one.
	stored := &stationv1alpha1.Machine{}
	_ = c.Get(ctx, client.ObjectKeyFromObject(machine), stored)
	stored.Generation = 2
	stored.Spec.Admit.RequestedAt = metav1.NewTime(time.Now().Add(time.Minute))
	_ = c.Update(ctx, stored)
	if got := status(machine); got.Operation != "join-s1u-01-x86-2" || got.Problem != "" {
		t.Fatalf("a new request starts a new join: %+v", got)
	}
	if err := c.Get(ctx, key, operation); err == nil && operation.DeletionTimestamp == nil {
		t.Fatal("the stopped operation is still there")
	}

	_ = c.Get(ctx, types.NamespacedName{Namespace: "agents-robots", Name: "join-s1u-01-x86-2"}, operation)
	operation.Status.Phase = "Succeeded"
	_ = c.Status().Update(ctx, operation)
	admission := &stationv1alpha1.NodeAdmission{}
	_ = c.Get(ctx, types.NamespacedName{Namespace: "agents-robots", Name: "s1u-01-x86"}, admission)
	admission.Annotations[EntryHostKeyAnnotation] = "SHA256:its-own"
	_ = c.Update(ctx, admission)
	if got := status(machine); got.Phase != stationv1alpha1.MachineJoined || got.Entry.HostKeyFingerprint != "SHA256:its-own" {
		t.Fatalf("a machine whose join succeeded: %+v", got)
	}
}

func TestAMachineIsLookedAtOnceItIsKnownWhereAndAgainAfterAFaultOfTheDial(t *testing.T) {
	ctx := context.Background()
	machine := discoveredMachine(1, false)
	address := machine.Status.Entry.Address
	// Station API has created the Machine and not yet said where it saw it.
	machine.Status.Entry = stationv1alpha1.MachineEntry{}
	reconciler, c, inspections := machineFixture(t, machine)
	if _, err := reconciler.Reconcile(ctx, machineRequest(machine)); err != nil || inspections.calls != 0 {
		t.Fatalf("a machine without an address was looked at: %d logins, %v", inspections.calls, err)
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(machine), machine); err != nil {
		t.Fatal(err)
	}
	seen := metav1.Now()
	machine.Status.Entry = stationv1alpha1.MachineEntry{Address: address, HostKeyFingerprint: factoryKey, LastSeen: &seen}
	if err := c.Status().Update(ctx, machine); err != nil {
		t.Fatal(err)
	}
	// The relay does not answer: nothing was learned of the machine, and the next pass logs in again.
	inspections.refused[address] = fmt.Errorf("connect to the site's relay: timeout")
	if _, err := reconciler.Reconcile(ctx, machineRequest(machine)); err == nil {
		t.Fatal("a fault of the dial was not returned")
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(machine), machine); err != nil || machine.Status.InspectedAt != nil || machine.Status.Problem != "" {
		t.Fatalf("a fault of the dial was taken for a look at the machine: %+v, %v", machine.Status, err)
	}
	delete(inspections.refused, address)
	inspections.seen[address] = MachineInspection{Serial: "1794925030610", NodeHostKey: "SHA256:node"}
	if _, err := reconciler.Reconcile(ctx, machineRequest(machine)); err != nil {
		t.Fatal(err)
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(machine), machine); err != nil || machine.Status.Phase != stationv1alpha1.MachineDiscovered || machine.Status.Serial != "1794925030610" {
		t.Fatalf("the machine was not read after the fault: %+v, %v", machine.Status, err)
	}
}

func TestAMachineWhoseClassLacksAPasswordIsNotLoggedInToUntilItIsEntered(t *testing.T) {
	ctx := context.Background()
	machine := discoveredMachine(1, false)
	// Where nothing listens: a login that is tried is refused at once.
	machine.Status.Entry.Address = "127.0.0.1"
	reconciler, c, _ := machineFixture(t, machine, rlarkSecret("s1u-access", map[string]string{"password": "entry-secret"}))
	class := &stationv1alpha1.NodeClass{}
	if err := c.Get(ctx, types.NamespacedName{Name: "s1u"}, class); err != nil {
		t.Fatal(err)
	}
	class.Spec.Access.Entry.Port = 1
	if err := c.Update(ctx, class); err != nil {
		t.Fatal(err)
	}
	reconciler.Inspector = SSHMachineInspector{Access: NodeClassAccess{Reader: c, Namespace: "default"}}
	status := func() stationv1alpha1.MachineStatus {
		if _, err := reconciler.Reconcile(ctx, machineRequest(machine)); err != nil {
			t.Fatal(err)
		}
		read := &stationv1alpha1.Machine{}
		if err := c.Get(ctx, client.ObjectKeyFromObject(machine), read); err != nil {
			t.Fatal(err)
		}
		return read.Status
	}
	if got := status(); got.Problem != stationv1alpha1.MachineAccessUnconfigured || strings.Contains(got.Message, "entry-secret") {
		t.Fatalf("a machine whose class has no x86 password: %+v", got)
	}
	// Entered: the next sighting logs in, and finds nobody at the address.
	if err := c.Update(ctx, rlarkSecret("s1u-access", map[string]string{"password": "entry-secret", "x86_password": "node-secret"})); err != nil {
		t.Fatal(err)
	}
	if _, err := reconciler.Reconcile(ctx, machineRequest(machine)); err == nil || !strings.Contains(err.Error(), "connect SSH") {
		t.Fatalf("the machine was not looked at once the password was entered: %v", err)
	}
}
