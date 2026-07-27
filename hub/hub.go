// Package hub provides a wrapper around the `gorilla/websocket` package that
// handles WebSocket-based communications between the web server and clients, where
// clients can be web-based, CLI-based, or from the AI server.
package hub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"

	"github.com/bcspragu/Codenames/codenames"
	"github.com/gorilla/websocket"
)

// Hub maintains the set of active connections and broadcasts messages to the
// connections.
type Hub struct {
	// Registered connections.
	connections map[codenames.GameID][]*connection

	// Messages to send to everyone in a game.
	broadcast chan *broadcastMsg

	// Messages to send to a single player in a game.
	player chan *playerMsg

	// Register requests from the connections.
	register chan *connection

	// Unregister requests from connections.
	unregister chan *connection

	// Requests to drop every connection belonging to a single player.
	disconnect chan *disconnectReq
}

// New creates a new Hub and starts it in a background Go routine.
func New() *Hub {
	h := &Hub{
		broadcast:   make(chan *broadcastMsg),
		player:      make(chan *playerMsg),
		register:    make(chan *connection),
		unregister:  make(chan *connection),
		disconnect:  make(chan *disconnectReq),
		connections: make(map[codenames.GameID][]*connection),
	}
	go h.run()
	return h
}

func (h *Hub) run() {
	for {
		select {
		case c := <-h.register:
			conns := h.connections[c.gameID]
			h.connections[c.gameID] = append(conns, c)
		case c := <-h.unregister:
			h.deleteConn(c)
		case d := <-h.disconnect:
			// Copy first: deleteConn mutates the slice we'd be ranging over.
			var doomed []*connection
			for _, c := range h.connections[d.gameID] {
				if c.playerID == d.playerID {
					doomed = append(doomed, c)
				}
			}
			for _, c := range doomed {
				h.deleteConn(c)
			}
			close(d.done)
		case m := <-h.broadcast:
			for _, c := range h.connections[m.gameID] {
				select {
				case c.send <- m.msg:
				default:
					h.deleteConn(c)
				}
			}
		case m := <-h.player:
			for _, c := range h.connections[m.gameID] {
				if c.playerID == m.playerID {
					select {
					case c.send <- m.msg:
					default:
						h.deleteConn(c)
					}
				}
			}
		}
	}
}

// deleteConn drops a connection and closes its send channel. It's idempotent:
// a connection can be torn down from several directions at once (a failed
// broadcast, an explicit Disconnect, and the readPump noticing the socket
// died), and closing send twice would panic. Only the call that actually finds
// the connection still registered does the close.
func (h *Hub) deleteConn(c *connection) {
	rconns := h.connections[c.gameID]
	for i, rconn := range rconns {
		if rconn.id == c.id {
			// Remove the connection.
			copy(rconns[i:], rconns[i+1:])
			rconns[len(rconns)-1] = nil
			h.connections[c.gameID] = rconns[:len(rconns)-1]
			close(c.send)
			return
		}
	}
}

type broadcastMsg struct {
	gameID codenames.GameID
	msg    []byte
}

// ToGame sends a message to everyone in a game.
func (h *Hub) ToGame(gID codenames.GameID, msg any) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		return fmt.Errorf("failed to encode message: %w", err)
	}

	h.broadcast <- &broadcastMsg{
		gameID: gID,
		msg:    buf.Bytes(),
	}

	return nil
}

type playerMsg struct {
	gameID   codenames.GameID
	playerID codenames.PlayerID
	msg      []byte
}

func (h *Hub) ToPlayer(gID codenames.GameID, pID codenames.PlayerID, msg any) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		return fmt.Errorf("failed to encode message: %w", err)
	}

	h.player <- &playerMsg{
		gameID:   gID,
		playerID: pID,
		msg:      buf.Bytes(),
	}

	return nil
}

type disconnectReq struct {
	gameID   codenames.GameID
	playerID codenames.PlayerID
	done     chan struct{}
}

// Disconnect closes every connection a player holds on a game, which is how we
// tell an AI that's been removed from a lobby to stop playing: its read loop
// fails and the robot's goroutine unwinds. Safe to call for a player with no
// open connections.
func (h *Hub) Disconnect(gID codenames.GameID, pID codenames.PlayerID) {
	req := &disconnectReq{
		gameID:   gID,
		playerID: pID,
		done:     make(chan struct{}),
	}
	h.disconnect <- req
	<-req.done
}

// Register associates a connection with the hub and a given game.
func (h *Hub) Register(ws *websocket.Conn, gID codenames.GameID, pID codenames.PlayerID) {
	conn := &connection{
		id:       newID(gID),
		h:        h,
		gameID:   gID,
		playerID: pID,
		send:     make(chan []byte, 256),
		ws:       ws,
	}
	h.register <- conn
	go conn.writePump()
	go conn.readPump()
}

func newID(gID codenames.GameID) string {
	return fmt.Sprintf("%s-%d", gID, rand.Int63())
}
