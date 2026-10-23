package observability

import "testing"

func TestEnvConfigSourceParsesDynamicSettings(t *testing.T) {
	t.Setenv(EnvPayloads, "plan, operations,manifests")
	t.Setenv(EnvMaxBytes, "4096")
	config, err := (EnvConfigSource{}).Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if config.MaxEventBytes != 4096 || !config.Enabled(PayloadPlan) || !config.Enabled(PayloadOperations) || !config.Enabled(PayloadManifests) || config.Enabled(PayloadTaskSnapshot) {
		t.Fatalf("config=%#v", config)
	}
}

func TestEnvConfigSourceRejectsUnknownPayload(t *testing.T) {
	t.Setenv(EnvPayloads, "plan,credentials")
	if _, err := (EnvConfigSource{}).Snapshot(); err == nil {
		t.Fatal("expected invalid payload error")
	}
}

func TestEnvConfigSourceRejectsInvalidMaxBytes(t *testing.T) {
	for _, value := range []string{"not-a-number", "0", "-1"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv(EnvMaxBytes, value)
			if _, err := (EnvConfigSource{}).Snapshot(); err == nil {
				t.Fatalf("expected invalid max bytes %q to be rejected", value)
			}
		})
	}
}
