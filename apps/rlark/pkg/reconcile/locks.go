package reconcile

import "sync"

// KeyedLocks serializes work per key within one process.
//
// A reconciler may be visited concurrently for different objects, but two
// passes over the same object must not interleave: each pass reads current
// state and then acts on it, so overlapping passes can both observe "absent"
// and both create. Serializing by key removes that race without serializing
// unrelated objects behind a single mutex.
//
// Entries are reference counted and removed when the last holder releases, so
// a long-lived process does not accumulate one mutex per object it has ever
// reconciled.
//
// A zero KeyedLocks is ready to use. It must not be copied after first use.
type KeyedLocks struct {
	mu      sync.Mutex
	entries map[string]*keyedLock
}

type keyedLock struct {
	mu   sync.Mutex
	refs int
}

// Lock acquires the lock for key and returns the function that releases it.
// The returned function must be called exactly once, normally via defer.
func (l *KeyedLocks) Lock(key string) func() {
	l.mu.Lock()
	if l.entries == nil {
		l.entries = make(map[string]*keyedLock)
	}
	entry := l.entries[key]
	if entry == nil {
		entry = &keyedLock{}
		l.entries[key] = entry
	}
	entry.refs++
	l.mu.Unlock()

	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		l.mu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(l.entries, key)
		}
		l.mu.Unlock()
	}
}
