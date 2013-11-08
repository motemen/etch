package main_test

import (
	. "github.com/motemen/etch"
	"io/ioutil"
	"net/url"
	"testing"
)

func TestCache(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "etch_test")
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Cache root: ", tmpDir)

	cache := &Cache{tmpDir}

	url, err := url.Parse("http://toro.2ch.net/book/dat/1363665368.dat")
	if err != nil {
		t.Fatal("Pargins URL failed: ", err)
	}

	if cache.UrlToFilePath(url) != tmpDir+"/toro.2ch.net/book/dat/1363665368.dat" {
		t.Error("cache.UrlToFilePath failed")
	}

	if err := cache.Set(url, "foobar"); err != nil {
		t.Fatal("cache.Set failed: ", err)
	}

	content, err := cache.Get(url)
	if err != nil {
		t.Fatal("cache.Get failed: ", err)
	}

	if content != "foobar" {
		t.Fatal("cache.Get failed: content mismatch")
	}
}
