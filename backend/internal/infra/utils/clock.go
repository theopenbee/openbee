package utils

import (
	"sync/atomic"
	"time"
)

var lastMillis atomic.Int64

func NextMillis() int64 {
	for {
		now := time.Now().UnixMilli()
		last := lastMillis.Load()
		if now <= last {
			now = last + 1
		}
		if lastMillis.CompareAndSwap(last, now) {
			return now
		}
	}
}
