package controlplane

import "sync"

type streamMessage struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

type streamBus struct {
	mu   sync.RWMutex
	subs map[string][]chan streamMessage
}

func newStreamBus() *streamBus {
	return &streamBus{subs: make(map[string][]chan streamMessage)}
}

func (b *streamBus) subscribe(sessionID string) (<-chan streamMessage, func()) {
	ch := make(chan streamMessage, 64)
	b.mu.Lock()
	b.subs[sessionID] = append(b.subs[sessionID], ch)
	b.mu.Unlock()

	unsub := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		chans := b.subs[sessionID]
		for i, c := range chans {
			if c == ch {
				b.subs[sessionID] = append(chans[:i], chans[i+1:]...)
				close(ch)
				break
			}
		}
		if len(b.subs[sessionID]) == 0 {
			delete(b.subs, sessionID)
		}
	}

	return ch, unsub
}

func (b *streamBus) publish(sessionID string, msg streamMessage) {
	b.mu.RLock()
	chans := b.subs[sessionID]
	b.mu.RUnlock()

	for _, ch := range chans {
		select {
		case ch <- msg:
		default:
		}
	}
}
