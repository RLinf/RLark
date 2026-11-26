package agent

import (
	"context"
	"errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rlinf/rlark/api/kubeclients/clientset/versioned"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type testStartupRole struct {
	build func(context.Context) error
	run   func(context.Context) error
}

func TestFirstRegistrationPrecedesManagementAccessAndWorkers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var registered atomic.Bool
	var reads atomic.Int32
	var started atomic.Bool
	tunnelJoined := make(chan struct{})
	a := preflightAgent(t, func(w http.ResponseWriter, r *http.Request) {
		if !registered.Load() {
			t.Error("management read before registration")
		}
		if reads.Add(1) == 1 {
			http.Error(w, "RBAC not propagated", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	})
	err := runWithRegistration(ctx, func(ctx context.Context, ready func()) error {
		defer close(tunnelJoined)
		registered.Store(true)
		ready()
		ready() // Reconnect notifications are idempotent.
		<-ctx.Done()
		return nil
	}, func(ctx context.Context) ([]func(context.Context) error, error) {
		if err := a.managementPreflight(ctx); err != nil {
			return nil, err
		}
		return []func(context.Context) error{func(ctx context.Context) error { started.Store(true); cancel(); <-ctx.Done(); return nil }}, nil
	})
	if err != nil || !started.Load() || reads.Load() < 2 {
		t.Fatalf("first registration did not recover: %v", err)
	}
	select {
	case <-tunnelJoined:
	default:
		t.Fatal("tunnel not joined")
	}
}

func TestRegistrationFailureAndPreparationFailureJoinTunnel(t *testing.T) {
	for _, stage := range []string{"registration", "preflight", "build", "cancel"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			fatal := errors.New("startup failed")
			joined := make(chan struct{})
			prepared := false
			err := runWithRegistration(ctx, func(ctx context.Context, ready func()) error {
				defer close(joined)
				if stage == "registration" {
					return fatal
				}
				if stage != "cancel" {
					ready()
				}
				<-ctx.Done()
				return nil
			}, func(ctx context.Context) ([]func(context.Context) error, error) {
				prepared = true
				if stage == "preflight" {
					return nil, waitManagementAccess(ctx, func(context.Context) error {
						return apierrors.NewForbidden(schema.GroupResource{Resource: "tasks"}, "", fatal)
					})
				}
				return nil, fatal
			})
			if err == nil {
				t.Fatal("startup failure accepted")
			}
			if (stage == "registration" || stage == "cancel") && prepared {
				t.Fatal("prepared without registration")
			}
			select {
			case <-joined:
			default:
				t.Fatal("failed startup leaked tunnel")
			}
		})
	}
}

func TestManagementAccessNeverSkipsDeniedPermission(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	denied := apierrors.NewForbidden(schema.GroupResource{Resource: "tasks"}, "", errors.New("denied"))
	err := waitManagementAccess(ctx, func(context.Context) error { return denied })
	if !errors.Is(err, context.DeadlineExceeded) || !apierrors.IsForbidden(err) {
		t.Fatalf("permission/deadline chain lost: %v", err)
	}
}

func (r testStartupRole) Build(ctx context.Context) error { return r.build(ctx) }
func (r testStartupRole) Run(ctx context.Context) error   { return r.run(ctx) }

func TestBuildRolesNoStartOnBuildFailure(t *testing.T) {
	fatal := errors.New("invalid node assembly")
	started := false
	cluster := testStartupRole{build: func(context.Context) error { return nil }, run: func(context.Context) error { started = true; return nil }}
	node := testStartupRole{build: func(context.Context) error { return fatal }, run: cluster.run}
	config := DefaultConfig()
	config.NodeStartup = "embedded"
	runs, err := buildRoles(context.Background(), config, cluster, node)
	if !errors.Is(err, fatal) || len(runs) != 0 || started {
		t.Fatalf("partial assembly started: %v", err)
	}
}

func TestBuiltRoleCompositionRunsNodeWithoutClusterLeadership(t *testing.T) {
	for _, test := range []struct {
		mode, startup         string
		wantCluster, wantNode bool
	}{
		{"cluster", "external", true, false}, {"cluster", "embedded", true, true}, {"node", "external", false, true},
	} {
		t.Run(test.mode+"/"+test.startup, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			clusterBuilt, nodeBuilt := false, false
			nodeStarted := make(chan struct{})
			cluster := testStartupRole{build: func(context.Context) error { clusterBuilt = true; return nil }, run: func(ctx context.Context) error {
				// This cluster candidate never becomes leader; node must still run.
				if test.wantNode {
					select {
					case <-nodeStarted:
						cancel()
					case <-ctx.Done():
						return ctx.Err()
					}
				} else {
					cancel()
				}
				<-ctx.Done()
				return nil
			}}
			node := testStartupRole{build: func(context.Context) error { nodeBuilt = true; return nil }, run: func(ctx context.Context) error {
				close(nodeStarted)
				if !test.wantCluster {
					cancel()
				}
				<-ctx.Done()
				return nil
			}}
			config := DefaultConfig()
			config.Mode, config.NodeStartup = test.mode, test.startup
			runs, err := buildRoles(ctx, config, cluster, node)
			if err != nil {
				t.Fatal(err)
			}
			if clusterBuilt != test.wantCluster || nodeBuilt != test.wantNode {
				t.Fatal("incorrect role construction")
			}
			if err := supervise(ctx, runs...); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRolesRequireBuildBeforeStart(t *testing.T) {
	if err := (&clusterAgent{}).Run(context.Background()); err == nil {
		t.Fatal("unbuilt cluster ran")
	}
	if err := (&nodeAgent{}).Run(context.Background()); err == nil {
		t.Fatal("unbuilt node ran")
	}
}

func TestRoleTunnelIdentity(t *testing.T) {
	config := DefaultConfig()
	config.NodeName = "configured-node"
	if got := tunnelRoles(config); !reflect.DeepEqual(got, []string{""}) {
		t.Fatal(got)
	}
	config.NodeStartup = "embedded"
	if got := tunnelRoles(config); !reflect.DeepEqual(got, []string{"", "node-agent:configured-node"}) {
		t.Fatal(got)
	}
	config.Mode, config.NodeStartup = "node", "external"
	if got := tunnelRoles(config); !reflect.DeepEqual(got, []string{"node-agent:configured-node"}) {
		t.Fatal(got)
	}
}

func TestBuildRolesHonorsCancellationAndRequiredComponents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	role := testStartupRole{build: func(context.Context) error { cancel(); return nil }}
	config := DefaultConfig()
	if runs, err := buildRoles(ctx, config, role, nil); !errors.Is(err, context.Canceled) || len(runs) != 0 {
		t.Fatalf("cancelled build published runs: %v", err)
	}
	if _, err := buildRoles(context.Background(), config, nil, nil); err == nil {
		t.Fatal("missing cluster allowed")
	}
	config.Mode = "node"
	if _, err := buildRoles(context.Background(), config, nil, nil); err == nil {
		t.Fatal("missing node allowed")
	}
}

func preflightAgent(t *testing.T, handler http.HandlerFunc) *Agent {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	config := &rest.Config{Host: server.URL}
	local, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	management, err := versioned.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	a := NewAgent(DefaultConfig())
	a.localKubeClient, a.managementClient = local, management
	a.config.ClientConfig.ServerNamespace = "scope-a"
	a.config.NodeName = "node-a"
	return a
}

func TestPreflightReadOnlyRoleDependencies(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	a := preflightAgent(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		if r.Method != http.MethodGet {
			t.Errorf("preflight effect: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version":
			_, _ = w.Write([]byte(`{"gitVersion":"v1.31.0"}`))
		case strings.HasSuffix(r.URL.Path, "/nodes/node-a"):
			_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"Node","metadata":{"name":"node-a"}}`))
		default:
			_, _ = w.Write([]byte(`{"items":[]}`))
		}
	})
	a.config.NodeStartup = "embedded"
	if err := a.preflight(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, suffix := range []string{"/version", "/nodes/node-a", "/namespaces/scope-a/domainpeers", "/namespaces/scope-a/pods", "/namespaces/scope-a/tasks", "/namespaces/scope-a/clusterupgradejobs"} {
		found := false
		for _, path := range paths {
			if strings.HasSuffix(path, suffix) {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing dependency check %s: %v", suffix, paths)
		}
	}
}

func TestUpgradePreflightPermissionFailureRecoveryAndRoleScope(t *testing.T) {
	var healthy atomic.Bool
	var upgradeReads atomic.Int32
	a := preflightAgent(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/clusterupgradejobs") {
			upgradeReads.Add(1)
			if !healthy.Load() {
				http.Error(w, "upgrade permission denied", http.StatusForbidden)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := a.managementPreflight(ctx); !errors.Is(err, context.DeadlineExceeded) || !apierrors.IsForbidden(err) {
		t.Fatalf("upgrade read denial did not fail closed: %v", err)
	}
	healthy.Store(true)
	if err := a.managementPreflight(context.Background()); err != nil {
		t.Fatalf("upgrade permissions did not recover: %v", err)
	}
	if upgradeReads.Load() < 2 {
		t.Fatal("cluster did not check upgrade dependency")
	}
	upgradeReads.Store(0)
	a.config.Mode = "node"
	healthy.Store(false)
	if err := a.managementPreflight(context.Background()); err != nil {
		t.Fatalf("node-only startup incorrectly depends on upgrades: %v", err)
	}
	if upgradeReads.Load() != 0 {
		t.Fatal("node-only startup read ClusterUpgradeJobs")
	}
}

func TestPreflightFailureCancellationAndRecovery(t *testing.T) {
	for _, failure := range []string{"denied", "unavailable", "cancelled"} {
		t.Run(failure, func(t *testing.T) {
			var mu sync.Mutex
			healthy := false
			a := preflightAgent(t, func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				ok := healthy
				mu.Unlock()
				if !ok {
					if failure == "cancelled" {
						<-r.Context().Done()
						return
					}
					code := http.StatusForbidden
					if failure == "unavailable" {
						code = http.StatusServiceUnavailable
					}
					http.Error(w, "secret-value", code)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"items":[]}`))
			})
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			err := a.preflight(ctx)
			if err == nil || strings.Contains(err.Error(), "secret-value") {
				t.Fatalf("preflight did not safely fail: %v", err)
			}
			if failure == "cancelled" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("deadline chain lost: %v", err)
			}
			mu.Lock()
			healthy = true
			mu.Unlock()
			if err := a.preflight(context.Background()); err != nil {
				t.Fatalf("recovery: %v", err)
			}
		})
	}
}
