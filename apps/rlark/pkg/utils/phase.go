package utils

// PhaseSummary summarizes results.
type PhaseSummary struct {
	AllSucceeded bool
	AllStopped   bool
	AnyFailed    bool
	AnyRunning   bool
	HasItems     bool
}

// SummarizePhases summarizes the phases.
func SummarizePhases(phases []string, succeeded, failed, running, stopped string) PhaseSummary {
	s := PhaseSummary{AllSucceeded: true, AllStopped: true}
	for _, p := range phases {
		s.HasItems = true
		switch p {
		case succeeded:
			s.AllStopped = false
		case failed:
			s.AnyFailed = true
			s.AllSucceeded = false
			s.AllStopped = false
		case running:
			s.AnyRunning = true
			s.AllSucceeded = false
			s.AllStopped = false
		case stopped:
			s.AllSucceeded = false
		default:
			s.AllSucceeded = false
			s.AllStopped = false
		}
	}
	return s
}
