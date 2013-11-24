package main

import (
	"bytes"
	"github.com/elazarl/goproxy"
	"io/ioutil"
	"net/http"
)

type EtchControlServer struct {
	etch *EtchProxy
}

type responseWriter struct {
	statusCode int
	header     http.Header
	buffer     *bytes.Buffer
}

func (rw *responseWriter) Header() http.Header {
	return rw.header
}

func (rw *responseWriter) Write(content []byte) (int, error) {
	return rw.buffer.Write(content)
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
}

func (rw *responseWriter) Response(req *http.Request) *http.Response {
	resp := &http.Response{}
	resp.Request = req
	resp.Header = rw.header
	resp.StatusCode = rw.statusCode
	resp.ContentLength = int64(rw.buffer.Len())
	resp.Body = ioutil.NopCloser(rw.buffer)
	return resp
}

func newResponseWriter() *responseWriter {
	return &responseWriter{200, make(http.Header), bytes.NewBuffer([]byte(""))}
}

// プロキシでないリクエストを処理

func (proxy *EtchProxy) HandleNonProxyRequest(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	if req.URL.Host != "" {
		return req, nil
	}

	rw := newResponseWriter()
	proxy.ControlMux.ServeHTTP(rw, req)

	return req, rw.Response(req)
}

func setupControlMux(proxy *EtchProxy) {
	proxy.ControlMux.HandleFunc("/", func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Add("Content-Type", "text/plain; charset=utf-8")

		keys := proxy.Cache.Keys()
		for _, key := range(keys) {
			rw.Write([]byte(key.String()))
		}
	})
}
