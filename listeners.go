package main

import (
	"sync"
)

type Listeners struct {
	sync.Mutex
	chans []chan Event
}

func (l *Listeners) Broadcast(e Event) {
	for _, ch := range l.chans {
		ch <- e
	}
}

func (l *Listeners) Create() chan Event {
	ch := make(chan Event)

	l.Lock()
	defer l.Unlock()

	l.chans = append(l.chans, ch)

	return ch
}

func (l *Listeners) Remove(ch <-chan Event) {
	l.Lock()
	defer l.Unlock()

	l.chans = make([]chan Event, len(l.chans)-1)
	for _, _ch := range l.chans {
		if ch != _ch {
			l.chans = append(l.chans, _ch)
		}
	}
}
