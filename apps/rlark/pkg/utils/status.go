package utils

// StatusEntry is a status entry carrying a Phase and a Message. The type
// parameter P is the entry's Phase enum type, so reusing this helper across
// resources does not erase the Phase type and does not allow one resource's
// phase value to be written into another resource's entry.
type StatusEntry[P ~string] interface {
	GetPhase() P
	SetPhase(P)
	SetMessage(string)
}

// SyncStatusEntry syncs the status entry. It reports whether the entry changed.
func SyncStatusEntry[P ~string](entry StatusEntry[P], newPhase P, newMessage string) bool {
	if newPhase == "" || newPhase == entry.GetPhase() {
		return false
	}
	entry.SetPhase(newPhase)
	entry.SetMessage(newMessage)
	return true
}
