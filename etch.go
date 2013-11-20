package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"github.com/elazarl/goproxy"
	"github.com/howbazaar/loggo"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type EtchProxy struct {
	goproxy.ProxyHttpServer
	Cache        *Cache
	RequestMutex *RequestMutex
}

type EtchContextData struct {
	CachedContent *bytes.Buffer
}

type EtchLogFormatter struct{}

func (*EtchLogFormatter) Format(level loggo.Level, module, filename string, line int, timestamp time.Time, message string) string {
	return fmt.Sprintf(
		"%s %5s [%s] %s:%d %s",
		timestamp.Format("2006-01-02 15:04:05 MST"),
		level,
		module,
		filepath.Base(filename),
		line,
		message)
}

func ReqMethodIs(method string) goproxy.ReqConditionFunc {
	return func(req *http.Request, ctx *goproxy.ProxyCtx) bool {
		return req.Method == method
	}
}

func StatusCodeIs(code int) goproxy.RespCondition {
	return goproxy.RespConditionFunc(func(resp *http.Response, ctx *goproxy.ProxyCtx) bool {
		if resp == nil {
			return false
		}
		return resp.StatusCode == code
	})
}

type RequestMutex struct {
	sync.Mutex
	resChans map[string][]chan *http.Response
}

func NewEtchProxy(cacheDir string) *EtchProxy {
	etch := &EtchProxy{}
	etch.ProxyHttpServer = *goproxy.NewProxyHttpServer()
	etch.Cache = &Cache{cacheDir}
	etch.RequestMutex = &RequestMutex{resChans: make(map[string][]chan *http.Response)}

	setupProxy(etch)

	return etch
}

func (proxy *EtchProxy) GetLogger() loggo.Logger {
	return loggo.GetLogger("proxy")
}

func (proxy *EtchProxy) GuardRequest(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	proxy.RequestMutex.Lock()
	chans, ok := proxy.RequestMutex.resChans[req.URL.String()]

	if ok {
		ch := make(chan *http.Response)
		proxy.RequestMutex.resChans[req.URL.String()] = append(chans, ch)
		proxy.RequestMutex.Unlock()

		proxy.GetLogger().Tracef("[%s] Waiting for ongoing request", req.URL)

		res := <-ch

		proxy.GetLogger().Tracef("[%s] Response got from chan: %s", req.URL, res)

		if res != nil {
			return req, res
		}
	} else {
		proxy.RequestMutex.resChans[req.URL.String()] = make([]chan *http.Response, 0)
		proxy.RequestMutex.Unlock()
	}

	return req, nil
}

func (proxy *EtchProxy) PrepareRangedRequest(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	cache := proxy.Cache

	if entry := cache.GetEntry(req.URL); entry.FileInfo != nil {
		proxy.GetLogger().Infof("%s: found cache entry", req.URL)

		content, mtime, err := entry.GetContent()
		if err != nil {
			proxy.GetLogger().Errorf("OnRequest: retrieving cache content: %s", err)
			return req, nil
		}

		cachedContent := bytes.NewBuffer(content)
		req.Header.Add("Range", fmt.Sprintf("bytes=%d-", cachedContent.Len()-1))
		req.Header.Add("If-Modified-Since", mtime.Format(time.RFC850))

		_, resp, err := proxy.Tr.DetailedRoundTrip(req)
		if err != nil {
			proxy.GetLogger().Errorf("OnRequest: executing request: %s", err)
			return req, nil
		}

		if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
			proxy.GetLogger().Infof("[%s] Got 416: attempting re-fetch", req.URL)

			// clear cache
			req.Header.Del("Range")
			req.Header.Del("If-Modified-Since")

			_, _resp, err := proxy.Tr.DetailedRoundTrip(req)

			if err != nil {
				proxy.GetLogger().Errorf("OnRequest: re-fetch: %s", err)
				return req, nil
			}

			resp = _resp
			cachedContent = new(bytes.Buffer)
		}

		ctx.UserData = &EtchContextData{cachedContent}

		return req, resp
	}

	return req, nil
}

func (proxy *EtchProxy) FixStatusCode(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
	switch resp.StatusCode {
	case http.StatusNonAuthoritativeInfo:
		// dat 落ち
		resp.Header.Add("X-Original-Status-Code", fmt.Sprint(resp.StatusCode))
		resp.StatusCode = http.StatusPaymentRequired
	}

	return resp
}

func (proxy *EtchProxy) RestoreCache(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
	if ctx.UserData == nil {
		return proxy.FixStatusCode(resp, ctx)
	}

	userData := ctx.UserData.(*EtchContextData)

	switch resp.StatusCode {
	case http.StatusOK:
		// nop

	case http.StatusPartialContent:
		buf := userData.CachedContent

		// TODO check Content-Range value
		contentRange := resp.Header.Get("Content-Range")
		proxy.GetLogger().Debugf("Content-Range: %s", contentRange)

		// 1 バイトだけキャッシュと重複してるはずなので
		// その部分を整合性チェックに使う
		responseBody := bufio.NewReader(resp.Body)
		firstByte, err := responseBody.ReadByte()

		if err != nil {
			proxy.GetLogger().Errorf("[%s] Reading response: %s", ctx.Req.URL, err)
			return goproxy.NewResponse(
				ctx.Req, goproxy.ContentTypeText, http.StatusInternalServerError, fmt.Sprintf("Reading response: %s", err))
		}

		if buf.Bytes()[buf.Len()-1] != firstByte {
			proxy.GetLogger().Infof("[%s] Cache mismatch; deleting cache", ctx.Req.URL)

			cacheEntry := proxy.Cache.GetEntry(ctx.Req.URL)
			if err := cacheEntry.Delete(); err != nil {
				proxy.GetLogger().Errorf("[%s] Deleting cache failed: %s", ctx.Req.URL, err)
			}

			proxy.GetLogger().Debugf("[%s] Attempting re-fetch", ctx.Req.URL)

			ctx.Req.Header.Del("Range")
			ctx.Req.Header.Del("If-Modified-Since")
			ctx.UserData = nil

			_, _resp, err := proxy.Tr.DetailedRoundTrip(ctx.Req)
			if _resp == nil || err != nil {
				proxy.GetLogger().Errorf("[%s] Re-fetch failed: %s", ctx.Req.URL, err)
				return resp
			}

			return _resp
		}

		io.Copy(buf, responseBody)

		resp.StatusCode = http.StatusOK
		resp.Header.Del("Content-Range")
		resp.Body = ioutil.NopCloser(buf)

	case http.StatusNotModified, // キャッシュから更新なし
		http.StatusNonAuthoritativeInfo: // DAT 落ち

		resp.StatusCode = http.StatusOK
		resp.Body = ioutil.NopCloser(userData.CachedContent)

	default:
		proxy.GetLogger().Errorf("[%s] Unhandled status code: %d", ctx.Req.URL, resp.StatusCode)
	}

	return resp
}

func (proxy *EtchProxy) StoreCache(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
	cache := proxy.Cache

	lastModified := time.Now()
	if lastModifiedString := resp.Header.Get("Last-Modified"); lastModifiedString != "" {
		if _lastModified, err := http.ParseTime(lastModifiedString); err != nil {
			proxy.GetLogger().Errorf(`[%s]: Parsing Last-Modified header "%s": %s`, ctx.Req.URL, lastModifiedString, err)
		} else {
			lastModified = _lastModified
		}
	}

	proxy.GetLogger().Infof("[%s] Update cache", ctx.Req.URL)

	cacheEntry := cache.GetEntry(ctx.Req.URL)
	buf := new(bytes.Buffer)
	io.Copy(buf, resp.Body)
	_, err := cacheEntry.FreshenContent(buf.Bytes(), lastModified)
	resp.Body = ioutil.NopCloser(buf)

	if err != nil {
		proxy.GetLogger().Warningf("[%s] FreshenContent failed: %s", ctx.Req.URL, err)
	}

	return resp
}

func (proxy *EtchProxy) UnguardRequest(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
	proxy.RequestMutex.Lock()
	defer proxy.RequestMutex.Unlock()
	if chans := proxy.RequestMutex.resChans[ctx.Req.URL.String()]; chans != nil {
		for _, ch := range chans {
			ch <- resp
		}
		delete(proxy.RequestMutex.resChans, ctx.Req.URL.String())
	}

	return resp
}

func setupProxy(proxy *EtchProxy) {
	if logger := proxy.GetLogger(); logger.IsDebugEnabled() {
		proxy.OnRequest().DoFunc(func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
			logger.Debugf("Request: %s %s", req.Method, req.URL)
			logger.Tracef("Request Headers: %+v", req.Header)
			return req, nil
		})
	}

	proxy.OnRequest().DoFunc(proxy.HandleNonProxyRequest)

	proxy.OnRequest(ReqMethodIs("GET")).DoFunc(proxy.GuardRequest)
	proxy.OnRequest(ReqMethodIs("GET")).DoFunc(proxy.PrepareRangedRequest)
	proxy.OnResponse(ReqMethodIs("GET")).DoFunc(proxy.RestoreCache)
	proxy.OnResponse(goproxy.ContentTypeIs("text/plain"), ReqMethodIs("GET"), StatusCodeIs(200), goproxy.Not(goproxy.ReqHostIs(""))).DoFunc(proxy.StoreCache)
	proxy.OnResponse().DoFunc(proxy.UnguardRequest)

	if logger := proxy.GetLogger(); logger.IsDebugEnabled() {
		proxy.OnResponse().DoFunc(func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
			logger.Debugf("Response: [%d] %s", resp.StatusCode, resp.Status)
			logger.Tracef("Response Headers: %+v", resp.Header)
			return resp
		})
	}
}

func main() {
	cacheDir := flag.String("cache-dir", "cache", "cache directory")
	proxyPort := flag.Int("port", 8080, "proxy port")

	flag.Parse()

	loggo.ConfigureLoggers("proxy=TRACE;cache=TRACE")
	loggo.ReplaceDefaultWriter(loggo.NewSimpleWriter(os.Stderr, &EtchLogFormatter{}))

	proxy := NewEtchProxy(*cacheDir)

	// proxy.Verbose = true

	proxy.GetLogger().Infof("Starting etch at localhost:%d ...", *proxyPort)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", *proxyPort), proxy); err != nil {
		proxy.GetLogger().Errorf("%s", err)
	}
}
