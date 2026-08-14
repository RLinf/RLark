package container

import (
	"strings"

	"github.com/google/uuid"
)

// podUIDFromHostsSource 尝试从容器的 hosts 挂载来源路径中提取 Kubernetes Pod UID。
// mountSource 格式应为 .../pods/<pod-uid>/etc/hosts 或 .../pods/<pod-uid>/etc-hosts，
// 其中 <pod-uid> 是标准 UUID。不管 /var/lib/kubelet 是不是软链或重挂载，只要路径尾缀匹配即可识别。
func podUIDFromHostsSource(mountSource string) (string, bool) {
	const suffix1 = "/etc/hosts"
	const suffix2 = "/etc-hosts"
	if !strings.HasSuffix(mountSource, suffix1) && !strings.HasSuffix(mountSource, suffix2) {
		return "", false
	}
	// 去掉尾缀后得到 .../pods/<uuid>
	beforeEtc := mountSource
	if strings.HasSuffix(mountSource, suffix1) {
		beforeEtc = strings.TrimSuffix(mountSource, suffix1)
	} else {
		beforeEtc = strings.TrimSuffix(mountSource, suffix2)
	}

	// 从后往前找 "/"，取最后一个 segment 作为 podUID
	lastSlash := strings.LastIndex(beforeEtc, "/")
	if lastSlash < 0 {
		return "", false
	}
	podUID := beforeEtc[lastSlash+1:]

	// 检查倒数第二个 segment 是否为 "pods"
	beforeLast := beforeEtc[:lastSlash]
	secondLastSlash := strings.LastIndex(beforeLast, "/")
	if secondLastSlash < 0 {
		return "", false
	}
	segment := beforeLast[secondLastSlash+1:]
	if segment != "pods" {
		return "", false
	}

	// 校验 UUID 格式：8-4-4-4-12 十六进制
	if !isUUID(podUID) {
		return "", false
	}

	return podUID, true
}

// isUUID 简单校验 UUID 格式 xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx.
func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}

	_, err := uuid.Parse(s)
	return err == nil
}
