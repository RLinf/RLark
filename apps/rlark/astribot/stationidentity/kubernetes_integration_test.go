package stationidentity

import (
	"context"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"testing"
	"time"
)

func TestLiveKubernetesIdentity(t *testing.T) {
	if os.Getenv("STATION_TEST_KUBECONFIG") == "" || os.Getenv("STATION_TEST_NODE") == "" {
		t.Skip("requires explicitly configured disposable Kubernetes test environment")
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", os.Getenv("STATION_TEST_KUBECONFIG"))
	if err != nil {
		t.Fatal(err)
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	ns, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "station-identity-test-"}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), 30*time.Second)
		defer done()
		if e := client.CoreV1().Namespaces().Delete(cleanup, ns.Name, metav1.DeleteOptions{}); e != nil {
			t.Errorf("cleanup namespace: %v", e)
		}
	})
	sa, err := client.CoreV1().ServiceAccounts(ns.Name).Create(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "station"}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	node, err := client.CoreV1().Nodes().Get(ctx, os.Getenv("STATION_TEST_NODE"), metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pod, err := client.CoreV1().Pods(ns.Name).Create(ctx, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "identity"}, Spec: corev1.PodSpec{NodeName: node.Name, ServiceAccountName: sa.Name, Containers: []corev1.Container{{Name: "placeholder", Image: "invalid.invalid/never-pull", ImagePullPolicy: corev1.PullNever}}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	seconds := int64(600)
	tok, err := client.CoreV1().ServiceAccounts(ns.Name).CreateToken(ctx, sa.Name, &authv1.TokenRequest{Spec: authv1.TokenRequestSpec{Audiences: []string{"station-api-test"}, ExpirationSeconds: &seconds, BoundObjectRef: &authv1.BoundObjectReference{APIVersion: "v1", Kind: "Pod", Name: pod.Name, UID: pod.UID}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	reg := Registration{ClusterID: "local", Namespace: ns.Name, ServiceAccount: sa.Name, ServiceAccountUID: string(sa.UID), Audience: "station-api-test"}
	bindings := mapBindings{"local/" + node.Name: {NodeUID: string(node.UID), StationUID: "test-station", Generation: 1}}
	if _, err = AuthenticateWithBindings(ctx, client, reg, bindings, tok.Status.Token); err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	wrong := reg
	wrong.Audience = "wrong-audience"
	if _, err = AuthenticateWithBindings(ctx, client, wrong, bindings, tok.Status.Token); err == nil {
		t.Fatal("wrong audience accepted")
	}
	zero := int64(0)
	if err = client.CoreV1().Pods(ns.Name).Delete(ctx, pod.Name, metav1.DeleteOptions{GracePeriodSeconds: &zero}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		_, err = AuthenticateWithBindings(ctx, client, reg, bindings, tok.Status.Token)
		if err != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("deleted pod token remained valid")
		}
		time.Sleep(200 * time.Millisecond)
	}
}
