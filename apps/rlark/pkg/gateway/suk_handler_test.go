package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/configs"
)

// An empty result must serialize as [] on both branches of the handler: the
// secret-missing branch already answered [], so a nil slice here would make the
// same endpoint report two different empty values.
func TestListSSHUserKeysReturnsEmptyArrayWhenFilterMatchesNothing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	namespace := "rlark-system"
	gateway := &Gateway{
		config: Config{KubeClientConfig: configs.KubernetesClientConfig{Namespace: namespace}},
		rawClient: fake.NewSimpleClientset(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: common.SSHUserKeySecretName, Namespace: namespace},
			Data:       map[string][]byte{"alice": []byte("ssh-ed25519 AAAA-alice")},
		}),
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/ssh-user-keys?user=bob", nil)

	gateway.handleListSSHUserKeys(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if got := recorder.Body.String(); got != "[]" {
		t.Fatalf("body = %s, want []", got)
	}
}

func TestListSSHUserKeysReturnsEmptyArrayWhenSecretIsMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := &Gateway{
		config:    Config{KubeClientConfig: configs.KubernetesClientConfig{Namespace: "rlark-system"}},
		rawClient: fake.NewSimpleClientset(),
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/ssh-user-keys", nil)

	gateway.handleListSSHUserKeys(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if got := recorder.Body.String(); got != "[]" {
		t.Fatalf("body = %s, want []", got)
	}
}

func TestFindSSHKeyDuplicate(t *testing.T) {
	keysByUser := map[string][]string{
		"alice": {"ssh-ed25519 AAAA-alice"},
	}

	tests := []struct {
		name      string
		user      string
		publicKey string
		want      string
	}{
		{name: "duplicate name", user: "alice", publicKey: "ssh-rsa AAAA-other", want: "public key name already exists"},
		{name: "duplicate key", user: "bob", publicKey: "ssh-ed25519 AAAA-alice", want: "public key already exists"},
		{name: "unique key", user: "bob", publicKey: "ssh-rsa AAAA-bob", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := findSSHKeyDuplicate(keysByUser, tt.user, tt.publicKey); got != tt.want {
				t.Fatalf("findSSHKeyDuplicate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSSHKeyAddedAtRoundTrip(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "ssh-user-keys",
			CreationTimestamp: metav1.NewTime(time.Date(2026, 8, 26, 14, 38, 32, 0, time.UTC)),
		},
		Data: map[string][]byte{},
	}

	// 老数据：没 annotation，回落到 CreationTimestamp
	if got := sshKeyAddedAt(secret, "alice", 0); !got.Equal(secret.CreationTimestamp.Time) {
		t.Fatalf("expected fallback to CreationTimestamp, got %v", got)
	}

	// 写入两个 key 的时间
	t1 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 9, 21, 15, 30, 0, 0, time.UTC)
	writeSSHKeyAddedAts(secret, "alice", []time.Time{t1, t2})

	if got := sshKeyAddedAt(secret, "alice", 0); !got.Equal(t1) {
		t.Fatalf("expected t1, got %v", got)
	}
	if got := sshKeyAddedAt(secret, "alice", 1); !got.Equal(t2) {
		t.Fatalf("expected t2, got %v", got)
	}
	// 越界回落
	if got := sshKeyAddedAt(secret, "alice", 5); !got.Equal(secret.CreationTimestamp.Time) {
		t.Fatalf("expected fallback for out-of-range index, got %v", got)
	}
	// 其他 user 不受影响
	if got := sshKeyAddedAt(secret, "bob", 0); !got.Equal(secret.CreationTimestamp.Time) {
		t.Fatalf("expected fallback for unknown user, got %v", got)
	}
}

// 删除中间一条后，后续 index 的时间应正确前移
func TestSSHKeyAddedAtDeleteShiftsIndices(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "ssh-user-keys",
			CreationTimestamp: metav1.NewTime(time.Now()),
		},
		Data: map[string][]byte{},
	}

	t1 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 9, 10, 10, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	writeSSHKeyAddedAts(secret, "alice", []time.Time{t1, t2, t3})

	// 模拟 delete index=1：删除中间一条
	ats := readSSHKeyAddedAts(secret, "alice")
	ats = append(ats[:1], ats[2:]...)
	writeSSHKeyAddedAts(secret, "alice", ats)

	if got := sshKeyAddedAt(secret, "alice", 0); !got.Equal(t1) {
		t.Fatalf("index 0 should still be t1, got %v", got)
	}
	if got := sshKeyAddedAt(secret, "alice", 1); !got.Equal(t3) {
		t.Fatalf("index 1 should be t3 after shift, got %v", got)
	}
	if len(readSSHKeyAddedAts(secret, "alice")) != 2 {
		t.Fatalf("expected 2 timestamps after delete")
	}
}

// 清空后 annotation 应被删除，避免积累空记录
func TestSSHKeyAddedAtClearedWhenEmpty(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "ssh-user-keys"},
		Data:       map[string][]byte{},
	}
	writeSSHKeyAddedAts(secret, "alice", []time.Time{time.Now()})
	if _, ok := secret.Annotations[sshKeyAddedAtAnnotationKey("alice")]; !ok {
		t.Fatal("expected annotation to be set")
	}
	writeSSHKeyAddedAts(secret, "alice", nil)
	if _, ok := secret.Annotations[sshKeyAddedAtAnnotationKey("alice")]; ok {
		t.Fatal("expected annotation to be removed when empty")
	}
}
