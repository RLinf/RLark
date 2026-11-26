package server

import (
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestSSHAdmissionConfigDefaultsFlagsAndValidation(t *testing.T) {
	config := DefaultConfig()
	require.Equal(t, 64, config.SSHMaxPendingTargets)
	require.Equal(t, 5*time.Second, config.SSHAdmissionWaitTimeout)
	require.NoError(t, config.Validate())
	flags := pflag.NewFlagSet("server", pflag.ContinueOnError)
	config.SetupFlags(flags)
	require.NoError(t, flags.Parse([]string{"--ssh-max-pending-targets=128", "--ssh-admission-wait-timeout=2s"}))
	require.Equal(t, 128, config.SSHMaxPendingTargets)
	require.Equal(t, 2*time.Second, config.SSHAdmissionWaitTimeout)
	require.NoError(t, config.Validate())
	for _, invalid := range []int{0, -1} {
		config := DefaultConfig()
		config.SSHMaxPendingTargets = invalid
		require.ErrorContains(t, config.Validate(), "SSH max pending targets")
	}
	for _, invalid := range []time.Duration{0, -time.Second} {
		config := DefaultConfig()
		config.SSHAdmissionWaitTimeout = invalid
		require.ErrorContains(t, config.Validate(), "SSH admission wait timeout")
	}
}

func TestClientConfigBuildRestConfigSetsRateLimits(t *testing.T) {
	config := ClientConfig{
		ServerAddress:         "https://rlark.example.com",
		ServerNamespace:       "data-cluster",
		InsecureSkipTLSVerify: true,
	}

	restConfig, err := config.BuildRestConfig()
	if err != nil {
		t.Fatal(err)
	}
	if restConfig.QPS != defaultClientQPS {
		t.Fatalf("QPS = %v, want %v", restConfig.QPS, defaultClientQPS)
	}
	if restConfig.Burst != defaultClientBurst {
		t.Fatalf("Burst = %d, want %d", restConfig.Burst, defaultClientBurst)
	}
}
