package domain

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/sweep"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

// OrphanPeerRule reclaims DomainPeers whose Domain is gone.
//
// The Domain controller already deletes its peers behind a finalizer, so this
// rule only sees what that path missed: a peer written before the finalizer
// existed, a peer left behind when the finalizer was force-removed, or a peer
// whose delete event never arrived. The parent name candidates are the same
// ones the field index uses, and ownership is then proven against the same
// identity the Domain controller uses, so a peer the controller would
// recognise as its own is never treated as an orphan here.
func OrphanPeerRule() sweep.Rule {
	return sweep.Rule{
		Name:        "DomainPeer",
		Child:       func() client.Object { return &rlarkv1alpha1.DomainPeer{} },
		ChildList:   func() client.ObjectList { return &rlarkv1alpha1.DomainPeerList{} },
		Parent:      func() client.Object { return &rlarkv1alpha1.Domain{} },
		ParentNames: indexDomainPeerByDomain,
		ParentRef: func(parent client.Object) utils.ParentRef {
			domain, ok := parent.(*rlarkv1alpha1.Domain)
			if !ok {
				return utils.ParentRef{}
			}
			return domainParentRef(domain)
		},
	}
}
