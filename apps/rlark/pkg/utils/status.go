package utils

// StatusEntry represents a status entry.
type StatusEntry interface {
	GetPhase() string
	SetPhase(string)
	SetMessage(string)
}

// SyncStatusEntry syncs the status entry.
func SyncStatusEntry(entry StatusEntry, newPhase, newMessage string) bool {
	if newPhase == "" || newPhase == entry.GetPhase() {
		return false
	}
	entry.SetPhase(newPhase)
	entry.SetMessage(newMessage)
	return true
}
