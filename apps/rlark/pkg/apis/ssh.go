package apis

// SSHDirectPayload represents a payload.
type SSHDirectPayload struct {
	Host              string
	Port              uint32
	OriginatorAddress string
	OriginatorPort    uint32
}
