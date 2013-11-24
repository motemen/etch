package main

import (
	"fmt"
	"github.com/elazarl/goproxy"
	"github.com/howbazaar/loggo"
	"time"
)

type EtchLogFormatter struct{}

func (*EtchLogFormatter) Format(level loggo.Level, module, filename string, line int, timestamp time.Time, message string) string {
	return fmt.Sprintf(
		"%s [%s] %5s %s",
		timestamp.Format("2006-01-02 15:04:05 MST"),
		module,
		level,
		message,
	)
}

func logConfig(context interface{}) (loggo.Logger, string, interface{}) {
	switch context := context.(type) {
	case *Cache:
		return loggo.GetLogger("cache"), "%s", nil
	case *CacheEntry:
		return loggo.GetLogger("cache"), "[%s] ", context.FilePath
	case *goproxy.ProxyCtx:
		return loggo.GetLogger("proxy"), "[%03d] ", context.Session & 0xFF
	default:
		return loggo.GetLogger(""), "[%s] ", context
	}
}

func tracef(context interface{}, pattern string, args ...interface{}) {
	logger, prefix, arg := logConfig(context)
	logger.Tracef(prefix+pattern, append([]interface{}{arg}, args...)...)
}

func debugf(context interface{}, pattern string, args ...interface{}) {
	logger, prefix, arg := logConfig(context)
	logger.Debugf(prefix+pattern, append([]interface{}{arg}, args...)...)
}

func infof(context interface{}, pattern string, args ...interface{}) {
	logger, prefix, arg := logConfig(context)
	logger.Infof(prefix+pattern, append([]interface{}{arg}, args...)...)
}

func warningf(context interface{}, pattern string, args ...interface{}) {
	logger, prefix, arg := logConfig(context)
	logger.Warningf(prefix+pattern, append([]interface{}{arg}, args...)...)
}

func errorf(context interface{}, pattern string, args ...interface{}) {
	logger, prefix, arg := logConfig(context)
	logger.Errorf(prefix+pattern, append([]interface{}{arg}, args...)...)
}
