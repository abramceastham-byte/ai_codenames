package client

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/bcspragu/Codenames/codenames"
	"github.com/bcspragu/Codenames/msgs"
	"github.com/gorilla/websocket"
)

type wsClient struct {
	conn  *websocket.Conn
	msgs  chan []byte
	done  chan struct{}
	hooks WSHooks
}

func (c *Client) ListenForUpdates(gID codenames.GameID, hooks WSHooks) error {
	endpointURL, err := url.Parse(c.endpoint)
	if err != nil {
		return fmt.Errorf("failed to parse endpoint URL %q: %w", c.endpoint, err)
	}
	scheme := "ws"
	if endpointURL.Scheme == "https" {
		scheme = "wss"
	}

	addr := scheme + "://" + endpointURL.Host + "/api/game/" + string(gID) + "/ws"

	dialer := &websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 45 * time.Second,
		Jar:              c.http.Jar,
	}
	conn, _, err := dialer.Dial(addr, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}

	if hooks.OnConnect != nil {
		go hooks.OnConnect()
	}

	wsc := &wsClient{
		conn: conn,
		done: make(chan struct{}),
		// We buffer it in case messages come in while we're waiting on user input.
		// We don't want to process messages concurrently, because that seems
		// likely to cause tricky problems.
		msgs:  make(chan []byte, 100),
		hooks: hooks,
	}

	go wsc.handleMessages()

	return wsc.read()
}

func (ws *wsClient) read() error {
	defer close(ws.done)
	for {
		messageType, message, err := ws.conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("ReadMessage: %w", err)
		}

		if messageType != websocket.TextMessage {
			continue
		}

		ws.msgs <- message
	}
}

func (ws *wsClient) handleMessages() {
	for {
		select {
		case <-ws.done:
			return
		case msg := <-ws.msgs:
			var justAction struct {
				Action string `json:"action"`
			}
			if err := json.Unmarshal(msg, &justAction); err != nil {
				log.Printf("failed to unmarshal action from server: %v", err)
				return
			}

			switch justAction.Action {
			case "GAME_START":
				ws.handleGameStart(msg)
			case "CLUE_GIVEN":
				ws.handleClueGiven(msg)
			case "PLAYER_VOTE":
				ws.handlePlayerVote(msg)
			case "GUESS_GIVEN":
				ws.handleGuessGiven(msg)
			case "GAME_END":
				ws.handleGameEnd(msg)
			default:
				log.Printf("unknown message action %q", justAction.Action)
			}
		}
	}
}

func (ws *wsClient) handleGameStart(dat []byte) {
	var gs msgs.GameStart
	if err := json.Unmarshal(dat, &gs); err != nil {
		log.Printf("handleGameStart: %v", err)
		return
	}

	if ws.hooks.OnStart == nil {
		return
	}
	ws.hooks.OnStart(&gs)
}

func (ws *wsClient) handleClueGiven(dat []byte) {
	var cg msgs.ClueGiven
	if err := json.Unmarshal(dat, &cg); err != nil {
		log.Printf("handleClueGiven: %v", err)
		return
	}

	if ws.hooks.OnClueGiven == nil {
		return
	}
	ws.hooks.OnClueGiven(&cg)
}

func (ws *wsClient) handlePlayerVote(dat []byte) {
	var pv msgs.PlayerVote
	if err := json.Unmarshal(dat, &pv); err != nil {
		log.Printf("handlePlayerVote: %v", err)
		return
	}

	if ws.hooks.OnPlayerVote == nil {
		return
	}
	ws.hooks.OnPlayerVote(&pv)
}

func (ws *wsClient) handleGuessGiven(dat []byte) {
	var gg msgs.GuessGiven
	if err := json.Unmarshal(dat, &gg); err != nil {
		log.Printf("handleGuessGiven: %v", err)
		return
	}

	if ws.hooks.OnGuessGiven == nil {
		return
	}
	ws.hooks.OnGuessGiven(&gg)
}

func (ws *wsClient) handleGameEnd(dat []byte) {
	var ge msgs.GameEnd
	if err := json.Unmarshal(dat, &ge); err != nil {
		log.Printf("handleGameEnd: %v", err)
		return
	}

	if ws.hooks.OnEnd == nil {
		return
	}
	ws.hooks.OnEnd(&ge)
}

type WSHooks struct {
	OnConnect    func()
	OnStart      func(*msgs.GameStart)
	OnClueGiven  func(*msgs.ClueGiven)
	OnPlayerVote func(*msgs.PlayerVote)
	OnGuessGiven func(*msgs.GuessGiven)
	OnEnd        func(*msgs.GameEnd)
}
