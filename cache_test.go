package main_test

import (
	. "github.com/motemen/etch"
	. "github.com/smartystreets/goconvey/convey"
	"io/ioutil"
	"net/url"
	"testing"
	"time"
)

func TestCache(t *testing.T) {
	Convey("A etch.Cache instance", t, func() {
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

		Convey("UrlToFilePath(url)", func() {
			So(cache.UrlToFilePath(url), ShouldEqual, tmpDir+"/toro.2ch.net/book/dat/1363665368.dat")
		})
	})
}

func TestCacheEntry(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "etch_test")
	if err != nil {
		t.Fatal(err)
	}

	url, err := url.Parse("http://toro.2ch.net/book/dat/1363665368.dat")
	if err != nil {
		t.Fatal("Pargins URL failed: ", err)
	}

	t.Log("Cache root: ", tmpDir)

	cache := &Cache{tmpDir}

	Convey("A CacheEntry", t, func() {
		entry := cache.GetEntry(url)

		Convey("FilePath", func() {
			So(entry.FilePath, ShouldEqual, tmpDir+"/toro.2ch.net/book/dat/1363665368.dat")
		})

		Convey("Does not exist at this time", func() {
			So(entry.Exists(), ShouldBeFalse)
		})

		Convey("FreshenContent(content, mtime)", func() {
			updated, err := entry.FreshenContent(([]byte)("foobar"), time.Now())
			So(updated, ShouldBeTrue)
			So(err, ShouldBeNil)
		})
	})

	Convey("A CacheEntry of the same url", t, func() {
		entry := cache.GetEntry(url)

		Convey("Now exists", func() {
			So(entry.Exists(), ShouldBeTrue)
		})

		Convey("GetContent()", func() {
			content, err := entry.GetContent()
			So(err, ShouldBeNil)
			So(content, ShouldResemble, []byte("foobar"))
		})
	})
}
