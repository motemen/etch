package etch

import (
	"encoding/json"
	"net/url"
)

type Event interface {
	Json() ([]byte, error)
}

type CacheUpdateEvent struct {
	URL   *url.URL
	Since int
}

func (e CacheUpdateEvent) Json() ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"event": "cacheUpdate",
		"url":   e.URL.String(),
		"since": e.Since,
	})
}

type CacheDeleteEvent struct {
	URL *url.URL
}

func (e CacheDeleteEvent) Json() ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"event": "cacheDelete",
		"url":   e.URL.String(),
	})
}
