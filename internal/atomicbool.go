package internal

import "sync/atomic"

type AtomicBool struct {
	val atomic.Value
}

func (b *AtomicBool) Set(value bool) {
	b.val.Store(value)
}

func (b *AtomicBool) Value() bool {
	v := b.val.Load()
	if v == nil {
		return false
	}
	return v.(bool)
}
