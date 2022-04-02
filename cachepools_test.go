package goramcache

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

// func TestDjb33(t *testing.T) {
// }

var shardedKeys = []string{
	"f",
	"fo",
	"foo",
	"barf",
	"barfo",
	"foobar",
	"bazbarf",
	"bazbarfo",
	"bazbarfoo",
	"foobarbazq",
	"foobarbazqu",
	"foobarbazquu",
	"foobarbazquux",
}

func TestCachePools(t *testing.T) {
	tc := NewCachePools[string](DefaultExpiration, 0, 13)
	for _, v := range shardedKeys {
		tc.Set(v, "value", DefaultExpiration)
	}
}

func BenchmarkCachePoolsGetExpiring(b *testing.B) {
	benchmarkCachePoolsGet(b, 5*time.Minute)
}

func BenchmarkCachePoolsGetNotExpiring(b *testing.B) {
	benchmarkCachePoolsGet(b, NoExpiration)
}

func benchmarkCachePoolsGet(b *testing.B, exp time.Duration) {
	b.StopTimer()
	tc := NewCachePools[interface{}](exp, 0, 10)
	tc.Set("foobarba", "zquux", DefaultExpiration)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		tc.Get("foobarba")
	}
}

func BenchmarkCachePoolsGetManyConcurrentExpiring(b *testing.B) {
	benchmarkCachePoolsGetManyConcurrent(b, 5*time.Minute)
}

func BenchmarkCachePoolsGetManyConcurrentNotExpiring(b *testing.B) {
	benchmarkCachePoolsGetManyConcurrent(b, NoExpiration)
}

func benchmarkCachePoolsGetManyConcurrent(b *testing.B, exp time.Duration) {
	b.StopTimer()
	n := 10000
	tsc := NewCachePools[interface{}](exp, 0, 20)
	keys := make([]string, n)
	for i := 0; i < n; i++ {
		k := "foo" + strconv.Itoa(i)
		keys[i] = k
		tsc.Set(k, "bar", DefaultExpiration)
	}
	each := b.N / n
	wg := new(sync.WaitGroup)
	wg.Add(n)
	for _, v := range keys {
		go func(k string) {
			for j := 0; j < each; j++ {
				tsc.Get(k)
			}
			wg.Done()
		}(v)
	}
	b.StartTimer()
	wg.Wait()
}
