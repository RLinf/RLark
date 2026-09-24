# SSH Dialer Conformance Tests

These long-running integration scenarios exercise the SSH dialer with a real
SSH server and TCP echo backend. They are intentionally excluded from regular
`go test` runs.

```bash
go run ./pkg/network/sshdialer/tests/conformance
```

The suite covers:

- concurrent multiplexed channels with sustained traffic;
- forced SSH transport interruption and automatic reconnection;
- SSH service shutdown, same-address restart, and recovery without restarting
  the dialer manager.

Use flags to increase or reduce the workload:

```bash
go run ./pkg/network/sshdialer/tests/conformance \
  -connections 64 \
  -payload-size 8388608 \
  -rounds 5 \
  -duration 1m \
  -timeout 60s
```
