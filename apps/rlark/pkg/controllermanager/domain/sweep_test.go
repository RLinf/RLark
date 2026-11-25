package domain

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/controller"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/sweep"
)

func TestOrphanPeerRuleIsUsableBySweeper(t *testing.T) {
	if _, err := sweep.New(nil, nil, controller.Shell{}, sweep.DefaultOptions(), OrphanPeerRule()); err != nil {
		t.Fatalf("sweeper rejected the DomainPeer rule: %v", err)
	}
}

// The rule has to resolve the same candidate parents as the field index that
// the controller watches with, otherwise the sweeper and the controller would
// disagree about which Domain owns a peer.
func TestOrphanPeerRuleMatchesDomainIndex(t *testing.T) {
	cases := map[string]client.Object{
		"name only": &rlarkv1alpha1.DomainPeer{ObjectMeta: metav1.ObjectMeta{
			Name:      "d1",
			Namespace: "rlark-system",
		}},
		"renamed label": &rlarkv1alpha1.DomainPeer{ObjectMeta: metav1.ObjectMeta{
			Name:      "d1",
			Namespace: "rlark-system",
			Labels:    map[string]string{domainPeerLabel: "d2"},
		}},
	}

	rule := OrphanPeerRule()
	for name, peer := range cases {
		t.Run(name, func(t *testing.T) {
			got := rule.ParentNames(peer)
			want := indexDomainPeerByDomain(peer)
			if len(got) != len(want) {
				t.Fatalf("parent names = %v, want %v", got, want)
			}
			for i := range got {
				if got[i] != want[i] {
					t.Fatalf("parent names = %v, want %v", got, want)
				}
			}
		})
	}
}
