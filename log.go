package main

import (
	"github.com/elazarl/goproxy"
	"github.com/howbazaar/loggo"
)

func logConfig(context interface{}) (string, string, interface{}) {
	switch context := context.(type) {
	case *CacheEntry:
		return "cache", "[%s] ", context.FilePath
	case *goproxy.ProxyCtx:
		return "proxy", "[%03d] ", context.Session & 0xFF
	default:
		return "", "[%s] ", context
	}
}

func tracef(context interface{}, pattern string, args ...interface{}) {
	group, prefix, arg := logConfig(context)
	logger := loggo.GetLogger(group)
	logger.Tracef(prefix+pattern, append([]interface{}{arg}, args...)...)
}

func debugf(context interface{}, pattern string, args ...interface{}) {
	group, prefix, arg := logConfig(context)
	logger := loggo.GetLogger(group)
	logger.Debugf(prefix+pattern, append([]interface{}{arg}, args...)...)
}

func infof(context interface{}, pattern string, args ...interface{}) {
	group, prefix, arg := logConfig(context)
	logger := loggo.GetLogger(group)
	logger.Infof(prefix+pattern, append([]interface{}{arg}, args...)...)
}

func warningf(context interface{}, pattern string, args ...interface{}) {
	group, prefix, arg := logConfig(context)
	logger := loggo.GetLogger(group)
	logger.Warningf(prefix+pattern, append([]interface{}{arg}, args...)...)
}

func errorf(context interface{}, pattern string, args ...interface{}) {
	group, prefix, arg := logConfig(context)
	logger := loggo.GetLogger(group)
	logger.Errorf(prefix+pattern, append([]interface{}{arg}, args...)...)
}
