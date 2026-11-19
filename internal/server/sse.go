package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Broadcaster manages active Server-Sent Events client connections.
type Broadcaster struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
}

// NewBroadcaster initializes a broadcaster instance.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		clients: make(map[chan []byte]struct{}),
	}
}

// Subscribe registers a new SSE listener channel.
func (b *Broadcaster) Subscribe() chan []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan []byte, 10)
	b.clients[ch] = struct{}{}
	return ch
}

// Unsubscribe removes a closed listener channel.
func (b *Broadcaster) Unsubscribe(ch chan []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.clients, ch)
	close(ch)
}

// Broadcast sends JSON payload to all active SSE subscribers.
func (b *Broadcaster) Broadcast(event string, data any) {
	b.mu.Lock()
	defer b.mu.Unlock()

	jsonData, err := json.Marshal(data)
	if err != nil {
		return
	}

	msg := []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", event, string(jsonData)))
	for ch := range b.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

// HandleSSE serves HTTP clients requesting persistent event stream.
func (b *Broadcaster) HandleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := b.Subscribe()
	defer b.Unsubscribe(ch)

	// Send initial ping
	fmt.Fprintf(w, "event: ping\ndata: {\"time\": %d}\n\n", time.Now().Unix())
	flusher.Flush()

	notify := r.Context().Done()
	for {
		select {
		case <-notify:
			return
		case msg := <-ch:
			w.Write(msg)
			flusher.Flush()
		}
	}
}
