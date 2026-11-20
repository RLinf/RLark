package release

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/rlinf/rlark/apps/rlark/pkg/addons"
)

const (
	oldRLark = "registry.rlark.internal:5443/rlark/agent@sha256:1111111111110000000000000000000000000000000000000000000000000001"
	newRLark = "registry.rlark.internal:5443/rlark/agent@sha256:2222222222220000000000000000000000000000000000000000000000000002"
	oldUI    = "registry.rlark.internal:5443/rlark/rlark-ui@sha256:33333333333300000000000000000000000000000000000000000000000000a1"
	newUI    = "registry.rlark.internal:5443/rlark/rlark-ui@sha256:44444444444400000000000000000000000000000000000000000000000000a2"
	station  = "registry.rlark.internal:5443/rlark/station-agent@sha256:55555555555500000000000000000000000000000000000000000000000000b1"
	wg       = "registry.rlark.internal:5443/rlark/station-wireguard@sha256:66666666666600000000000000000000000000000000000000000000000000c1"
	oldRobot = "registry.rlark.internal:5443/rlark/s1u-robot@sha256:77777777777700000000000000000000000000000000000000000000000000d1"
	newRobot = "registry.rlark.internal:5443/rlark/s1u-robot@sha256:88888888888800000000000000000000000000000000000000000000000000d2"
	device   = "registry.rlark.internal:5443/fleet/robot@sha256:99999999999900000000000000000000000000000000000000000000000000e1"
	cache    = "registry.rlark.internal:5443/rlark/registry@sha256:aaaaaaaaaaaa00000000000000000000000000000000000000000000000000f1"
)

// fakeSite is a site of two clusters (office holds Station API) that does
// what the real one does when it is written to: a Deployment or DaemonSet
// written gets Pods of its new template, an Addon written is observed and
// published, a compose service set runs its image. Every write is logged.
type fakeSite struct {
	t          *testing.T
	management *dynamicfake.FakeDynamicClient
	members    map[string]*fake.Clientset
	// memberDefinitions are the same clusters as the definitions the release writes reach them.
	memberDefinitions map[string]*dynamicfake.FakeDynamicClient
	// mcore is the management plane's ConfigMaps, where Addon Packages are published.
	mcore *fake.Clientset
	plane map[string]string
	// compose is the control plane's compose file and values as its machine has them; configs the hash of the
	// definition each running container was made from.
	compose ComposeFile
	configs map[string]string
	writes  []string
	uid     int
	// stuck is a Deployment ("<cluster>/<name>") whose new Pods never get ready.
	stuck string
	// unrolled is a DaemonSet ("<cluster>/<name>") whose Pods the cluster agent does not come to replace.
	unrolled string
	// resets is how many Addon writes the management plane fails as one that is busy fails them.
	resets int
	// alsoRecreate is a Pod owner that a write of the cluster agent also
	// replaces, as an agent that renders Addons from its built-in templates does.
	alsoRecreate string
	// cronJob is a CronJob whose Job makes a Pod while the cluster agent is written (an hourly backup).
	cronJob string
}

var listKinds = map[schema.GroupVersionResource]string{
	clusterGVR: "ClusterList", nodeGVR: "NodeList", addonGVR: "AddonList", siteNetworkGVR: "SiteNetworkList",
	enrollmentGVR: "NetworkEnrollmentList", leaseGVR: "LeaseList",
	{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}: "CustomResourceDefinitionList",
}

func object(apiVersion, kind, namespace, name string, fields map[string]any) *unstructured.Unstructured {
	content := map[string]any{"apiVersion": apiVersion, "kind": kind, "metadata": map[string]any{"name": name, "namespace": namespace}}
	maps.Copy(content, fields)
	return &unstructured.Unstructured{Object: content}
}

func newFakeSite(t *testing.T) *fakeSite {
	s := &fakeSite{t: t, members: map[string]*fake.Clientset{}, memberDefinitions: map[string]*dynamicfake.FakeDynamicClient{}, mcore: fake.NewClientset(),
		plane:   map[string]string{"rlark-server": oldRLark, "rlark-controller-manager": oldRLark, "rlark-gateway": oldRLark, "rlark-ui": oldUI},
		compose: ComposeFile{File: "services: {}\n", Variables: map[string]string{"R": "/var/lib/rlark"}}, configs: map[string]string{}}
	for service, image := range s.plane {
		s.configs[service] = fakeHash(s.compose, service, image)
	}
	var objects []runtime.Object
	objects = append(objects, object("station.rlark.io/v1alpha1", "SiteNetwork", "station-system", "default", map[string]any{"status": map[string]any{"members": int64(2)}}))
	for _, cluster := range []string{"lab", "office"} {
		objects = append(objects, object("rlinf.io/v1alpha1", "Cluster", "", cluster, map[string]any{
			"spec":   map[string]any{"agentNamespace": cluster, "nodeAgents": map[string]any{"enabled": true, "image": oldRLark, "nodeSelector": map[string]any{"rlark.io/enabled": "true"}}},
			"status": map[string]any{"phase": "Online"},
		}))
		objects = append(objects, object("rlinf.io/v1alpha1", "Node", cluster, cluster+"-1", map[string]any{"status": map[string]any{"phase": "Online"}}))
	}
	objects = append(objects,
		s.addon("lab", "s1u-robot", "v1.1.0", map[string]string{"image": oldRobot, "deviceImage": device, "reboot": "idle"}),
		s.addon("lab", "site-registry", "v0.2.0", map[string]string{"image": cache, "role": "cache", "prewarm": oldRobot}))
	s.management = dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds, objects...)
	s.reactors()

	for _, cluster := range []string{"lab", "office"} {
		member := fake.NewClientset()
		s.members[cluster] = member
		definitions := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
		// A server-side apply makes the object, or makes it what it states.
		definitions.PrependReactor("patch", "*", func(action k8stesting.Action) (bool, runtime.Object, error) {
			patch := action.(k8stesting.PatchAction)
			s.writes = append(s.writes, fmt.Sprintf("%s definition %s %s", cluster, action.GetResource().Resource, patch.GetName()))
			applied := &unstructured.Unstructured{}
			if err := json.Unmarshal(patch.GetPatch(), &applied.Object); err != nil {
				return true, nil, err
			}
			tracker := definitions.Tracker()
			if _, err := tracker.Get(action.GetResource(), action.GetNamespace(), patch.GetName()); apierrors.IsNotFound(err) {
				return true, applied, tracker.Create(action.GetResource(), applied, action.GetNamespace())
			}
			return true, applied, tracker.Update(action.GetResource(), applied, action.GetNamespace())
		})
		s.memberDefinitions[cluster] = definitions
		node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: cluster + "-1"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}}
		s.create(member, node)
		replicas := int32(1)
		// As the live site's were installed by hand: named and its container named each its own way.
		agentName, containerName, hostNetwork := map[string]string{"lab": "rlark-agent", "office": "rlark-cluster-agent"}[cluster], map[string]string{"lab": "rlark-agent", "office": "agent"}[cluster], cluster == "office"
		agent := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: agentNamespace, Name: agentName}, Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{HostNetwork: hostNetwork, Containers: []corev1.Container{{Name: containerName, Image: oldRLark, Args: map[string][]string{"lab": {"--mode", "cluster", "--image=" + oldRLark}, "office": {"--mode=cluster", "--image", oldRLark}}[cluster]}}}},
		}}
		s.create(member, agent)
		s.rollDeployment(cluster, agent)
		nodeAgents := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Namespace: agentNamespace, Name: nodeAgentDaemon}, Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": nodeAgentDaemon}},
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "agent", Image: oldRLark}}}},
		}}
		s.create(member, nodeAgents)
		s.rollDaemonSet(cluster, nodeAgents)
		for _, name := range StationDaemonSets {
			containers := []corev1.Container{{Name: "station-agent", Image: station}}
			if name == "station-agent-network" {
				containers = append(containers, corev1.Container{Name: "wireguard", Image: wg})
			}
			daemonSet := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Namespace: stationNamespace, Name: name}, Spec: appsv1.DaemonSetSpec{
				Selector:       &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
				UpdateStrategy: appsv1.DaemonSetUpdateStrategy{Type: appsv1.RollingUpdateDaemonSetStrategyType},
				Template:       corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: containers}},
			}}
			s.create(member, daemonSet)
			s.rollDaemonSet(cluster, daemonSet)
		}
		if cluster == "office" {
			for _, name := range []string{stationAPI, stationController} {
				deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: stationNamespace, Name: name}, Spec: appsv1.DeploymentSpec{
					Replicas: &replicas, Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: name, Image: oldRLark}}}},
				}}
				s.create(member, deployment)
				s.rollDeployment(cluster, deployment)
			}
		} else {
			s.pod(cluster, "rlark-system", "s1u-robot", "DaemonSet", "", corev1.Container{Name: "robot", Image: oldRobot})
		}
		member.PrependReactor("update", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
			deployment := action.(k8stesting.UpdateAction).GetObject().(*appsv1.Deployment)
			s.writes = append(s.writes, fmt.Sprintf("%s deployment %s %s", cluster, deployment.Name, deployment.Spec.Template.Spec.Containers[0].Image))
			_ = member.Tracker().Update(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, deployment, deployment.Namespace)
			s.rollDeployment(cluster, deployment)
			if runsInClusterMode(deployment.Spec.Template.Spec.Containers[0].Args) && s.alsoRecreate != "" {
				s.pod(cluster, "rlark-system", s.alsoRecreate, "DaemonSet", "", corev1.Container{Name: "robot", Image: oldRobot})
			}
			if s.cronJob != "" {
				s.pod(cluster, "rlark-system", s.cronJob+"-29913725", "Job", "", corev1.Container{Name: "backup", Image: oldRobot})
			}
			return true, deployment, nil
		})
		member.PrependReactor("update", "daemonsets", func(action k8stesting.Action) (bool, runtime.Object, error) {
			update := action.(k8stesting.UpdateActionImpl)
			daemonSets := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}
			daemonSet := update.GetObject().(*appsv1.DaemonSet).DeepCopy()
			// What an API server makes of the one it is given: it fills in what is not stated, and a template that
			// is not the one it holds is the next generation.
			daemonSet.Spec.Template.Spec.SchedulerName = corev1.DefaultSchedulerName
			for i := range daemonSet.Spec.Template.Spec.Containers {
				daemonSet.Spec.Template.Spec.Containers[i].TerminationMessagePath = corev1.TerminationMessagePathDefault
			}
			object, _ := member.Tracker().Get(daemonSets, daemonSet.Namespace, daemonSet.Name)
			held := object.(*appsv1.DaemonSet)
			daemonSet.Generation = held.Generation
			if !equality.Semantic.DeepEqual(held.Spec.Template, daemonSet.Spec.Template) {
				daemonSet.Generation++
				metav1.SetMetaDataAnnotation(&daemonSet.ObjectMeta, appsv1.DeprecatedTemplateGeneration, fmt.Sprint(daemonSet.Generation))
			}
			// A dry run answers what the object would be and writes nothing.
			if len(update.UpdateOptions.DryRun) > 0 {
				return true, daemonSet, nil
			}
			daemonSet.Status.ObservedGeneration = daemonSet.Generation
			s.writes = append(s.writes, fmt.Sprintf("%s daemonset %s %s", cluster, daemonSet.Name, daemonSet.Spec.UpdateStrategy.Type))
			_ = member.Tracker().Update(daemonSets, daemonSet, daemonSet.Namespace)
			if cluster+"/"+daemonSet.Name != s.unrolled {
				s.rollDaemonSet(cluster, daemonSet)
			}
			return true, daemonSet, nil
		})
	}
	return s
}

func (s *fakeSite) create(member *fake.Clientset, object runtime.Object) {
	if err := member.Tracker().Add(object); err != nil {
		s.t.Fatal(err)
	}
}

func (s *fakeSite) addon(cluster, name, version string, values map[string]string) *unstructured.Unstructured {
	anyValues := map[string]any{}
	for key, value := range values {
		anyValues[key] = value
	}
	addon := object("rlinf.io/v1alpha1", "Addon", cluster, name, map[string]any{
		"spec":   map[string]any{"addonName": name, "version": version, "values": anyValues},
		"status": map[string]any{"phase": "Ready", "observedGeneration": int64(1), "image": values["image"], "version": version},
	})
	addon.SetGeneration(1)
	return addon
}

// pod replaces the Pods of an owner with one of the given containers.
func (s *fakeSite) pod(cluster, namespace, owner, kind, ownerObject string, containers ...corev1.Container) *corev1.Pod {
	member := s.members[cluster]
	// The tracker, not the client: this runs inside the client's reactors.
	listed, _ := member.Tracker().List(schema.GroupVersionResource{Version: "v1", Resource: "pods"}, schema.GroupVersionKind{Version: "v1", Kind: "Pod"}, namespace)
	for _, pod := range listed.(*corev1.PodList).Items {
		if ownerOf(pod) == owner {
			_ = member.Tracker().Delete(schema.GroupVersionResource{Version: "v1", Resource: "pods"}, namespace, pod.Name)
		}
	}
	s.uid++
	ready := corev1.ConditionTrue
	if cluster+"/"+owner == s.stuck {
		ready = corev1.ConditionFalse
	}
	ownerName := cmpOr(ownerObject, owner)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: fmt.Sprintf("%s-%d", owner, s.uid), UID: types.UID(fmt.Sprint(s.uid)),
			Labels: map[string]string{"app": owner}, OwnerReferences: []metav1.OwnerReference{{Kind: kind, Name: ownerName}}},
		Spec:   corev1.PodSpec{NodeName: cluster + "-1", Containers: containers},
		Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: ready}}},
	}
	s.create(member, pod)
	return pod
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (s *fakeSite) rollDeployment(cluster string, deployment *appsv1.Deployment) {
	s.pod(cluster, deployment.Namespace, deployment.Name, "ReplicaSet", deployment.Name+"-5d8f", deployment.Spec.Template.Spec.Containers...)
	ready := int32(1)
	if cluster+"/"+deployment.Name == s.stuck {
		ready = 0
	}
	deployment.Status = appsv1.DeploymentStatus{ObservedGeneration: deployment.Generation, Replicas: 1, UpdatedReplicas: 1, ReadyReplicas: ready, AvailableReplicas: ready}
	_ = s.members[cluster].Tracker().Update(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, deployment, deployment.Namespace)
}

func (s *fakeSite) rollDaemonSet(cluster string, daemonSet *appsv1.DaemonSet) {
	pod := s.pod(cluster, daemonSet.Namespace, daemonSet.Name, "DaemonSet", "", daemonSet.Spec.Template.Spec.Containers...)
	pod.Labels[templateGenerationLabel] = daemonSet.Annotations[appsv1.DeprecatedTemplateGeneration]
	_ = s.members[cluster].Tracker().Update(schema.GroupVersionResource{Version: "v1", Resource: "pods"}, pod, pod.Namespace)
}

func (s *fakeSite) Images(context.Context) (map[string]string, error) {
	return maps.Clone(s.plane), nil
}

// fakeHash is a service's definition by compose and its image (every value counts for every service).
func fakeHash(compose ComposeFile, service, image string) string {
	return fileDigest(fmt.Sprint(compose.File, compose.Variables, service, image))[7:23]
}

func (s *fakeSite) Configs(context.Context) (map[string]string, error) {
	return maps.Clone(s.configs), nil
}

func (s *fakeSite) Compose(context.Context) (*ComposeFile, error) {
	return &ComposeFile{File: s.compose.File, Variables: maps.Clone(s.compose.Variables)}, nil
}

func (s *fakeSite) Hashes(_ context.Context, compose ComposeFile, images map[string]string) (map[string]string, error) {
	hashes := map[string]string{}
	for service, image := range images {
		hashes[service] = fakeHash(compose, service, image)
	}
	return hashes, nil
}

func (s *fakeSite) Write(_ context.Context, compose ComposeFile) error {
	s.writes = append(s.writes, "compose files")
	s.compose = ComposeFile{File: compose.File, Variables: maps.Clone(compose.Variables)}
	return nil
}

func (s *fakeSite) Pull(_ context.Context, image string) error {
	if strings.Contains(image, "missing") {
		return errors.New("manifest unknown")
	}
	return nil
}

func (s *fakeSite) Set(_ context.Context, service, image string) error {
	s.writes = append(s.writes, "compose "+service+" "+image)
	s.plane[service] = image
	s.configs[service] = fakeHash(s.compose, service, image)
	return nil
}

// reactors are the management plane's controllers as far as a release meets them: an Addon written is observed
// (its generation, Ready) and the robots' one replaces its Pods when its image changed; a Cluster's node agents
// written are rolled; every write is logged.
func (s *fakeSite) reactors() {
	tracker := s.management.Tracker()
	s.management.PrependReactor("*", "addons", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.GetVerb() != "update" && action.GetVerb() != "create" {
			return false, nil, nil
		}
		if s.resets > 0 {
			s.resets--
			return true, nil, apierrors.NewServiceUnavailable("the management plane is starting")
		}
		addon := action.(k8stesting.CreateAction).GetObject().(*unstructured.Unstructured).DeepCopy()
		version, _, _ := unstructured.NestedString(addon.Object, "spec", "version")
		image, _, _ := unstructured.NestedString(addon.Object, "spec", "values", "image")
		s.writes = append(s.writes, fmt.Sprintf("addon %s/%s %s", addon.GetNamespace(), addon.GetName(), version))
		addon.SetGeneration(addon.GetGeneration() + 1)
		_ = unstructured.SetNestedField(addon.Object, addon.GetGeneration(), "status", "observedGeneration")
		_ = unstructured.SetNestedField(addon.Object, "Ready", "status", "phase")
		_ = unstructured.SetNestedField(addon.Object, version, "status", "version")
		if was, _, _ := unstructured.NestedString(addon.Object, "status", "image"); addon.GetName() == "s1u-robot" && was != image {
			_ = unstructured.SetNestedField(addon.Object, image, "status", "image")
			s.pod(addon.GetNamespace(), "rlark-system", "s1u-robot", "DaemonSet", "", corev1.Container{Name: "robot", Image: image})
		}
		if action.GetVerb() == "create" {
			return true, addon, tracker.Create(addonGVR, addon, addon.GetNamespace())
		}
		return true, addon, tracker.Update(addonGVR, addon, addon.GetNamespace())
	})
	s.management.PrependReactor("update", "clusters", func(action k8stesting.Action) (bool, runtime.Object, error) {
		cluster := action.(k8stesting.UpdateAction).GetObject().(*unstructured.Unstructured)
		s.writes = append(s.writes, "node-agents "+cluster.GetName())
		if selector, _, _ := unstructured.NestedStringMap(cluster.Object, "spec", "nodeAgents", "nodeSelector"); len(selector) == 0 {
			s.t.Errorf("node-agents of %s written without the selector it had", cluster.GetName())
		}
		image, _, _ := unstructured.NestedString(cluster.Object, "spec", "nodeAgents", "image")
		member := s.members[cluster.GetName()]
		object, err := member.Tracker().Get(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}, agentNamespace, nodeAgentDaemon)
		if err != nil {
			return true, nil, err
		}
		daemonSet := object.(*appsv1.DaemonSet)
		daemonSet.Spec.Template.Spec.Containers[0].Image = image
		_ = member.Tracker().Update(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}, daemonSet, agentNamespace)
		s.rollDaemonSet(cluster.GetName(), daemonSet)
		return true, cluster, tracker.Update(clusterGVR, cluster, "")
	})
}

func (s *fakeSite) site(environment Environment) *Site {
	members := map[string]kubernetes.Interface{}
	for name, member := range s.members {
		members[name] = member
	}
	memberDefinitions := map[string]dynamic.Interface{}
	for name, definitions := range s.memberDefinitions {
		memberDefinitions[name] = definitions
	}
	return &Site{Environment: environment, Management: s.management, Core: s.mcore, Members: members, MemberDefinitions: memberDefinitions, Plane: s}
}

const releaseYAML = `name: candidate
source: {rlark: 1111111111111111111111111111111111111111}
images:
  rlark-server: &rlark ` + newRLark + `
  rlark-controller-manager: *rlark
  rlark-gateway: *rlark
  rlark-ui: ` + newUI + `
  station-api: *rlark
  station-controller: *rlark
  station-agent: ` + station + `
  station-wireguard: ` + wg + `
  cluster-agent: *rlark
  node-agent: *rlark
  tools: ` + oldRLark + `
  s1u-robot: ` + newRobot + `
  fleet-robot: ` + device + `
  site-registry: ` + cache + `
addons:
  s1u-robot:
    version: v1.3.9
    values: {image: "{{images.s1u-robot}}", deviceImage: "{{images.fleet-robot}}"}
    after: [site-registry]
  site-registry:
    version: v0.2.9
    values: {image: "{{images.site-registry}}", prewarm: "{{images.s1u-robot}},{{images.fleet-robot}}"}
`

const environmentYAML = `name: live
release: ../releases/candidate.yaml
management: {kubeconfig: /dev/null}
addonCatalog: /dev/null
controlPlane: {project: rlark, compose: /dev/null, env: /dev/null}
station: {cluster: office, pullSecret: ir-0123456789abcdef}
registry: {name: registry.rlark.internal:5443, source: {address: "http://registry.example:5000"}}
state: STATE
`

// testClusters are the fake site's clusters and the Addons installed in each, as the management plane has them
// (Site.Installed).
var testClusters = map[string][]string{"lab": {"s1u-robot", "site-registry"}, "office": nil}

func writeFiles(t *testing.T) (string, Environment, Release) {
	dir := t.TempDir()
	// The versions of the test's Addons are the catalog directory's (as the Gateway's runtime one), none built in.
	for _, item := range []string{"s1u-robot v1.1.0", "s1u-robot v1.3.9", "site-registry v0.2.0", "site-registry v0.2.9"} {
		name, version, _ := strings.Cut(item, " ")
		path := filepath.Join(dir, "catalog", name+"-"+version, "addon.yaml")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("name: "+name+"\nversion: "+version+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	addons.Catalog.SetDirectory(filepath.Join(dir, "catalog"))
	for name, content := range map[string]string{
		"releases/candidate.yaml": releaseYAML,
		"environments/live.yaml":  strings.Replace(environmentYAML, "STATE", filepath.Join(dir, "state"), 1),
	} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	environment, release, err := LoadEnvironment(filepath.Join(dir, "environments/live.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return dir, environment, release
}

func newRun(t *testing.T, s *fakeSite, environment Environment, desired State, out *bytes.Buffer) *Run {
	stationAgents, err := os.ReadFile("../../../../astribot/station/deploy/station-agent.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return &Run{
		Site: s.site(environment), Desired: desired, Tree: &Tree{Commit: "1111111111111111111111111111111111111111", Files: map[string]string{"station-agent.yaml": string(stationAgents)}},
		Name: "candidate", Timeout: 50 * time.Millisecond, Poll: time.Millisecond, Out: out,
		RollbackCommand: func(before string) string { return "rlarkctl release apply -e live.yaml --to " + before + " --back" },
	}
}

func TestDesired(t *testing.T) {
	_, environment, release := writeFiles(t)
	desired := environment.Desired(release, testClusters)
	lab := desired.Clusters["lab"]
	if got := lab.Addons["s1u-robot"]; got.Version != "v1.3.9" || got.Values["image"] != newRobot || got.Values["deviceImage"] != device {
		t.Errorf("s1u-robot: %+v", got)
	}
	if got := lab.Addons["site-registry"].Values["prewarm"]; got != newRobot+","+device {
		t.Errorf("prewarm: %s", got)
	}
	if lab.Agent != newRLark || lab.Tools != oldRLark || desired.ControlPlane["rlark-ui"] != newUI {
		t.Errorf("components: %+v", desired)
	}
	// A cluster runs the release's Addons it has installed, and no other.
	made := environment.Desired(release, map[string][]string{"robots-2": {"s1u-robot", "csi"}})
	if got := made.Clusters["robots-2"]; got.Agent != newRLark || len(got.Addons) != 1 || got.Addons["s1u-robot"].Version != "v1.3.9" || got.Addons["s1u-robot"].Values["image"] != newRobot {
		t.Errorf("a cluster the environment does not state: %+v", got)
	}
}

func TestLoadReleaseRefusesAnImageWithoutItsDigest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "r.yaml")
	_ = os.WriteFile(path, []byte(strings.Replace(releaseYAML, "rlark-ui: "+newUI, "rlark-ui: registry.rlark.internal:5443/rlark/rlark-ui:latest", 1)), 0o644)
	if _, err := LoadRelease(path); err == nil || !strings.Contains(err.Error(), "rlark-ui is not named by its digest") {
		t.Errorf("%v", err)
	}
	_ = os.WriteFile(path, []byte(strings.Replace(releaseYAML, "{{images.fleet-robot}}\"}", "{{images.fleet-gpu}}\"}", 1)), 0o644)
	if _, err := LoadRelease(path); err == nil || !strings.Contains(err.Error(), "fleet-gpu") {
		t.Errorf("%v", err)
	}
}

func TestApplyWalksTheStagesInOrderAndReadsBack(t *testing.T) {
	s := newFakeSite(t)
	_, environment, release := writeFiles(t)
	desired := environment.Desired(release, testClusters)
	var out bytes.Buffer
	run := newRun(t, s, environment, desired, &out)
	if err := run.Apply(context.Background()); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	at := func(prefix string) int {
		i := slices.IndexFunc(s.writes, func(write string) bool { return strings.HasPrefix(write, prefix) })
		if i < 0 {
			t.Fatalf("no write %q in %v", prefix, s.writes)
		}
		return i
	}
	for _, pair := range [][2]string{
		{"compose rlark-gateway", "lab deployment rlark-agent"},
		{"office deployment station-api", "office deployment station-controller"},
		{"office deployment station-controller", "lab deployment rlark-agent"},
		{"lab deployment rlark-agent", "office deployment rlark-cluster-agent"}, // one cluster at a time
		{"office deployment rlark-cluster-agent", "node-agents lab"},
		{"node-agents office", "lab daemonset station-agent-host OnDelete"},
		{"office daemonset station-agent-host", "compose rlark-server"}, // the server after every agent
		{"compose rlark-controller-manager", "compose rlark-ui"},
		{"compose rlark-ui", "addon lab/site-registry v0.2.9"},
		{"addon lab/site-registry v0.2.9", "addon lab/s1u-robot v1.3.9"}, // the cache before what pulls through it
	} {
		if at(pair[0]) >= at(pair[1]) {
			t.Errorf("%s is not before %s: %v", pair[0], pair[1], s.writes)
		}
	}
	current, err := run.Site.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := run.Plan(context.Background(), current)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 0 {
		t.Errorf("the site is not the release after the run: %v", plan)
	}
	for _, name := range []string{stationAPI, stationController} {
		deployment, _ := s.members["office"].AppsV1().Deployments(stationNamespace).Get(context.Background(), name, metav1.GetOptions{})
		if secrets := deployment.Spec.Template.Spec.ImagePullSecrets; len(secrets) != 1 || secrets[0].Name != "ir-0123456789abcdef" {
			t.Errorf("%s pulls with %v, not the site registry's account", name, secrets)
		}
	}
	agent, _ := s.members["office"].AppsV1().Deployments(agentNamespace).Get(context.Background(), "rlark-cluster-agent", metav1.GetOptions{})
	if args := agent.Spec.Template.Spec.Containers[0].Args; !slices.Equal(args, []string{"--mode=cluster", "--image", oldRLark}) {
		t.Errorf("the office agent's arguments: %v", args)
	}
	if probe := agent.Spec.Template.Spec.Containers[0].ReadinessProbe; probe == nil || probe.HTTPGet.Port.IntVal != 10382 || agent.Spec.Strategy.Type != appsv1.RecreateDeploymentStrategyType {
		t.Errorf("the agent's Deployment: %+v %+v", probe, agent.Spec.Strategy)
	}
	for _, line := range []string{"the state before is recorded in", "cluster-agents: done in", "Pods made anew: lab rlark-system/rlark-agent(1)", "every stage done"} {
		if !strings.Contains(out.String(), line) {
			t.Errorf("the record lacks %q:\n%s", line, out.String())
		}
	}

	// A second run finds nothing to do and writes nothing.
	s.writes = nil
	out.Reset()
	if err := newRun(t, s, environment, desired, &out).Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(s.writes) != 0 {
		t.Errorf("a second run wrote %v", s.writes)
	}

	// Back to the record of the state before: the server goes back before the agents.
	records, _ := filepath.Glob(filepath.Join(environment.State, "*-before.json"))
	slices.Sort(records)
	data, err := os.ReadFile(records[0])
	if err != nil {
		t.Fatal(err)
	}
	var before State
	if err := json.Unmarshal(data, &before); err != nil {
		t.Fatal(err)
	}
	back := newRun(t, s, environment, before, &out)
	back.Back, back.Tree = true, nil
	s.writes = nil
	if err := back.Apply(context.Background()); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if at("compose rlark-server") >= at("office deployment rlark-cluster-agent") || at("addon lab/s1u-robot v1.1.0") >= at("compose rlark-server") {
		t.Errorf("going back is not in reverse: %v", s.writes)
	}
	if current, _ = back.Site.Read(context.Background()); current.Clusters["lab"].Agent != oldRLark || current.ControlPlane["rlark-gateway"] != oldRLark {
		t.Errorf("not back: %+v", current)
	}
}

func TestApplyWaitsForAGatewayThatIsStarting(t *testing.T) {
	s := newFakeSite(t)
	s.resets = 2
	_, environment, release := writeFiles(t)
	desired := environment.Desired(release, testClusters)
	var out bytes.Buffer
	if err := newRun(t, s, environment, desired, &out).Apply(context.Background()); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
}

// A Station container that runs the release's image by a registry's address (live 11-17: the cloud's wireguard by
// 10.254.240.1:5443, the tunnel's entry to be removed) is given the release's name, and the run settles on it.
func TestApplyGivesStationContainersTheReleasesName(t *testing.T) {
	s := newFakeSite(t)
	daemonSets := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}
	byAddress := strings.Replace(wg, "registry.rlark.internal:5443", "10.254.240.1:5443", 1)
	object, _ := s.members["lab"].Tracker().Get(daemonSets, stationNamespace, "station-agent-network")
	network := object.(*appsv1.DaemonSet)
	for i, container := range network.Spec.Template.Spec.Containers {
		if container.Name == "wireguard" {
			network.Spec.Template.Spec.Containers[i].Image = byAddress
		}
	}
	_ = s.members["lab"].Tracker().Update(daemonSets, network, stationNamespace)
	_, environment, release := writeFiles(t)
	desired := environment.Desired(release, testClusters)
	var out bytes.Buffer
	run := newRun(t, s, environment, desired, &out)
	if err := run.Apply(context.Background()); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "lab station-wireguard: "+byAddress[:len(byAddress)-52]) {
		t.Errorf("the plan does not change the name:\n%s", out.String())
	}
	object, _ = s.members["lab"].Tracker().Get(daemonSets, stationNamespace, "station-agent-network")
	for _, container := range object.(*appsv1.DaemonSet).Spec.Template.Spec.Containers {
		if container.Name == "wireguard" && container.Image != wg {
			t.Errorf("wireguard runs %s, not %s", container.Image, wg)
		}
	}
	if plan, err := run.Plan(context.Background(), mustRead(t, run.Site)); err != nil || len(plan) != 0 {
		t.Errorf("after the run: %v %v", plan, err)
	}
}

func withStation(cluster ClusterState, agent, wireguard string) ClusterState {
	cluster.StationAgent, cluster.StationWireguard = agent, wireguard
	return cluster
}

func mustRead(t *testing.T, site *Site) State {
	state, err := site.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestApplyStopsAtAStageThatDoesNotSettle(t *testing.T) {
	s := newFakeSite(t)
	s.stuck = "lab/rlark-agent"
	_, environment, release := writeFiles(t)
	desired := environment.Desired(release, testClusters)
	var out bytes.Buffer
	err := newRun(t, s, environment, desired, &out).Apply(context.Background())
	var stopped *StageError
	if !errors.As(err, &stopped) || stopped.Stage != "cluster-agents" || !strings.Contains(stopped.Reading, "0 ready") {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "cluster-agents: cluster lab: waiting (until ") {
		t.Errorf("the wait does not say what it waits for:\n%s", out.String())
	}
	if slices.ContainsFunc(s.writes, func(write string) bool {
		return strings.HasPrefix(write, "office deployment rlark-cluster-agent") || strings.HasPrefix(write, "compose rlark-server")
	}) {
		t.Errorf("went on after the stage stopped: %v", s.writes)
	}
	if !strings.Contains(out.String(), "--to "+stopped.Before+" --back") {
		t.Errorf("no way back printed:\n%s", out.String())
	}
}

func TestApplyGoesOnWhereARunStopped(t *testing.T) {
	s := newFakeSite(t)
	s.stuck = "office/station-api"
	_, environment, release := writeFiles(t)
	desired := environment.Desired(release, testClusters)
	var out bytes.Buffer
	if err := newRun(t, s, environment, desired, &out).Apply(context.Background()); err == nil {
		t.Fatal("a Station API that does not come up did not stop the run")
	}
	// What stopped it is mended; the same command again: the stage whose image is already as stated is done again.
	s.stuck, s.writes = "", nil
	out.Reset()
	if err := newRun(t, s, environment, desired, &out).Apply(context.Background()); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "station-api: as the release states but not settled") || !slices.Contains(s.writes, "office deployment station-api "+newRLark) {
		t.Errorf("the stage was not done again: %v\n%s", s.writes, out.String())
	}
}

// A Pod a Job makes meanwhile (a CronJob's, live 11-16: datatools-postgres-backup-hourly) is not one a stage made.
func TestAJobsPodIsNoneOfAStages(t *testing.T) {
	s := newFakeSite(t)
	s.cronJob = "datatools-postgres-backup-hourly"
	_, environment, release := writeFiles(t)
	desired := environment.Desired(release, testClusters)
	var out bytes.Buffer
	if err := newRun(t, s, environment, desired, &out).Apply(context.Background()); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
}

func TestApplyStopsWhenAStageReplacesPodsItDoesNotOwn(t *testing.T) {
	s := newFakeSite(t)
	s.alsoRecreate = "s1u-robot"
	_, environment, release := writeFiles(t)
	desired := environment.Desired(release, testClusters)
	var out bytes.Buffer
	err := newRun(t, s, environment, desired, &out).Apply(context.Background())
	var stopped *StageError
	if !errors.As(err, &stopped) || stopped.Stage != "cluster-agents" || !strings.Contains(stopped.Reading, "lab rlark-system/s1u-robot") {
		t.Fatalf("%v\n%s", err, out.String())
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	s := newFakeSite(t)
	_, environment, release := writeFiles(t)
	desired := environment.Desired(release, testClusters)
	var out bytes.Buffer
	run := newRun(t, s, environment, desired, &out)
	run.DryRun = true
	if err := run.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(s.writes) != 0 || len(s.management.Actions()) == 0 && false {
		t.Errorf("a dry run wrote %v", s.writes)
	}
	for _, line := range []string{
		"cluster-agents   lab agent: registry.rlark.internal:5443/rlark/agent@sha256:111111111111 -> registry.rlark.internal:5443/rlark/agent@sha256:222222222222",
		"addons           lab/s1u-robot version: v1.1.0 -> v1.3.9",
	} {
		if !strings.Contains(out.String(), line) {
			t.Errorf("the plan lacks %q:\n%s", line, out.String())
		}
	}
	if entries, _ := os.ReadDir(environment.State); len(entries) != 0 {
		t.Errorf("a dry run left records")
	}
}

func TestOneRunAtATime(t *testing.T) {
	s := newFakeSite(t)
	_, environment, release := writeFiles(t)
	desired := environment.Desired(release, testClusters)
	lease := object("coordination.k8s.io/v1", "Lease", leaseNamespace, leaseName, map[string]any{"spec": map[string]any{
		"holderIdentity": "someone else", "renewTime": time.Now().UTC().Format(time.RFC3339Nano), "leaseDurationSeconds": int64(leaseSeconds)}})
	if _, err := s.management.Resource(leaseGVR).Namespace(leaseNamespace).Create(context.Background(), lease, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := newRun(t, s, environment, desired, &out).Apply(context.Background()); err == nil || !strings.Contains(err.Error(), `held by "someone else"`) {
		t.Errorf("%v", err)
	}
	if len(s.writes) != 0 {
		t.Errorf("wrote while another held the site: %v", s.writes)
	}
}

// The releases and environments in the repository are what the command reads.
func TestShippedReleases(t *testing.T) {
	releases, _ := filepath.Glob("../../../../astribot/release/releases/*.yaml")
	environments, _ := filepath.Glob("../../../../astribot/release/environments/*.yaml")
	if len(releases) < 1 || len(environments) < 1 {
		t.Fatalf("releases %v, environments %v", releases, environments)
	}
	for _, path := range environments {
		if _, _, err := LoadEnvironment(path); err != nil {
			t.Error(err)
		}
	}
	// What the tree pins renders over every release.
	var pinned Pinned
	if err := readYAML("../../../../"+pinnedFile, &pinned); err != nil {
		t.Fatal(err)
	}
	for _, releasePath := range releases {
		release, err := LoadRelease(releasePath)
		if err != nil {
			t.Fatal(err)
		}
		maps.Copy(release.Images, pinned.Images)
		for name, addon := range pinned.Addons {
			if _, err := release.render(addon.Values); err != nil {
				t.Errorf("%s over %s: Addon %s: %v", pinnedFile, releasePath, name, err)
			}
		}
	}
}

// An Addon is written as the Gateway writes one: its Package published first, a required value missing refused at
// once (not asked again until the stage's time is up).
func TestAnAddonIsRefusedAsTheGatewayRefusesIt(t *testing.T) {
	s := newFakeSite(t)
	_, environment, _ := writeFiles(t)
	var out bytes.Buffer
	run := newRun(t, s, environment, State{}, &out)
	run.Timeout = time.Minute
	started := time.Now()
	err := run.retryWrite(context.Background(), func() error {
		return run.Site.PutAddon(context.Background(), "lab", "s1u-robot", AddonState{Version: "v1.3.9", Values: map[string]string{"image": newRobot}}, false)
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.mcore.CoreV1().ConfigMaps("lab").Get(context.Background(), addons.PackageName("s1u-robot", "v1.3.9"), metav1.GetOptions{}); err != nil {
		t.Errorf("the Package was not published: %v", err)
	}
	builtIn, _ := addons.Catalog.Get("s1u-robot")
	err = run.retryWrite(context.Background(), func() error {
		return run.Site.PutAddon(context.Background(), "lab", "s1u-robot", AddonState{Version: builtIn.Meta().Version}, false)
	})
	if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "required parameters are missing") || time.Since(started) > 10*time.Second {
		t.Errorf("%v after %s", err, time.Since(started))
	}
}

// The cluster agent is the Deployment of rlark-system that runs the agent with --mode=cluster, whatever its name,
// labels and container: live 11-16 has rlark-cluster-agent (container agent), rlark-agent (container rlark-agent).
func TestTheClusterAgentIsFoundByWhatItRuns(t *testing.T) {
	member := fake.NewClientset(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: agentNamespace, Name: "something-else"}, Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "x", Args: []string{"--mode=node"}}}}}}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: agentNamespace, Name: "rlark-cluster-agent"}, Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "sidecar"}, {Name: "agent", Args: []string{"--mode=cluster"}}}}}}},
	)
	agent, at, err := clusterAgent(context.Background(), member)
	if err != nil || agent.Name != "rlark-cluster-agent" || at != 1 {
		t.Errorf("%v %d %v", agent, at, err)
	}
	if _, _, err := clusterAgent(context.Background(), fake.NewClientset()); err == nil || !strings.Contains(err.Error(), "0 Deployments") {
		t.Errorf("no agent: %v", err)
	}
	if !runsInClusterMode([]string{"/agent", "--mode", "cluster"}) || runsInClusterMode([]string{"--mode"}) || runsInClusterMode([]string{"--mode", "node"}) {
		t.Error("the mode is not read as it is given")
	}
}

// An image several of Station's containers run (wireguard, gateway-program) is read as one only while they agree,
// by name.
func TestStationImagesThatDisagreeAreAChange(t *testing.T) {
	s := newFakeSite(t)
	member := s.members["lab"]
	object, _ := member.Tracker().Get(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}, stationNamespace, "station-agent-host")
	host := object.(*appsv1.DaemonSet)
	host.Spec.Template.Spec.Containers = append(host.Spec.Template.Spec.Containers, corev1.Container{Name: "gateway-program", Image: wg + "-older"})
	_ = member.Tracker().Update(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}, host, stationNamespace)
	_, environment, release := writeFiles(t)
	current := mustRead(t, s.site(environment))
	desired := environment.Desired(release, testClusters)
	if changes := Changes("station-agents", current, desired); !slices.ContainsFunc(changes, func(change Change) bool { return change.What == "lab station-wireguard" }) {
		t.Errorf("%v (read %q)", changes, current.Clusters["lab"].StationWireguard)
	}
	// The same digest by a registry's address (live 11-17: station-agent-host by 10.11.14.128:5443) is a change too: a
	// node pulls the name, and the address is no registry of the site's.
	host.Spec.Template.Spec.Containers[len(host.Spec.Template.Spec.Containers)-1].Image = strings.Replace(wg, "registry.rlark.internal:5443", "10.11.14.128:5443", 1)
	_ = member.Tracker().Update(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}, host, stationNamespace)
	current = mustRead(t, s.site(environment))
	if changes := Changes("station-agents", current, desired); !slices.ContainsFunc(changes, func(change Change) bool { return change.What == "lab station-wireguard" }) {
		t.Errorf("one image by two names: %v (read %q)", changes, current.Clusters["lab"].StationWireguard)
	}
}

// One image of the control plane by another name (the live control plane's machine pulls from its own registry) is no
// change; a change writes the name the machine pulls by, and is pulled first.
func TestImagesAreTheirDigests(t *testing.T) {
	s := newFakeSite(t)
	_, environment, release := writeFiles(t)
	environment.ControlPlane.Images = map[string]string{"registry.rlark.internal:5443/rlark/agent": "localhost:5555/rlark"}
	desired := environment.Desired(release, testClusters)
	_, digest, _ := strings.Cut(newRLark, "@")
	if desired.ControlPlane["rlark-server"] != "localhost:5555/rlark@"+digest || desired.ControlPlane["rlark-ui"] != newUI {
		t.Errorf("the control plane's names: %v", desired.ControlPlane)
	}
	current := mustRead(t, s.site(environment))
	current.ControlPlane["rlark-gateway"] = "10.254.240.1:5443/rlark/agent@" + digest
	if changes := Changes("gateway", current, desired); len(changes) != 0 {
		t.Errorf("the same digest by another name: %v", changes)
	}
	var out bytes.Buffer
	run := newRun(t, s, environment, desired, &out)
	run.Desired.ControlPlane["rlark-ui"] = "localhost:5555/rlark-ui@sha256:" + strings.Repeat("e", 52) + "missing00000"
	err := run.Apply(context.Background())
	var stopped *StageError
	if !errors.As(err, &stopped) || stopped.Stage != "prewarm" || !strings.Contains(stopped.Reading, "rlark-ui") || len(s.writes) != 0 {
		t.Errorf("an image the machine cannot pull: %v, wrote %v", err, s.writes)
	}
}

func TestNodesOutOfServiceHoldNoRolloutUp(t *testing.T) {
	ready := []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}
	member := fake.NewClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "serving"}, Status: corev1.NodeStatus{Conditions: ready}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "cordoned"}, Spec: corev1.NodeSpec{Unschedulable: true}, Status: corev1.NodeStatus{Conditions: ready}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "down"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionUnknown}}}},
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Namespace: agentNamespace, Name: nodeAgentDaemon}, Spec: appsv1.DaemonSetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": nodeAgentDaemon}}}},
	)
	pod := func(node, image string, ready corev1.ConditionStatus) {
		member.CoreV1().Pods(agentNamespace).Create(context.Background(), &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Namespace: agentNamespace, Name: "agent-" + node, Labels: map[string]string{"app": nodeAgentDaemon}},
			Spec:       corev1.PodSpec{NodeName: node, Containers: []corev1.Container{{Name: "agent", Image: image}}},
			Status:     corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: ready}}},
		}, metav1.CreateOptions{})
	}
	pod("serving", newRLark, corev1.ConditionTrue)
	pod("cordoned", newRLark, corev1.ConditionFalse) // ContainerCreating on a node whose network is broken
	pod("down", oldRLark, corev1.ConditionTrue)
	reading, err := daemonSetSettled(context.Background(), member, agentNamespace, nodeAgentDaemon, map[string]string{"": newRLark})
	if err != nil || reading != "" {
		t.Fatalf("%q %v", reading, err)
	}

	s := newFakeSite(t)
	s.create(s.members["lab"], &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "lab-2"}, Spec: corev1.NodeSpec{Unschedulable: true}, Status: corev1.NodeStatus{Conditions: ready}})
	_, environment, release := writeFiles(t)
	desired := environment.Desired(release, testClusters)
	var out bytes.Buffer
	run := newRun(t, s, environment, desired, &out)
	run.DryRun = true
	if err := run.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "lab/lab-2 is cordoned: left out of the read-back") {
		t.Errorf("the cordoned node is not named:\n%s", out.String())
	}
}

func TestEveryMemberClusterGetsTheTreesMemberDefinitions(t *testing.T) {
	s := newFakeSite(t)
	_, environment, release := writeFiles(t)
	desired := environment.Desired(release, testClusters)
	role := object("rbac.authorization.k8s.io/v1", "ClusterRole", "", "station-node-operator", map[string]any{
		"rules": []any{map[string]any{"apiGroups": []any{""}, "resources": []any{"pods"}, "verbs": []any{"list", "delete"}}}})
	reset := object("rbac.authorization.k8s.io/v1", "Role", "rlark-system", "station-node-reset", map[string]any{
		"rules": []any{map[string]any{"apiGroups": []any{""}, "resources": []any{"pods"}, "verbs": []any{"get", "create"}}}})
	stationAgents, err := os.ReadFile("../../../../astribot/station/deploy/station-agent.yaml")
	if err != nil {
		t.Fatal(err)
	}
	tree := &Tree{Commit: "1111111111111111111111111111111111111111", MemberDefinitions: []*unstructured.Unstructured{role, reset}, Files: map[string]string{"station-agent.yaml": string(stationAgents)}}
	// The office cluster holds the ClusterRole as it was put there before: without delete.
	old := role.DeepCopy()
	_ = unstructured.SetNestedSlice(old.Object, []any{map[string]any{"apiGroups": []any{""}, "resources": []any{"pods"}, "verbs": []any{"list"}}}, "rules")
	if _, err := s.memberDefinitions["office"].Resource(definitionKinds["ClusterRole"].resource).Create(context.Background(), old, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	dry := newRun(t, s, environment, desired, &out)
	dry.Tree, dry.DryRun = tree, true
	if err := dry.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"crds-rbac        lab ClusterRole /station-node-operator", "crds-rbac        lab Role rlark-system/station-node-reset",
		"crds-rbac        office ClusterRole /station-node-operator: differs at rules[0].verbs", "crds-rbac        office Role rlark-system/station-node-reset"} {
		if !strings.Contains(out.String(), line) {
			t.Errorf("the plan lacks %q:\n%s", line, out.String())
		}
	}
	if len(s.writes) != 0 {
		t.Fatalf("a dry run wrote %v", s.writes)
	}

	run := newRun(t, s, environment, desired, &out)
	run.Tree = tree
	if err := run.Apply(context.Background()); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	for _, write := range []string{"lab definition clusterroles station-node-operator", "lab definition roles station-node-reset",
		"office definition clusterroles station-node-operator", "office definition roles station-node-reset"} {
		if !slices.Contains(s.writes, write) {
			t.Errorf("no write %q in %v", write, s.writes)
		}
	}
	current, err := run.Site.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if changes, err := run.changes(context.Background(), "crds-rbac", current); err != nil || len(changes) != 0 {
		t.Errorf("after the run: %v %v", changes, err)
	}

	// A second run writes nothing; going back leaves the definitions.
	s.writes = nil
	again := newRun(t, s, environment, desired, &out)
	again.Tree = tree
	if err := again.Apply(context.Background()); err != nil || slices.ContainsFunc(s.writes, func(write string) bool { return strings.Contains(write, " definition ") }) {
		t.Errorf("a second run: %v %v", err, s.writes)
	}
	records, _ := filepath.Glob(filepath.Join(environment.State, "*-before.json"))
	slices.Sort(records)
	data, _ := os.ReadFile(records[0])
	var before State
	if err := json.Unmarshal(data, &before); err != nil {
		t.Fatal(err)
	}
	back := newRun(t, s, environment, before, &out)
	back.Back, back.Tree = true, nil
	for _, member := range s.memberDefinitions {
		member.ClearActions()
	}
	if err := back.Apply(context.Background()); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	for cluster, member := range s.memberDefinitions {
		if len(member.Actions()) != 0 {
			t.Errorf("going back touched %s: %v", cluster, member.Actions())
		}
	}
}

// A run that is done records the release the site runs; a run back writes back the record there was.
func TestTheReleaseASiteRunsIsRecorded(t *testing.T) {
	s := newFakeSite(t)
	_, environment, release := writeFiles(t)
	desired := environment.Desired(release, testClusters)
	desired.Record = map[string]string{"release": "candidate", "commit": "1111111111111111111111111111111111111111"}
	var out bytes.Buffer
	if err := newRun(t, s, environment, desired, &out).Apply(context.Background()); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	current, err := s.mcore.CoreV1().ConfigMaps("rlark-system").Get(context.Background(), "rlark-release-current", metav1.GetOptions{})
	if err != nil || current.Data["release"] != "candidate" {
		t.Fatalf("%v %v\n%s", current, err, out.String())
	}
	before := regexp.MustCompile(`the state before is recorded in (\S+)`).FindStringSubmatch(out.String())[1]
	data, _ := os.ReadFile(before)
	var was State
	if err := json.Unmarshal(data, &was); err != nil || was.Record != nil {
		t.Fatalf("the state before: %v %v", was.Record, err)
	}

	// A second run, of another release with nothing else to change; then back to the record before it.
	s.mcore.ClearActions()
	desired.Record = map[string]string{"release": "next"}
	out.Reset()
	if err := newRun(t, s, environment, desired, &out).Apply(context.Background()); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "the record rlark-system/rlark-release-current is release next") {
		t.Errorf("the record is not written when nothing else changes:\n%s", out.String())
	}
	back := environment.Desired(release, testClusters)
	back.Record = map[string]string{"release": "candidate", "commit": "1111111111111111111111111111111111111111"}
	run := newRun(t, s, environment, back, &out)
	run.Back = true
	if err := run.Apply(context.Background()); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if current, _ := s.mcore.CoreV1().ConfigMaps("rlark-system").Get(context.Background(), "rlark-release-current", metav1.GetOptions{}); current.Data["release"] != "candidate" {
		t.Errorf("going back left the record at %v", current.Data)
	}
	// A run without a tree states none: the record stays.
	back.Record = nil
	out.Reset()
	if err := newRun(t, s, environment, back, &out).Apply(context.Background()); err != nil || !strings.Contains(out.String(), "is left as it is: the run has no tree") {
		t.Errorf("%v\n%s", err, out.String())
	}
}

// The release's credential for a member cluster: its permissions written there, its token kept in the management
// plane as a kubeconfig at the address the SiteNetwork's names give the cluster's API (or as its administrator
// reaches it, where they give none); the release reaches every cluster of the management plane by it.
func TestTheReleaseReachesAMemberByItsCredential(t *testing.T) {
	s := newFakeSite(t)
	network, _ := s.management.Resource(siteNetworkGVR).Namespace("station-system").Get(context.Background(), "default", metav1.GetOptions{})
	_ = unstructured.SetNestedSlice(network.Object, []any{map[string]any{"name": "cp.lab.rlark.internal", "address": "10.254.240.5"}}, "spec", "names")
	if _, err := s.management.Resource(siteNetworkGVR).Namespace("station-system").Update(context.Background(), network, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	server, serverName, err := ReachedAs(context.Background(), s.management, &rest.Config{Host: "https://cp.lab.rlark.internal:6443"})
	if err != nil || server != "https://10.254.240.5:6443" || serverName != "cp.lab.rlark.internal" {
		t.Fatalf("%s %s %v", server, serverName, err)
	}
	// A cluster whose API the site does not name: as its administrator reaches it.
	if server, serverName, err := ReachedAs(context.Background(), s.management, &rest.Config{Host: "https://10.11.14.128:6443", TLSClientConfig: rest.TLSClientConfig{ServerName: "office"}}); err != nil || server != "https://10.11.14.128:6443" || serverName != "office" {
		t.Errorf("%s %s %v", server, serverName, err)
	}

	rbac, err := os.ReadFile("../../../../" + releaseMemberRBAC)
	if err != nil {
		t.Fatal(err)
	}
	// The token the cluster fills in for the ServiceAccount.
	s.create(s.members["lab"], &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "rlark-system", Name: "rlark-release-token"},
		Data: map[string][]byte{"token": []byte("the-token"), "ca.crt": []byte("the-ca")}})
	if err := IssueMemberCredential(context.Background(), s.members["lab"], s.memberDefinitions["lab"], s.mcore, "lab", server, serverName, rbac); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(s.writes, "lab definition clusterroles rlark-release") || !slices.Contains(s.writes, "lab definition secrets rlark-release-token") {
		t.Errorf("the permissions are not written: %v", s.writes)
	}
	secret, err := s.mcore.CoreV1().Secrets(CredentialNamespace).Get(context.Background(), "release-lab", metav1.GetOptions{})
	if err != nil || secret.Labels[ClusterLabel] != "lab" {
		t.Fatalf("%v %v", secret, err)
	}
	config, err := clientcmd.Load(secret.Data["kubeconfig"])
	if err != nil {
		t.Fatal(err)
	}
	if api := config.Clusters["lab"]; api.Server != server || api.TLSServerName != "cp.lab.rlark.internal" || string(api.CertificateAuthorityData) != "the-ca" || config.AuthInfos["rlark-release"].Token != "the-token" {
		t.Errorf("the credential: %+v %+v", api, config.AuthInfos)
	}
	// Issued again (a token replaced): the same Secret, updated.
	if err := IssueMemberCredential(context.Background(), s.members["lab"], s.memberDefinitions["lab"], s.mcore, "lab", server, serverName, rbac); err != nil {
		t.Fatal(err)
	}

	if _, err := MemberConfigs(context.Background(), s.management, s.mcore); err == nil || !strings.Contains(err.Error(), "cluster office has no credential of the release") {
		t.Errorf("a cluster without its credential: %v", err)
	}
	if err := IssueMemberCredential(context.Background(), s.members["lab"], s.memberDefinitions["lab"], s.mcore, "office", "https://10.11.14.128:6443", "", rbac); err != nil {
		t.Fatal(err)
	}
	configs, err := MemberConfigs(context.Background(), s.management, s.mcore)
	if err != nil || len(configs) != 2 || configs["lab"].Host != server || configs["lab"].TLSClientConfig.ServerName != "cp.lab.rlark.internal" || configs["lab"].BearerToken != "the-token" {
		t.Errorf("%+v %v", configs, err)
	}
}

// Every Station DaemonSet pulls with the cluster's account of the site's registry and no other (live 11-16: the
// office's station-agent had none, and a node that joined could not pull its image; 11-17: station-agent-network
// named the temporary station-registry besides); a cluster that states its own takes it, Station's Deployments too.
func TestStationDaemonSetsPullWithTheClustersAccount(t *testing.T) {
	s := newFakeSite(t)
	daemonSets := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}
	object, _ := s.members["lab"].Tracker().Get(daemonSets, stationNamespace, "station-agent-host")
	host := object.(*appsv1.DaemonSet)
	host.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "station-registry"}, {Name: "ir-0123456789abcdef"}}
	_ = s.members["lab"].Tracker().Update(daemonSets, host, stationNamespace)
	_, environment, release := writeFiles(t)
	desired := environment.Desired(release, testClusters)
	var out bytes.Buffer
	dry := newRun(t, s, environment, desired, &out)
	dry.DryRun = true
	if err := dry.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"station-agents   lab Station DaemonSets' pull secret:  | station-registry,ir-0123456789abcdef -> ir-0123456789abcdef",
		"station-agents   office Station DaemonSets' pull secret: (none) -> ir-0123456789abcdef"} {
		if !strings.Contains(out.String(), line) {
			t.Errorf("the plan lacks %q:\n%s", line, out.String())
		}
	}
	out.Reset()
	if err := newRun(t, s, environment, desired, &out).Apply(context.Background()); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	for _, cluster := range []string{"lab", "office"} {
		want := "ir-0123456789abcdef"
		for _, name := range StationDaemonSets {
			object, err := s.members[cluster].Tracker().Get(daemonSets, stationNamespace, name)
			if err != nil {
				continue
			}
			if pulls := object.(*appsv1.DaemonSet).Spec.Template.Spec.ImagePullSecrets; !slices.Equal(pulls, []corev1.LocalObjectReference{{Name: want}}) {
				t.Errorf("%s %s pulls with %v", cluster, name, pulls)
			}
		}
	}
	for _, name := range []string{stationAPI, stationController} {
		deployment, _ := s.members["office"].AppsV1().Deployments(stationNamespace).Get(context.Background(), name, metav1.GetOptions{})
		if pulls := deployment.Spec.Template.Spec.ImagePullSecrets; !slices.Equal(pulls, []corev1.LocalObjectReference{{Name: "ir-0123456789abcdef"}}) {
			t.Errorf("%s pulls with %v", name, pulls)
		}
	}
}

// The control plane's compose file and values are the tree's and the environment's: written once, before the first
// service the run starts anew, and each service is started anew where its definition changed though its image did
// not. Going back writes the ones the machine had.
func TestTheControlPlaneRunsTheTreesComposeFile(t *testing.T) {
	s := newFakeSite(t)
	_, environment, release := writeFiles(t)
	environment.ControlPlane.Variables = map[string]string{"R": "/var/lib/rlark", "STATION_API_ADDRESS": "https://10.254.240.1:18443", "GATEWAY_TOKEN_TTL": "168h"}
	environment.Station.BootstrapURL, environment.AddonCatalog = "https://10.11.14.128:18443", "/var/lib/rlark/addons"
	desired := environment.Desired(release, testClusters)
	// The release's images are the ones running: only the definitions change.
	desired.ControlPlane = maps.Clone(s.plane)
	desired.Compose = environment.Compose(Tree{Compose: "services: {gateway: {}}\n"})
	if want := map[string]string{"R": "/var/lib/rlark", "STATION_API_ADDRESS": "https://10.254.240.1:18443", "GATEWAY_TOKEN_TTL": "168h",
		"STATION_API_BOOTSTRAP_URL": "https://10.11.14.128:18443", "ADDONS": "/var/lib/rlark/addons"}; !maps.Equal(desired.Compose.Variables, want) {
		t.Fatalf("the compose values: %v", desired.Compose.Variables)
	}
	machine := s.compose
	var out bytes.Buffer
	run := newRun(t, s, environment, desired, &out)
	current, _ := run.Site.Read(context.Background())
	plan, err := run.Plan(context.Background(), current)
	if err != nil {
		t.Fatal(err)
	}
	var planned []string
	for _, change := range plan {
		if slices.Contains([]string{"gateway", "server", "console"}, change.Stage) {
			planned = append(planned, change.Stage+" "+change.What)
		}
	}
	if want := []string{"gateway control plane compose file", "gateway control plane ADDONS", "gateway control plane GATEWAY_TOKEN_TTL",
		"gateway control plane STATION_API_ADDRESS", "gateway control plane STATION_API_BOOTSTRAP_URL", "gateway rlark-gateway definition",
		"server rlark-server definition", "server rlark-controller-manager definition", "console rlark-ui definition"}; !slices.Equal(planned, want) {
		t.Errorf("the plan: %v", planned)
	}
	if err := run.Apply(context.Background()); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	var plane []string
	for _, write := range s.writes {
		if strings.HasPrefix(write, "compose") {
			plane = append(plane, strings.Fields(write)[1])
		}
	}
	if want := []string{"files", "rlark-gateway", "rlark-server", "rlark-controller-manager", "rlark-ui"}; !slices.Equal(plane, want) {
		t.Errorf("the control plane's writes: %v", plane)
	}
	if s.compose.File != desired.Compose.File || !maps.Equal(s.compose.Variables, desired.Compose.Variables) {
		t.Errorf("the machine's compose: %+v", s.compose)
	}

	records, _ := filepath.Glob(filepath.Join(environment.State, "*-before.json"))
	data, _ := os.ReadFile(records[0])
	var before State
	if err := json.Unmarshal(data, &before); err != nil {
		t.Fatal(err)
	}
	back := newRun(t, s, environment, before, &out)
	back.Back, back.Tree = true, nil
	s.writes = nil
	if err := back.Apply(context.Background()); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	// Going back the console is the first service started anew: the machine's files are written before it.
	files, console := slices.Index(s.writes, "compose files"), slices.IndexFunc(s.writes, func(write string) bool { return strings.HasPrefix(write, "compose rlark-ui") })
	if s.compose.File != machine.File || !maps.Equal(s.compose.Variables, machine.Variables) || files < 0 || files > console {
		t.Errorf("back: the machine's compose %+v, writes %v", s.compose, s.writes)
	}
}

// A Station DaemonSet a cluster has is made what the tree states (station-agent.yaml) by the stage that sets its
// images: one that kept what the tree dropped (live 11-20, the wireguard-config ConfigMap's volume, its mount and
// STATION_GATEWAY_CONFIG) loses it, the plan names each field, one that is as stated (what the API server fills in
// aside) is no change, and the stage waits for the Pods of the template before.
func TestStationDaemonSetsAreMadeWhatTheTreeStates(t *testing.T) {
	s := newFakeSite(t)
	_, environment, release := writeFiles(t)
	desired := environment.Desired(release, testClusters)
	ctx := context.Background()
	var out bytes.Buffer
	if err := newRun(t, s, environment, desired, &out).Apply(ctx); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	daemonSets := s.members["lab"].AppsV1().DaemonSets(stationNamespace)
	stated, _ := daemonSets.Get(ctx, "station-agent-network", metav1.GetOptions{})
	if stated.Spec.UpdateStrategy.Type != appsv1.OnDeleteDaemonSetStrategyType || stated.Annotations[rolloutAnnotation] != "{}" || stated.Spec.Template.Spec.Affinity == nil ||
		stated.Spec.Template.Spec.Containers[1].Image != wg || len(stated.Spec.Template.Spec.Volumes) != 7 {
		t.Fatalf("station-agent-network is not as the tree states it: %+v", stated)
	}
	plan := func() []string {
		run := newRun(t, s, environment, desired, &out)
		current, _ := run.Site.Read(ctx)
		changes, err := run.Plan(ctx, current)
		if err != nil {
			t.Fatal(err)
		}
		var lines []string
		for _, change := range changes {
			lines = append(lines, strings.Join(strings.Fields(change.String()), " "))
		}
		return lines
	}
	if lines := plan(); len(lines) > 0 {
		t.Fatalf("a site as the release states has changes: %v", lines)
	}

	kept := stated.DeepCopy()
	kept.Spec.Template.Spec.Volumes = append(kept.Spec.Template.Spec.Volumes, corev1.Volume{Name: "wireguard-config",
		VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "station-wireguard-config"}}}})
	wireguard := &kept.Spec.Template.Spec.Containers[1]
	wireguard.VolumeMounts = append(wireguard.VolumeMounts, corev1.VolumeMount{Name: "wireguard-config", MountPath: "/etc/station"})
	wireguard.Env = append(wireguard.Env, corev1.EnvVar{Name: "STATION_GATEWAY_CONFIG", Value: "/etc/station/config.json"})
	if _, err := daemonSets.Update(ctx, kept, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"station-agents lab DaemonSet station-agent-network spec.template.spec.containers[wireguard].env[STATION_GATEWAY_CONFIG]: {\"name\":\"STATION_GATEWAY_CONFIG\",\"value\":\"/etc/station/config.json\"} -> (none)",
		"station-agents lab DaemonSet station-agent-network spec.template.spec.containers[wireguard].volumeMounts[wireguard-config]: {\"mountPath\":\"/etc/station\",\"name\":\"wireguard-config\"} -> (none)",
		"station-agents lab DaemonSet station-agent-network spec.template.spec.volumes[wireguard-config]: {\"configMap\":{\"name\":\"station-wireguard-config\"},\"name\":\"wireguard-config\"} -> (none)",
	}
	if lines := plan(); !slices.Equal(lines, want) {
		t.Fatalf("the plan is\n%s", strings.Join(lines, "\n"))
	}
	out.Reset()
	if err := newRun(t, s, environment, desired, &out).Apply(ctx); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if lines := plan(); len(lines) > 0 {
		t.Errorf("after the run: %v", lines)
	}
	if made, _ := daemonSets.Get(ctx, "station-agent-network", metav1.GetOptions{}); !equality.Semantic.DeepEqual(made.Spec, stated.Spec) {
		t.Errorf("station-agent-network is not as the tree states it again: %+v", made.Spec)
	}

	// The cluster agent has not come to replace the Pods of the template before: the stage is not done.
	if _, err := daemonSets.Update(ctx, kept, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	s.unrolled = "lab/station-agent-network"
	if err := newRun(t, s, environment, desired, &out).Apply(ctx); err == nil || !strings.Contains(err.Error(), "station-system/station-agent-network not yet replaced and ready on lab-1") {
		t.Errorf("a run whose Station agents were not replaced: %v", err)
	}
}
