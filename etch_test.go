package main_test

import (
	. "github.com/motemen/etch"
	. "github.com/smartystreets/goconvey/convey"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func init() {
	http.DefaultServeMux.Handle("/200.dat", &OKHandler{})
}

type OKHandler struct{}

func (h OKHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const (
		ContentAtFirst = "OK<>1<>dat\n"
		ContentDelta   = "delta<>2\n"
	)

	w.Header().Add("Content-Type", "text/plain")

	if r.Header.Get("Range") == "" {
		w.Write([]byte(ContentAtFirst))
	} else {
		w.WriteHeader(206)
		w.Write([]byte(ContentAtFirst[len(ContentAtFirst)-1:]))
		w.Write([]byte(ContentDelta))
	}
}

func Test200(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "etch_test")
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Cache root: %s", tmpDir)

	proxy := NewEtchProxy(tmpDir)

	es := httptest.NewServer(nil)
	defer es.Close()

	ps := httptest.NewServer(proxy)
	defer ps.Close()

	proxyURL, _ := url.Parse(ps.URL)
	tr := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
	client := &http.Client{Transport: tr}

	Convey("An EtchProxy", t, func() {
		Convey("When requested for a live URL", func() {
			resp, err := client.Get(es.URL + "/200.dat")
			if err != nil {
				t.Fatal(err)
			}
			content, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}

			Convey("Returns sane content", func() {
				So(string(content), ShouldEqual, "OK<>1<>dat\n")
			})
		})

		Convey("When requested for the same URL again", func() {
			resp2, err := client.Get(es.URL + "/200.dat")
			if err != nil {
				t.Fatal(err)
			}
			content, err := ioutil.ReadAll(resp2.Body)
			if err != nil {
				t.Fatal(err)
			}

			Convey("Returns sane content, with delta", func() {
				So(string(content), ShouldEqual, "OK<>1<>dat\ndelta<>2\n")
			})
		})
	})
}
