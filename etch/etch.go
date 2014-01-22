package main

import (
	"flag"
	"github.com/motemen/etch"
	"os"
	"strings"
)

func main() {
	cacheDir := flag.String("cache-dir", "cache", "cache directory")
	port := flag.Int("port", 25252, "proxy port")
	hosts := flag.String("host", "2ch.net,bbspink.com", "hosts to proxy")

	flag.Parse()

	etch.ConfigureLoggers()

	etchServer := etch.NewServer(*cacheDir, strings.Split(*hosts, ","))

	err := etchServer.ListenAndServe(*port)
	if err != nil {
		os.Exit(1);
	}
}
