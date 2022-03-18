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

func TestCacheGroups(t *testing.T) {
	tc := NewCacheGroups[string](DefaultExpiration, 0, 13)
	for _, v := range shardedKeys {
		tc.Set(v, "value", DefaultExpiration)
	}
}

func BenchmarkCacheGroupsGetExpiring(b *testing.B) {
	benchmarkCacheGroupsGet(b, 5*time.Minute)
}

func BenchmarkCacheGroupsGetNotExpiring(b *testing.B) {
	benchmarkCacheGroupsGet(b, NoExpiration)
}

func benchmarkCacheGroupsGet(b *testing.B, exp time.Duration) {
	b.StopTimer()
	tc := NewCacheGroups[interface{}](exp, 0, 10)
	tc.Set("foobarba", "zquux", DefaultExpiration)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		tc.Get("foobarba")
	}
}

func BenchmarkCacheGroupsGetManyConcurrentExpiring(b *testing.B) {
	benchmarkCacheGroupsGetManyConcurrent(b, 5*time.Minute)
}

func BenchmarkCacheGroupsGetManyConcurrentNotExpiring(b *testing.B) {
	benchmarkCacheGroupsGetManyConcurrent(b, NoExpiration)
}

func benchmarkCacheGroupsGetManyConcurrent(b *testing.B, exp time.Duration) {
	b.StopTimer()
	n := 10000
	tsc := NewCacheGroups[interface{}](exp, 0, 20)
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
