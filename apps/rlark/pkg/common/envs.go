package common

import (
	"cmp"
	"fmt"
	"os"
)

func PodIP(defaultValue string) string {
	return cmp.Or(os.Getenv("POD_IP"), defaultValue)
}

func PodName(defaultValue string) string {
	return cmp.Or(os.Getenv("POD_NAME"), defaultValue)
}

func Hostname(defaultValue string) string {
	return cmp.Or(os.Getenv("HOSTNAME"), PodName(defaultValue))
}

func PodNamespace(defaultValue string) string {
	return cmp.Or(os.Getenv("POD_NAMESPACE"), defaultValue)
}

func CurrentPodFromEnv() (string, string, error) {
	podName, podNamespace := PodName(""), PodNamespace("")
	if podName == "" || podNamespace == "" {
		return "", "", fmt.Errorf(
			"POD_NAME and POD_NAMESPACE env vars must be set (via the downward API)")
	}
	return podName, podNamespace, nil
}

func NodeName(defaultValue string) string {
	return cmp.Or(os.Getenv("NODE_NAME"), defaultValue)
}
