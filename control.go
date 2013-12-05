package main

import (
	"net/http"
	"net/url"
	"os"
)

type EtchControlServer struct {
	*http.ServeMux
	proxy *EtchProxy
}

func NewEtchControl (proxy *EtchProxy) *EtchControlServer {
	controlServer := &EtchControlServer{http.NewServeMux(),proxy}

	controlServer.HandleFunc("/", func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Add("Content-Type", "text/plain; charset=utf-8")

		keys := proxy.Cache.Keys()
		for _, key := range keys {
			rw.Write([]byte(key.String() + "\n"))
		}
	})

	controlServer.HandleFunc("/cache", func(rw http.ResponseWriter, req *http.Request) {
		urlString := req.URL.Query().Get("url")
		if urlString == "" {
			rw.WriteHeader(http.StatusBadRequest)
			return
		}

		u, err := url.Parse(urlString)
		if err != nil {
			rw.WriteHeader(http.StatusBadRequest)
			return
		}

		cacheEntry := proxy.Cache.GetEntry(u)
		content, mtime, err := cacheEntry.GetContent()

		if os.IsNotExist(err) {
			rw.WriteHeader(http.StatusNotFound)
			return
		}

		switch req.Method {
		case "HEAD":
			rw.Header().Set("Last-Modified", mtime.Format(http.TimeFormat))

		case "GET":
			rw.Header().Set("Last-Modified", mtime.Format(http.TimeFormat))
			rw.Write(content)

		case "DELETE":
			if err := cacheEntry.Delete(); err != nil {
				errorf(proxy, "Deleting cache %s: %s", cacheEntry, err)
				rw.WriteHeader(http.StatusInternalServerError)
				return
			}

			rw.WriteHeader(http.StatusNoContent)

		default:
			rw.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	return controlServer
}
