package main

import (
	"flag"
	"fmt"
	"github.com/howbazaar/loggo"
	"net/http"
	"os"
	"strings"
)

func main() {
	cacheDir := flag.String("cache-dir", "cache", "cache directory")
	port := flag.Int("port", 25252, "proxy port")
	hosts := flag.String("host", "2ch.net,bbspink.com", "hosts to proxy")

	flag.Parse()

	loggo.ConfigureLoggers("<root>=TRACE;proxy=TRACE;cache=TRACE")
	loggo.ReplaceDefaultWriter(loggo.NewSimpleWriter(os.Stderr, &EtchLogFormatter{}))

	proxy := NewProxyServer(*cacheDir)
	control := NewControlServer(proxy)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		for _, host := range strings.Split(*hosts, ",") {
			if strings.HasSuffix(req.Host, "."+host) {
				proxy.ServeHTTP(w, req)
				return
			}
		}

		control.ServeHTTP(w, req)
	})

	infof(proxy, "Starting etch at localhost:%d...", *port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), mux); err != nil {
		errorf(proxy, "%s", err)
		os.Exit(1)
	}
}
