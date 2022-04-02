package goramcache

import (
	"encoding/gob"
	"fmt"
	"io"
	"math"
	"os"
	"reflect"
	"regexp"
	"sort"
	"sync"
	"time"
)

type Item[T any] struct {
	Object     T
	Expiration int64
}

// Returns true if the item has expired.
func (item Item[T]) Expired() bool {
	return (item.Expiration != 0) && (time.Now().UnixNano() > item.Expiration)
}

const (
	NoExpirationCheck time.Duration = -1
	// For use with functions that take an expiration time.
	NoExpiration time.Duration = -1
	// For use with functions that take an expiration time. Equivalent to
	// passing in the same expiration duration as was given to NewCache() or
	// NewCacheFrom() when the cache was created (e.g. 5 minutes.)
	DefaultExpiration time.Duration = 0
)

type cache[T any] struct {
	errorAllowTimeExpiration int64
	defaultExpiration        time.Duration
	items                    map[string]Item[T]
	mu                       sync.RWMutex
	onEvicted                func(string, T)
}

type Cache[T any] struct {
	*cache[T]
	janitor *Janitor
}

// Add an item to the cache, replacing any existing item. If the duration is 0
// (DefaultExpiration), the cache's default expiration time is used. If it is -1
// (NoExpiration), the item never expires.
func (c *cache[T]) Set(k string, x T, d time.Duration) {
	// "Inlining" of set
	var e int64
	if d == DefaultExpiration {
		d = c.defaultExpiration
	}
	if d > 0 {
		e = time.Now().Add(d).UnixNano()
	}
	c.mu.Lock()
	c.items[k] = Item[T]{
		Object:     x,
		Expiration: e,
	}
	c.mu.Unlock()
}

func (c *cache[T]) set(k string, x T, d time.Duration) {
	var e int64
	if d == DefaultExpiration {
		d = c.defaultExpiration
	}
	if d > 0 {
		e = time.Now().Add(d).UnixNano()
	}
	c.items[k] = Item[T]{
		Object:     x,
		Expiration: e,
	}
}

// Add an item to the cache, replacing any existing item, using the default
// expiration.
func (c *cache[T]) SetDefault(k string, x T) {
	c.Set(k, x, DefaultExpiration)
}

// Add an item to the cache only if an item doesn't already exist for the given
// key, or if the existing item has expired. Returns an error otherwise.
func (c *cache[T]) Add(k string, x T, d time.Duration) error {
	c.mu.Lock()
	_, found := c.get(k)
	if found {
		c.mu.Unlock()
		return fmt.Errorf("Item %s already exists", k)
	}
	c.set(k, x, d)
	c.mu.Unlock()
	return nil
}

func (c *cache[T]) Edit(k string, x interface{}, apFunc func(T, interface{}) (T, error)) error { //TODO update to run
	c.mu.Lock()
	v, found := c.items[k]
	if !found || v.Expired() {
		c.mu.Unlock()
		return fmt.Errorf("Item %q not found", k)
	}
	edited, err := apFunc(v.Object, x)
	if err == nil {
		v.Object = edited
		c.items[k] = v
	}
	c.mu.Unlock()
	return err
}

// Set a new value for the cache key only if it already exists, and the existing
// item hasn't expired. Returns an error otherwise.
func (c *cache[T]) Replace(k string, x T, d time.Duration) error {
	c.mu.Lock()
	_, found := c.get(k)
	if !found {
		c.mu.Unlock()
		return fmt.Errorf("Item %s doesn't exist", k)
	}
	c.set(k, x, d)
	c.mu.Unlock()
	return nil
}

// Get an item from the cache. Returns the item or nil, and a bool indicating
// whether the key was found.
func (c *cache[T]) Get(k string) (T, bool) {
	var zero T
	c.mu.RLock()
	// "Inlining" of get and Expired
	item, found := c.items[k]
	if !found {
		c.mu.RUnlock()
		return zero, false
	}
	if item.Expiration > 0 {
		if time.Now().UnixNano() > item.Expiration {
			c.mu.RUnlock()
			return zero, false
		}
	}
	c.mu.RUnlock()
	return item.Object, true
}

func (c *cache[T]) GetThenDelete(k string) (T, bool) {
	c.mu.Lock()
	t, ok := c.get(k)
	if ok {
		c.delete(k)
	}
	c.mu.Unlock()
	return t, ok
}

func (c *cache[T]) GetOrCreateNew(k string) (T, bool) {
	c.mu.RLock()
	if v, ok := c.get(k); ok {
		c.mu.Unlock()
		return v, false
	} else {
		var zero T
		value := reflect.ValueOf(zero)
		if value.Kind() == reflect.Pointer {
			// value.Elem()
			newvalue := reflect.New(value.Elem().Type())
			zero = reflect.ValueOf(newvalue.Addr()).Interface().(T)
		}
		c.mu.Unlock()
		c.SetDefault(k, zero)
		return zero, true
	}
}

// GetWithExpirationUpdate returns item and updates its cache expiration time
// It returns the item or nil, the expiration time if one is set (if the item
// never expires a zero value for time.Time is returned), and a bool indicating
// whether the key was found.
func (c *cache[T]) GetWithExpirationUpdate(k string, d time.Duration) (T, bool) {
	var zero T
	c.mu.RLock()
	item, found := c.items[k]
	if !found {
		c.mu.RUnlock()
		return zero, false
	}
	if item.Expiration > 0 {
		if time.Now().UnixNano() > item.Expiration {
			c.mu.RUnlock()
			return zero, false
		}
	}
	c.mu.RUnlock()

	c.mu.Lock()
	if d == DefaultExpiration {
		d = c.defaultExpiration
	}
	if d > 0 {
		item.Expiration = time.Now().Add(d).UnixNano()
	}
	c.items[k] = item
	c.mu.Unlock()

	return item.Object, true
}

func (c *cache[T]) GetWithDefaultExpirationUpdate(k string) (T, bool) {
	return c.GetWithExpirationUpdate(k, DefaultExpiration)
}

// Keys returns a sorted slice of all the keys in the cache.
func (c *cache[T]) Keys() []string {
	var i int
	c.mu.RLock()
	keys := make([]string, len(c.items))
	for k := range c.items {
		keys[i] = k
		i++
	}
	c.mu.RUnlock()
	sort.Strings(keys)
	return keys
}

func (c *cache[T]) Values() []T {
	var i int
	now := time.Now().UnixNano()
	c.mu.RLock()
	values := make([]T, len(c.items))
	for _, v := range c.items {
		if v.Expiration > 0 && now > v.Expiration {
			continue
		}
		values[i] = v.Object
		i++
	}
	c.mu.RUnlock()
	return values
}

func (c *cache[T]) Length() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

//GetMultipleItems returns an array of items corresponding to the input array
func (c *cache[T]) GetMultipleItems(keys []string) []T {
	length := len(keys)
	var items = make([]T, length)
	c.mu.RLock()
	for i := 0; i < length; i++ {
		item, _ := c.get(keys[i])
		items[i] = item
	}
	c.mu.RUnlock()
	return items
}

func (c *cache[T]) IncrementExpiration(k string, d time.Duration) error {
	c.mu.Lock()
	v, found := c.items[k]
	if !found || v.Expired() {
		c.mu.Unlock()
		return fmt.Errorf("key %s not found.", k)
	}

	var e int64
	if d == DefaultExpiration {
		d = c.defaultExpiration
	}
	if d > 0 {
		e = time.Now().Add(d).UnixNano()
	}
	v.Expiration = e

	c.mu.Unlock()
	return nil
}

// GetWithExpiration returns an item and its expiration time from the cache.
// It returns the item or nil, the expiration time if one is set (if the item
// never expires a zero value for time.Time is returned), and a bool indicating
// whether the key was found.
func (c *cache[T]) GetWithExpiration(k string) (T, time.Time, bool) {
	var zero T
	c.mu.RLock()
	// "Inlining" of get and Expired
	item, found := c.items[k]
	if !found {
		c.mu.RUnlock()
		return zero, time.Time{}, false
	}

	if item.Expiration > 0 {
		if time.Now().UnixNano() > item.Expiration {
			c.mu.RUnlock()
			return zero, time.Time{}, false
		}

		// Return the item and the expiration time
		c.mu.RUnlock()
		return item.Object, time.Unix(0, item.Expiration), true
	}

	// If expiration <= 0 (i.e. no expiration time set) then return the item
	// and a zeroed time.Time
	c.mu.RUnlock()
	return item.Object, time.Time{}, true
}

func (c *cache[T]) get(k string) (T, bool) {
	item, found := c.items[k]
	var zero T
	if !found {
		return zero, false
	}
	// "Inlining" of Expired
	if item.Expiration > 0 {
		if time.Now().UnixNano() > item.Expiration {
			return zero, false
		}
	}
	return item.Object, true
}

// Increment and return an item of type int, int8, int16, int32, int64, uintptr, uint,
// uint8, uint32, or uint64, float32 or float64 by n. Returns an error if the
// item's value is not an integer, if it was not found, or if it is not
// possible to increment it by n.
func (c *cache[T]) Increment(k string, n int64) (T, error) {

	// TODO: Consider adding a constraint to avoid the type switch and provide
	// compile-time safety
	var zero T
	c.mu.Lock()
	v, found := c.items[k]
	if !found || v.Expired() {
		c.mu.Unlock()
		return zero, fmt.Errorf("Item %s not found", k)
	}
	// Generics does not (currently?) support type switching
	// To workaround, we convert the value into a interface{}, and switching on that
	var untypedValue interface{}

	untypedValue = v.Object
	switch untypedValue.(type) {
	case int:
		untypedValue = untypedValue.(int) + int(n)
	case int8:
		untypedValue = untypedValue.(int8) + int8(n)
	case int16:
		untypedValue = untypedValue.(int16) + int16(n)
	case int32:
		untypedValue = untypedValue.(int32) + int32(n)
	case int64:
		untypedValue = untypedValue.(int64) + n
	case uint:
		untypedValue = untypedValue.(uint) + uint(n)
	case uintptr:
		untypedValue = untypedValue.(uintptr) + uintptr(n)
	case uint8:
		untypedValue = untypedValue.(uint8) + uint8(n)
	case uint16:
		untypedValue = untypedValue.(uint16) + uint16(n)
	case uint32:
		untypedValue = untypedValue.(uint32) + uint32(n)
	case uint64:
		untypedValue = untypedValue.(uint64) + uint64(n)
	case float32:
		untypedValue = untypedValue.(float32) + float32(n)
	case float64:
		untypedValue = untypedValue.(float64) + float64(n)
	default:
		c.mu.Unlock()
		return zero, fmt.Errorf("The value for %s is not an integer", k)
	}
	v.Object = untypedValue.(T)
	c.items[k] = v
	c.mu.Unlock()
	return zero, nil
}

// Decrement and return an item of type int, int8, int16, int32, int64, uintptr, uint,
// uint8, uint32, or uint64, float32 or float64 by n. Returns an error if the
// item's value is not an integer, if it was not found, or if it is not
// possible to decrement it by n.
func (c *cache[T]) Decrement(k string, n int64) (T, error) {

	// TODO: Consider adding a constraint to avoid the type switch and provide
	// compile-time safety
	c.mu.Lock()
	var zero T
	v, found := c.items[k]
	if !found || v.Expired() {
		c.mu.Unlock()
		return zero, fmt.Errorf("Item %s not found", k)
	}
	// Generics does not (currently?) support type switching
	// To workaround, we convert the value into a interface{}, and switching on that
	var untypedValue interface{}

	untypedValue = v.Object
	switch untypedValue.(type) {
	case int:
		untypedValue = untypedValue.(int) - int(n)
	case int8:
		untypedValue = untypedValue.(int8) - int8(n)
	case int16:
		untypedValue = untypedValue.(int16) - int16(n)
	case int32:
		untypedValue = untypedValue.(int32) - int32(n)
	case int64:
		untypedValue = untypedValue.(int64) - n
	case uint:
		untypedValue = untypedValue.(uint) - uint(n)
	case uintptr:
		untypedValue = untypedValue.(uintptr) - uintptr(n)
	case uint8:
		untypedValue = untypedValue.(uint8) - uint8(n)
	case uint16:
		untypedValue = untypedValue.(uint16) - uint16(n)
	case uint32:
		untypedValue = untypedValue.(uint32) - uint32(n)
	case uint64:
		untypedValue = untypedValue.(uint64) - uint64(n)
	case float32:
		untypedValue = untypedValue.(float32) - float32(n)
	case float64:
		untypedValue = untypedValue.(float64) - float64(n)
	default:
		c.mu.Unlock()
		return zero, fmt.Errorf("The value for %s is not an integer", k)
	}
	v.Object = untypedValue.(T)
	c.items[k] = v
	c.mu.Unlock()
	return zero, nil
}

// Delete an item from the cache. Does nothing if the key is not in the cache.
func (c *cache[T]) Delete(k string) {
	c.mu.Lock()
	v, evicted := c.delete(k)
	c.mu.Unlock()
	if evicted {
		c.onEvicted(k, v)
	}
}

func (c *cache[T]) DeleteRegex(rule string) {
	re, _ := regexp.Compile(rule)
	for k := range c.items {
		if re.MatchString(k) {
			c.Delete(k)
		}
	}
}

func (c *cache[T]) delete(k string) (T, bool) {
	var zero T
	if c.onEvicted != nil {
		if v, found := c.items[k]; found {
			delete(c.items, k)
			return v.Object, true
		}
	}
	delete(c.items, k)
	return zero, false
}

type keyAndValue[T any] struct {
	key   string
	value T
}

// Delete all expired items from the cache.
func (c *cache[T]) DeleteExpired() {
	var evictedItems []keyAndValue[T]
	now := time.Now().UnixNano()
	c.mu.Lock()
	for k, v := range c.items {
		// "Inlining" of expired
		if v.Expiration > 0 && now > v.Expiration {
			ov, evicted := c.delete(k)
			if evicted {
				evictedItems = append(evictedItems, keyAndValue[T]{k, ov})
			}
		}
	}
	c.mu.Unlock()
	for _, v := range evictedItems {
		c.onEvicted(v.key, v.value)
	}
}

// Delete all expired items from the cache, call by janitor
func (c *cache[T]) deleteExpired() (nextTimeCheck time.Time, needUpdate bool) {
	var evictedItems []keyAndValue[T]
	var minExpiration int64
	now := time.Now().UnixNano()
	minExpiration = math.MaxInt64
	c.mu.Lock()
	for k, v := range c.items {
		if v.Expiration > 0 {
			if now+c.errorAllowTimeExpiration > v.Expiration {
				ov, evicted := c.delete(k)
				if evicted {
					evictedItems = append(evictedItems, keyAndValue[T]{k, ov})
				}
			} else {
				if v.Expiration < minExpiration {
					minExpiration = v.Expiration
				}
			}
		}
	}
	needUpdate = len(c.items) != 0
	nextTimeCheck = time.UnixMicro(minExpiration / 1000)
	c.mu.Unlock()
	for _, v := range evictedItems {
		c.onEvicted(v.key, v.value)
	}
	return
}

// Sets an (optional) function that is called with the key and value when an
// item is evicted from the cache. (Including when it is deleted manually, but
// not when it is overwritten.) Set to nil to disable.
func (c *cache[T]) OnEvicted(f func(string, T)) {
	c.mu.Lock()
	c.onEvicted = f
	c.mu.Unlock()
}

// Write the cache's items (using Gob) to an io.Writer.
//
// NOTE: This method is deprecated in favor of c.Items() and NewFrom() (see the
// documentation for NewFrom().)
func (c *cache[T]) Save(w io.Writer) (err error) {
	enc := gob.NewEncoder(w)
	defer func() {
		if x := recover(); x != nil {
			err = fmt.Errorf("Error Encode item with Gob library")
		}
	}()
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, v := range c.items {
		gob.Register(v.Object)
	}
	err = enc.Encode(&c.items)
	return
}

// Save the cache's items to the given filename, creating the file if it
// doesn't exist, and overwriting it if it does.
//
// NOTE: This method is deprecated in favor of c.Items() and NewFrom() (see the
// documentation for NewFrom().)
func (c *cache[T]) SaveFile(fname string) error {
	fp, err := os.Create(fname)
	if err != nil {
		return err
	}
	err = c.Save(fp)
	if err != nil {
		fp.Close()
		return err
	}
	return fp.Close()
}

// Add (Gob-serialized) cache items from an io.Reader, excluding any items with
// keys that already exist (and haven't expired) in the current cache.
//
// NOTE: This method is deprecated in favor of c.Items() and NewFrom() (see the
// documentation for NewFrom().)
func (c *cache[T]) Load(r io.Reader) error {
	dec := gob.NewDecoder(r)
	items := map[string]Item[T]{}
	err := dec.Decode(&items)
	if err == nil {
		c.mu.Lock()
		defer c.mu.Unlock()
		for k, v := range items {
			ov, found := c.items[k]
			if !found || ov.Expired() {
				c.items[k] = v
			}
		}
	}
	return err
}

// Load and add cache items from the given filename, excluding any items with
// keys that already exist in the current cache.
//
// NOTE: This method is deprecated in favor of c.Items() and NewFrom() (see the
// documentation for NewFrom().)
func (c *cache[T]) LoadFile(fname string) error {
	fp, err := os.Open(fname)
	if err != nil {
		return err
	}
	err = c.Load(fp)
	if err != nil {
		fp.Close()
		return err
	}
	return fp.Close()
}

// Iterate every item by item handle items from cache,and if the handle returns to false,
// it will be interrupted and return false.
func (c *cache[T]) Iterate(f func(key string, item T) bool) bool {
	now := time.Now().UnixNano()
	c.mu.RLock()
	keys := make([]string, len(c.items))
	i := 0
	for k, v := range c.items {
		// "Inlining" of Expired
		if v.Expiration > 0 && now > v.Expiration {
			continue
		}
		keys[i] = k
		i++
	}
	c.mu.RUnlock()
	keys = keys[:i]
	for _, key := range keys {
		c.mu.RLock()
		item, ok := c.items[key]
		c.mu.RUnlock()
		if !ok {
			continue
		}
		if !f(key, item.Object) {
			return false
		}
	}
	return true
}

// Copies all unexpired items in the cache into a new map and returns it.
func (c *cache[T]) Items() map[string]Item[T] {
	c.mu.RLock()
	defer c.mu.RUnlock()
	m := make(map[string]Item[T], len(c.items))
	now := time.Now().UnixNano()
	for k, v := range c.items {
		if v.Expiration > 0 {
			if now > v.Expiration {
				continue
			}
		}
		m[k] = v
	}
	return m
}

// Returns the number of items in the cache. This may include items that have
// expired, but have not yet been cleaned up.
func (c *cache[T]) ItemCount() int {
	c.mu.RLock()
	n := len(c.items)
	c.mu.RUnlock()
	return n
}

// Delete all items from the cache.
func (c *cache[T]) Flush() {
	c.mu.Lock()
	c.items = map[string]Item[T]{}
	c.mu.Unlock()
}

func (c *Cache[T]) SetNextCheckExpireate(d time.Duration) {
	c.janitor.SetNextCheckExpire(d)
}

func newcache[T any](de, errorAllowTimeExpiration time.Duration, m map[string]Item[T]) *cache[T] {
	if de == 0 {
		de = -1
	}
	c := &cache[T]{
		defaultExpiration: de,
		items:             m,
	}
	return c
}

func newCache[T any](de time.Duration, errorAllowTimeExpiration time.Duration, m map[string]Item[T]) *Cache[T] {
	c := newcache(de, errorAllowTimeExpiration, m)
	C := &Cache[T]{
		cache: c,
	}
	if errorAllowTimeExpiration > 0 {
		C.janitor = NewJanitor(errorAllowTimeExpiration)
		C.janitor.Start(c, c.deleteExpired)
	}
	return C
}

// Return a new cache with a given default expiration duration and cleanup
// interval. If the expiration duration is less than one (or NoExpiration),
// the items in the cache never expire (by default), and must be deleted
// manually. If the cleanup errorAllowTimeExpiration is less than one, expired items are not
// deleted from the cache before calling c.DeleteExpired().
func NewCache[T any](defaultExpiration, errorAllowTimeExpiration time.Duration) *Cache[T] {
	items := make(map[string]Item[T])
	return newCache[T](defaultExpiration, errorAllowTimeExpiration, items)
}

// Return a new cache with a given default expiration duration and cleanup
// interval. If the expiration duration is less than one (or NoExpiration),
// the items in the cache never expire (by default), and must be deleted
// manually. If the cleanup interval is less than one, expired items are not
// deleted from the cache before calling c.DeleteExpired().
//
// NewFrom() also accepts an items map which will serve as the underlying map
// for the cache. This is useful for starting from a deserialized cache
// (serialized using e.g. gob.Encode() on c.Items()), or passing in e.g.
// make(map[string]Item, 500) to improve startup performance when the cache
// is expected to reach a certain minimum size.
//
// Only the cache's methods synchronize access to this map, so it is not
// recommended to keep any references to the map around after creating a cache.
// If need be, the map can be accessed at a later point using c.Items() (subject
// to the same caveat.)
//
// Note regarding serialization: When using e.g. gob, make sure to
// gob.Register() the individual types stored in the cache before encoding a
// map retrieved with c.Items(), and to register those same types before
// decoding a blob containing an items map.
func NewCacheFrom[T any](defaultExpiration, errorAllowTimeExpiration time.Duration, items map[string]Item[T]) *Cache[T] {
	return newCache(defaultExpiration, errorAllowTimeExpiration, items)
}
