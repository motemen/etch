package main

import (
	"github.com/howbazaar/loggo"
)

type LogPrefixable interface {
	LogPrefixValue() interface{}
	LogGroup() string
}

func tracef(prefix LogPrefixable, pattern string, args ...interface{}) {
	logger := loggo.GetLogger(prefix.LogGroup())
	logger.Tracef("[%s] " + pattern, append([]interface{}{prefix.LogPrefixValue()}, args...)...)
}

func debugf(prefix LogPrefixable, pattern string, args ...interface{}) {
	logger := loggo.GetLogger(prefix.LogGroup())
	logger.Debugf("[%s] " + pattern, append([]interface{}{prefix.LogPrefixValue()}, args...)...)
}

func infof(prefix LogPrefixable, pattern string, args ...interface{}) {
	logger := loggo.GetLogger(prefix.LogGroup())
	logger.Infof("[%s] " + pattern, append([]interface{}{prefix.LogPrefixValue()}, args...)...)
}

func warningf(prefix LogPrefixable, pattern string, args ...interface{}) {
	logger := loggo.GetLogger(prefix.LogGroup())
	logger.Warningf("[%s] " + pattern, append([]interface{}{prefix.LogPrefixValue()}, args...)...)
}
