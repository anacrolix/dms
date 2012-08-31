package cache

import (
	"sync"
)

type Data interface{}
type Key interface{}
type GenFunc func() (Data, Stamp, error)
type Stamp interface{}

type cacheValue struct {
	stamp interface{}
	data Data
	*sync.Mutex
}

type Cache struct {
	store map[interface{}]*cacheValue
	*sync.Mutex
}

func New() *Cache {
	return &Cache{
		store: map[interface{}]*cacheValue{},
		sync.Mutex: &sync.Mutex{},
	}
}

func (me *Cache) Get(key Key, stamp Stamp, genfn GenFunc) (data Data, err error) {
	me.Lock()
	val, ok := me.store[key]
	if !ok {
		val = &cacheValue{
			sync.Mutex: &sync.Mutex{},
		}
		me.store[key] = val
	}
	me.Unlock()
	val.Lock()
	if val.stamp != stamp {
		val.data, val.stamp, err = genfn()
		if err != nil {
			val.stamp = nil
		}
	}
	data = val.data
	val.Unlock()
	return
}

