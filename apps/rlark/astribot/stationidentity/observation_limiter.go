package stationidentity

import (
	"math"
	"sync"
	"time"
)

type observationBucket struct {
	tokens   float64
	last     time.Time
	accessed time.Time
}

// TokenBucketObservationLimiter bounds both per-Station request rate and
// memory use. Identity keys are server-verified cluster and Node UIDs.
type TokenBucketObservationLimiter struct {
	mu       sync.Mutex
	buckets  map[string]observationBucket
	rate     float64
	burst    float64
	capacity int
	idleTTL  time.Duration
}

func NewTokenBucketObservationLimiter(requestsPerSecond float64, burst, capacity int, idleTTL time.Duration) *TokenBucketObservationLimiter {
	return &TokenBucketObservationLimiter{
		buckets: make(map[string]observationBucket), rate: requestsPerSecond,
		burst: float64(burst), capacity: capacity, idleTTL: idleTTL,
	}
}

func (l *TokenBucketObservationLimiter) Allow(evidence Evidence, now time.Time) bool {
	key := evidence.ClusterID + "\x00" + evidence.NodeUID
	now = now.UTC()
	l.mu.Lock()
	defer l.mu.Unlock()
	bucket, exists := l.buckets[key]
	if !exists {
		if len(l.buckets) >= l.capacity {
			for existingKey, existing := range l.buckets {
				if now.Sub(existing.accessed) >= l.idleTTL {
					delete(l.buckets, existingKey)
				}
			}
			if len(l.buckets) >= l.capacity {
				return false
			}
		}
		bucket = observationBucket{tokens: l.burst, last: now, accessed: now}
	}
	if now.Before(bucket.last) {
		bucket.accessed = now
		l.buckets[key] = bucket
		return false
	}
	bucket.tokens = math.Min(l.burst, bucket.tokens+now.Sub(bucket.last).Seconds()*l.rate)
	bucket.last = now
	bucket.accessed = now
	if bucket.tokens < 1 {
		l.buckets[key] = bucket
		return false
	}
	bucket.tokens--
	l.buckets[key] = bucket
	return true
}
