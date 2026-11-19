package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	clientset "github.com/rlinf/rlark/api/kubeclients/clientset/versioned/fake"
	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/addons"
)

func addonRequest(t *testing.T, gateway *Gateway, method, name string, body map[string]interface{}) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/clusters/:cluster_id/addons", gateway.installClusterAddon)
	router.PUT("/clusters/:cluster_id/addons/:name", gateway.updateClusterAddon)
	router.DELETE("/clusters/:cluster_id/addons/:name", gateway.deleteClusterAddon)
	encoded, _ := json.Marshal(body)
	path := "/clusters/cluster-a/addons"
	if name != "" {
		path += "/" + name
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(method, path, bytes.NewReader(encoded)))
	return response
}

// Installing an Addon publishes the Package of its version to the cluster and pins the Addon to that version;
// the cluster's agent renders from the Package.
func TestInstallPublishesThePackageAndPinsTheVersion(t *testing.T) {
	ctx := context.Background()
	gateway := &Gateway{kubeClient: clientset.NewSimpleClientset(), rawClient: k8sfake.NewSimpleClientset()}
	item, _ := addons.Registry.Get("fluent-bit")
	version := item.Meta().Version

	// What the item requires is asked for before anything is written.
	if response := addonRequest(t, gateway, http.MethodPost, "", map[string]interface{}{"addonName": "fluent-bit"}); response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "endpoint") {
		t.Fatalf("install without required parameters: %d %s", response.Code, response.Body)
	}
	values := map[string]string{"endpoint": "logs.example:9200", "logstore": "rlark", "clusterId": "cluster-a"}
	if response := addonRequest(t, gateway, http.MethodPost, "", map[string]interface{}{"addonName": "fluent-bit", "values": values}); response.Code != http.StatusCreated {
		t.Fatalf("install: %d %s", response.Code, response.Body)
	}
	addon, err := gateway.kubeClient.RlinfV1alpha1().Addons("cluster-a").Get(ctx, "fluent-bit", metav1.GetOptions{})
	if err != nil || addon.Spec.Version != version {
		t.Fatalf("addon=%v err=%v, want version %s", addon, err, version)
	}
	published, err := gateway.rawClient.CoreV1().ConfigMaps("cluster-a").Get(ctx, addons.PackageName("fluent-bit", version), metav1.GetOptions{})
	if err != nil || published.Data["addon.yaml"] == "" || len(published.Data) < 2 || !*published.Immutable {
		t.Fatalf("published=%v err=%v", published, err)
	}

	// An update that names nothing keeps the version and publishes nothing new.
	if response := addonRequest(t, gateway, http.MethodPut, "fluent-bit", map[string]interface{}{}); response.Code != http.StatusOK {
		t.Fatalf("update: %d %s", response.Code, response.Body)
	}
	// A version nobody has is refused, and the Addon stays.
	if response := addonRequest(t, gateway, http.MethodPut, "fluent-bit", map[string]interface{}{"version": "v9"}); response.Code != http.StatusNotFound {
		t.Fatalf("update to a version that is nowhere: %d %s", response.Code, response.Body)
	}
	if addon, _ = gateway.kubeClient.RlinfV1alpha1().Addons("cluster-a").Get(ctx, "fluent-bit", metav1.GetOptions{}); addon.Spec.Version != version {
		t.Fatalf("version %s after a refused update", addon.Spec.Version)
	}

	// Removing the Addon removes what was published for it.
	if response := addonRequest(t, gateway, http.MethodDelete, "fluent-bit", nil); response.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", response.Code, response.Body)
	}
}

// A version the catalog no longer holds is still the one that was published to the cluster when the Addon was
// installed: its values can be changed, checked against that version's parameters.
func TestUpdateUsesThePackagePublishedToTheClusterWhenTheCatalogHasMovedOn(t *testing.T) {
	ctx := context.Background()
	old := addons.Package{Name: "s1u-robot", Version: "v0.9.0", Files: map[string]string{
		"addon.yaml":     "name: s1u-robot\nversion: v0.9.0\nparameters:\n- {name: image, type: string, required: true}\n",
		"daemonset.yaml": "kind: DaemonSet",
	}}.ConfigMap("cluster-a")
	installed := &rlarkv1alpha1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "s1u-robot", Namespace: "cluster-a"},
		Spec: rlarkv1alpha1.AddonSpec{AddonName: "s1u-robot", Version: "v0.9.0", Values: map[string]string{"image": "robot@sha256:1"}}}
	gateway := &Gateway{kubeClient: clientset.NewSimpleClientset(installed), rawClient: k8sfake.NewSimpleClientset(old)}

	if response := addonRequest(t, gateway, http.MethodPut, "s1u-robot", map[string]interface{}{"values": map[string]string{"image": "robot@sha256:2"}}); response.Code != http.StatusOK {
		t.Fatalf("update: %d %s", response.Code, response.Body)
	}
	addon, _ := gateway.kubeClient.RlinfV1alpha1().Addons("cluster-a").Get(ctx, "s1u-robot", metav1.GetOptions{})
	if addon.Spec.Version != "v0.9.0" || addon.Spec.Values["image"] != "robot@sha256:2" {
		t.Fatalf("spec=%#v", addon.Spec)
	}
	// Values that leave a required parameter out are refused.
	response := addonRequest(t, gateway, http.MethodPut, "s1u-robot", map[string]interface{}{"values": map[string]string{"reboot": "never"}})
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "image") {
		t.Fatalf("values without a required parameter: %d %s", response.Code, response.Body)
	}
	if addon, _ = gateway.kubeClient.RlinfV1alpha1().Addons("cluster-a").Get(ctx, "s1u-robot", metav1.GetOptions{}); addon.Spec.Values["image"] != "robot@sha256:2" {
		t.Fatalf("refused values were written: %#v", addon.Spec.Values)
	}
}

// A name at a version has one content. A cluster that holds other content for the catalog's version is not
// given the catalog's under the same name.
func TestPublishRefusesOtherContentForAPublishedVersion(t *testing.T) {
	item, _ := addons.Registry.Get("fluent-bit")
	other := addons.PackageOf(item).ConfigMap("cluster-a")
	other.Data = map[string]string{"addon.yaml": "name: fluent-bit\nversion: " + item.Meta().Version + "\n", "daemonset.yaml": "kind: DaemonSet # as it was"}
	gateway := &Gateway{kubeClient: clientset.NewSimpleClientset(), rawClient: k8sfake.NewSimpleClientset(other)}
	response := addonRequest(t, gateway, http.MethodPost, "", map[string]interface{}{"addonName": "fluent-bit", "values": map[string]string{"endpoint": "e", "logstore": "l", "clusterId": "c"}})
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "new version") {
		t.Fatalf("%d %s", response.Code, response.Body)
	}
	if list, _ := gateway.kubeClient.RlinfV1alpha1().Addons("cluster-a").List(context.Background(), metav1.ListOptions{}); len(list.Items) != 0 {
		t.Fatal("the Addon was written although its Package could not be published")
	}
}
