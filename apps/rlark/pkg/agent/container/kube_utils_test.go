package container

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// makeTestToken 构造一个 JWT 格式的 token 字符串，payload 由传入对象序列化后做 base64url 编码。
func makeTestToken(t *testing.T, payload any) string {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(b)
	return "header." + encoded + ".signature"
}

// writeTokenFile 在临时目录中写入 token 文件并返回其路径。
func writeTokenFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	return path
}

func TestReadPodUIDFromTokenFile(t *testing.T) {
	const wantUID = "b801a9be-d7d8-4ccd-a7f2-1adf39119e39"
	payload := map[string]any{
		"iss": "https://kubernetes.default.svc.cluster.local",
		"kubernetes.io": map[string]any{
			"namespace": "rlark-system",
			"pod": map[string]any{
				"name": "dual-arm-hg-dagger-actor-0",
				"uid":  wantUID,
			},
			"serviceaccount": map[string]any{
				"name": "default",
				"uid":  "8385d8a9-fc5e-4774-9373-60ee4d70e9ca",
			},
		},
	}
	path := writeTokenFile(t, makeTestToken(t, payload))

	got, err := readPodUIDFromTokenFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != wantUID {
		t.Errorf("got %q, want %q", got, wantUID)
	}
}

func TestReadPodUIDFromTokenFile_PaddedPayload(t *testing.T) {
	// 某些实现会对 payload 做 base64url 带 padding 编码，需兼容。
	const wantUID = "abc-123"
	b, _ := json.Marshal(map[string]any{
		"kubernetes.io": map[string]any{
			"pod": map[string]any{"uid": wantUID},
		},
	})
	encoded := base64.RawURLEncoding.EncodeToString(b)
	token := "header." + encoded + ".sig"
	path := writeTokenFile(t, token)

	got, err := readPodUIDFromTokenFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != wantUID {
		t.Errorf("got %q, want %q", got, wantUID)
	}
}

func TestReadPodUIDFromTokenFile_NoDot(t *testing.T) {
	path := writeTokenFile(t, "not-a-valid-token")
	if _, err := readPodUIDFromTokenFile(path); err == nil {
		t.Fatal("expected error for token without '.', got nil")
	}
}

func TestReadPodUIDFromTokenFile_InvalidBase64(t *testing.T) {
	path := writeTokenFile(t, "header.!!!invalid!!!.sig")
	if _, err := readPodUIDFromTokenFile(path); err == nil {
		t.Fatal("expected error for invalid base64 payload, got nil")
	}
}

func TestReadPodUIDFromTokenFile_InvalidJSON(t *testing.T) {
	encoded := base64.RawURLEncoding.EncodeToString([]byte("{not json"))
	path := writeTokenFile(t, "header."+encoded+".sig")
	if _, err := readPodUIDFromTokenFile(path); err == nil {
		t.Fatal("expected error for invalid JSON payload, got nil")
	}
}

func TestReadPodUIDFromTokenFile_NoPodUID(t *testing.T) {
	payload := map[string]any{
		"kubernetes.io": map[string]any{
			"namespace": "rlark-system",
		},
	}
	path := writeTokenFile(t, makeTestToken(t, payload))
	if _, err := readPodUIDFromTokenFile(path); err == nil {
		t.Fatal("expected error for missing pod uid, got nil")
	}
}

func TestReadPodUIDFromTokenFile_MissingFile(t *testing.T) {
	if _, err := readPodUIDFromTokenFile("/nonexistent/path/token"); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestReadPodUIDFromKubeAPIAccess_NotFound(t *testing.T) {
	// kubelet 目录不存在或无 kube-api-access 子目录时应原样返回 podUID。
	dir := t.TempDir()
	uid := "host-cluster-uid"
	if got := readPodUIDFromKubeAPIAccess(dir, uid); got != uid {
		t.Errorf("got %q, want %q (fallback to original)", got, uid)
	}
}

func TestReadPodUIDFromKubeAPIAccess_Found(t *testing.T) {
	const wantUID = "virtual-cluster-uid"
	// 模拟 kubelet 的 projected volume 目录结构
	kubeletDir := t.TempDir()
	hostUID := "host-cluster-uid"
	accessDir := filepath.Join(kubeletDir, "pods", hostUID, "volumes", "kubernetes.io~projected", "kube-api-access-mm")
	if err := os.MkdirAll(accessDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	payload := map[string]any{
		"kubernetes.io": map[string]any{
			"pod": map[string]any{"uid": wantUID},
		},
	}
	if err := os.WriteFile(filepath.Join(accessDir, "token"), []byte(makeTestToken(t, payload)), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}

	got := readPodUIDFromKubeAPIAccess(kubeletDir, hostUID)
	if got != wantUID {
		t.Errorf("got %q, want %q", got, wantUID)
	}
}

func TestReadPodUIDFromKubeAPIAccess_InvalidTokenFallsBack(t *testing.T) {
	// token 文件内容无效时应回退到原始 podUID。
	kubeletDir := t.TempDir()
	hostUID := "host-cluster-uid"
	accessDir := filepath.Join(kubeletDir, "pods", hostUID, "volumes", "kubernetes.io~projected", "kube-api-access-mm")
	if err := os.MkdirAll(accessDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(accessDir, "token"), []byte("invalid"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}

	got := readPodUIDFromKubeAPIAccess(kubeletDir, hostUID)
	if got != hostUID {
		t.Errorf("got %q, want %q (fallback)", got, hostUID)
	}
}
