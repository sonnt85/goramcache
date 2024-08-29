package goramcache

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func t() {

}
func TestSyncpool(t *testing.T) {
	var ga sync.WaitGroup
	p := NewPool[*int](10, time.Second*10)
	for i := 0; i < 100; i++ {
		ga.Add(1)
		go func() {
			e := p.Get()
			fmt.Printf("%v\n", *e)
			defer p.Put(e)
			ga.Done()
		}()
	}
	ga.Wait()
}
