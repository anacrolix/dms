package futures

import (
	"sync"
	"bitbucket.org/anacrolix/dms/queue"
)

type Executor struct {
	waiting *queue.Queue
}

func NewExecutor(maxWorkers int) *Executor {
	ret := &Executor{
		waiting: queue.New(),
	}
	for a := 0; a < maxWorkers; a++ {
		go func() {
			for {
				val, ok := ret.waiting.Get()
				if !ok {
					return
				}
				val.(*Future).run()
			}
		}()
	}
	return ret
}

func (me *Executor) Shutdown() {
	me.waiting.Close()
}

func (me *Executor) Submit(fn func() R) *Future {
	fut := &Future{
		fn: fn,
		done: make(chan struct{}),
		do: &sync.Once{},
	}
	me.waiting.Put(fut)
	return fut
}

type Future struct {
	fn func() R
	done chan struct{}
	result R
	do *sync.Once
}

func (me *Future) Result() R {
	<-me.done
	return me.result
}

func (me *Future) run() {
	me.do.Do(func() {
		me.result = me.fn()
		close(me.done)
	})
}

type R interface{} // the return type
type I interface{} // the input type

func (me *Executor) Map(fn func(I) R, inputs <-chan I) <-chan R {
	ret := make(chan R)
	go func() {
		futs := queue.New()
		go func() {
			for {
				_fut, ok := futs.Get()
				if !ok {
					break
				}
				ret<-_fut.(*Future).Result()
			}
			close(ret)
		}()
		for a := range inputs {
			fut := me.Submit(func(a I) func() R {
				return func() R {
					return fn(a)
				}
			}(a))
			futs.Put(fut)
		}
		futs.Close()
	}()
	return ret
}
