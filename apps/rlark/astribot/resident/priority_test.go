package resident

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers"
)

func TestPriorityClassIsCreatedAndAForeignOneIsLeft(t *testing.T) {
	local := fake.NewClientBuilder().WithScheme(controllers.MgmtScheme).Build()
	keeper := &PriorityClass{Local: local}
	// Twice: the second finds its own and changes nothing.
	for range 2 {
		if err := keeper.ensure(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	var class schedulingv1.PriorityClass
	if err := local.Get(context.Background(), client.ObjectKey{Name: PriorityClassName}, &class); err != nil {
		t.Fatal(err)
	}
	if class.Value != 1000000 || *class.PreemptionPolicy != corev1.PreemptNever || class.GlobalDefault {
		t.Fatalf("class=%#v", class)
	}

	// A class of the name with another value cannot be changed (the value is immutable) and is not replaced.
	foreign := &schedulingv1.PriorityClass{ObjectMeta: metav1.ObjectMeta{Name: PriorityClassName}, Value: 10}
	local = fake.NewClientBuilder().WithScheme(controllers.MgmtScheme).WithObjects(foreign).Build()
	if err := (&PriorityClass{Local: local}).ensure(context.Background()); err == nil {
		t.Fatal("a foreign PriorityClass of another value was accepted")
	}
	if err := local.Get(context.Background(), client.ObjectKey{Name: PriorityClassName}, &class); err != nil || class.Value != 10 {
		t.Fatalf("foreign class=%#v err=%v", class, err)
	}
}
