package goramcache

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sonnt85/gosutils/sutils"
)

type CacheFiles struct {
	*Cache[string]
	rootPath string
}

func NewCacheFiles(rootDir string, defaultExpiration, errorAllowTimeExpiration time.Duration) *CacheFiles {
	cf := CacheFiles{
		Cache:    NewCache[string](defaultExpiration, errorAllowTimeExpiration),
		rootPath: rootDir,
	}
	cf.OnEvicted(func(k string, v string) {
		if sutils.PathIsFile(v) {
			os.Remove(v)
		}
	})
	os.MkdirAll(rootDir, 0755)
	// if nil != os.MkdirAll(rootDir, 0755) {
	// 	return nil
	// }
	if !sutils.PathIsDir(rootDir) {
		return nil
	}
	for _, f := range sutils.FindFile(rootDir) {
		cf.SetDefault(filepath.Base(f), f)
	}
	return &cf
}

func (cf *CacheFiles) GetCacheFromUrl(fname, url, user, password string) (string, error) {
	if filePath, ok := cf.GetWithDefaultExpirationUpdate(fname); ok && sutils.PathIsFile(filePath) {
		return filePath, nil
	} else { //download file to cache
		if len(url) == 0 {
			return "", fmt.Errorf("Can not download. Link is empty")
		}
		filePath = filepath.Join(cf.rootPath, fname)
		if err := sutils.HTTPDownLoadUrlToFile(url, user, password, false, filePath, time.Minute*30); err == nil {
			cf.SetDefault(fname, filePath)
			return filePath, nil
		} else {
			return "", err
		}
	}
}

func (cf *CacheFiles) GetCacheFileOrCreate(fname string) (pathCacheFile string, isOldFile bool) {
	if filePath, ok := cf.GetWithDefaultExpirationUpdate(fname); ok && sutils.PathIsFile(filePath) {
		return filePath, ok
	} else {
		filePath := filepath.Join(cf.rootPath, fname)
		cf.SetDefault(fname, filePath)
		return filePath, false // creaate new path
	}
}

func (cf *CacheFiles) GetCacheFileOrCreateWithExpiration(fname string, d time.Duration) (pathCacheFile string, isOldFile bool) {
	if filePath, ok := cf.GetWithDefaultExpirationUpdate(fname); ok && sutils.PathIsFile(filePath) {
		return filePath, ok
	} else {
		filePath := filepath.Join(cf.rootPath, fname)
		cf.Set(fname, filePath, d)
		return filePath, false // creaate new path
	}
}

func (cf *CacheFiles) GetCacheFile(fname string) (string, bool) {
	if filePath, ok := cf.GetWithDefaultExpirationUpdate(fname); ok && sutils.PathIsFile(filePath) {
		return filePath, ok
	} else {
		return filepath.Join(cf.rootPath, fname), false // creaate new path
	}
}
