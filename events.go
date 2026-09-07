package main

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// emit delivers a backend event to whichever frontend is attached. In the
// desktop build that is the Wails runtime; under -server it is the SSE hub,
// which App reaches through the events field rather than by knowing about it.
//
// Every backend event goes through here, so the two modes cannot drift: an
// event added for the desktop window shows up in the browser for free.
func (a *App) emit(name string, data interface{}) {
	if a.events != nil {
		a.events(name, data)
		return
	}
	wailsruntime.EventsEmit(a.ctx, name, data)
}

// sseEvent is one message on the wire. Data is a list because Wails' EventsEmit
// is variadic on the JS side and the shim spreads it back into the callback;
// keeping the shape identical is what lets the frontend stay unmodified.
type sseEvent struct {
	Name string        `json:"name"`
	Data []interface{} `json:"data"`
}

// eventHub fans backend events out to every connected browser tab. More than one
// may be open at a time, and each gets its own queue.
type eventHub struct {
	mu      sync.Mutex
	clients map[chan sseEvent]struct{}
}

func newEventHub() *eventHub {
	return &eventHub{clients: make(map[chan sseEvent]struct{})}
}

// eventQueueDepth is deliberately generous: a build emits install:log for every
// line a compiler writes, and a browser that stalls for a moment should not lose
// the middle of a transcript.
const eventQueueDepth = 1024

func (h *eventHub) add() chan sseEvent {
	ch := make(chan sseEvent, eventQueueDepth)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *eventHub) remove(ch chan sseEvent) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
	close(ch)
}

// broadcast never blocks. A build must not stall because a browser tab stopped
// reading, so a client whose queue is full loses the event rather than holding
// up the run that produced it.
func (h *eventHub) broadcast(name string, data interface{}) {
	ev := sseEvent{Name: name, Data: []interface{}{data}}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- ev:
		default:
		}
	}
}

// ServeHTTP streams events to one browser tab for as long as it stays connected.
func (h *eventHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Proxies that buffer would defeat the point of streaming.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch := h.add()
	defer h.remove(ch)

	// A comment line keeps the connection from being reaped by an idle timeout
	// during the long quiet stretches between installs.
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()

	enc := json.NewEncoder(w)
	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-ch:
			if _, err := w.Write([]byte("data: ")); err != nil {
				return
			}
			if err := enc.Encode(ev); err != nil { // Encode writes the trailing newline
				return
			}
			if _, err := w.Write([]byte("\n")); err != nil {
				return
			}
			flusher.Flush()
		case <-ping.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
