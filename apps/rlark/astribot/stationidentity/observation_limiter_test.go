package stationidentity

import (
	"testing"
	"time"
)

func TestTokenBucketObservationLimiterRateCapacityAndIdleEviction(t *testing.T) {
	limiter := NewTokenBucketObservationLimiter(1, 2, 1, time.Minute)
	start := time.Unix(1000, 0)
	evidence := validEvidence()
	if !limiter.Allow(evidence, start) || !limiter.Allow(evidence, start) || limiter.Allow(evidence, start) {
		t.Fatal("burst limit was not enforced")
	}
	other := evidence
	other.NodeUID = "other-node"
	if limiter.Allow(other, start) {
		t.Fatal("limiter exceeded its bounded key capacity")
	}
	if !limiter.Allow(evidence, start.Add(time.Second)) {
		t.Fatal("token bucket did not refill")
	}
	if !limiter.Allow(other, start.Add(2*time.Minute)) {
		t.Fatal("idle bucket was not evicted at capacity")
	}
}
