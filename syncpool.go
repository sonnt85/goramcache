package goramcache

import (
	"reflect"
	"sync"
	"time"
)

type Pool[T any] struct {
	pool       *sync.Pool
	maxElement int
	numElement int
	m          *sync.RWMutex
	lastGet    time.Time
	expireTime time.Duration
	j          *Janitor
}

func NewPool[T any](maxElement int, expireTime time.Duration, fn ...func() T) *Pool[T] {
	f := (func() T)(nil)
	if len(fn) != 0 {
		f = fn[0]
	}
	p := &Pool[T]{
		pool: &sync.Pool{
			New: func() interface{} {
				if f != nil {
					return f()
				} else {
					var t T
					if reflect.TypeOf((*T)(nil)).Elem().Kind() == reflect.Ptr {
						ptr := reflect.New(reflect.TypeOf(t).Elem())
						ptr.Elem().Set(reflect.Zero(reflect.TypeOf(t).Elem()))
						obj := ptr.Interface()
						return obj
					} else {
						return t
					}
				}
			},
		},
		maxElement: maxElement,
		m:          new(sync.RWMutex),
		expireTime: expireTime,
	}
	p.j = NewJanitor(expireTime * 2)
	p.j.Start(p, p.deleteExpired)
	return p
}

func (p *Pool[T]) deleteExpired() (nextTimeCheck time.Time, needUpdate bool) {
	now := time.Now()
	p.m.Lock()
	defer p.m.Unlock()
	nextTimeCheck = p.lastGet.Add(p.expireTime)
	if nextTimeCheck.Before(now) {
		if p.numElement > 1 {
			for i := 1; i < p.numElement-1; i++ {
				p.pool.Get() //remove
			}
		}
	} else {
		needUpdate = true
	}
	return
}

func (p *Pool[T]) Get() T {
	// p.m.RLock()
	// defer p.m.RUnlock()
	p.m.Lock()
	defer p.m.Unlock()
	if p.numElement > 0 {
		p.numElement--
	}
	// p.j.SetNextCheckExpire(p.j.GetInterval())
	p.lastGet = time.Now()
	return p.pool.Get().(T)
}

func (p *Pool[T]) Put(obj T) (ok bool) {
	p.m.Lock()
	defer p.m.Unlock()
	if p.numElement == p.maxElement {
		return false
	}
	p.pool.Put(obj)
	p.numElement++
	return true
}
