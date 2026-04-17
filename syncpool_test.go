package goramcache

import (
	"sync"
	"testing"
	"time"
)

func TestSyncpool(t *testing.T) {
	var ga sync.WaitGroup
	p := NewPool[*int](10, time.Second*10)
	for i := 0; i < 100; i++ {
		ga.Add(1)
		go func() {
			defer ga.Done()
			e := p.Get()
			defer p.Put(e)
			_ = *e
		}()
	}
	ga.Wait()
}
