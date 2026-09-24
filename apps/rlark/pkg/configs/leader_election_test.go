package configs

import (
	"testing"
	"time"

	"github.com/spf13/pflag"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/leaderelection"
)

func TestLeaderElectionConfigSetupFlags(t *testing.T) {
	cfg := LeaderElectionConfig{}
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg.SetupFlags(fs, "")
	if err := fs.Parse([]string{
		"--leader-election=true",
		"--leader-election-key=rlark-system/test",
		"--leader-election-id=instance-1",
	}); err != nil {
		t.Fatal(err)
	}
	if !cfg.Enabled || cfg.Key != "rlark-system/test" || cfg.Identity != "instance-1" {
		t.Fatalf("leader election config = %+v", cfg)
	}
}

func TestLeaderElectionConfigSetupFlagsPrefix(t *testing.T) {
	cfg := LeaderElectionConfig{}
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg.SetupFlags(fs, "controller")
	if fs.Lookup("controller-leader-election") == nil || fs.Lookup("controller-leader-election-key") == nil || fs.Lookup("controller-leader-election-id") == nil {
		t.Fatal("SetupFlags did not register prefixed flags")
	}
}

func TestDefaultLeaderElectionConfig(t *testing.T) {
	cfg := DefaultLeaderElectionConfig()
	if cfg.LeaseDuration != 30*time.Second || cfg.RenewDeadline != 10*time.Second || cfg.RetryPeriod != 5*time.Second {
		t.Fatalf("default leader election config = %+v", cfg)
	}
}

func TestLeaderElectionConfigNamespaceAndName(t *testing.T) {
	tests := []struct {
		key           string
		wantNamespace string
		wantName      string
		wantErr       bool
	}{
		{key: "lock", wantNamespace: "default", wantName: "lock"},
		{key: "rlark-system/lock", wantNamespace: "rlark-system", wantName: "lock"},
		{key: "", wantErr: true},
		{key: "/lock", wantErr: true},
		{key: "namespace/", wantErr: true},
		{key: "a/b/c", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			namespace, name, err := (LeaderElectionConfig{Key: tt.key}).NamespaceAndName("default")
			if tt.wantErr {
				if err == nil {
					t.Fatal("NamespaceAndName() expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("NamespaceAndName() error = %v", err)
			}
			if namespace != tt.wantNamespace || name != tt.wantName {
				t.Fatalf("NamespaceAndName() = %s/%s, want %s/%s", namespace, name, tt.wantNamespace, tt.wantName)
			}
		})
	}
}

func TestLeaderElectionConfigBuild(t *testing.T) {
	cfg := LeaderElectionConfig{
		Key:           "rlark-system/test-lock",
		Identity:      "test-instance",
		LeaseDuration: 30 * time.Second,
		RenewDeadline: 10 * time.Second,
		RetryPeriod:   5 * time.Second,
	}
	built, err := cfg.Build(fake.NewSimpleClientset(), "default", leaderelection.LeaderCallbacks{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if built.Lock.Describe() != "rlark-system/test-lock" {
		t.Fatalf("lock = %q, want rlark-system/test-lock", built.Lock.Describe())
	}
	if built.LeaseDuration != cfg.LeaseDuration || built.RenewDeadline != cfg.RenewDeadline || built.RetryPeriod != cfg.RetryPeriod {
		t.Fatalf("unexpected timings: %#v", built)
	}
}

func TestLeaderElectionConfigValidateTimings(t *testing.T) {
	cfg := LeaderElectionConfig{
		Key:           "lock",
		Identity:      "instance",
		LeaseDuration: 5 * time.Second,
		RenewDeadline: 5 * time.Second,
		RetryPeriod:   time.Second,
	}
	if err := cfg.Validate("default"); err == nil {
		t.Fatal("Validate() expected lease duration error")
	}
}
