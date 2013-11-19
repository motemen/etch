package main

import (
	"github.com/howbazaar/loggo"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Cache struct {
	Root string
}

type CacheEntry struct {
	FilePath string
	os.FileInfo
	sync.RWMutex
	loggo.Logger
}

func (proxy *Cache) GetLogger() loggo.Logger {
	return loggo.GetLogger("cache")
}

func (cache *Cache) UrlToFilePath(url *url.URL) string {
	s := []string{cache.Root, url.Host}
	s = append(s, strings.Split(url.Path, "/")...)
	return path.Join(s...)
}

func (cache *Cache) Keys() []*url.URL {
	keys := make([]*url.URL, 0)
	filepath.Walk(cache.Root, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(cache.Root, path)
		if err == nil {
			pathParts := filepath.SplitList(relPath)
			url := &url.URL{Scheme: "http", Host: pathParts[0], Path: strings.Join(pathParts[1:], "/")}
			keys = append(keys, url)
		}

		return nil
	})
	return keys
}

func (cache *Cache) GetEntry(url *url.URL) *CacheEntry {
	filePath := cache.UrlToFilePath(url)
	fileInfo, _ := os.Stat(filePath)
	return &CacheEntry{FilePath: filePath, FileInfo: fileInfo, Logger: cache.GetLogger()}
}

func (cacheEntry *CacheEntry) GetContent() ([]byte, error) {
	cacheEntry.RLock()
	defer cacheEntry.RUnlock()
	return ioutil.ReadFile(cacheEntry.FilePath)
}

func (cacheEntry *CacheEntry) FreshenContent(content []byte, mtime time.Time) (bool, error) {
	cacheEntry.Lock()
	defer cacheEntry.Unlock()

	fileInfo, _ := os.Stat(cacheEntry.FilePath)

	if fileInfo != nil && !fileInfo.ModTime().Before(mtime) {
		cacheEntry.Logger.Infof("FreshenContent: mtime is not fresher than cache entry: %s <= %s", mtime, cacheEntry)
		return false, nil
	}

	cacheEntry.Logger.Debugf("[%s] FreshenContent()", cacheEntry.FilePath)

	dir, _ := path.Split(cacheEntry.FilePath)

	if err := os.MkdirAll(dir, 0777); err != nil {
		return false, err
	}

	cacheEntry.Logger.Debugf("[%s] Writing content", cacheEntry.FilePath)

	if err := ioutil.WriteFile(cacheEntry.FilePath, content, 0666); err != nil {
		return false, err
	}

	cacheEntry.Logger.Debugf("[%s] Setting mtime to %s", cacheEntry.FilePath, mtime)

	if err := os.Chtimes(cacheEntry.FilePath, mtime, mtime); err != nil {
		return false, err
	} else {
		return true, nil
	}
}

func (cacheEntry *CacheEntry) Exists() bool {
	return cacheEntry.FileInfo != nil
}

func (cacheEntry *CacheEntry) Delete() error {
	cacheEntry.Lock()
	defer cacheEntry.Unlock()
	return os.Remove(cacheEntry.FilePath)
}
