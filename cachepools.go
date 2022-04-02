package goramcache

import (
	"crypto/rand"
	"encoding/gob"
	"io"
	"math"
	"math/big"
	insecurerand "math/rand"
	"os"
	"time"
)

// This is an experimental and unexported (for now) attempt at making a cache
// with better algorithmic complexity than the standard one, namely by
// preventing write locks of the entire cache when an item is added. As of the
// time of writing, the overhead of selecting getpools results in cache
// operations being about twice as slow as for the standard cache with small
// total cache sizes, and faster for larger ones.
//
// See cache_test.go for a few benchmarks.

type CachePools[T any] struct {
	seed    uint32
	m       uint32
	cs      []*cache[T]
	janitor *Janitor
}

// djb2 with better shuffling. 5x faster than FNV with the hash.Hash overhead.
func djb33(seed uint32, k string) uint32 {
	var (
		l = uint32(len(k))
		d = 5381 + seed + l
		i = uint32(0)
	)
	// Why is all this 5x faster than a for loop?
	if l >= 4 {
		for i < l-4 {
			d = (d * 33) ^ uint32(k[i])
			d = (d * 33) ^ uint32(k[i+1])
			d = (d * 33) ^ uint32(k[i+2])
			d = (d * 33) ^ uint32(k[i+3])
			i += 4
		}
	}
	switch l - i {
	case 1:
	case 2:
		d = (d * 33) ^ uint32(k[i])
	case 3:
		d = (d * 33) ^ uint32(k[i])
		d = (d * 33) ^ uint32(k[i+1])
	case 4:
		d = (d * 33) ^ uint32(k[i])
		d = (d * 33) ^ uint32(k[i+1])
		d = (d * 33) ^ uint32(k[i+2])
	}
	return d ^ (d >> 16)
}

func (sc *CachePools[T]) getpool(k string) *cache[T] {
	return sc.cs[djb33(sc.seed, k)%sc.m]
}

func (sc *CachePools[T]) Set(k string, x T, d time.Duration) {
	sc.getpool(k).Set(k, x, d)
}

func (sc *CachePools[T]) SetDefault(k string, x T) {
	sc.getpool(k).SetDefault(k, x)
}

func (sc *CachePools[T]) Add(k string, x T, d time.Duration) error {
	return sc.getpool(k).Add(k, x, d)
}

func (sc *CachePools[T]) Replace(k string, x T, d time.Duration) error {
	return sc.getpool(k).Replace(k, x, d)
}

func (sc *CachePools[T]) Edit(k string, x interface{}, apFunc func(T, interface{}) (T, error)) error {
	return sc.getpool(k).Edit(k, x, apFunc)
}
func (sc *CachePools[T]) Get(k string) (T, bool) {
	return sc.getpool(k).Get(k)
}

func (sc *CachePools[T]) Keys() (keys []string) {
	keys = make([]string, 0)
	for i, _ := range sc.cs {
		keys = append(keys, sc.cs[i].Keys()...)
	}
	return keys
}
func (sc *CachePools[T]) GetWithExpirationGet(k string) (T, time.Time, bool) {
	return sc.getpool(k).GetWithExpiration(k)
}

func (sc *CachePools[T]) GetWithExpirationUpdate(k string, d time.Duration) (T, bool) {
	return sc.getpool(k).GetWithExpirationUpdate(k, d)
}

func (sc *CachePools[T]) GetWithDefaultExpirationUpdate(k string) (T, bool) {
	return sc.GetWithDefaultExpirationUpdate(k)
}

func (sc *CachePools[T]) Increment(k string, n int64) error {
	_, err := sc.getpool(k).Increment(k, n)
	return err
}

func (sc *CachePools[T]) Decrement(k string, n int64) error {
	_, err := sc.getpool(k).Decrement(k, n)
	return err
}

func (sc *CachePools[T]) Delete(k string) {
	sc.getpool(k).Delete(k)
}

func (sc *CachePools[T]) DeleteExpired() {
	for _, v := range sc.cs {
		v.DeleteExpired()
	}
}

func (sc *CachePools[T]) deleteExpired() (nextTimeCheck time.Time, needUpdate bool) {
	var nexTime time.Time
	nextTimeCheck = time.UnixMilli(math.MaxInt64)
	for _, v := range sc.cs {
		nexTime, needUpdate = v.deleteExpired()
		if needUpdate && nextTimeCheck.After(nexTime) {
			nextTimeCheck = nexTime
		}
	}
	return
}

func (sc *CachePools[T]) Load(r io.Reader) error {
	dec := gob.NewDecoder(r)
	items := map[string]Item[T]{}
	err := dec.Decode(&items)
	if err == nil {
		for k, v := range items {
			sc.getpool(k).mu.Lock()
			ov, found := sc.getpool(k).items[k]
			if !found || ov.Expired() {
				sc.getpool(k).items[k] = v
			}
			sc.getpool(k).mu.Unlock()
		}
	}
	return err
}

// Load and add cache items from the given filename, excluding any items with
// keys that already exist in the current cache.
func (sc *CachePools[T]) LoadFile(fname string) error {
	fp, err := os.Open(fname)
	if err != nil {
		return err
	}
	err = sc.Load(fp)
	if err != nil {
		fp.Close()
		return err
	}
	return fp.Close()
}

func (sc *CachePools[T]) Save(w io.Writer) (err error) {
	c := NewCache[T](NoExpiration, NoExpirationCheck)
	for i, _ := range sc.cs {
		sc.cs[i].mu.RLock()
		defer sc.cs[i].mu.RUnlock()
		now := time.Now().UnixNano()
		for k, v := range sc.cs[i].items {
			if v.Expiration > 0 {
				if now > v.Expiration {
					continue
				}
			}
			c.items[k] = v
		}
	}
	return c.Save(w)
}

func (sc *CachePools[T]) SaveFile(fname string) error {
	fp, err := os.Create(fname)
	if err != nil {
		return err
	}
	err = sc.Save(fp)
	if err != nil {
		fp.Close()
		return err
	}
	return fp.Close()
}

// Returns the items in the cache. This may include items that have expired,
// but have not yet been cleaned up. If this is significant, the Expiration
// fields of the items should be checked. Note that explicit synchronization
// is needed to use a cache and its corresponding Items() return values at
// the same time, as the maps are shared.
func (sc *CachePools[T]) Items() []map[string]Item[T] {
	res := make([]map[string]Item[T], len(sc.cs))
	for i, v := range sc.cs {
		res[i] = v.Items()
	}
	return res
}

func (sc *CachePools[T]) Flush() {
	for _, v := range sc.cs {
		v.Flush()
	}
}

func newCachePools[T any](n int, de time.Duration) *CachePools[T] {
	max := big.NewInt(0).SetUint64(uint64(math.MaxUint32))
	rnd, err := rand.Int(rand.Reader, max)
	var seed uint32
	if err != nil {
		os.Stderr.Write([]byte("WARNING: goramcache's newCachePools failed to read from the system CSPRNG (/dev/urandom or equivalent.) Your system's security may be compromised. Continuing with an insecure seed.\n"))
		seed = insecurerand.Uint32()
	} else {
		seed = uint32(rnd.Uint64())
	}
	sc := &CachePools[T]{
		seed: seed,
		m:    uint32(n),
		cs:   make([]*cache[T], n),
	}
	for i := 0; i < n; i++ {
		c := &cache[T]{ //not via NewCache to disable janitor
			defaultExpiration: de,
			items:             map[string]Item[T]{},
		}
		sc.cs[i] = c
	}
	return sc
}

func NewCachePools[T any](defaultExpiration, errorAllowTimeExpiration time.Duration, numpools int) *CachePools[T] {
	if defaultExpiration == 0 {
		defaultExpiration = -1
	}
	sc := newCachePools[T](numpools, defaultExpiration)
	if errorAllowTimeExpiration > 0 {
		sc.janitor = NewJanitor(errorAllowTimeExpiration)
		sc.janitor.Start(sc, sc.deleteExpired)
	}
	return sc
}
