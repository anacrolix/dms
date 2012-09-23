package futures

import (
	"bitbucket.org/anacrolix/dms/queue"
	"sync"
)

// Maintains the pool of workers and receives new work.
type Executor struct {
	waiting *queue.Queue
}

// Create a new Executor that does up to maxWorkers tasks in parallel.
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

// Prevents new tasks being submitted, and cleans up workers when all futures have been processed.
func (me *Executor) Shutdown() {
	me.waiting.Close()
}

// Submit the function to the Executor, returning a Future that represents it.
func (me *Executor) Submit(fn func() interface{}) *Future {
	fut := &Future{
		fn:   fn,
		done: make(chan struct{}),
		do:   &sync.Once{},
	}
	me.waiting.Put(fut)
	return fut
}

// Represents some asynchronous execution.
type Future struct {
	fn     func() interface{}
	done   chan struct{}
	result interface{}
	do     *sync.Once
}

// Blocks until the Future completes, and returns the computed value.
func (me *Future) Result() interface{} {
	<-me.done
	return me.result
}

func (me *Future) run() {
	me.do.Do(func() {
		me.result = me.fn()
		close(me.done)
	})
}

// Calls fn with each item received from inputs, and outputs the results in the same order to the returned channel.
func (me *Executor) Map(fn func(interface{}) interface{}, inputs <-chan interface{}) <-chan interface{} {
	ret := make(chan interface{})
	go func() {
		futs := queue.New()
		go func() {
			for {
				_fut, ok := futs.Get()
				if !ok {
					break
				}
				ret <- _fut.(*Future).Result()
			}
			close(ret)
		}()
		for a := range inputs {
			fut := me.Submit(func(a interface{}) func() interface{} {
				return func() interface{} {
					return fn(a)
				}
			}(a))
			futs.Put(fut)
		}
		futs.Close()
	}()
	return ret
}
