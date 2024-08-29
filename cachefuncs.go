package goramcache

import (
	"fmt"
	"time"

	"github.com/sonnt85/gosutils/funcmap"
)

type CacheFuncs struct {
	// *Cache[string, string]
	*Cache[string, *funcmap.Task[string]]
}

func NewCacheFuncs(defaultExpiration, errorAllowTimeExpiration time.Duration) *CacheFuncs {
	cf := CacheFuncs{
		Cache: NewCache[string, *funcmap.Task[string]](defaultExpiration, errorAllowTimeExpiration),
	}
	// cf.OnEvicted(func(k string, v funcmap.Task[string]) {
	// })
	return &cf
}

func (cf *CacheFuncs) GetFunc(fname string) (*funcmap.Task[string], bool) {
	return cf.GetWithDefaultExpirationUpdate(fname)
}

func (cf *CacheFuncs) CallFunc(fname string) (results []interface{}, err error) {
	if f, ok := cf.GetWithDefaultExpirationUpdate(fname); ok {
		return f.Call()
	} else {
		return nil, fmt.Errorf("func %s does not exist", fname)
	}
}

func (cf *CacheFuncs) AddFunc(fname string, f interface{}, params ...interface{}) (task *funcmap.Task[string], err error) {
	var fid func() string
	task, err = funcmap.NewTask(fname, fid, f, params...)
	if err != nil {
		return
	}
	// task.Id
	err = cf.Add(fname, task, DefaultExpiration)
	return
}
func (cf *CacheFuncs) AddFuncIfNotExist(fname string, f interface{}, params ...interface{}) (task *funcmap.Task[string], err error) {
	var fid func() string
	var ok bool
	if task, ok = cf.GetWithDefaultExpirationUpdate(fname); ok {
		return
	}
	task, err = funcmap.NewTask(fname, fid, f, params...)
	if err != nil {
		return
	}

	err = cf.Add(fname, task, DefaultExpiration)
	return
}
