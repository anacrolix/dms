package dms

import (
	"sync"
)

// A semaphore that doesn't release you until there are no slots left and you
// drop off the far end of the "belt".
type semabelt struct {
	belt []chan struct{}
	mu   sync.Mutex
	cap  int
}

func newSemabelt(cap int) *semabelt {
	return &semabelt{
		cap: cap,
	}
}

func (me *semabelt) Ride() {
	me.mu.Lock()
	for len(me.belt) >= me.cap {
		close(me.belt[0])
		me.belt = me.belt[1:]
	}
	ch := make(chan struct{})
	me.belt = append(me.belt, ch)
	me.mu.Unlock()
	<-ch
}
