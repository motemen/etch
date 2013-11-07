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
	Cache *Cache
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

var requestMutex = &RequestMutex{resChans: make(map[string][]chan *http.Response)}

func (proxy *EtchProxy) GuardRequest(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	requestMutex.Lock()
	chans, ok := requestMutex.resChans[req.URL.String()]

	if ok {
		ch := make(chan *http.Response)
		requestMutex.resChans[req.URL.String()] = append(chans, ch)
		requestMutex.Unlock()

		glog.V(3).Infof("[%s] Waiting for ongoing request", req.URL)

		res := <-ch

		glog.V(3).Infof("[%s] Response got from chan: %s", req.URL, res)

		if res != nil {
			return req, res
		}
	} else {
		requestMutex.resChans[req.URL.String()] = make([]chan *http.Response, 0)
		requestMutex.Unlock()
	}

	return req, nil
}

func (proxy *EtchProxy) MakeRangedRequest(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	cache := proxy.Cache

	if entry := cache.GetEntry(req.URL); entry.FileInfo != nil {
		glog.Infof("%s: found cache entry", req.URL)

		content, err := entry.Content()
		if err != nil {
			glog.Errorf("OnRequest: retrieving cache content: %s", err)
			return req, nil
		}

		// TODO mutex
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

func (proxy *EtchProxy) RestoreCache(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
	glog.V(3).Infof("[%s] Headers: %s", ctx.Req.URL, resp.Header)

	if ctx.UserData == nil {
		return resp
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
			// TODO re-attempt
			glog.Errorf("[%s] Cache mismatch", ctx.Req.URL)
			return resp
		}

		io.Copy(buf, responseBody)

		resp.StatusCode = http.StatusOK
		resp.Header.Del("Content-Range")
		resp.Body = ioutil.NopCloser(buf)

	case http.StatusNotModified:
		resp.StatusCode = http.StatusOK
		resp.Body = ioutil.NopCloser(userData.CachedContent)

	case http.StatusNonAuthoritativeInfo:
		resp.StatusCode = http.StatusGone

	default:
		glog.Errorf("[%s] Unhandled status code: %d", ctx.Req.URL, resp.StatusCode)
	}

	return resp
}

func (proxy *EtchProxy) StoreCache(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
	cache := proxy.Cache

	lastModified := time.Now()
	lastModifiedString := resp.Header.Get("Last-Modified")
	if lastModifiedString != "" {
		var err error
		lastModified, err = http.ParseTime(lastModifiedString)
		if err != nil {
			glog.Errorf(`[%s]: Parsing Last-Modified header "%s": %s`, ctx.Req.URL, lastModifiedString, err)
		}
	}

	cacheEntry := cache.GetEntry(ctx.Req.URL)
	if cacheEntry.FileInfo == nil || cacheEntry.FileInfo.ModTime().Before(lastModified) {
		glog.Infof("[%s] update cache: %s", ctx.Req.URL, cacheEntry.FilePath)

		cacheWriter, err := cacheEntry.GetWriter()
		if err != nil {
			glog.Errorf("[%s]: cacheEntry.GetWriter: %s", ctx.Req.URL, err)
			return resp
		}

		buf := new(bytes.Buffer)
		io.Copy(io.MultiWriter(buf, cacheWriter), resp.Body)
		cacheWriter.Close()

		cacheEntry.SetMtime(lastModified)

		resp.Body = ioutil.NopCloser(buf)
	} else {
		glog.V(2).Infof("[%s] Response is not fresher than cache: %s <= %s", ctx.Req.URL, lastModified, cacheEntry.FileInfo.ModTime())
	}

	return resp
}

func (proxy *EtchProxy) UnguardRequest(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
	requestMutex.Lock()
	defer requestMutex.Unlock()
	if chans := requestMutex.resChans[ctx.Req.URL.String()]; chans != nil {
		for _, ch := range chans {
			ch <- resp
		}
		delete(requestMutex.resChans, ctx.Req.URL.String())
	}

	return resp
}

func main() {
	cacheDir := flag.String("cache-dir", "cache", "cache directory")
	proxyPort := flag.Int("port", 8080, "proxy port")

	flag.Parse()

	cache := &Cache{*cacheDir}
	proxy := &EtchProxy{*goproxy.NewProxyHttpServer(), cache}

	proxy.OnRequest(ReqMethodIs("GET")).DoFunc(proxy.GuardRequest)
	proxy.OnRequest(ReqMethodIs("GET")).DoFunc(proxy.MakeRangedRequest)
	proxy.OnResponse(ReqMethodIs("GET")).DoFunc(proxy.RestoreCache)
	proxy.OnResponse(goproxy.ContentTypeIs("text/plain"), ReqMethodIs("GET"), StatusCodeIs(200)).DoFunc(proxy.StoreCache)
	proxy.OnResponse().DoFunc(proxy.UnguardRequest)

	proxy.Verbose = true

	glog.Errorf("%s", http.ListenAndServe(fmt.Sprintf(":%d", *proxyPort), proxy))
}
