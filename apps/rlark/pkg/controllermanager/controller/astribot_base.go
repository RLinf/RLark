package controller

import (
	"errors"
	"time"
)

// ErrRequeueWaiting asks the generic reconciler to save the status it set and
// retry later: the resource waits for something it does not watch, such as a
// storage a Job names becoming available in a cluster.
var ErrRequeueWaiting = errors.New("requeue while waiting")

const WaitingRequeue = 15 * time.Second
