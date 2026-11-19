package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	kubernetesobjects "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/compatibility/kubernetes/kubernetesobjects"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/evidence"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/observability"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

func TestProviderGetPreservesKubernetesObject(t *testing.T) {
	scheme := testScheme(t)
	deployment := managedDeployment()
	provider := testProvider(scheme, deployment)
	object, err := provider.Get(context.Background(), agentruntime.ObjectRef{RuntimeID: DefaultID, ID: "apps/v1, Kind=Deployment", Namespace: "runtime", Name: "task", UID: "deployment-uid", LogicalTask: logicalTask()})
	if err != nil {
		t.Fatal(err)
	}
	var got appsv1.Deployment
	if err := json.Unmarshal(object.Manifest.Object, &got); err != nil {
		t.Fatal(err)
	}
	if got.Spec.Template.Spec.Containers[0].Image != "example:v1" || object.Manifest.Type.Kind != "Deployment" {
		t.Fatalf("object=%#v manifest=%#v", object.Ref, got)
	}
}

func TestProviderListManagedReturnsCompleteCompatibleSet(t *testing.T) {
	scheme := testScheme(t)
	deployment := managedDeployment()
	conflict := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "runtime", Annotations: managementAnnotations("other-uid")}}
	unrelated := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "unrelated", Namespace: "runtime"}}
	provider := testProvider(scheme, deployment, conflict, unrelated)
	snapshot, err := provider.ListManaged(context.Background(), taskIdentity())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Objects) != 1 {
		t.Fatalf("managed objects=%v, want only the deployment", snapshot.Objects)
	}
	if snapshot.Revision == "" {
		t.Fatal("snapshot revision is empty")
	}
}

func TestProviderListManagedDiscoversOriginMainWorkloadWithoutAnnotations(t *testing.T) {
	scheme := testScheme(t)
	for _, object := range []client.Object{
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "runtime", UID: "deployment-uid"}},
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "runtime", UID: "daemonset-uid"}},
		&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "runtime", UID: "statefulset-uid"}},
	} {
		t.Run(object.GetObjectKind().GroupVersionKind().Kind+string(object.GetUID()), func(t *testing.T) {
			provider := testProvider(scheme, object)
			snapshot, err := provider.ListManaged(context.Background(), taskIdentity())
			if err != nil {
				t.Fatal(err)
			}
			if len(snapshot.Objects) != 1 || snapshot.Objects[0].Ref.UID != string(object.GetUID()) {
				t.Fatalf("managed=%#v", snapshot.Objects)
			}
		})
	}
}

func TestProviderListManagedDoesNotAdoptNameOnlyAuxiliaryResources(t *testing.T) {
	scheme := testScheme(t)
	config := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "runtime", UID: "config-uid"}}
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "runtime", UID: "service-uid"}}
	provider := testProvider(scheme, config, service)
	snapshot, err := provider.ListManaged(context.Background(), taskIdentity())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Objects) != 0 {
		t.Fatalf("adopted unproven auxiliary resources: %#v", snapshot.Objects)
	}
}

func TestProviderListCandidatesDiscoversOnlyCurrentDeclaredService(t *testing.T) {
	scheme := testScheme(t)
	current := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "task-service", Namespace: "runtime", UID: "service-uid", Annotations: managementAnnotations("task-uid")}}
	provider := testProvider(scheme, current)
	query := agentruntime.ManagedObjectQuery{Task: taskIdentity(), Candidates: []agentruntime.ObjectCandidate{{Kind: agentruntime.ObjectKind{Version: "v1", Kind: "Service"}, Namespace: "runtime", Name: "task-service", Ownership: agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid"}, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup}}}
	snapshot, err := provider.ListCandidates(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Objects) != 1 || snapshot.Objects[0].Ref.UID != "service-uid" {
		t.Fatalf("managed=%#v, want current declared Service", snapshot.Objects)
	}

	current.Annotations = managementAnnotations("old-task-uid")
	provider = testProvider(scheme, current)
	snapshot, err = provider.ListCandidates(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Objects) != 0 {
		t.Fatalf("managed stale declared Service: %#v", snapshot.Objects)
	}
}

func TestProviderListCandidatesProvesLegacyAuxiliaryThroughOwnedController(t *testing.T) {
	scheme := testScheme(t)
	root := managedDeployment()
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "task-service", Namespace: "runtime", UID: "service-uid", OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: root.Name, UID: root.UID, Controller: ptr(true)}}}}
	provider := testProvider(scheme, root, service)
	adopt := agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid", IncarnationPolicy: agentruntime.IncarnationOwnershipAllowAdoption}
	query := agentruntime.ManagedObjectQuery{Task: taskIdentity(), Candidates: []agentruntime.ObjectCandidate{
		{Kind: agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"}, Namespace: "runtime", Name: "task", Ownership: adopt, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup},
		{Kind: agentruntime.ObjectKind{Version: "v1", Kind: "Service"}, Namespace: "runtime", Name: "task-service", Ownership: adopt, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup},
	}}
	snapshot, err := provider.ListCandidates(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Objects) != 2 {
		t.Fatalf("managed=%#v, want proven root and auxiliary Service", snapshot.Objects)
	}
}

func TestProviderListCandidatesIncludesExactLegacyAuxiliaryForUniqueWorkloadRoot(t *testing.T) {
	scheme := testScheme(t)
	root := managedDeployment()
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "task-ray-head", Namespace: "runtime", UID: "service-uid", Labels: map[string]string{"app": "task"}}}
	provider := testProvider(scheme, root, service)
	adopt := agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid", IncarnationPolicy: agentruntime.IncarnationOwnershipAllowAdoption}
	query := agentruntime.ManagedObjectQuery{Task: taskIdentity(), Candidates: []agentruntime.ObjectCandidate{
		{Kind: agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"}, Namespace: "runtime", Name: "task", Ownership: adopt, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup},
		{Kind: agentruntime.ObjectKind{Version: "v1", Kind: "Service"}, Namespace: "runtime", Name: service.Name, Ownership: adopt, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup},
	}}
	snapshot, err := provider.ListCandidates(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Objects) != 2 {
		t.Fatalf("managed=%#v, want proven root and exact legacy auxiliary", snapshot.Objects)
	}
}

func TestProviderListCandidatesExactLegacyAuxiliaryRequiresProvenRoot(t *testing.T) {
	scheme := testScheme(t)
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "task-ray-head", Namespace: "runtime", UID: "service-uid"}}
	adopt := agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid", IncarnationPolicy: agentruntime.IncarnationOwnershipAllowAdoption}
	query := agentruntime.ManagedObjectQuery{Task: taskIdentity(), Candidates: []agentruntime.ObjectCandidate{{Kind: agentruntime.ObjectKind{Version: "v1", Kind: "Service"}, Namespace: "runtime", Name: service.Name, Ownership: adopt, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup}}}
	snapshot, err := testProvider(scheme, service).ListCandidates(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Objects) != 0 {
		t.Fatalf("managed=%#v, legacy auxiliary without root must remain unclaimed", snapshot.Objects)
	}
}

func TestProviderListCandidatesDoesNotForceAdoptLabelCandidate(t *testing.T) {
	scheme := testScheme(t)
	root := managedDeployment()
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "task-ray-head", Namespace: "runtime", UID: "service-uid", Labels: map[string]string{"legacy": "true"}}}
	adopt := agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid", IncarnationPolicy: agentruntime.IncarnationOwnershipAllowAdoption}
	query := agentruntime.ManagedObjectQuery{Task: taskIdentity(), Candidates: []agentruntime.ObjectCandidate{
		{Kind: agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"}, Namespace: "runtime", Name: "task", Ownership: adopt, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup},
		{Kind: agentruntime.ObjectKind{Version: "v1", Kind: "Service"}, Namespace: "runtime", Labels: map[string]string{"legacy": "true"}, Ownership: adopt, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup},
	}}
	snapshot, err := testProvider(scheme, root, service).ListCandidates(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Objects) != 1 || snapshot.Objects[0].Ref.UID != string(root.UID) {
		t.Fatalf("managed=%#v, label-selected auxiliary must remain unclaimed", snapshot.Objects)
	}
}

func TestProviderListCandidatesIncludesOwnedTypedLabelCandidate(t *testing.T) {
	scheme := testScheme(t)
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Name:        "task-service",
		Namespace:   "runtime",
		UID:         "service-uid",
		Labels:      map[string]string{"app": "task"},
		Annotations: managementAnnotations("task-uid"),
	}}
	ownership := agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid", IncarnationPolicy: agentruntime.IncarnationOwnershipAllowAdoption}
	query := agentruntime.ManagedObjectQuery{Task: taskIdentity(), Candidates: []agentruntime.ObjectCandidate{{
		Kind: agentruntime.ObjectKind{Version: "v1", Kind: "Service"}, Namespace: "runtime",
		Labels: map[string]string{"app": "task"}, Ownership: ownership, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup,
	}}}
	snapshot, err := testProvider(scheme, service).ListCandidates(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Objects) != 1 || snapshot.Objects[0].Ref.UID != string(service.UID) || snapshot.Objects[0].Manifest.Type.Kind != "Service" {
		t.Fatalf("managed=%#v, want owned typed Service label candidate", snapshot.Objects)
	}
}

func TestProviderListCandidatesRejectsUnprovenLegacyAuxiliary(t *testing.T) {
	scheme := testScheme(t)
	root := managedDeployment()
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "task-service", Namespace: "runtime", UID: "service-uid", OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: root.Name, UID: "foreign-root", Controller: ptr(true)}}}}
	provider := testProvider(scheme, root, service)
	adopt := agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid", IncarnationPolicy: agentruntime.IncarnationOwnershipAllowAdoption}
	query := agentruntime.ManagedObjectQuery{Task: taskIdentity(), Candidates: []agentruntime.ObjectCandidate{
		{Kind: agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"}, Namespace: "runtime", Name: "task", Ownership: adopt, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup},
		{Kind: agentruntime.ObjectKind{Version: "v1", Kind: "Service"}, Namespace: "runtime", Name: "task-service", Ownership: adopt, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup},
	}}
	snapshot, err := provider.ListCandidates(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Objects) != 1 || snapshot.Objects[0].Ref.UID != string(root.UID) {
		t.Fatalf("managed=%#v, want only proven root", snapshot.Objects)
	}
}

func TestProviderListCandidatesRejectsForeignCanonicalAuxiliary(t *testing.T) {
	scheme := testScheme(t)
	root := managedDeployment()
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "task-service", Namespace: "runtime", UID: "service-uid", Annotations: managementAnnotations("other-task-uid"), OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: root.Name, UID: root.UID, Controller: ptr(true)}}}}
	provider := testProvider(scheme, root, service)
	adopt := agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid", IncarnationPolicy: agentruntime.IncarnationOwnershipAllowAdoption}
	query := agentruntime.ManagedObjectQuery{Task: taskIdentity(), Candidates: []agentruntime.ObjectCandidate{
		{Kind: agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"}, Namespace: "runtime", Name: "task", Ownership: adopt, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup},
		{Kind: agentruntime.ObjectKind{Version: "v1", Kind: "Service"}, Namespace: "runtime", Name: "task-service", Ownership: adopt, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup},
	}}
	snapshot, err := provider.ListCandidates(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Objects) != 1 || snapshot.Objects[0].Ref.UID != string(root.UID) {
		t.Fatalf("managed=%#v, want foreign Service excluded", snapshot.Objects)
	}
}

func TestProviderListCandidatesUsesOnlyDeclaredTargetedQueries(t *testing.T) {
	scheme := testScheme(t)
	base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(managedDeployment()).Build()
	recording := &recordingClient{Client: base}
	provider := New(recording, scheme, func(agentruntime.TaskIdentity) string { return "runtime" })
	query := agentruntime.ManagedObjectQuery{Task: taskIdentity(), Candidates: []agentruntime.ObjectCandidate{
		{Kind: agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"}, Namespace: "runtime", Name: "task", Ownership: agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid"}, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup},
		{Kind: agentruntime.ObjectKind{Version: "v1", Kind: "Service"}, Namespace: "runtime", Name: "task-service", Ownership: agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid"}, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup},
	}}
	if _, err := provider.ListCandidates(context.Background(), query); err != nil {
		t.Fatal(err)
	}
	if len(recording.gets) != 2 || recording.gets[0] != "*v1.Deployment runtime/task" || recording.gets[1] != "*v1.Service runtime/task-service" {
		t.Fatalf("gets=%v, want only declared exact candidates", recording.gets)
	}
	if len(recording.lists) != 0 {
		t.Fatalf("lists=%v, want no label-selected compatibility queries", recording.lists)
	}
}

func TestProviderListCandidatesRejectsUnboundedOrInvalidCandidates(t *testing.T) {
	provider := testProvider(testScheme(t))
	base := agentruntime.ObjectCandidate{Kind: agentruntime.ObjectKind{Version: "v1", Kind: "Service"}, Namespace: "runtime", Name: "task-head", Ownership: agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid"}, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup}
	tests := map[string]agentruntime.ObjectCandidate{
		"missing kind": func() agentruntime.ObjectCandidate { candidate := base; candidate.Kind.Kind = ""; return candidate }(),
		"outside namespace": func() agentruntime.ObjectCandidate {
			candidate := base
			candidate.Namespace = "other"
			return candidate
		}(),
		"missing selector": func() agentruntime.ObjectCandidate { candidate := base; candidate.Name = ""; return candidate }(),
		"ambiguous selector": func() agentruntime.ObjectCandidate {
			candidate := base
			candidate.Labels = map[string]string{"app": "task"}
			return candidate
		}(),
		"missing ownership": func() agentruntime.ObjectCandidate {
			candidate := base
			candidate.Ownership = agentruntime.ResourceOwnership{}
			return candidate
		}(),
	}
	for name, candidate := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := provider.ListCandidates(context.Background(), agentruntime.ManagedObjectQuery{Task: taskIdentity(), Candidates: []agentruntime.ObjectCandidate{candidate}}); !fault.Is(err, fault.InvalidArgument) {
				t.Fatalf("error=%v, want invalid argument", err)
			}
		})
	}
}

func TestProviderClaimedWorkloadRequiresUniqueOwnedRevision(t *testing.T) {
	scheme := testScheme(t)
	revision := agentruntime.ExecutionRevision{RuntimeID: DefaultID, RuntimeKind: Kind, WorkloadType: "Deployment", WorkloadVersion: "v1alpha1", CompiledDigest: string(evidence.NewDigest([]byte("compiled"))), ExecutableDigest: string(evidence.NewDigest([]byte("executable"))), BindingRevision: string(evidence.NewDigest([]byte("binding")))}
	canonical := managedDeployment()
	if err := WriteExecutionRevision(canonical, revision); err != nil {
		t.Fatal(err)
	}
	t.Run("recovers unique canonical root", func(t *testing.T) {
		provider := testProvider(scheme, canonical.DeepCopy())
		workloadType, version, err := provider.ClaimedWorkload(context.Background(), taskIdentity())
		if err != nil || workloadType != "Deployment" || version != "v1alpha1" {
			t.Fatalf("ClaimedWorkload()=%q, %q, %v", workloadType, version, err)
		}
	})
	t.Run("rejects incomplete canonical revision", func(t *testing.T) {
		provider := testProvider(scheme, managedDeployment())
		if _, _, err := provider.ClaimedWorkload(context.Background(), taskIdentity()); !fault.Is(err, fault.FailedPrecondition) {
			t.Fatalf("error=%v, want failed precondition", err)
		}
	})
	t.Run("ignores name-only root", func(t *testing.T) {
		foreign := managedDeployment()
		foreign.Annotations = nil
		provider := testProvider(scheme, foreign)
		if _, _, err := provider.ClaimedWorkload(context.Background(), taskIdentity()); !fault.Is(err, fault.NotFound) {
			t.Fatalf("error=%v, want not found", err)
		}
	})
	t.Run("rejects ambiguous roots", func(t *testing.T) {
		stateful := &appsv1.StatefulSet{ObjectMeta: canonical.ObjectMeta}
		stateful.UID = "stateful-uid"
		statefulRevision := revision
		statefulRevision.WorkloadType = "StatefulSet"
		if err := WriteExecutionRevision(stateful, statefulRevision); err != nil {
			t.Fatal(err)
		}
		provider := testProvider(scheme, canonical.DeepCopy(), stateful)
		if _, _, err := provider.ClaimedWorkload(context.Background(), taskIdentity()); !fault.Is(err, fault.Conflict) {
			t.Fatalf("error=%v, want conflict", err)
		}
	})
	t.Run("rejects foreign runtime", func(t *testing.T) {
		foreign := managedDeployment()
		foreignRevision := revision
		foreignRevision.RuntimeID = "other"
		if err := WriteExecutionRevision(foreign, foreignRevision); err != nil {
			t.Fatal(err)
		}
		provider := testProvider(scheme, foreign)
		if _, _, err := provider.ClaimedWorkload(context.Background(), taskIdentity()); !fault.Is(err, fault.Conflict) {
			t.Fatalf("error=%v, want conflict", err)
		}
	})
}

func TestProviderListInstancesUsesControllerOwnership(t *testing.T) {
	scheme := testScheme(t)
	deployment := managedDeployment()
	controller := true
	replicaSet := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "task-rs", Namespace: "runtime", UID: "rs-uid", OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "task", UID: deployment.UID, Controller: &controller}}}}
	owned := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "owned", Namespace: "runtime", UID: "pod-uid", Labels: map[string]string{"app": "task"}, OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: replicaSet.Name, UID: replicaSet.UID, Controller: &controller}}}, Status: corev1.PodStatus{Phase: corev1.PodRunning}}
	stale := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "stale", Namespace: "runtime", UID: "stale-uid", Labels: map[string]string{"app": "task"}, OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: replicaSet.Name, UID: "old-rs", Controller: &controller}}}}
	provider := testProvider(scheme, deployment, replicaSet, owned, stale)
	ref := objectRef(deployment)
	snapshot, err := provider.ListInstances(context.Background(), agentruntime.InstanceQuery{Task: taskIdentity(), Workloads: []agentruntime.ObjectRef{ref}})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Instances) != 1 || snapshot.Instances[0].Name != "owned" || snapshot.Instances[0].Phase != agentruntime.InstancePhase(corev1.PodRunning) {
		t.Fatalf("instances=%#v", snapshot.Instances)
	}
}

func TestProviderListInstancesProjectsContainerFailuresAndSchedulingEvents(t *testing.T) {
	scheme := testScheme(t)
	deployment := managedDeployment()
	replicaSet := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "task-rs", Namespace: "runtime", UID: "rs-uid", OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: deployment.Name, UID: deployment.UID, Controller: ptr(true)}}}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "owned", Namespace: "runtime", UID: "pod-uid", Labels: map[string]string{"app": "task"}, OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: replicaSet.Name, UID: replicaSet.UID, Controller: ptr(true)}}}, Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: "trainer", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff", Message: "back-off"}}}}}}
	event := &corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: "schedule", Namespace: "runtime"}, InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: pod.Name, UID: pod.UID}, Reason: "FailedScheduling", Message: "no nodes"}
	provider := testProvider(scheme, deployment, replicaSet, pod, event)
	snapshot, err := provider.ListInstances(context.Background(), agentruntime.InstanceQuery{Task: taskIdentity(), Workloads: []agentruntime.ObjectRef{objectRef(deployment)}})
	if err != nil {
		t.Fatal(err)
	}
	conditions := snapshot.Instances[0].Conditions
	if len(conditions) != 2 {
		t.Fatalf("conditions=%#v", conditions)
	}
	if conditions[0].Scope != "container" || conditions[0].Subject != "trainer" || conditions[0].Reason != "CrashLoopBackOff" {
		t.Fatalf("container condition=%#v", conditions[0])
	}
	if conditions[1].Scope != "instance" || conditions[1].Reason != "FailedScheduling" {
		t.Fatalf("event condition=%#v", conditions[1])
	}
	withoutEvent := testProvider(scheme, deployment, replicaSet, pod)
	other, err := withoutEvent.ListInstances(context.Background(), agentruntime.InstanceQuery{Task: taskIdentity(), Workloads: []agentruntime.ObjectRef{objectRef(deployment)}})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Revision == other.Revision {
		t.Fatal("event changes did not change the instance snapshot revision")
	}
}

func TestProviderListInstancesUsesPodUIDEventQueries(t *testing.T) {
	scheme := testScheme(t)
	deployment := managedDeployment()
	replicaSet := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "task-rs", Namespace: "runtime", UID: "rs-uid", OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: deployment.Name, UID: deployment.UID, Controller: ptr(true)}}}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "owned", Namespace: "runtime", UID: "pod-uid", Labels: map[string]string{"app": "task"}, OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: replicaSet.Name, UID: replicaSet.UID, Controller: ptr(true)}}}}
	base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deployment, replicaSet, pod).
		WithIndex(&corev1.Event{}, "involvedObject.uid", func(object client.Object) []string {
			return []string{string(object.(*corev1.Event).InvolvedObject.UID)}
		}).Build()
	recording := &recordingClient{Client: base}
	provider := New(recording, scheme, func(agentruntime.TaskIdentity) string { return "runtime" })
	if _, err := provider.ListInstances(context.Background(), agentruntime.InstanceQuery{Task: taskIdentity(), Workloads: []agentruntime.ObjectRef{objectRef(deployment)}}); err != nil {
		t.Fatal(err)
	}
	want := "*v1.EventList namespace=runtime labels= fields=involvedObject.uid=pod-uid"
	found := false
	for _, query := range recording.lists {
		if query == want {
			found = true
		}
		if query == "*v1.EventList namespace=runtime labels= fields=" {
			t.Fatalf("namespace-wide Event query recorded: %v", recording.lists)
		}
	}
	if !found {
		t.Fatalf("lists=%v, want %q", recording.lists, want)
	}
}

func TestProviderListInstancesIgnoresEventsForReplacedPod(t *testing.T) {
	scheme := testScheme(t)
	deployment := managedDeployment()
	replicaSet := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "task-rs", Namespace: "runtime", UID: "rs-uid", OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: deployment.Name, UID: deployment.UID, Controller: ptr(true)}}}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "owned", Namespace: "runtime", UID: "new-pod-uid", Labels: map[string]string{"app": "task"}, OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: replicaSet.Name, UID: replicaSet.UID, Controller: ptr(true)}}}}
	oldEvent := &corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: "runtime"}, InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: pod.Name, UID: "old-pod-uid"}, Reason: "FailedScheduling", Message: "stale"}
	provider := testProvider(scheme, deployment, replicaSet, pod, oldEvent)
	snapshot, err := provider.ListInstances(context.Background(), agentruntime.InstanceQuery{Task: taskIdentity(), Workloads: []agentruntime.ObjectRef{objectRef(deployment)}})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Instances) != 1 || len(snapshot.Instances[0].Conditions) != 0 {
		t.Fatalf("instances=%#v, want replaced Pod event ignored", snapshot.Instances)
	}
}

func TestProviderApplyCreatesAndSameGenerationDoesNotUpdate(t *testing.T) {
	scheme := testScheme(t)
	counting := &countingClient{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	provider := New(counting, scheme, func(agentruntime.TaskIdentity) string { return "runtime" })
	want := managedDeployment()
	want.UID, want.ResourceVersion = "", ""
	want.Annotations[kubernetesobjects.ManagementTaskGenerationAnnotation] = "3"
	desired := desiredObject(t, want)
	result, err := provider.Apply(context.Background(), agentruntime.ApplyRequest{IdempotencyKey: "create", Task: taskIdentity(), Desired: desired})
	if err != nil {
		t.Fatal(err)
	}
	if counting.creates != 1 || counting.updates != 0 {
		t.Fatalf("creates=%d updates=%d", counting.creates, counting.updates)
	}
	if _, err := provider.Apply(context.Background(), agentruntime.ApplyRequest{IdempotencyKey: "repeat", Task: taskIdentity(), Desired: desired, Current: &result.Ref}); err != nil {
		t.Fatal(err)
	}
	if counting.updates != 0 {
		t.Fatalf("same Task generation caused %d updates and may restart Pods", counting.updates)
	}
}

func TestProviderApplyNormalizesKubernetesDefaultsBeforeComparingTemplate(t *testing.T) {
	scheme := testScheme(t)
	existing := managedDeployment()
	existing.Annotations[kubernetesobjects.ManagementTaskGenerationAnnotation] = "3"
	grace := int64(30)
	existing.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyAlways
	existing.Spec.Template.Spec.DNSPolicy = corev1.DNSClusterFirst
	existing.Spec.Template.Spec.SchedulerName = corev1.DefaultSchedulerName
	existing.Spec.Template.Spec.TerminationGracePeriodSeconds = &grace
	existing.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{}
	existing.Spec.Template.Spec.Containers[0].ImagePullPolicy = corev1.PullIfNotPresent
	existing.Spec.Template.Spec.Containers[0].TerminationMessagePath = corev1.TerminationMessagePathDefault
	existing.Spec.Template.Spec.Containers[0].TerminationMessagePolicy = corev1.TerminationMessageReadFile
	desired := managedDeployment()
	desired.UID, desired.ResourceVersion = "", ""
	desired.Annotations[kubernetesobjects.ManagementTaskGenerationAnnotation] = "3"
	if desired.Spec.Template.Spec.RestartPolicy != "" || desired.Spec.Template.Spec.DNSPolicy != "" || desired.Spec.Template.Spec.SchedulerName != "" {
		t.Fatal("test desired workload unexpectedly contains API defaults")
	}
	counting := &countingClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()}
	provider := New(counting, scheme, func(agentruntime.TaskIdentity) string { return "runtime" })

	if _, err := provider.Apply(context.Background(), agentruntime.ApplyRequest{Task: taskIdentity(), Desired: desiredObject(t, desired), Current: ptrRef(objectRef(existing))}); err != nil {
		t.Fatal(err)
	}
	if counting.updates != 0 {
		t.Fatalf("API-defaulted Pod template caused %d updates, want zero", counting.updates)
	}
}

func TestPreserveObservedPodDefaultsDoesNotOverrideExplicitDesiredValues(t *testing.T) {
	current := &corev1.PodTemplateSpec{Spec: corev1.PodSpec{
		RestartPolicy: corev1.RestartPolicyAlways,
		DNSPolicy:     corev1.DNSClusterFirst,
		SchedulerName: corev1.DefaultSchedulerName,
		Containers:    []corev1.Container{{Name: "main", ImagePullPolicy: corev1.PullIfNotPresent, TerminationMessagePath: corev1.TerminationMessagePathDefault, TerminationMessagePolicy: corev1.TerminationMessageReadFile}},
	}}
	desired := &corev1.PodTemplateSpec{Spec: corev1.PodSpec{
		RestartPolicy: corev1.RestartPolicyNever,
		DNSPolicy:     corev1.DNSNone,
		SchedulerName: "custom-scheduler",
		Containers:    []corev1.Container{{Name: "main", ImagePullPolicy: corev1.PullAlways, TerminationMessagePath: "/custom", TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError}},
	}}
	preserveObservedPodDefaults(current, desired)
	if desired.Spec.RestartPolicy != corev1.RestartPolicyNever || desired.Spec.DNSPolicy != corev1.DNSNone || desired.Spec.SchedulerName != "custom-scheduler" {
		t.Fatalf("explicit Pod defaults were overwritten: %#v", desired.Spec)
	}
	container := desired.Spec.Containers[0]
	if container.ImagePullPolicy != corev1.PullAlways || container.TerminationMessagePath != "/custom" || container.TerminationMessagePolicy != corev1.TerminationMessageFallbackToLogsOnError {
		t.Fatalf("explicit container defaults were overwritten: %#v", container)
	}
}

func TestPreserveObservedPodDefaultsDoesNotPreserveNonDefaultDrift(t *testing.T) {
	grace := int64(60)
	current := &corev1.PodTemplateSpec{Spec: corev1.PodSpec{
		RestartPolicy:                 corev1.RestartPolicyNever,
		DNSPolicy:                     corev1.DNSNone,
		SchedulerName:                 "external-scheduler",
		TerminationGracePeriodSeconds: &grace,
		Containers: []corev1.Container{{
			Name:                     "main",
			Image:                    "example:v1",
			ImagePullPolicy:          corev1.PullAlways,
			TerminationMessagePath:   "/external",
			TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
		}},
	}}
	desired := &corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "example:v1"}}}}
	preserveObservedPodDefaults(current, desired)
	if desired.Spec.RestartPolicy != "" || desired.Spec.DNSPolicy != "" || desired.Spec.SchedulerName != "" || desired.Spec.TerminationGracePeriodSeconds != nil {
		t.Fatalf("non-default Pod drift was preserved: %#v", desired.Spec)
	}
	container := desired.Spec.Containers[0]
	if container.ImagePullPolicy != "" || container.TerminationMessagePath != "" || container.TerminationMessagePolicy != "" {
		t.Fatalf("non-default container drift was preserved: %#v", container)
	}
}

func TestDefaultImagePullPolicy(t *testing.T) {
	tests := map[string]corev1.PullPolicy{
		"example":                         corev1.PullAlways,
		"registry:5000/example":           corev1.PullAlways,
		"example:latest":                  corev1.PullAlways,
		"example:v1":                      corev1.PullIfNotPresent,
		"example@sha256:0123456789abcdef": corev1.PullIfNotPresent,
	}
	for image, want := range tests {
		if got := defaultImagePullPolicy(image); got != want {
			t.Errorf("defaultImagePullPolicy(%q)=%q, want %q", image, got, want)
		}
	}
}

func TestProviderApplyAdoptsLegacyWithoutChangingTemplate(t *testing.T) {
	scheme := testScheme(t)
	legacy := managedDeployment()
	legacy.Annotations = nil
	legacy.Spec.Template.Spec.Containers[0].Image = "running:v1"
	counting := &countingClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(legacy).Build()}
	provider := New(counting, scheme, func(agentruntime.TaskIdentity) string { return "runtime" })
	desired := managedDeployment()
	desired.Spec.Template.Spec.Containers[0].Image = "desired:v2"
	object := desiredObject(t, desired)
	object.Ownership.IncarnationPolicy = agentruntime.IncarnationOwnershipAllowAdoption
	if _, err := provider.Apply(context.Background(), agentruntime.ApplyRequest{Task: taskIdentity(), Desired: object}); err != nil {
		t.Fatal(err)
	}
	var got appsv1.Deployment
	if err := counting.Get(context.Background(), client.ObjectKeyFromObject(legacy), &got); err != nil {
		t.Fatal(err)
	}
	if got.Spec.Template.Spec.Containers[0].Image != "running:v1" {
		t.Fatal("legacy adoption changed PodTemplate and could restart Pods")
	}
	if counting.updates != 1 || got.Annotations[kubernetesobjects.ManagementTaskAdoptedGenerationAnnotation] != "3" {
		t.Fatalf("updates=%d annotations=%v", counting.updates, got.Annotations)
	}
}

func TestProviderApplyWritesCanonicalRootRevisionAndRepairsOwnedTemplateDrift(t *testing.T) {
	scheme := testScheme(t)
	existing := managedDeployment()
	existing.Annotations[kubernetesobjects.ManagementTaskGenerationAnnotation] = "3"
	existing.Spec.Template.Spec.Containers[0].Image = "running:v1"
	counting := &countingClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()}
	provider := New(counting, scheme, func(agentruntime.TaskIdentity) string { return "runtime" })
	desired := existing.DeepCopy()
	desired.UID, desired.ResourceVersion = "", ""
	desired.Spec.Template.Spec.Containers[0].Image = "desired:v2"
	object := desiredObject(t, desired)
	object.Role = "workload"
	revision := testExecutionRevision()
	if _, err := provider.Apply(context.Background(), agentruntime.ApplyRequest{Task: taskIdentity(), Desired: object, Current: ptrRef(objectRef(existing)), Revision: revision}); err != nil {
		t.Fatal(err)
	}
	var got appsv1.Deployment
	if err := counting.Get(context.Background(), client.ObjectKeyFromObject(existing), &got); err != nil {
		t.Fatal(err)
	}
	read, err := ReadExecutionRevision(&got)
	if err != nil || read != revision {
		t.Fatalf("revision=%#v error=%v annotations=%v", read, err, got.Annotations)
	}
	if got.Spec.Template.Spec.Containers[0].Image != "desired:v2" || counting.updates != 1 {
		t.Fatalf("owned template drift was not repaired: image=%q updates=%d", got.Spec.Template.Spec.Containers[0].Image, counting.updates)
	}
}

func TestProviderApplyRepairsSameGenerationStatefulSetReplicaDrift(t *testing.T) {
	scheme := testScheme(t)
	one, zero := int32(1), int32(0)
	existing := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "runtime", UID: types.UID("statefulset-uid"), ResourceVersion: "7", Annotations: managementAnnotations("task-uid")},
		Spec:       appsv1.StatefulSetSpec{Replicas: &zero, Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "task"}}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "task"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "example:v1"}}}}},
	}
	existing.Annotations[kubernetesobjects.ManagementTaskGenerationAnnotation] = "3"
	desired := existing.DeepCopy()
	desired.UID, desired.ResourceVersion = "", ""
	desired.Spec.Replicas = &one
	counting := &countingClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()}
	provider := New(counting, scheme, func(agentruntime.TaskIdentity) string { return "runtime" })
	if _, err := provider.Apply(context.Background(), agentruntime.ApplyRequest{Task: taskIdentity(), Desired: desiredObject(t, desired), Current: ptrRef(objectRefFor(existing, "apps/v1, Kind=StatefulSet"))}); err != nil {
		t.Fatal(err)
	}
	var got appsv1.StatefulSet
	if err := counting.Get(context.Background(), client.ObjectKeyFromObject(existing), &got); err != nil {
		t.Fatal(err)
	}
	if got.Spec.Replicas == nil || *got.Spec.Replicas != 1 || counting.updates != 1 {
		t.Fatalf("replicas=%v updates=%d, want replicas=1 and one repair update", got.Spec.Replicas, counting.updates)
	}
}

func TestProviderApplyRepairsSameGenerationCanonicalRevision(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*agentruntime.ExecutionRevision)
	}{
		{name: "runtime ID", mutate: func(value *agentruntime.ExecutionRevision) { value.RuntimeID = "kubernetes/legacy" }},
		{name: "runtime kind", mutate: func(value *agentruntime.ExecutionRevision) { value.RuntimeKind = "KubernetesLegacy" }},
		{name: "workload type", mutate: func(value *agentruntime.ExecutionRevision) { value.WorkloadType = "LegacyDeployment" }},
		{name: "workload version", mutate: func(value *agentruntime.ExecutionRevision) { value.WorkloadVersion = "v1alpha0" }},
		{name: "compiled digest", mutate: func(value *agentruntime.ExecutionRevision) {
			value.CompiledDigest = string(evidence.NewDigest([]byte("legacy-compiled")))
		}},
		{name: "executable digest", mutate: func(value *agentruntime.ExecutionRevision) {
			value.ExecutableDigest = string(evidence.NewDigest([]byte("legacy-executable")))
		}},
		{name: "binding revision", mutate: func(value *agentruntime.ExecutionRevision) {
			value.BindingRevision = string(evidence.NewDigest([]byte("legacy-binding")))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scheme := testScheme(t)
			existing := managedDeployment()
			existing.Annotations[kubernetesobjects.ManagementTaskGenerationAnnotation] = "3"
			legacyRevision := testExecutionRevision()
			test.mutate(&legacyRevision)
			if err := WriteExecutionRevision(existing, legacyRevision); err != nil {
				t.Fatal(err)
			}
			templateBefore := existing.Spec.Template.DeepCopy()
			counting := &countingClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()}
			provider := New(counting, scheme, func(agentruntime.TaskIdentity) string { return "runtime" })
			desired := existing.DeepCopy()
			desired.UID, desired.ResourceVersion = "", ""
			object := desiredObject(t, desired)
			object.Role = "workload"
			canonical := testExecutionRevision()
			request := agentruntime.ApplyRequest{Task: taskIdentity(), Desired: object, Current: ptrRef(objectRef(existing)), Revision: canonical}
			if _, err := provider.Apply(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			var got appsv1.Deployment
			if err := counting.Get(context.Background(), client.ObjectKeyFromObject(existing), &got); err != nil {
				t.Fatal(err)
			}
			read, err := ReadExecutionRevision(&got)
			if err != nil || read != canonical {
				t.Fatalf("revision=%#v error=%v, want %#v", read, err, canonical)
			}
			if !reflect.DeepEqual(got.Spec.Template, *templateBefore) {
				t.Fatalf("revision repair changed Pod template: %#v", got.Spec.Template)
			}
			if counting.updates != 1 {
				t.Fatalf("updates=%d, want one revision repair", counting.updates)
			}
			request.Current = ptrRef(objectRef(&got))
			if _, err := provider.Apply(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			if counting.updates != 1 {
				t.Fatalf("canonical repeat caused update: updates=%d", counting.updates)
			}
		})
	}
}

func TestProviderApplyDoesNotRepairRevisionForForeignTask(t *testing.T) {
	scheme := testScheme(t)
	existing := managedDeployment()
	existing.Annotations = managementAnnotations("other-task-uid")
	existing.Annotations[kubernetesobjects.ManagementTaskGenerationAnnotation] = "3"
	legacyRevision := testExecutionRevision()
	legacyRevision.RuntimeID = "kubernetes/legacy"
	if err := WriteExecutionRevision(existing, legacyRevision); err != nil {
		t.Fatal(err)
	}
	before := existing.DeepCopy()
	counting := &countingClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()}
	provider := New(counting, scheme, func(agentruntime.TaskIdentity) string { return "runtime" })
	desired := managedDeployment()
	desired.UID, desired.ResourceVersion = "", ""
	object := desiredObject(t, desired)
	object.Role = "workload"
	_, err := provider.Apply(context.Background(), agentruntime.ApplyRequest{
		Task: taskIdentity(), Desired: object, Current: ptrRef(objectRef(existing)), Revision: testExecutionRevision(),
	})
	if !fault.Is(err, fault.Conflict) {
		t.Fatalf("error=%v, want ownership conflict", err)
	}
	var got appsv1.Deployment
	if err := counting.Get(context.Background(), client.ObjectKeyFromObject(existing), &got); err != nil {
		t.Fatal(err)
	}
	if counting.updates != 0 || !reflect.DeepEqual(got.Annotations, before.Annotations) {
		t.Fatalf("foreign workload was changed: updates=%d annotations=%v", counting.updates, got.Annotations)
	}
}

func TestProviderObserverPanicDoesNotChangeApply(t *testing.T) {
	scheme := testScheme(t)
	provider := New(fake.NewClientBuilder().WithScheme(scheme).Build(), scheme, func(agentruntime.TaskIdentity) string { return "runtime" }, WithObserver(panicObserver{}))
	desired := managedDeployment()
	desired.UID, desired.ResourceVersion = "", ""
	if _, err := provider.Apply(context.Background(), agentruntime.ApplyRequest{Task: taskIdentity(), Desired: desiredObject(t, desired)}); err != nil {
		t.Fatalf("observer panic changed Apply: %v", err)
	}
	var got appsv1.Deployment
	if err := provider.client.Get(context.Background(), client.ObjectKeyFromObject(desired), &got); err != nil {
		t.Fatalf("Apply effect missing after observer panic: %v", err)
	}
}

func TestProviderApplyReusesCompatibleSharedConfigMap(t *testing.T) {
	scheme := testScheme(t)
	existing := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "shared-bootstrap", Namespace: "runtime", UID: "config-uid"}, Data: map[string]string{"bootstrap.sh": "script"}}
	provider := testProvider(scheme, existing)
	desired := existing.DeepCopy()
	desired.UID = ""
	object := desiredObject(t, desired)
	object.Ownership = agentruntime.ResourceOwnership{Scope: agentruntime.IdentitySharedResource, OwnerKey: "kubernetes/shared-config"}
	object.OwnedFields = sharedConfigOwnedFields()
	result, err := provider.Apply(context.Background(), agentruntime.ApplyRequest{Task: taskIdentity(), Desired: object})
	if err != nil {
		t.Fatal(err)
	}
	if result.Ref.UID != string(existing.UID) {
		t.Fatalf("reused UID=%q, want %q", result.Ref.UID, existing.UID)
	}
}

func TestProviderApplyRepairsSharedConfigMapContent(t *testing.T) {
	scheme := testScheme(t)
	existing := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "shared-bootstrap", Namespace: "runtime", UID: "config-uid", Labels: map[string]string{"foreign": "kept"}, Annotations: map[string]string{"foreign": "kept"}}, Data: map[string]string{"bootstrap.sh": "old"}}
	provider := testProvider(scheme, existing)
	desired := existing.DeepCopy()
	desired.UID = ""
	desired.Data = map[string]string{"bootstrap.sh": "new"}
	desired.Annotations[sharedConfigRoleAnnotation] = "worker"
	object := desiredObject(t, desired)
	object.Ownership = agentruntime.ResourceOwnership{Scope: agentruntime.IdentitySharedResource, OwnerKey: "kubernetes/shared-config"}
	object.OwnedFields = sharedConfigOwnedFields()
	if _, err := provider.Apply(context.Background(), agentruntime.ApplyRequest{Task: taskIdentity(), Desired: object}); err != nil {
		t.Fatal(err)
	}
	var got corev1.ConfigMap
	if err := provider.client.Get(context.Background(), client.ObjectKeyFromObject(existing), &got); err != nil {
		t.Fatal(err)
	}
	if got.Data["bootstrap.sh"] != "new" {
		t.Fatalf("shared ConfigMap data=%v, want repaired script", got.Data)
	}
	if got.Labels["foreign"] != "kept" || got.Annotations["foreign"] != "kept" || got.Annotations[sharedConfigRoleAnnotation] != "worker" {
		t.Fatalf("shared ConfigMap metadata=%#v/%#v, want foreign metadata preserved and ownership normalized", got.Labels, got.Annotations)
	}
}

func TestProviderApplyRejectsConflictingSharedConfigMapIdentity(t *testing.T) {
	scheme := testScheme(t)
	existing := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "shared-bootstrap", Namespace: "runtime", UID: "config-uid", Annotations: map[string]string{sharedConfigRoleAnnotation: "primary"}}, Data: map[string]string{"bootstrap.sh": "old"}}
	provider := testProvider(scheme, existing)
	desired := existing.DeepCopy()
	desired.UID = ""
	desired.Annotations[sharedConfigRoleAnnotation] = "secondary"
	object := desiredObject(t, desired)
	object.Ownership = agentruntime.ResourceOwnership{Scope: agentruntime.IdentitySharedResource, OwnerKey: "kubernetes/shared-config"}
	object.OwnedFields = sharedConfigOwnedFields()
	if _, err := provider.Apply(context.Background(), agentruntime.ApplyRequest{Task: taskIdentity(), Desired: object}); !fault.Is(err, fault.Conflict) {
		t.Fatalf("error=%v, want conflict", err)
	}
}

func sharedConfigOwnedFields() []string {
	return []string{"data", "binaryData", "metadata.annotations[" + sharedConfigRoleAnnotation + "]"}
}

const sharedConfigRoleAnnotation = "example.rlark.io/shared-role"

func TestProviderApplyUpdatesServiceOwnedFields(t *testing.T) {
	scheme := testScheme(t)
	existing := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "task-service", Namespace: "runtime", UID: "service-uid", Annotations: managementAnnotations("task-uid"), Labels: map[string]string{"foreign": "kept"}}, Spec: corev1.ServiceSpec{ClusterIP: "None", ClusterIPs: []string{"None"}, Selector: map[string]string{"app": "old"}, Ports: []corev1.ServicePort{{Name: "api", Port: 1, NodePort: 30001}}}}
	counting := &countingClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()}
	provider := New(counting, scheme, func(agentruntime.TaskIdentity) string { return "runtime" })
	desired := existing.DeepCopy()
	desired.UID, desired.ResourceVersion = "", ""
	desired.Labels = map[string]string{"app": "task"}
	desired.Spec.ClusterIPs = nil
	desired.Spec.Selector = map[string]string{"app": "task"}
	desired.Spec.Ports = []corev1.ServicePort{{Name: "api", Port: 8080}}
	if _, err := provider.Apply(context.Background(), agentruntime.ApplyRequest{Task: taskIdentity(), Desired: desiredObject(t, desired), Current: ptrRef(objectRefFor(existing, "v1, Kind=Service"))}); err != nil {
		t.Fatal(err)
	}
	if counting.updates != 1 {
		t.Fatalf("updates=%d, want 1", counting.updates)
	}
	var got corev1.Service
	if err := counting.Get(context.Background(), client.ObjectKeyFromObject(existing), &got); err != nil {
		t.Fatal(err)
	}
	if got.Spec.ClusterIP != "None" || !reflect.DeepEqual(got.Spec.ClusterIPs, []string{"None"}) || got.Spec.Ports[0].NodePort != 30001 || got.Labels["foreign"] != "kept" || got.Spec.Selector["app"] != "task" {
		t.Fatalf("updated Service=%#v", got)
	}
}

func TestProviderApplyServiceDefaultsAreSteadyState(t *testing.T) {
	scheme := testScheme(t)
	existing := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "task-service", Namespace: "runtime", UID: "service-uid", Annotations: managementAnnotations("task-uid"), Labels: map[string]string{"app": "task", "foreign": "kept"}}, Spec: corev1.ServiceSpec{ClusterIP: "10.0.0.1", ClusterIPs: []string{"10.0.0.1"}, IPFamilies: []corev1.IPFamily{corev1.IPv4Protocol}, IPFamilyPolicy: ptr(corev1.IPFamilyPolicySingleStack), InternalTrafficPolicy: ptr(corev1.ServiceInternalTrafficPolicyCluster), SessionAffinity: corev1.ServiceAffinityNone, Type: corev1.ServiceTypeClusterIP, Selector: map[string]string{"app": "task"}, Ports: []corev1.ServicePort{{Name: "api", Protocol: corev1.ProtocolTCP, Port: 8080, TargetPort: intstr.FromInt32(8080), NodePort: 30001}}}}
	counting := &countingClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()}
	provider := New(counting, scheme, func(agentruntime.TaskIdentity) string { return "runtime" })
	desired := existing.DeepCopy()
	desired.UID, desired.ResourceVersion = "", ""
	desired.Labels = map[string]string{"app": "task"}
	desired.Spec.ClusterIP, desired.Spec.ClusterIPs, desired.Spec.IPFamilies, desired.Spec.IPFamilyPolicy = "", nil, nil, nil
	desired.Spec.InternalTrafficPolicy, desired.Spec.SessionAffinity, desired.Spec.Type = nil, "", ""
	desired.Spec.Ports = []corev1.ServicePort{{Name: "api", Port: 8080}}
	if _, err := provider.Apply(context.Background(), agentruntime.ApplyRequest{Task: taskIdentity(), Desired: desiredObject(t, desired), Current: ptrRef(objectRefFor(existing, "v1, Kind=Service"))}); err != nil {
		t.Fatal(err)
	}
	if counting.updates != 0 {
		t.Fatalf("updates=%d, API Server defaults must be steady state", counting.updates)
	}
}

func TestProviderApplyRejectsMissingOrMismatchedReconcileProfile(t *testing.T) {
	scheme := testScheme(t)
	existing := managedDeployment()
	provider := testProvider(scheme, existing)
	desired := existing.DeepCopy()
	desired.UID, desired.ResourceVersion = "", ""
	tests := []struct {
		name    string
		profile agentruntime.ObjectKind
	}{
		{name: "missing"},
		{name: "mismatched", profile: kubernetesobjects.ServiceReconcileProfile},
		{name: "unknown", profile: agentruntime.ObjectKind{Group: "example.io", Version: "v1", Kind: "Unknown"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			object := desiredObject(t, desired)
			object.ReconcileProfile = test.profile
			_, err := provider.Apply(context.Background(), agentruntime.ApplyRequest{Task: taskIdentity(), Desired: object, Current: ptrRef(objectRef(existing))})
			if !fault.Is(err, fault.InvalidArgument) {
				t.Fatalf("error=%v, want invalid profile", err)
			}
		})
	}
}

func TestProviderApplyAdoptsServiceControlledByProvenRoot(t *testing.T) {
	scheme := testScheme(t)
	root := managedDeployment()
	existing := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "task-service", Namespace: "runtime", UID: "service-uid", OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: root.Name, UID: root.UID, Controller: ptr(true)}}}, Spec: corev1.ServiceSpec{ClusterIP: "None", Selector: map[string]string{"app": "task"}}}
	counting := &countingClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(root, existing).Build()}
	provider := New(counting, scheme, func(agentruntime.TaskIdentity) string { return "runtime" })
	desired := existing.DeepCopy()
	desired.UID, desired.ResourceVersion = "", ""
	desired.Annotations = managementAnnotations("task-uid")
	object := desiredObject(t, desired)
	object.Ownership.IncarnationPolicy = agentruntime.IncarnationOwnershipAllowAdoption
	request := agentruntime.ApplyRequest{Task: taskIdentity(), Desired: object, Current: ptrRef(objectRefFor(existing, "v1, Kind=Service")), ProvenOwners: []agentruntime.ObjectRef{objectRef(root)}}
	if _, err := provider.Apply(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	var got corev1.Service
	if err := counting.Get(context.Background(), client.ObjectKeyFromObject(existing), &got); err != nil {
		t.Fatal(err)
	}
	if !managedByTaskIncarnation(&got, taskIdentity()) || counting.updates != 1 {
		t.Fatalf("annotations=%v updates=%d, want one canonicalizing update", got.Annotations, counting.updates)
	}
	request.Current = ptrRef(objectRefFor(&got, "v1, Kind=Service"))
	if _, err := provider.Apply(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if counting.updates != 1 {
		t.Fatalf("canonical repeat caused update: %d", counting.updates)
	}
}

func TestProviderApplyRejectsServiceWithoutProvenController(t *testing.T) {
	scheme := testScheme(t)
	existing := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "task-service", Namespace: "runtime", UID: "service-uid"}}
	provider := testProvider(scheme, existing)
	desired := existing.DeepCopy()
	desired.UID = ""
	desired.Annotations = managementAnnotations("task-uid")
	object := desiredObject(t, desired)
	object.Ownership.IncarnationPolicy = agentruntime.IncarnationOwnershipAllowAdoption
	_, err := provider.Apply(context.Background(), agentruntime.ApplyRequest{Task: taskIdentity(), Desired: object, Current: ptrRef(objectRefFor(existing, "v1, Kind=Service"))})
	if !fault.Is(err, fault.Conflict) {
		t.Fatalf("error=%v, want ownership conflict", err)
	}
}

func TestProviderDeleteIsIdempotentAndUIDFenced(t *testing.T) {
	scheme := testScheme(t)
	deployment := managedDeployment()
	counting := &countingClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(deployment).Build()}
	provider := New(counting, scheme, func(agentruntime.TaskIdentity) string { return "runtime" })
	wrong := objectRef(deployment)
	wrong.UID = "replacement"
	if err := provider.Delete(context.Background(), agentruntime.DeleteRequest{Task: taskIdentity(), Ref: wrong}); !fault.Is(err, fault.Conflict) {
		t.Fatalf("UID mismatch error=%v", err)
	}
	if counting.deletes != 0 {
		t.Fatal("UID mismatch deleted replacement object")
	}
	request := agentruntime.DeleteRequest{IdempotencyKey: "delete", Task: taskIdentity(), Ref: objectRef(deployment)}
	if err := provider.Delete(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if err := provider.Delete(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if counting.deletes != 1 {
		t.Fatalf("deletes=%d, want 1", counting.deletes)
	}
}

type countingClient struct {
	client.Client
	creates, updates, deletes int
}

type recordingClient struct {
	client.Client
	gets, lists []string
}

func (c *recordingClient) Get(ctx context.Context, key client.ObjectKey, object client.Object, options ...client.GetOption) error {
	c.gets = append(c.gets, fmt.Sprintf("%T %s/%s", object, key.Namespace, key.Name))
	return c.Client.Get(ctx, key, object, options...)
}

func (c *recordingClient) List(ctx context.Context, list client.ObjectList, options ...client.ListOption) error {
	listOptions := &client.ListOptions{}
	listOptions.ApplyOptions(options)
	labels, fields := "", ""
	if listOptions.LabelSelector != nil {
		labels = listOptions.LabelSelector.String()
	}
	if listOptions.FieldSelector != nil {
		fields = listOptions.FieldSelector.String()
	}
	c.lists = append(c.lists, fmt.Sprintf("%T namespace=%s labels=%s fields=%s", list, listOptions.Namespace, labels, fields))
	return c.Client.List(ctx, list, options...)
}

func (c *countingClient) Create(ctx context.Context, object client.Object, options ...client.CreateOption) error {
	c.creates++
	return c.Client.Create(ctx, object, options...)
}
func (c *countingClient) Update(ctx context.Context, object client.Object, options ...client.UpdateOption) error {
	c.updates++
	return c.Client.Update(ctx, object, options...)
}
func (c *countingClient) Delete(ctx context.Context, object client.Object, options ...client.DeleteOption) error {
	c.deletes++
	return c.Client.Delete(ctx, object, options...)
}

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func testProvider(scheme *runtime.Scheme, objects ...client.Object) *Provider {
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).
		WithIndex(&corev1.Event{}, "involvedObject.uid", func(object client.Object) []string {
			return []string{string(object.(*corev1.Event).InvolvedObject.UID)}
		}).Build()
	return New(client, scheme, func(agentruntime.TaskIdentity) string { return "runtime" })
}

func managedDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "runtime", UID: types.UID("deployment-uid"), ResourceVersion: "7", Generation: 3, Annotations: managementAnnotations("task-uid")}, Spec: appsv1.DeploymentSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "task"}}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "task"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "example:v1"}}}}}}
}

func managementAnnotations(uid string) map[string]string {
	return map[string]string{kubernetesobjects.ManagementTaskNameAnnotation: "task", kubernetesobjects.ManagementTaskNamespaceAnnotation: "management", kubernetesobjects.ManagementTaskUIDAnnotation: uid}
}
func logicalTask() agentruntime.LogicalTaskID {
	return agentruntime.LogicalTaskID{Namespace: "management", Name: "task"}
}
func taskIdentity() agentruntime.TaskIdentity {
	return agentruntime.TaskIdentity{Logical: logicalTask(), ObjectUID: "task-uid", Generation: 3}
}
func objectRef(object client.Object) agentruntime.ObjectRef {
	return agentruntime.ObjectRef{RuntimeID: DefaultID, ID: "apps/v1, Kind=Deployment", Namespace: object.GetNamespace(), Name: object.GetName(), UID: string(object.GetUID()), LogicalTask: logicalTask()}
}

func objectRefFor(object client.Object, id string) agentruntime.ObjectRef {
	ref := objectRef(object)
	ref.ID = id
	return ref
}
func ptrRef(ref agentruntime.ObjectRef) *agentruntime.ObjectRef { return &ref }
func ptr[T any](value T) *T                                     { return &value }
func desiredObject(t *testing.T, object client.Object) agentruntime.DesiredObject {
	t.Helper()
	data, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	gvk := object.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		gvks, _, err := testScheme(t).ObjectKinds(object)
		if err != nil {
			t.Fatal(err)
		}
		gvk = gvks[0]
	}
	kind := agentruntime.ObjectKind{Group: gvk.Group, Version: gvk.Version, Kind: gvk.Kind}
	profile := kubernetesobjects.WorkloadReconcileProfile
	switch object.(type) {
	case *corev1.Service:
		profile = kubernetesobjects.ServiceReconcileProfile
	case *corev1.ConfigMap:
		profile = kubernetesobjects.SharedReconcileProfile
	}
	return agentruntime.DesiredObject{LogicalName: "object", Kind: kind, Manifest: agentruntime.TypedManifest{Type: kind, Object: data}, Ownership: agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid"}, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup, UpdateMode: agentruntime.UpdateApplyInPlace, ReconcileProfile: profile}
}

func testExecutionRevision() agentruntime.ExecutionRevision {
	return agentruntime.ExecutionRevision{
		RuntimeID: DefaultID, RuntimeKind: Kind, WorkloadType: "Deployment", WorkloadVersion: "v1alpha1",
		CompiledDigest: string(evidence.NewDigest([]byte("compiled"))), ExecutableDigest: string(evidence.NewDigest([]byte("executable"))), BindingRevision: string(evidence.NewDigest([]byte("binding"))),
	}
}

type panicObserver struct{}

func (panicObserver) Start(context.Context, observability.Reconcile) (context.Context, observability.Scope) {
	panic("diagnostic failure")
}
