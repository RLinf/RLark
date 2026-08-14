package container

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func readPodUIDFromTokenFile(tokenFile string) (string, error) {
	data, err := os.ReadFile(tokenFile)
	if err != nil {
		return "", err
	}
	fields := strings.SplitN(string(data), ".", 3)
	if len(fields) < 2 {
		return "", fmt.Errorf("invalid token format: %s", string(data))
	}
	payload := fields[1]
	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return "", fmt.Errorf("decode token payload: %w", err)
	}
	var claims struct {
		Kubernetes struct {
			Pod struct {
				Name string `json:"name"`
				UID  string `json:"uid"`
			} `json:"pod"`
		} `json:"kubernetes.io"`
	}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return "", fmt.Errorf("unmarshal token payload: %w", err)
	}
	if claims.Kubernetes.Pod.UID == "" {
		return "", fmt.Errorf("pod uid not found in token")
	}
	return claims.Kubernetes.Pod.UID, nil
}

func readPodUIDFromKubeAPIAccess(kubeletDir, podUID string) string {
	// 在某些虚拟 Kubernetes 环境中，Rlark 控制面识别到的 Pod UID 是虚拟集群中的 UID，
	// 而通过进程识别到的 Pod UID 是宿主集群中的 UID。为了统一识别到虚拟集群中的 Pod UID，
	// 需要从 kubelet 目录下，读取 Pod 的 kube-api-access 文件，获取虚拟集群中的 Pod UID。
	projectedDir := filepath.Join(kubeletDir, "pods", podUID, "volumes", "kubernetes.io~projected")
	entries, _ := os.ReadDir(projectedDir)
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "kube-api-access-") {
			if ret, err := readPodUIDFromTokenFile(filepath.Join(projectedDir, entry.Name(), "token")); err == nil {
				return ret
			}
		}
	}
	return podUID
}
