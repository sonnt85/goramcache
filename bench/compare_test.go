// Package bench compares goramcache against ecosystem alternatives.
//
// Covered libraries:
//   - sonnt85/goramcache (this library, both Cache and CachePools variants)
//   - patrickmn/go-cache (the most widely-used Go cache)
//   - dgraph-io/ristretto (high-performance, admission/eviction based)
//   - coocood/freecache (lock-free shards, off-heap)
//   - VictoriaMetrics/fastcache (byte-oriented, off-heap)
//
// Run: go test -bench=. -benchmem -run=^$ -benchtime=2s
package bench

import (
	"strconv"
	"testing"
	"time"

	"github.com/VictoriaMetrics/fastcache"
	"github.com/coocood/freecache"
	"github.com/dgraph-io/ristretto"
	gocache "github.com/patrickmn/go-cache"
	"github.com/sonnt85/goramcache"
)

const (
	benchKeySet = 10000
	benchValue  = "value"
)

// precomputed keys + byte-keys isolate library cost from strconv.
var (
	benchKeys  = mkStrKeys(benchKeySet)
	benchBKeys = mkByteKeys(benchKeySet)
)

func mkStrKeys(n int) []string {
	keys := make([]string, n)
	for i := 0; i < n; i++ {
		keys[i] = "key" + strconv.Itoa(i)
	}
	return keys
}

func mkByteKeys(n int) [][]byte {
	keys := make([][]byte, n)
	for i := 0; i < n; i++ {
		keys[i] = []byte("key" + strconv.Itoa(i))
	}
	return keys
}

// --- goramcache.Cache (single-lock variant) ---

func BenchmarkGet_Goramcache(b *testing.B) {
	c := goramcache.NewCache[string, string](5*time.Minute, 0)
	for _, k := range benchKeys {
		c.Set(k, benchValue, goramcache.DefaultExpiration)
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			c.Get(benchKeys[i%benchKeySet])
			i++
		}
	})
}

func BenchmarkSet_Goramcache(b *testing.B) {
	c := goramcache.NewCache[string, string](5*time.Minute, 0)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			c.Set(benchKeys[i%benchKeySet], benchValue, goramcache.DefaultExpiration)
			i++
		}
	})
}

func BenchmarkMixed_Goramcache(b *testing.B) {
	c := goramcache.NewCache[string, string](5*time.Minute, 0)
	for _, k := range benchKeys {
		c.Set(k, benchValue, goramcache.DefaultExpiration)
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			k := benchKeys[i%benchKeySet]
			if i%10 == 0 {
				c.Set(k, benchValue, goramcache.DefaultExpiration)
			} else {
				c.Get(k)
			}
			i++
		}
	})
}

// --- goramcache.CachePools (sharded variant) ---

func BenchmarkGet_GoramcachePools(b *testing.B) {
	c := goramcache.NewCachePools[string, string](5*time.Minute, time.Minute, 16)
	for _, k := range benchKeys {
		c.Set(k, benchValue, goramcache.DefaultExpiration)
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			c.Get(benchKeys[i%benchKeySet])
			i++
		}
	})
}

func BenchmarkSet_GoramcachePools(b *testing.B) {
	c := goramcache.NewCachePools[string, string](5*time.Minute, time.Minute, 16)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			c.Set(benchKeys[i%benchKeySet], benchValue, goramcache.DefaultExpiration)
			i++
		}
	})
}

func BenchmarkMixed_GoramcachePools(b *testing.B) {
	c := goramcache.NewCachePools[string, string](5*time.Minute, time.Minute, 16)
	for _, k := range benchKeys {
		c.Set(k, benchValue, goramcache.DefaultExpiration)
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			k := benchKeys[i%benchKeySet]
			if i%10 == 0 {
				c.Set(k, benchValue, goramcache.DefaultExpiration)
			} else {
				c.Get(k)
			}
			i++
		}
	})
}

// --- patrickmn/go-cache ---

func BenchmarkGet_Gocache(b *testing.B) {
	c := gocache.New(5*time.Minute, 0)
	for _, k := range benchKeys {
		c.Set(k, benchValue, gocache.DefaultExpiration)
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			c.Get(benchKeys[i%benchKeySet])
			i++
		}
	})
}

func BenchmarkSet_Gocache(b *testing.B) {
	c := gocache.New(5*time.Minute, 0)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			c.Set(benchKeys[i%benchKeySet], benchValue, gocache.DefaultExpiration)
			i++
		}
	})
}

func BenchmarkMixed_Gocache(b *testing.B) {
	c := gocache.New(5*time.Minute, 0)
	for _, k := range benchKeys {
		c.Set(k, benchValue, gocache.DefaultExpiration)
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			k := benchKeys[i%benchKeySet]
			if i%10 == 0 {
				c.Set(k, benchValue, gocache.DefaultExpiration)
			} else {
				c.Get(k)
			}
			i++
		}
	})
}

// --- dgraph-io/ristretto ---

func newRistretto(b *testing.B) *ristretto.Cache {
	c, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: benchKeySet * 10,
		MaxCost:     1 << 30,
		BufferItems: 64,
	})
	if err != nil {
		b.Fatal(err)
	}
	return c
}

func BenchmarkGet_Ristretto(b *testing.B) {
	c := newRistretto(b)
	for _, k := range benchKeys {
		c.Set(k, benchValue, 1)
	}
	c.Wait()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			c.Get(benchKeys[i%benchKeySet])
			i++
		}
	})
}

func BenchmarkSet_Ristretto(b *testing.B) {
	c := newRistretto(b)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			c.Set(benchKeys[i%benchKeySet], benchValue, 1)
			i++
		}
	})
}

func BenchmarkMixed_Ristretto(b *testing.B) {
	c := newRistretto(b)
	for _, k := range benchKeys {
		c.Set(k, benchValue, 1)
	}
	c.Wait()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			k := benchKeys[i%benchKeySet]
			if i%10 == 0 {
				c.Set(k, benchValue, 1)
			} else {
				c.Get(k)
			}
			i++
		}
	})
}

// --- coocood/freecache ---

func BenchmarkGet_Freecache(b *testing.B) {
	c := freecache.NewCache(64 << 20)
	for i, k := range benchBKeys {
		_ = c.Set(k, []byte(benchValue+strconv.Itoa(i)), 300)
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_, _ = c.Get(benchBKeys[i%benchKeySet])
			i++
		}
	})
}

func BenchmarkSet_Freecache(b *testing.B) {
	c := freecache.NewCache(64 << 20)
	val := []byte(benchValue)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_ = c.Set(benchBKeys[i%benchKeySet], val, 300)
			i++
		}
	})
}

func BenchmarkMixed_Freecache(b *testing.B) {
	c := freecache.NewCache(64 << 20)
	val := []byte(benchValue)
	for _, k := range benchBKeys {
		_ = c.Set(k, val, 300)
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			k := benchBKeys[i%benchKeySet]
			if i%10 == 0 {
				_ = c.Set(k, val, 300)
			} else {
				_, _ = c.Get(k)
			}
			i++
		}
	})
}

// --- VictoriaMetrics/fastcache ---

func BenchmarkGet_Fastcache(b *testing.B) {
	c := fastcache.New(64 << 20)
	val := []byte(benchValue)
	for _, k := range benchBKeys {
		c.Set(k, val)
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		dst := make([]byte, 0, 32)
		for pb.Next() {
			dst = c.Get(dst[:0], benchBKeys[i%benchKeySet])
			_ = dst
			i++
		}
	})
}

func BenchmarkSet_Fastcache(b *testing.B) {
	c := fastcache.New(64 << 20)
	val := []byte(benchValue)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			c.Set(benchBKeys[i%benchKeySet], val)
			i++
		}
	})
}

func BenchmarkMixed_Fastcache(b *testing.B) {
	c := fastcache.New(64 << 20)
	val := []byte(benchValue)
	for _, k := range benchBKeys {
		c.Set(k, val)
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		dst := make([]byte, 0, 32)
		for pb.Next() {
			k := benchBKeys[i%benchKeySet]
			if i%10 == 0 {
				c.Set(k, val)
			} else {
				dst = c.Get(dst[:0], k)
				_ = dst
			}
			i++
		}
	})
}
