package main_test

import (
	. "github.com/motemen/etch"
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

	resp, err := client.Get(es.URL + "/200.dat")
	if err != nil {
		t.Fatal(err)
	}
	if content, err := ioutil.ReadAll(resp.Body); err != nil {
		t.Fatal(err)
	} else if string(content) == "OK<>1<>dat\n" {
	} else {
		t.Fail()
	}

	resp2, err := client.Get(es.URL + "/200.dat")
	if err != nil {
		t.Fatal(err)
	}
	if content, err := ioutil.ReadAll(resp2.Body); err != nil {
		t.Fatal(err)
	} else if string(content) == "OK<>1<>dat\ndelta<>2\n" {
	} else {
		t.Fail()
	}
}
