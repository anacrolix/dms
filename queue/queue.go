package queue

import (
	"container/list"
)

type Queue struct {
	in, out chan interface{}
}

func New() *Queue {
	ret := &Queue{
		in:  make(chan interface{}),
		out: make(chan interface{}),
	}
	go func() {
		l := list.New()
		for {
			if l.Len() == 0 {
				if ret.in == nil {
					break
				}
				v, ok := <-ret.in
				if !ok {
					break
				}
				l.PushBack(v)
			} else {
				select {
				case ret.out <- l.Front().Value:
					l.Remove(l.Front())
				case v, ok := <-ret.in:
					if !ok {
						ret.in = nil
					} else {
						l.PushBack(v)
					}
				}
			}
		}
		close(ret.out)
	}()
	return ret
}

func (me *Queue) Put(v interface{}) {
	me.in <- v
}

func (me *Queue) Get() (val interface{}, ok bool) {
	val, ok = <-me.out
	return
}

func (me *Queue) Close() {
	close(me.in)
}
