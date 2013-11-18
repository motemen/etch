package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"github.com/elazarl/goproxy"
	"github.com/golang/glog"
	"io"
	"io/ioutil"
	"net/http"
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

func (proxy *EtchProxy) GuardRequest(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	proxy.RequestMutex.Lock()
	chans, ok := proxy.RequestMutex.resChans[req.URL.String()]

	if ok {
		ch := make(chan *http.Response)
		proxy.RequestMutex.resChans[req.URL.String()] = append(chans, ch)
		proxy.RequestMutex.Unlock()

		glog.V(3).Infof("[%s] Waiting for ongoing request", req.URL)

		res := <-ch

		glog.V(3).Infof("[%s] Response got from chan: %s", req.URL, res)

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
		glog.Infof("%s: found cache entry", req.URL)

		content, err := entry.GetContent()
		if err != nil {
			glog.Errorf("OnRequest: retrieving cache content: %s", err)
			return req, nil
		}

		cachedContent := bytes.NewBuffer(content)
		req.Header.Add("Range", fmt.Sprintf("bytes=%d-", cachedContent.Len()-1))
		req.Header.Add("If-Modified-Since", entry.FileInfo.ModTime().Format(time.RFC850))

		_, resp, err := proxy.Tr.DetailedRoundTrip(req)
		if err != nil {
			glog.Errorf("OnRequest: executing request: %s", err)
			return req, nil
		}

		if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
			glog.Infof("[%s] Got 416: attempting re-fetch", req.URL)

			// clear cache
			req.Header.Del("Range")
			req.Header.Del("If-Modified-Since")

			_, _resp, err := proxy.Tr.DetailedRoundTrip(req)

			if err != nil {
				glog.Errorf("OnRequest: re-fetch: %s", err)
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
		glog.V(2).Infof("Content-Range: %s", contentRange)

		// 1 バイトだけキャッシュと重複してるはずなので
		// その部分を整合性チェックに使う
		responseBody := bufio.NewReader(resp.Body)
		firstByte, err := responseBody.ReadByte()

		if err != nil {
			glog.Errorf("[%s] Reading response: %s", ctx.Req.URL, err)
			return goproxy.NewResponse(
				ctx.Req, goproxy.ContentTypeText, http.StatusInternalServerError, fmt.Sprintf("Reading response: %s", err))
		}

		if buf.Bytes()[buf.Len()-1] != firstByte {
			glog.V(2).Infof("[%s] Cache mismatch", ctx.Req.URL)
			// TODO invalidate cache

			glog.V(2).Infof("[%s] Attempting re-fetch", ctx.Req.URL)

			ctx.Req.Header.Del("Range")
			ctx.Req.Header.Del("If-Modified-Since")
			ctx.UserData = nil

			_, _resp, err := proxy.Tr.DetailedRoundTrip(ctx.Req)
			if _resp == nil || err != nil {
				glog.Errorf("[%s] Re-fetch failed: %s", ctx.Req.URL, err)
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
		glog.Errorf("[%s] Unhandled status code: %d", ctx.Req.URL, resp.StatusCode)
	}

	return resp
}

func (proxy *EtchProxy) StoreCache(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
	cache := proxy.Cache

	lastModified := time.Now()
	if lastModifiedString := resp.Header.Get("Last-Modified"); lastModifiedString != "" {
		if _lastModified, err := http.ParseTime(lastModifiedString); err != nil {
			glog.Errorf(`[%s]: Parsing Last-Modified header "%s": %s`, ctx.Req.URL, lastModifiedString, err)
		} else {
			lastModified = _lastModified
		}
	}

	cacheEntry := cache.GetEntry(ctx.Req.URL)
	if cacheEntry.FileInfo == nil || cacheEntry.FileInfo.ModTime().Before(lastModified) {
		glog.Infof("[%s] Update cache: %s", ctx.Req.URL, cacheEntry.FilePath)

		buf := new(bytes.Buffer)
		io.Copy(buf, resp.Body)
		cacheEntry.SetContent(buf.Bytes(), lastModified)

		resp.Body = ioutil.NopCloser(buf)
	} else {
		glog.V(2).Infof("[%s] Response is not fresher than cache: %s <= %s; no update", ctx.Req.URL, lastModified, cacheEntry.FileInfo.ModTime())
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
	if glog.V(3) {
		proxy.OnRequest().DoFunc(func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
			glog.Infof("Request: %+v", req)
			return req, nil
		})
		proxy.OnResponse().DoFunc(func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
			glog.Infof("Response: %+v", resp)
			return resp
		})
	}

	proxy.OnRequest().DoFunc(proxy.HandleNonProxyRequest)

	proxy.OnRequest(ReqMethodIs("GET")).DoFunc(proxy.GuardRequest)
	proxy.OnRequest(ReqMethodIs("GET")).DoFunc(proxy.PrepareRangedRequest)
	proxy.OnResponse(ReqMethodIs("GET")).DoFunc(proxy.RestoreCache)
	proxy.OnResponse(goproxy.ContentTypeIs("text/plain"), ReqMethodIs("GET"), StatusCodeIs(200), goproxy.Not(goproxy.ReqHostIs(""))).DoFunc(proxy.StoreCache)
	proxy.OnResponse().DoFunc(proxy.UnguardRequest)
}

func main() {
	cacheDir := flag.String("cache-dir", "cache", "cache directory")
	proxyPort := flag.Int("port", 8080, "proxy port")

	flag.Parse()

	proxy := NewEtchProxy(*cacheDir)

	proxy.Verbose = true

	glog.Errorf("%s", http.ListenAndServe(fmt.Sprintf(":%d", *proxyPort), proxy))
}
