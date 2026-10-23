package persistence

import (
	"context"
	"slices"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/rlinf/rlark/apps/rlark/pkg/db"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
)

// storedJob builds the Secret the backend writes for one archived Job.
func storedJob(name, tenantID string) *corev1.Secret {
	labels := ""
	if tenantID != "" {
		labels = `"labels":{"` + tenant.IDLabel + `":"` + tenantID + `"},`
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName("jobs", "", name),
			Namespace: "rlark-system",
			Labels:    map[string]string{NameLabel: name, KindLabel: "jobs"},
		},
		Data: map[string][]byte{"raw": []byte(`{
			"apiVersion":"rlinf.io/v1alpha1","kind":"Job",
			"metadata":{` + labels + `"name":"` + name + `"},
			"spec":{"domain":"dom"}
		}`)},
	}
}

func listedNames(t *testing.T, result *ListResult) []string {
	t.Helper()
	names := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		metadata, ok := item["metadata"].(map[string]any)
		if !ok {
			t.Fatalf("listed item has no metadata: %#v", item)
		}
		name, ok := metadata["name"].(string)
		if !ok {
			t.Fatalf("listed item has no name: %#v", item)
		}
		names = append(names, name)
	}
	return names
}

// A caller restricted to its own tenants expresses that restriction as a label
// selector, so a backend that ignores the selectors hands every stored object to
// every caller. The default tenant is part of the assertion because an object
// stored without the tenant label must stay readable by the tenant it is
// attributed to, exactly as on the database backed path.
func TestSecretBackendListRestrictsListingToTheSelectedTenants(t *testing.T) {
	client := k8sfake.NewSimpleClientset(
		storedJob("mine", "team-a"),
		storedJob("theirs", "team-b"),
		storedJob("unlabeled", ""),
	)
	backend := NewSecretBackendFromClientGo(client, "rlark-system")

	result, err := backend.List(context.Background(), "jobs", db.ListOptions{
		LabelSelector: []db.LabelSelector{{
			Key: tenant.IDLabel, Op: "in", Value: "team-a," + tenant.DefaultID,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	names := listedNames(t, result)
	slices.Sort(names)
	if !slices.Equal(names, []string{"mine", "unlabeled"}) {
		t.Fatalf("listed names = %v, want the team-a job and the unlabeled job", names)
	}
	if result.Total != 2 {
		t.Fatalf("total = %d, want 2: the reported total must describe the objects the caller may see", result.Total)
	}
}

// The pagination window and the reported total are what a client uses to decide
// whether more pages exist, so both must be computed after the selectors have
// been applied rather than over the whole store.
func TestSecretBackendListPaginatesTheSelectedTenantOnly(t *testing.T) {
	client := k8sfake.NewSimpleClientset(
		storedJob("mine-a", "team-a"),
		storedJob("mine-b", "team-a"),
		storedJob("theirs", "team-b"),
	)
	backend := NewSecretBackendFromClientGo(client, "rlark-system")

	result, err := backend.List(context.Background(), "jobs", db.ListOptions{
		LabelSelector: []db.LabelSelector{{Key: tenant.IDLabel, Value: "team-a"}},
		Limit:         1,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Items) != 1 {
		t.Fatalf("items = %v, want one page entry", listedNames(t, result))
	}
	if result.Total != 2 {
		t.Fatalf("total = %d, want 2: the total must count only the two jobs of team-a", result.Total)
	}
}
