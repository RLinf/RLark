package framework

import "testing"

func TestNewClusterValidatesGenericIdentity(t *testing.T) {
	if _, err := NewCluster("", ClusterConfig{Kubeconfig: "unused"}, "example-e2e"); err == nil {
		t.Fatal("empty cluster name was accepted")
	}
	if _, err := NewCluster("arbitrary-cluster", ClusterConfig{}, "example-e2e"); err == nil {
		t.Fatal("empty kubeconfig was accepted")
	}
}

func TestNewClustersAcceptsEmptySet(t *testing.T) {
	clusters, err := NewClusters(map[string]ClusterConfig{}, "example-e2e")
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 0 {
		t.Fatalf("clusters = %#v", clusters)
	}
}
