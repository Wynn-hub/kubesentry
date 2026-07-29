package hublink

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
)

type stubDiscovery struct {
	info *version.Info
	err  error
}

func (s stubDiscovery) ServerVersion() (*version.Info, error) { return s.info, s.err }

type stubNodeLister struct {
	nodes []corev1.Node
	err   error
}

func (s stubNodeLister) ListNodes(context.Context) ([]corev1.Node, error) { return s.nodes, s.err }

func node(name string, ready corev1.ConditionStatus) corev1.Node {
	return corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{
			{Type: corev1.NodeReady, Status: ready},
		}},
	}
}

func TestClusterInventoryCollectsVersionAndNodeCounts(t *testing.T) {
	link := New(Config{
		ClusterName: "member-a",
		Discovery:   stubDiscovery{info: &version.Info{GitVersion: "v1.31.2"}},
		NodeLister: stubNodeLister{nodes: []corev1.Node{
			node("n1", corev1.ConditionTrue),
			node("n2", corev1.ConditionTrue),
			node("n3", corev1.ConditionFalse),
		}},
	})

	gotVersion, nodes, ready := link.clusterInventory(context.Background())
	if gotVersion != "v1.31.2" {
		t.Errorf("version = %q, want v1.31.2", gotVersion)
	}
	if nodes != 3 || ready != 2 {
		t.Errorf("nodes = %d/%d, want 2/3 ready", ready, nodes)
	}
}

// Inventory is best-effort: a member that cannot read one source must still
// report the others rather than failing the whole status report.
func TestClusterInventoryToleratesPartialFailures(t *testing.T) {
	t.Run("discovery fails", func(t *testing.T) {
		link := New(Config{
			Discovery:  stubDiscovery{err: errors.New("forbidden")},
			NodeLister: stubNodeLister{nodes: []corev1.Node{node("n1", corev1.ConditionTrue)}},
		})
		gotVersion, nodes, ready := link.clusterInventory(context.Background())
		if gotVersion != "" || nodes != 1 || ready != 1 {
			t.Errorf("inventory = (%q, %d, %d), want empty version with node counts", gotVersion, nodes, ready)
		}
	})

	t.Run("node list fails", func(t *testing.T) {
		link := New(Config{
			Discovery:  stubDiscovery{info: &version.Info{GitVersion: "v1.30.0"}},
			NodeLister: stubNodeLister{err: errors.New("forbidden")},
		})
		gotVersion, nodes, ready := link.clusterInventory(context.Background())
		if gotVersion != "v1.30.0" || nodes != 0 || ready != 0 {
			t.Errorf("inventory = (%q, %d, %d), want version with zero node counts", gotVersion, nodes, ready)
		}
	})

	t.Run("no collectors configured", func(t *testing.T) {
		link := New(Config{})
		gotVersion, nodes, ready := link.clusterInventory(context.Background())
		if gotVersion != "" || nodes != 0 || ready != 0 {
			t.Errorf("inventory = (%q, %d, %d), want all empty", gotVersion, nodes, ready)
		}
	})
}

// A node without an explicit Ready condition must not be counted as ready.
func TestNodeReadyRequiresExplicitTrueCondition(t *testing.T) {
	if nodeReady(&corev1.Node{}) {
		t.Error("node without conditions must not be ready")
	}
	n := node("n", corev1.ConditionUnknown)
	if nodeReady(&n) {
		t.Error("node with Unknown Ready condition must not be ready")
	}
}
