package stationidentity

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// NodeLimiter bounds the requests of each node. Its keys are server-verified
// cluster and Node UIDs, so it holds as many buckets as there are nodes.
type NodeLimiter struct {
	Rate  rate.Limit
	Burst int

	mu      sync.Mutex
	buckets map[string]*rate.Limiter
}

func (l *NodeLimiter) Allow(evidence Evidence, now time.Time) bool {
	key := evidence.ClusterID + "\x00" + evidence.NodeUID
	l.mu.Lock()
	bucket := l.buckets[key]
	if bucket == nil {
		if l.buckets == nil {
			l.buckets = map[string]*rate.Limiter{}
		}
		bucket = rate.NewLimiter(l.Rate, l.Burst)
		l.buckets[key] = bucket
	}
	l.mu.Unlock()
	return bucket.AllowN(now, 1)
}
