// Package remotedialer provides bidirectional network dialing over persistent
// WebSocket connections.
//
// A client establishes a Session with a Server. The server can then use a
// Dialer associated with that client to open logical connections through the
// session. Sessions use smux to multiplex those logical connections over one
// WebSocket transport. Servers may also connect as peers and advertise clients
// reachable through another server.
package remotedialer
