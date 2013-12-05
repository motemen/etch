package main

import (
	"net/http"
	"net/url"
	"os"
)

type ControlServer struct {
	*http.ServeMux
	Proxy *ProxyServer
}

func NewControlServer (proxy *ProxyServer) *ControlServer {
	controlServer := &ControlServer{http.NewServeMux(),proxy}
	controlServer.Setup()

	return controlServer
}

func (control *ControlServer) Setup() {
	control.HandleFunc("/", func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Add("Content-Type", "text/plain; charset=utf-8")

		keys := control.Proxy.Cache.Keys()
		for _, key := range keys {
			rw.Write([]byte(key.String() + "\n"))
		}
	})

	control.HandleFunc("/cache", func(rw http.ResponseWriter, req *http.Request) {
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

		cacheEntry := control.Proxy.Cache.GetEntry(u)
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
				errorf(control, "Deleting cache %s: %s", cacheEntry, err)
				rw.WriteHeader(http.StatusInternalServerError)
				return
			}

			rw.WriteHeader(http.StatusNoContent)

		default:
			rw.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}
