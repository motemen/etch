package main

import (
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
	sync.RWMutex
}

func (cacheEntry CacheEntry) LogPrefixValue() interface{} {
	return cacheEntry.FilePath
}

func (cacheEntry CacheEntry) LogGroup() string {
	return "cache"
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
	return &CacheEntry{FilePath: filePath}
}

func (cacheEntry *CacheEntry) GetContent() ([]byte, time.Time, error) {
	cacheEntry.RLock()
	defer cacheEntry.RUnlock()

	fileInfo, err := os.Stat(cacheEntry.FilePath)
	if err != nil {
		return nil, time.Time{}, err
	}

	content, err := ioutil.ReadFile(cacheEntry.FilePath)
	if err != nil {
		return nil, time.Time{}, err
	}

	return content, fileInfo.ModTime(), nil
}

func (cacheEntry *CacheEntry) FreshenContent(content []byte, mtime time.Time) (bool, error) {
	cacheEntry.Lock()
	defer cacheEntry.Unlock()

	tracef(cacheEntry, "FreshenContent")

	fileInfo, _ := os.Stat(cacheEntry.FilePath)

	if fileInfo != nil && mtime.Before(fileInfo.ModTime()) {
		infof(cacheEntry, "FreshenContent: mtime is not fresher than cache entry: %s < %s", mtime, cacheEntry)
		return false, nil
	}

	dir, _ := path.Split(cacheEntry.FilePath)

	if err := os.MkdirAll(dir, 0777); err != nil {
		return false, err
	}

	debugf(cacheEntry, "Writing content")

	if err := ioutil.WriteFile(cacheEntry.FilePath, content, 0666); err != nil {
		return false, err
	}

	debugf(cacheEntry, "Setting mtime to %s", mtime)

	if err := os.Chtimes(cacheEntry.FilePath, mtime, mtime); err != nil {
		return false, err
	} else {
		return true, nil
	}
}

func (cacheEntry *CacheEntry) Delete() error {
	cacheEntry.Lock()
	defer cacheEntry.Unlock()
	return os.Remove(cacheEntry.FilePath)
}
