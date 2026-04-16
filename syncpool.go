package goramcache

import (
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

type Pool[T any] struct {
	numElement atomic.Int64
	lastGet    atomic.Int64 // store UnixNano
	pool       sync.Pool
	maxElement int
	expireTime time.Duration
	j          *Janitor
}

func NewPool[T any](maxElement int, expireTime time.Duration, fn ...func() T) *Pool[T] {
	f := (func() T)(nil)
	if len(fn) != 0 {
		f = fn[0]
	}
	p := &Pool[T]{
		maxElement: maxElement,
		expireTime: expireTime,
	}
	p.pool.New = func() interface{} {
		if f != nil {
			return f()
		}
		var t T
		if reflect.TypeOf((*T)(nil)).Elem().Kind() == reflect.Ptr {
			ptr := reflect.New(reflect.TypeOf(t).Elem())
			ptr.Elem().Set(reflect.Zero(reflect.TypeOf(t).Elem()))
			return ptr.Interface()
		}
		return t
	}
	p.j = NewJanitor(expireTime * 2)
	p.j.Start(p, p.deleteExpired)
	return p
}

func (p *Pool[T]) deleteExpired() (nextTimeCheck time.Time, needUpdate bool) {
	now := time.Now()
	lastGetNano := p.lastGet.Load()
	lastGetTime := time.Unix(0, lastGetNano)
	nextTimeCheck = lastGetTime.Add(p.expireTime)
	if nextTimeCheck.Before(now) {
		n := p.numElement.Load()
		if n > 1 {
			for i := int64(1); i < n-1; i++ {
				p.pool.Get() // remove excess items
			}
		}
	} else {
		needUpdate = true
	}
	return
}

func (p *Pool[T]) Get() T {
	p.numElement.Add(-1)
	if p.numElement.Load() < 0 {
		p.numElement.Store(0)
	}
	p.lastGet.Store(time.Now().UnixNano())
	return p.pool.Get().(T)
}

func (p *Pool[T]) Put(obj T) (ok bool) {
	if p.numElement.Load() >= int64(p.maxElement) {
		return false
	}
	p.pool.Put(obj)
	p.numElement.Add(1)
	return true
}
