package etch

import (
	"fmt"
	"net/http"
	"strings"
)

type Server struct {
	*ProxyServer
	*ControlServer
	*http.ServeMux
}

func NewServer(cacheDir string, hosts []string) *Server {
	proxy := NewProxyServer(cacheDir)
	control := NewControlServer(proxy)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		for _, host := range hosts {
			if strings.HasSuffix(req.Host, "."+host) {
				proxy.ServeHTTP(w, req)
				return
			}
		}

		control.ServeHTTP(w, req)
	})
	return &Server{proxy, control, mux}
}

func (server *Server) ListenAndServe(port int) error {
	infof(server.ProxyServer, "Starting etch at localhost:%d...", port)

	err := http.ListenAndServe(fmt.Sprintf(":%d", port), server.ServeMux)

	if err != nil {
		errorf(server.ProxyServer, "%s", err)
	}

	return err
}
