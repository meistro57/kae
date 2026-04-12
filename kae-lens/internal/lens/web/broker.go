package web

import (
	"encoding/json"
	"log"
	"sync"
)

// Broker manages SSE client connections and broadcasts events to all of them.
// Pattern: central register/unregister/broadcast channels, single listen() goroutine.
type Broker struct {
	// broadcast receives encoded event bytes to send to all clients
	broadcast chan []byte
	// register receives new client channels
	register chan chan []byte
	// unregister receives client channels to remove
	unregister chan chan []byte
	// clients is the registry of active connections
	clients map[chan []byte]bool
	mu      sync.RWMutex
}

// NewBroker creates and starts a Broker.
func NewBroker() *Broker {
	b := &Broker{
		broadcast:  make(chan []byte, 64),
		register:   make(chan chan []byte),
		unregister: make(chan chan []byte),
		clients:    make(map[chan []byte]bool),
	}
	go b.listen()
	return b
}

// listen is the single goroutine that orchestrates client registration and fan-out.
// All map access happens here — no mutex needed on the map itself.
func (b *Broker) listen() {
	for {
		select {
		case client := <-b.register:
			b.clients[client] = true
			log.Printf("[sse-broker] client connected — total: %d", len(b.clients))

		case client := <-b.unregister:
			if _, ok := b.clients[client]; ok {
				delete(b.clients, client)
				close(client)
				log.Printf("[sse-broker] client disconnected — total: %d", len(b.clients))
			}

		case data := <-b.broadcast:
			for client := range b.clients {
				select {
				case client <- data:
				default:
					// Skip slow clients rather than blocking the broker
					log.Println("[sse-broker] slow client, dropping event")
				}
			}
		}
	}
}

// Publish serializes an SSE event and queues it for broadcast.
// eventType is the SSE `event:` field name (e.g. "finding", "stats").
func (b *Broker) Publish(eventType string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[sse-broker] marshal error for event %q: %v", eventType, err)
		return
	}

	// Format: "event: <type>\ndata: <json>\n\n"
	msg := []byte("event: " + eventType + "\ndata: " + string(data) + "\n\n")

	select {
	case b.broadcast <- msg:
	default:
		log.Println("[sse-broker] broadcast channel full, dropping event")
	}
}

// ClientCount returns the current number of connected SSE clients.
func (b *Broker) ClientCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.clients)
}

// NewClientChannel registers a new client and returns its channel.
// Call UnregisterClient when the client disconnects.
func (b *Broker) NewClientChannel() chan []byte {
	ch := make(chan []byte, 20)
	b.register <- ch
	return ch
}

// UnregisterClient removes a client channel.
func (b *Broker) UnregisterClient(ch chan []byte) {
	b.unregister <- ch
}
