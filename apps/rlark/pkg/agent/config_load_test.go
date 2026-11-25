package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/spf13/pflag"
)

func startupFlags(t *testing.T, args ...string) *pflag.FlagSet {
	t.Helper()
	for _, env := range []string{"KUBECONFIG", "NODE_NAME", "RLARK_TENSORBOARD_IMAGE", "KUBERNETES_SUPPORT_SIDECAR_FEATURE"} {
		t.Setenv(env, "")
	}
	c := DefaultConfig()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	c.SetupFlags(fs)
	base := []string{"--client-cert=cert.pem", "--client-key=key.pem", "--image=trusted/tools:v1"}
	if err := fs.Parse(append(base, args...)); err != nil {
		t.Fatal(err)
	}
	return fs
}

func startupFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agent.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestStartupConfigPrecedencePathsAndZero(t *testing.T) {
	fs := startupFlags(t, "--image-pull-enabled=false", "--task-runtime-resync-period=0", "--client-key=cli-key.pem")
	t.Setenv("RLARK_TENSORBOARD_IMAGE", "environment/tensorboard:v1")
	t.Setenv("NODE_NAME", "env-node")
	path := startupFile(t, "version: v1\noptions:\n  node-name: file-node\n  ca-cert: file-cert.pem\n  tensorboard-image: file/tensorboard:v2\n  image-pull-enabled: true\n  task-runtime-resync-period: 2m\n  agent-type: KuBeRnEtEs\n")
	effective, err := LoadStartupConfig(path, fs)
	if err != nil {
		t.Fatal(err)
	}
	c := effective.Config
	if c.ImagePullEnabled || c.TaskRuntimeResyncPeriod != 0 || c.NodeName != "file-node" || c.Execution.TensorBoardImage != "file/tensorboard:v2" || c.AgentType != "kubernetes" {
		t.Fatalf("bad overlay: %+v", c)
	}
	if c.ClientConfig.CAPath != filepath.Join(filepath.Dir(path), "file-cert.pem") || !filepath.IsAbs(c.ClientConfig.ClientKeyPath) {
		t.Fatal("path bases not respected")
	}
	if effective.Sources["ca-cert"] != "file" || effective.Sources["image-pull-enabled"] != "cli" {
		t.Fatal("wrong diagnostic sources")
	}
	if c.RuntimeKind() != rlarkv1alpha1.AgentTypeKubernetes {
		t.Fatal("canonical input changed API identity")
	}
}

func TestStartupConfigEnvironmentAndIdentity(t *testing.T) {
	for _, kind := range []string{"Kubernetes", "kubernetes", "KUBERNETES"} {
		fs := startupFlags(t, "--agent-type="+kind)
		t.Setenv("RLARK_TENSORBOARD_IMAGE", "environment/tensorboard:v1")
		t.Setenv("KUBERNETES_SUPPORT_SIDECAR_FEATURE", "false")
		effective, err := LoadStartupConfig("", fs)
		if err != nil {
			t.Fatal(err)
		}
		if effective.Config.Execution.RestartableInitContainers || effective.Config.Execution.TensorBoardImage != "environment/tensorboard:v1" || effective.Sources["tensorboard-image"] != "environment:RLARK_TENSORBOARD_IMAGE" {
			t.Fatal("environment compatibility lost")
		}
		if effective.Config.RuntimeKind() != rlarkv1alpha1.AgentTypeKubernetes {
			t.Fatal("API/claimant kind changed")
		}
	}
}

func TestStartupConfigRejectsInvalidInputs(t *testing.T) {
	for _, content := range []string{
		"version: v2\n", "version: v1\nunknown: true\n", "version: v1\nversion: v1\n",
		"version: v1\noptions:\n  mode: cluster\n  mode: node\n",
		"version: v1\noptions:\n  unknown: secret-value\n",
		"version: v1\noptions:\n  image-pull-enabled: 'false'\n",
		"version: v1\noptions:\n  kube-qps: .nan\n",
		"version: v1\noptions:\n  pod-orphan-sweep-page-size: 0\n",
		"version: v1\noptions:\n  image: null\n",
		"version: v1\noptions:\n  mode: [cluster]\n", "version: v1\n---\nversion: v1\n",
		"version: v1\noptions:\n  image: &secret secret-value\n  client-key: *secret\n",
		"version: v1\noptions: null\n",
		"version: v1\noptions:\n  <<: {mode: cluster}\n",
	} {
		t.Run(content, func(t *testing.T) {
			_, err := LoadStartupConfig(startupFile(t, content), startupFlags(t))
			if err == nil {
				t.Fatal("accepted invalid input")
			}
			if strings.Contains(err.Error(), "secret-value") {
				t.Fatal("error leaked secret")
			}
		})
	}
}

func TestStartupConfigRoleMatrix(t *testing.T) {
	for _, test := range []struct {
		args        []string
		valid, node bool
	}{
		{[]string{}, true, false},
		{[]string{"--node-startup=embedded", "--node-name=node-a"}, true, true},
		{[]string{"--mode=node", "--node-name=node-a", "--image="}, true, true},
		{[]string{"--mode=both"}, false, false},
		{[]string{"--mode=node", "--node-name=node-a", "--node-startup=external"}, false, false},
		{[]string{"--node-startup=managed"}, false, false},
		{[]string{"--agent-type=Docker"}, false, false},
		{[]string{"--agent-type=raw"}, false, false},
		{[]string{"--agent-type=unknown"}, false, false},
		{[]string{"--node-startup=embedded"}, false, false},
		{[]string{"--node-startup=embedded", "--node-name=invalid:node"}, false, false},
		{[]string{"--mode=node", "--node-name=node-a", "--tensorboard-image=x"}, false, false},
		{[]string{"--embedded-node-metrics-bind-address=:8082"}, false, false},
		{[]string{"--node-startup=embedded", "--node-name=node-a", "--embedded-node-metrics-bind-address=0.0.0.0:8081"}, false, false},
		{[]string{"--metrics-bind-address=:invalid"}, false, false},
		{[]string{"--in-cluster", "--kubeconfig=x"}, false, false},
		{[]string{"--leader-election", "--leader-election-key=invalid/key/extra"}, false, false},
		{[]string{"--server-address=https://user:secret-value@example.com"}, false, false},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			effective, err := LoadStartupConfig("", startupFlags(t, test.args...))
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v err=%v", test.valid, err)
			}
			if err == nil && effective.Config.HasNode() != test.node {
				t.Fatal("wrong role composition")
			}
		})
	}
}

func TestStartupDiagnosticsRedactArbitraryStrings(t *testing.T) {
	effective, err := LoadStartupConfig("", startupFlags(t, "--client-key=secret-value", "--image=secret-value", "--rlark-server-ssh-host-key=secret-value"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(struct{ Options, Sources map[string]string }{effective.Options, effective.Sources})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret-value") {
		t.Fatal("diagnostics leaked arbitrary string")
	}
}

func TestInvalidStartupFailsBeforeEffects(t *testing.T) {
	a := NewAgent(DefaultConfig())
	if err := a.Run(context.Background()); err == nil {
		t.Fatal("invalid config ran")
	}
	if a.serverClient != nil || a.managementClient != nil || a.localListener != nil {
		t.Fatal("validation occurred after effects")
	}
}

func TestSupervisorCancelsAndJoinsIndependentRoles(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	nodeStarted, nodeJoined := make(chan struct{}), make(chan struct{})
	fatal := errors.New("cluster startup failed")
	err := supervise(ctx,
		func(ctx context.Context) error {
			select {
			case <-nodeStarted:
				return fatal
			case <-ctx.Done():
				return ctx.Err()
			}
		},
		func(ctx context.Context) error { close(nodeStarted); <-ctx.Done(); close(nodeJoined); return nil },
	)
	if !errors.Is(err, fatal) {
		t.Fatalf("fatal error lost: %v", err)
	}
	select {
	case <-nodeJoined:
	default:
		t.Fatal("returned without joining node")
	}
}

func TestSupervisorNormalCancellationAndUnexpectedExit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := supervise(ctx, func(ctx context.Context) error { <-ctx.Done(); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := supervise(context.Background(), func(context.Context) error { return nil }); err == nil {
		t.Fatal("unexpected component exit accepted")
	}
}

func TestHTTPBindFailureJoinsShutdown(t *testing.T) {
	fatal := errors.New("bind failed")
	if err := serveHTTP(context.Background(), &http.Server{}, func() error { return fatal }); !errors.Is(err, fatal) {
		t.Fatal(err)
	}
}

func TestDocumentedStartupExamples(t *testing.T) {
	for _, page := range []string{"reference/configuration.md", "zh/reference/configuration.md"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "docs", page))
		if err != nil {
			t.Fatal(err)
		}
		_, example, found := strings.Cut(string(data), "```yaml\nversion: v1\n")
		if !found {
			t.Fatalf("%s has no startup example", page)
		}
		example, _, found = strings.Cut(example, "```")
		if !found {
			t.Fatal("unterminated startup example")
		}
		startupFlags(t) // Reset compatibility environment, but do not overlay CLI.
		effective, err := LoadStartupConfig(startupFile(t, "version: v1\n"+example), nil)
		if err != nil {
			t.Fatalf("%s startup example: %v", page, err)
		}
		if effective.Config.Execution.RestartableInitContainers || effective.Config.TaskRuntimeResyncPeriod != 0 {
			t.Fatal("documented false/zero did not apply")
		}
	}
}
