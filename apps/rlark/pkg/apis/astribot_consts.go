package apis

const (
	// RemoteDialerReadyHeader is "true" on the tunnel of an agent that is ready (its first watches
	// have synced and it does its work). The Server keeps the heartbeat of such a tunnel only.
	RemoteDialerReadyHeader = "X-Remote-Dialer-Ready"
)
