package main

import (
	"fmt"
	"github.com/elazarl/goproxy"
	"net/http"
)

// プロキシでないリクエストを処理

func (proxy *EtchProxy) HandleNonProxyRequest(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	if req.URL.Host != "" {
		return req, nil
	}

	keys := proxy.Cache.Keys()
	content := ""
	for _, key := range(keys) {
		content += fmt.Sprintln(key)
	}

	return req, goproxy.NewResponse(req, goproxy.ContentTypeText, 200, content)
}
