package domain

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/auth/cert"
)

type countingClient struct {
	client.Client
	updates       int
	statusUpdates int
}

func (c *countingClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	c.updates++
	return c.Client.Update(ctx, obj, opts...)
}

func (c *countingClient) Status() client.StatusWriter {
	return &countingStatusWriter{SubResourceWriter: c.Client.Status(), parent: c}
}

type countingStatusWriter struct {
	client.SubResourceWriter
	parent *countingClient
}

func (w *countingStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	w.parent.statusUpdates++
	return w.SubResourceWriter.Update(ctx, obj, opts...)
}

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}
	return scheme
}

func newTestClientBuilder(t *testing.T) *fake.ClientBuilder {
	t.Helper()
	return fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithIndex(&rlarkv1alpha1.Pod{}, podDomainField, func(obj client.Object) []string {
			pod := obj.(*rlarkv1alpha1.Pod)
			if pod.Spec.Domain == "" {
				return nil
			}
			return []string{pod.Spec.Domain}
		})
}

func validCertPEM(t *testing.T) (string, string) {
	t.Helper()
	ca, err := cert.GenerateCA(cert.GenerateTemplateCA())
	if err != nil {
		t.Fatalf("failed to generate CA: %v", err)
	}
	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate leaf key: %v", err)
	}
	sshKey, err := gossh.NewPublicKey(leafKey.Public())
	if err != nil {
		t.Fatalf("failed to create ssh public key: %v", err)
	}
	sshTemplate := &gossh.Certificate{
		Key:             sshKey,
		CertType:        gossh.UserCert,
		KeyId:           "test",
		ValidPrincipals: []string{"test"},
		ValidAfter:      uint64(time.Now().Add(-1 * time.Hour).Unix()),
		ValidBefore:     uint64(time.Now().Add(20 * 365 * 24 * time.Hour).Unix()),
	}
	sshPEM, err := ca.SignSSHCertificate(sshTemplate)
	if err != nil {
		t.Fatalf("failed to sign ssh certificate: %v", err)
	}
	keyPEM, err := cert.EncodePrivateKeyToPEM(leafKey)
	if err != nil {
		t.Fatalf("failed to encode private key: %v", err)
	}
	return string(sshPEM), string(keyPEM)
}

func makePod(name, namespace, domain, podNamespace, podName, node, localIP string, phase rlarkv1alpha1.PodPhase) *rlarkv1alpha1.Pod {
	return &rlarkv1alpha1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: rlarkv1alpha1.PodSpec{
			Domain:       domain,
			PodNamespace: podNamespace,
			PodName:      podName,
		},
		Status: rlarkv1alpha1.PodStatus{
			Phase: phase,
			Node:  node,
			IP:    localIP,
		},
	}
}

func makeDomainPeer(name, namespace, certPEM, keyPEM string) *rlarkv1alpha1.DomainPeer {
	return &rlarkv1alpha1.DomainPeer{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: rlarkv1alpha1.DomainPeerSpec{
			Cert: certPEM,
			Key:  keyPEM,
		},
	}
}

func doReconcile(t *testing.T, objs ...client.Object) (rlarkv1alpha1.Domain, map[string][]rlarkv1alpha1.DomainPeer) {
	t.Helper()
	scheme := newTestScheme(t)
	cl := newTestClientBuilder(t).
		WithObjects(objs...).
		WithStatusSubresource(&rlarkv1alpha1.Domain{}).
		Build()

	r := &Reconciler{Client: cl, Scheme: scheme}

	var domain rlarkv1alpha1.Domain
	for _, o := range objs {
		if d, ok := o.(*rlarkv1alpha1.Domain); ok {
			domain = *d
			break
		}
	}
	if domain.Name == "" {
		t.Fatalf("no Domain found in test objects")
	}

	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: domain.Name}}); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	var updatedDomain rlarkv1alpha1.Domain
	if err := cl.Get(context.Background(), types.NamespacedName{Name: domain.Name}, &updatedDomain); err != nil {
		t.Fatalf("failed to get domain after reconcile: %v", err)
	}

	var peerList rlarkv1alpha1.DomainPeerList
	if err := cl.List(context.Background(), &peerList); err != nil {
		t.Fatalf("failed to list peers: %v", err)
	}
	peersByNS := make(map[string][]rlarkv1alpha1.DomainPeer)
	for _, p := range peerList.Items {
		peersByNS[p.Namespace] = append(peersByNS[p.Namespace], p)
	}

	return updatedDomain, peersByNS
}

func allocMap(t *testing.T, d rlarkv1alpha1.Domain) map[string]string {
	t.Helper()
	m := make(map[string]string)
	for _, a := range d.Status.IPAllocations {
		m[a.Pod] = a.IP
	}
	return m
}

func TestReconcileSkipsUnchangedDomainAndPeerUpdates(t *testing.T) {
	const (
		domainName = "test-domain"
		namespace  = "cluster-a"
	)
	certPEM, keyPEM := validCertPEM(t)
	domain := &rlarkv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{Name: domainName},
		Spec:       rlarkv1alpha1.DomainSpec{CIDR: "10.244.0.0/29"},
		Status: rlarkv1alpha1.DomainStatus{IPAllocations: []rlarkv1alpha1.DomainIPAllocation{{
			IP: "10.244.0.1", Pod: namespace + "/rlark-system/pod-a",
		}}},
	}
	pod := makePod("uid-a", namespace, domainName, "rlark-system", "pod-a", "node-a", "10.0.0.1", rlarkv1alpha1.PodPhaseRunning)
	peer := makeDomainPeer(domainName, namespace, certPEM, keyPEM)
	peer.Spec.PrefixLen = 29
	peer.Spec.Pods = []rlarkv1alpha1.DomainPodInfo{{
		GlobalNamespace: namespace,
		Namespace:       "rlark-system",
		Name:            "pod-a",
		UID:             "uid-a",
		Node:            "node-a",
		IP:              "10.244.0.1",
		LocalIP:         "10.0.0.1",
	}}
	scheme := newTestScheme(t)
	wrapped := &countingClient{Client: newTestClientBuilder(t).
		WithObjects(domain, pod, peer).
		WithStatusSubresource(&rlarkv1alpha1.Domain{}).
		Build()}
	r := &Reconciler{Client: wrapped, Scheme: scheme}

	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: domainName}}); err != nil {
		t.Fatal(err)
	}
	if wrapped.statusUpdates != 0 || wrapped.updates != 0 {
		t.Fatalf("unchanged Domain caused writes: status=%d peers=%d", wrapped.statusUpdates, wrapped.updates)
	}
}

func TestReconcileSkipsPodWithoutNetworkStatus(t *testing.T) {
	const (
		domainName = "test-domain"
		namespace  = "cluster-a"
	)
	certPEM, keyPEM := validCertPEM(t)
	domain := &rlarkv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{Name: domainName},
		Spec:       rlarkv1alpha1.DomainSpec{CIDR: "10.244.0.0/29"},
	}
	pod := makePod("uid-a", namespace, domainName, "rlark-system", "pod-a", "", "", rlarkv1alpha1.PodPhasePending)
	peer := makeDomainPeer(domainName, namespace, certPEM, keyPEM)

	updated, peersByNS := doReconcile(t, domain, pod, peer)
	if len(updated.Status.IPAllocations) != 0 {
		t.Fatalf("expected no allocation for incomplete Pod status, got %d", len(updated.Status.IPAllocations))
	}
	if len(peersByNS[namespace]) != 1 {
		t.Fatalf("expected existing DomainPeer to be retained, got %d", len(peersByNS[namespace]))
	}
	if len(peersByNS[namespace][0].Spec.Pods) != 0 {
		t.Fatalf("expected incomplete Pod to be excluded from DomainPeer, got %d pods", len(peersByNS[namespace][0].Spec.Pods))
	}
}

func TestReconcileListsOnlyPodsInDomain(t *testing.T) {
	certPEM, keyPEM := validCertPEM(t)
	domain := &rlarkv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{Name: "domain-a"},
		Spec:       rlarkv1alpha1.DomainSpec{CIDR: "10.244.0.0/29"},
	}
	podA := makePod("uid-a", "cluster-a", domain.Name, "rlark-system", "pod-a", "node-a", "10.0.0.1", rlarkv1alpha1.PodPhaseRunning)
	podB := makePod("uid-b", "cluster-b", "domain-b", "rlark-system", "pod-b", "node-b", "10.0.0.2", rlarkv1alpha1.PodPhaseRunning)
	peer := makeDomainPeer(domain.Name, podA.Namespace, certPEM, keyPEM)

	updated, peersByNS := doReconcile(t, domain, podA, podB, peer)
	if len(updated.Status.IPAllocations) != 1 {
		t.Fatalf("expected one allocation, got %d", len(updated.Status.IPAllocations))
	}
	if got := updated.Status.IPAllocations[0].Pod; got != "cluster-a/rlark-system/pod-a" {
		t.Fatalf("unexpected allocation for indexed Domain query: %s", got)
	}
	if _, exists := peersByNS[podB.Namespace]; exists {
		t.Fatal("Pod from another Domain created a DomainPeer")
	}
}

// TestReconcile_PodRestartReusesIP verifies that when a pod is recreated
// (new UID) while its old terminal CR still exists, the new pod reuses the
// original IP and the terminal pod is skipped.
func TestReconcile_PodRestartReusesIP(t *testing.T) {
	const (
		domainName = "test-domain"
		ns         = "cluster-1"
	)
	certPEM, keyPEM := validCertPEM(t)

	domain := &rlarkv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{Name: domainName},
		Spec:       rlarkv1alpha1.DomainSpec{CIDR: "10.244.0.0/29"},
		Status: rlarkv1alpha1.DomainStatus{
			IPAllocations: []rlarkv1alpha1.DomainIPAllocation{
				{IP: "10.244.0.2", Pod: ns + "/rlark-system/env-0"},
				{IP: "10.244.0.1", Pod: ns + "/rlark-system/env-1"},
			},
		},
	}

	objs := []client.Object{
		domain,
		makeDomainPeer(domainName, ns, certPEM, keyPEM),
		// Old env-0 pod (terminal — should be skipped).
		makePod("uid-old-env0", ns, domainName, "rlark-system", "env-0", "node-1", "10.42.0.1", rlarkv1alpha1.PodPhaseSucceeded),
		// New env-0 pod (running — should reuse 10.244.0.2).
		makePod("uid-new-env0", ns, domainName, "rlark-system", "env-0", "node-1", "10.42.0.2", rlarkv1alpha1.PodPhaseRunning),
		// env-1 pod (running — should reuse 10.244.0.1).
		makePod("uid-env1", ns, domainName, "rlark-system", "env-1", "node-2", "10.42.0.3", rlarkv1alpha1.PodPhaseRunning),
	}

	updatedDomain, peersByNS := doReconcile(t, objs...)
	m := allocMap(t, updatedDomain)

	if ip := m[ns+"/rlark-system/env-0"]; ip != "10.244.0.2" {
		t.Errorf("env-0 IP: expected 10.244.0.2 (reused), got %s", ip)
	}
	if ip := m[ns+"/rlark-system/env-1"]; ip != "10.244.0.1" {
		t.Errorf("env-1 IP: expected 10.244.0.1 (reused), got %s", ip)
	}
	if len(m) != 2 {
		t.Errorf("expected exactly 2 allocations, got %d: %v", len(m), m)
	}

	peers := peersByNS[ns]
	if len(peers) != 1 {
		t.Fatalf("expected 1 DomainPeer in %s, got %d", ns, len(peers))
	}
	if len(peers[0].Spec.Pods) != 2 {
		t.Errorf("expected 2 pods in DomainPeer (terminal skipped), got %d", len(peers[0].Spec.Pods))
	}
	for _, p := range peers[0].Spec.Pods {
		if p.Name == "env-0" && p.IP != "10.244.0.2" {
			t.Errorf("peer pod env-0 IP: expected 10.244.0.2, got %s", p.IP)
		}
		if p.Name == "env-1" && p.IP != "10.244.0.1" {
			t.Errorf("peer pod env-1 IP: expected 10.244.0.1, got %s", p.IP)
		}
	}
}

// TestReconcile_TerminalPodKeepsDomainPeer verifies that the controller does
// not remove a cluster's DomainPeer during the gap between a container exit
// and its workload becoming active again. The Pod CR still exists throughout
// that gap, and its network sidecar may still be serving connections.
func TestReconcile_TerminalPodKeepsDomainPeer(t *testing.T) {
	const (
		domainName = "test-domain"
		ns         = "cluster-1"
	)
	certPEM, keyPEM := validCertPEM(t)
	peer := makeDomainPeer(domainName, ns, certPEM, keyPEM)
	peer.Spec.PrefixLen = 29
	peer.Spec.Pods = []rlarkv1alpha1.DomainPodInfo{{
		GlobalNamespace: ns,
		Namespace:       "rlark-system",
		Name:            "env-0",
		UID:             "uid-env0",
		Node:            "node-1",
		IP:              "10.244.0.1",
		LocalIP:         "10.42.0.1",
	}}

	domain := &rlarkv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{Name: domainName},
		Spec:       rlarkv1alpha1.DomainSpec{CIDR: "10.244.0.0/29"},
		Status: rlarkv1alpha1.DomainStatus{IPAllocations: []rlarkv1alpha1.DomainIPAllocation{{
			IP:  "10.244.0.1",
			Pod: ns + "/rlark-system/env-0",
		}}},
	}

	_, peersByNS := doReconcile(t,
		domain,
		peer,
		makePod("uid-env0", ns, domainName, "rlark-system", "env-0", "node-1", "10.42.0.1", rlarkv1alpha1.PodPhaseSucceeded),
	)

	peers := peersByNS[ns]
	if len(peers) != 1 {
		t.Fatalf("expected DomainPeer to be retained while terminal Pod CR exists, got %d", len(peers))
	}
	if len(peers[0].Spec.Pods) != 1 || peers[0].Spec.Pods[0].IP != "10.244.0.1" {
		t.Fatalf("expected existing route to be retained, got %+v", peers[0].Spec.Pods)
	}
}

func TestReconcile_RemovedPodDeletesDomainPeer(t *testing.T) {
	const (
		domainName = "test-domain"
		ns         = "cluster-1"
	)
	certPEM, keyPEM := validCertPEM(t)
	domain := &rlarkv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{Name: domainName},
		Spec:       rlarkv1alpha1.DomainSpec{CIDR: "10.244.0.0/29"},
	}

	_, peersByNS := doReconcile(t,
		domain,
		makeDomainPeer(domainName, ns, certPEM, keyPEM),
	)

	if peers := peersByNS[ns]; len(peers) != 0 {
		t.Fatalf("expected stale DomainPeer to be deleted after all Pod CRs are removed, got %d", len(peers))
	}
}

// TestReconcile_DuplicateIPConflictResolved verifies that when two different
// pods have allocations pointing to the same IP (historical corruption), only
// the first one reuses it; the other gets a fresh IP.
func TestReconcile_DuplicateIPConflictResolved(t *testing.T) {
	const (
		domainName = "test-domain"
		ns1        = "cluster-1"
		ns2        = "cluster-2"
	)
	certPEM, keyPEM := validCertPEM(t)

	domain := &rlarkv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{Name: domainName},
		Spec:       rlarkv1alpha1.DomainSpec{CIDR: "10.244.0.0/29"},
		Status: rlarkv1alpha1.DomainStatus{
			IPAllocations: []rlarkv1alpha1.DomainIPAllocation{
				{IP: "10.244.0.2", Pod: ns1 + "/rlark-system/env-0"},
				{IP: "10.244.0.1", Pod: ns1 + "/rlark-system/env-1"},
				{IP: "10.244.0.1", Pod: ns2 + "/rlark-system/actor-0"}, // conflict with env-1
			},
		},
	}

	objs := []client.Object{
		domain,
		makeDomainPeer(domainName, ns1, certPEM, keyPEM),
		makeDomainPeer(domainName, ns2, certPEM, keyPEM),
		makePod("uid-env0", ns1, domainName, "rlark-system", "env-0", "node-1", "10.42.0.1", rlarkv1alpha1.PodPhaseRunning),
		makePod("uid-env1", ns1, domainName, "rlark-system", "env-1", "node-2", "10.42.0.2", rlarkv1alpha1.PodPhaseRunning),
		makePod("uid-actor0", ns2, domainName, "rlark-system", "actor-0", "node-3", "172.27.0.1", rlarkv1alpha1.PodPhaseRunning),
	}

	updatedDomain, _ := doReconcile(t, objs...)
	m := allocMap(t, updatedDomain)

	if ip := m[ns1+"/rlark-system/env-0"]; ip != "10.244.0.2" {
		t.Errorf("env-0 IP: expected 10.244.0.2, got %s", ip)
	}
	if ip := m[ns1+"/rlark-system/env-1"]; ip != "10.244.0.1" {
		t.Errorf("env-1 IP: expected 10.244.0.1, got %s", ip)
	}
	if ip := m[ns2+"/rlark-system/actor-0"]; ip != "10.244.0.3" {
		t.Errorf("actor-0 IP: expected 10.244.0.3 (freshly allocated), got %s", ip)
	}

	ipCount := make(map[string]int)
	for _, a := range updatedDomain.Status.IPAllocations {
		ipCount[a.IP]++
	}
	for ip, c := range ipCount {
		if c > 1 {
			t.Errorf("IP %s allocated to %d pods (expected 1)", ip, c)
		}
	}
}

// TestReconcile_NewPodGetsFreshIP verifies that a brand-new pod with no prior
// allocation gets the next available IP.
func TestReconcile_NewPodGetsFreshIP(t *testing.T) {
	const (
		domainName = "test-domain"
		ns         = "cluster-1"
	)
	certPEM, keyPEM := validCertPEM(t)

	domain := &rlarkv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{Name: domainName},
		Spec:       rlarkv1alpha1.DomainSpec{CIDR: "10.244.0.0/29"},
	}

	objs := []client.Object{
		domain,
		makeDomainPeer(domainName, ns, certPEM, keyPEM),
		makePod("uid-env0", ns, domainName, "rlark-system", "env-0", "node-1", "10.42.0.1", rlarkv1alpha1.PodPhaseRunning),
		makePod("uid-env1", ns, domainName, "rlark-system", "env-1", "node-2", "10.42.0.2", rlarkv1alpha1.PodPhaseRunning),
	}

	updatedDomain, peersByNS := doReconcile(t, objs...)
	m := allocMap(t, updatedDomain)

	if ip := m[ns+"/rlark-system/env-0"]; ip != "10.244.0.1" {
		t.Errorf("env-0 IP: expected 10.244.0.1, got %s", ip)
	}
	if ip := m[ns+"/rlark-system/env-1"]; ip != "10.244.0.2" {
		t.Errorf("env-1 IP: expected 10.244.0.2, got %s", ip)
	}

	peers := peersByNS[ns]
	if len(peers) != 1 || len(peers[0].Spec.Pods) != 2 {
		t.Errorf("expected 2 pods in DomainPeer, got peers=%d", len(peers))
	}
}

// TestReconcile_RealWorldDirtyData reproduces the exact dirty state from the
// bug report and verifies it is corrected in a single reconcile pass:
//
//   - env-0 was restarted: IPAllocations has two entries for the same podKey
//     (10.244.0.2 and 10.244.0.3), and both the terminal old CR and the new CR
//     exist. The new env-0 must reuse 10.244.0.2; the duplicate entry is dropped.
//   - actor-0 and env-1 were both allocated 10.244.0.1. env-1 keeps it; actor-0
//     is reassigned a fresh, non-conflicting IP.
//
// After reconcile: no duplicate podKey, no duplicate IP, env-0 reuses its
// original IP, and the terminal old env-0 pod is excluded from DomainPeer.Pods.
func TestReconcile_RealWorldDirtyData(t *testing.T) {
	const (
		domainName = "dual-arm-hg-dagger"
		ns1        = "rlark-zhengxing-poc"
		ns2        = "rlark-zhengxing-vc-dddgjjrejazrzyik"
	)
	certPEM, keyPEM := validCertPEM(t)

	domain := &rlarkv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{Name: domainName},
		Spec:       rlarkv1alpha1.DomainSpec{CIDR: "10.244.0.0/29"},
		Status: rlarkv1alpha1.DomainStatus{
			IPAllocations: []rlarkv1alpha1.DomainIPAllocation{
				{IP: "10.244.0.2", Pod: ns1 + "/rlark-system/dual-arm-hg-dagger-environment-0"},
				{IP: "10.244.0.1", Pod: ns1 + "/rlark-system/dual-arm-hg-dagger-environment-1"},
				{IP: "10.244.0.3", Pod: ns1 + "/rlark-system/dual-arm-hg-dagger-environment-0"}, // 重复 podKey（重启后误分配）
				{IP: "10.244.0.1", Pod: ns2 + "/rlark-system/dual-arm-hg-dagger-actor-0"},       // 与 env-1 IP 冲突
			},
		},
	}

	objs := []client.Object{
		domain,
		makeDomainPeer(domainName, ns1, certPEM, keyPEM),
		makeDomainPeer(domainName, ns2, certPEM, keyPEM),
		// 旧 env-0（终态，应被跳过）
		makePod("06355f3f-6b2d-4ea3-855e-3204740a4e1a", ns1, domainName, "rlark-system", "dual-arm-hg-dagger-environment-0", "sohu-dual-master", "10.42.9.158", rlarkv1alpha1.PodPhaseSucceeded),
		// env-1
		makePod("95b83a75-7cce-4918-aba1-d4b19226f381", ns1, domainName, "rlark-system", "dual-arm-hg-dagger-environment-1", "sohu-dual-slave", "10.42.16.81", rlarkv1alpha1.PodPhaseRunning),
		// 新 env-0（Running，应复用 10.244.0.2）
		makePod("67ee5c09-b07c-4221-b476-aaaee62ea26f", ns1, domainName, "rlark-system", "dual-arm-hg-dagger-environment-0", "sohu-dual-master", "10.42.9.157", rlarkv1alpha1.PodPhaseRunning),
		// actor-0（IP 与 env-1 冲突，应重新分配）
		makePod("02a7dc05-9161-4401-aa53-b5371c904f85", ns2, domainName, "rlark-system", "dual-arm-hg-dagger-actor-0", "re-dcvb3iurizg5vjsv", "172.27.29.123", rlarkv1alpha1.PodPhaseRunning),
	}

	updatedDomain, peersByNS := doReconcile(t, objs...)
	m := allocMap(t, updatedDomain)

	// 1. env-0 复用原始 IP 10.244.0.2
	if ip := m[ns1+"/rlark-system/dual-arm-hg-dagger-environment-0"]; ip != "10.244.0.2" {
		t.Errorf("env-0 IP: expected 10.244.0.2 (reused), got %s", ip)
	}
	// 2. env-1 保留 10.244.0.1
	if ip := m[ns1+"/rlark-system/dual-arm-hg-dagger-environment-1"]; ip != "10.244.0.1" {
		t.Errorf("env-1 IP: expected 10.244.0.1, got %s", ip)
	}
	// 3. actor-0 重新分配到 10.244.0.3（与 env-1 不再冲突）
	if ip := m[ns2+"/rlark-system/dual-arm-hg-dagger-actor-0"]; ip != "10.244.0.3" {
		t.Errorf("actor-0 IP: expected 10.244.0.3 (reassigned), got %s", ip)
	}
	// 4. 无重复 podKey
	if len(m) != 3 {
		t.Errorf("expected exactly 3 allocations, got %d: %v", len(m), m)
	}
	// 5. 无重复 IP
	ipCount := make(map[string]int)
	for _, a := range updatedDomain.Status.IPAllocations {
		ipCount[a.IP]++
	}
	for ip, c := range ipCount {
		if c > 1 {
			t.Errorf("IP %s allocated to %d pods (expected 1)", ip, c)
		}
	}

	// 6. DomainPeer.Pods 不应包含终态的旧 env-0
	for ns, peers := range peersByNS {
		for _, peer := range peers {
			for _, p := range peer.Spec.Pods {
				if p.Name == "dual-arm-hg-dagger-environment-0" && p.UID == "06355f3f-6b2d-4ea3-855e-3204740a4e1a" {
					t.Errorf("terminal old env-0 should be excluded from DomainPeer.Pods (ns=%s)", ns)
				}
			}
		}
	}
}
