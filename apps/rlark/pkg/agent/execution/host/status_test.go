package host

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMergeStatusIntentsUsesPriorityAndFencing(t *testing.T) {
	result := MergeStatusIntents("uid", 3,
		StatusIntent{TaskUID: "old", Generation: 3, Phase: "Failed", Priority: StatusPriorityCleanup},
		StatusIntent{TaskUID: "uid", Generation: 3, Phase: "Running", Priority: StatusPriorityWorkload},
		StatusIntent{TaskUID: "uid", Generation: 3, Phase: "Pending", MessageSet: true, Message: "cleanup", Priority: StatusPriorityCleanup, Conditions: []ConditionMutation{{Condition: metav1.Condition{Type: "Progressing"}}}},
		StatusIntent{TaskUID: "uid", Generation: 3, MessageSet: true, Message: "ignored", Priority: StatusPriorityEnvironment},
	)
	if result == nil || result.Phase != "Pending" || result.Message != "cleanup" || len(result.Conditions) != 1 {
		t.Fatalf("result=%#v", result)
	}
}

func TestMergeStatusIntentsMergesEqualPriority(t *testing.T) {
	result := MergeStatusIntents("uid", 3,
		StatusIntent{TaskUID: "uid", Generation: 3, Phase: "Running", Priority: StatusPriorityWorkload},
		StatusIntent{TaskUID: "uid", Generation: 3, MessageSet: true, Message: "ready", ObservedNodesSet: true, ObservedNodes: []string{"node"}, Priority: StatusPriorityWorkload},
	)
	if result == nil || result.Phase != "Running" || result.Message != "ready" || len(result.ObservedNodes) != 1 {
		t.Fatalf("result=%#v", result)
	}
}

func TestMergeStatusIntentsPreservesExplicitEmptyValues(t *testing.T) {
	result := MergeStatusIntents("uid", 3,
		StatusIntent{Priority: StatusPriorityWorkload, MessageSet: true, Message: "ready", ObservedNodesSet: true, ObservedNodes: []string{"old"}},
		StatusIntent{Priority: StatusPriorityWorkload, MessageSet: true, Message: "", ObservedNodesSet: true, ObservedNodes: nil},
	)
	if result == nil || result.TaskUID != "uid" || result.Generation != 3 || !result.MessageSet || result.Message != "" || !result.ObservedNodesSet || result.ObservedNodes != nil {
		t.Fatalf("result=%#v", result)
	}
}
