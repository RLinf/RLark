package host

import "slices"

// MergeStatusIntents returns the highest-priority compatible intent for the
// current Task incarnation. Equal-priority intents are merged in input order,
// with later non-empty scalar values replacing earlier values and conditions
// appended for typed upsert/removal by the Status writer.
func MergeStatusIntents(taskUID string, generation int64, intents ...StatusIntent) *StatusIntent {
	var merged *StatusIntent
	for _, intent := range intents {
		if intent.TaskUID != "" && intent.TaskUID != taskUID {
			continue
		}
		if intent.Generation != 0 && intent.Generation != generation {
			continue
		}
		if merged == nil || intent.Priority > merged.Priority {
			copy := intent
			copy.ObservedNodes = slices.Clone(intent.ObservedNodes)
			copy.Conditions = slices.Clone(intent.Conditions)
			merged = &copy
			continue
		}
		if intent.Priority < merged.Priority {
			continue
		}
		if intent.Phase != "" {
			merged.Phase = intent.Phase
		}
		if intent.MessageSet {
			merged.MessageSet = true
			merged.Message = intent.Message
		}
		if intent.ObservedNodesSet {
			merged.ObservedNodesSet = true
			merged.ObservedNodes = slices.Clone(intent.ObservedNodes)
		}
		merged.Conditions = append(merged.Conditions, intent.Conditions...)
		if intent.ReasonCode != "" {
			merged.ReasonCode = intent.ReasonCode
		}
	}
	if merged != nil {
		if merged.TaskUID == "" {
			merged.TaskUID = taskUID
		}
		if merged.Generation == 0 {
			merged.Generation = generation
		}
	}
	return merged
}
