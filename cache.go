package main

import (
	"github.com/golang/glog"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

type Cache struct {
	Root string
}

type CacheEntry struct {
	FilePath string
	os.FileInfo
}

func (cache *Cache) UrlToFilePath(url *url.URL) string {
	s := []string{cache.Root, url.Host}
	s = append(s, strings.Split(url.Path, "/")...)
	return path.Join(s...)
}

func (cache *Cache) Files() ([]string) {
	files := make([]string, 0)
	filepath.Walk(cache.Root, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() == false {
			files = append(files, path)
		}
		return nil
	})
	return files
}

func (cache *Cache) Get(url *url.URL) (string, error) {
	filePath := cache.UrlToFilePath(url)
	bytes, err := ioutil.ReadFile(filePath)
	return string(bytes), err
}

func (cache *Cache) Set(url *url.URL, content string) error {
	filePath := cache.UrlToFilePath(url)
	dir, _ := path.Split(filePath)

	if err := os.MkdirAll(dir, 0777); err != nil {
		return err
	}

	return ioutil.WriteFile(filePath, []byte(content), 0666)
}

func (cache *Cache) GetWriter(url *url.URL) (io.Writer, error) {
	filePath := cache.UrlToFilePath(url)
	glog.Infof("GetWriter: %s -> %s", url, filePath)

	dir, _ := path.Split(filePath)

	if err := os.MkdirAll(dir, 0777); err != nil {
		return nil, err
	}

	return os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0666)
}

func (cache *Cache) GetEntry(url *url.URL) *CacheEntry {
	filePath := cache.UrlToFilePath(url)
	fileInfo, _ := os.Stat(filePath)
	return &CacheEntry{filePath, fileInfo}
}

func (cacheEntry *CacheEntry) Content() ([]byte, error) {
	return ioutil.ReadFile(cacheEntry.FilePath)
}

func (cacheEntry *CacheEntry) GetWriter() (io.WriteCloser, error) {
	glog.Infof("[%s] GetWriter", cacheEntry.FilePath)

	dir, _ := path.Split(cacheEntry.FilePath)

	if err := os.MkdirAll(dir, 0777); err != nil {
		return nil, err
	}

	return os.OpenFile(cacheEntry.FilePath, os.O_WRONLY|os.O_CREATE, 0666)
}

func (cacheEntry *CacheEntry) SetMtime(mtime time.Time) error {
	glog.Infof("[%s] Setting mtime to %s", cacheEntry.FilePath, mtime)
	return os.Chtimes(cacheEntry.FilePath, mtime, mtime)
}

func (cacheEntry *CacheEntry) Exists() bool {
	return cacheEntry.FileInfo != nil
}
