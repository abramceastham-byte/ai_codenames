package hub

import (
	"errors"
	"log"
	"time"

	"github.com/bcspragu/Codenames/codenames"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

// connection is an middleman between the websocket connection and the hub.
type connection struct {
	id string
	h  *Hub
	// What room this connection is associated with.
	gameID   codenames.GameID
	playerID codenames.PlayerID
	// The websocket connection.
	ws *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte
}

// readPump pumps messages from the websocket connection to the hub.
func (c *connection) readPump() {
	defer func() {
		c.h.unregister <- c
		if err := c.ws.Close(); err != nil {
			log.Printf("[read] error closing websocket: %v", err)
		}

	}()
	c.ws.SetReadLimit(maxMessageSize)
	if err := c.ws.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		log.Printf("failed to set read deadline: %v", err)
	}
	c.ws.SetPongHandler(func(string) error {
		if err := c.ws.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
			log.Printf("failed to set read deadline on pong: %v", err)
		}
		return nil
	})
	for {
		_, _, err := c.ws.ReadMessage()
		if websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) {
			// Fine
			break
		} else if err != nil {
			log.Printf("failed to read WebSocket message from client: %v", err)
			break
		}
	}
}

// write writes a message with the given message type and payload.
func (c *connection) write(mt int, payload []byte) error {
	if err := c.ws.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
		log.Printf("failed to set write deadline: %v", err)
	}
	return c.ws.WriteMessage(mt, payload)
}

// writePump pumps messages from the hub to the websocket connection.
func (c *connection) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		if err := c.ws.Close(); err != nil {
			log.Printf("[write] error closing websocket: %v", err)
		}
	}()
	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				if err := c.write(websocket.CloseMessage, []byte{}); err != nil && !errors.Is(err, websocket.ErrCloseSent) {
					log.Printf("failed to write close message: %v", err)
				}
				return
			}
			if err := c.write(websocket.TextMessage, message); err != nil {
				log.Printf("failed to write WebSocket message: %v", err)
				return
			}
		case <-ticker.C:
			if err := c.write(websocket.PingMessage, nil); err != nil {
				log.Printf("failed to write WebSocket ping: %v", err)
				return
			}
		}
	}
}
