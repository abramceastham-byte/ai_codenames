// Throwaway program to verify all_games.csv model-column logging end to
// end. Not part of the app; delete this directory after use.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const base = "http://localhost:8090"

var client = &http.Client{}

func post(path string, body any) map[string]any {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", base+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != 200 {
		log.Fatalf("POST %s: %d: %+v", path, resp.StatusCode, out)
	}
	return out
}

type historyEntry struct {
	Round      int     `json:"round"`
	Team       string  `json:"team"`
	Type       string  `json:"type"`
	Detail     string  `json:"detail"`
	Result     string  `json:"result"`
	DurationMs float64 `json:"durationMs"`
}

func main() {
	jar, _ := cookiejar.New(nil)
	client.Jar = jar

	post("/api/user", map[string]any{})
	g := post("/api/game", map[string]any{"private": true})
	gid := g["id"].(string)
	fmt.Println("game:", gid)

	for _, team := range []string{"RED", "BLUE"} {
		for _, role := range []string{"SPYMASTER", "OPERATIVE"} {
			post(fmt.Sprintf("/api/game/%s/requestAI", gid), map[string]any{"team": team, "role": role})
		}
	}
	time.Sleep(2 * time.Second)
	post(fmt.Sprintf("/api/game/%s/start", gid), map[string]any{})
	fmt.Println("started")

	var cookies []string
	u, _ := http.NewRequest("GET", base, nil)
	for _, c := range jar.Cookies(u.URL) {
		cookies = append(cookies, c.String())
	}
	header := http.Header{}
	header.Set("Cookie", strings.Join(cookies, "; "))

	wsURL := fmt.Sprintf("ws://localhost:8090/api/game/%s/ws", gid)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	var history []historyEntry
	teamClueCount := map[string]int{"RED": 0, "BLUE": 0}
	actionStart := time.Now()

	agentResult := map[float64]string{1: "red", 2: "blue", 3: "bystander", 4: "assassin"}

	deadline := time.Now().Add(8 * time.Minute)
	for time.Now().Before(deadline) {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			log.Fatalf("read: %v", err)
		}
		var msg map[string]any
		json.Unmarshal(raw, &msg)
		action, _ := msg["action"].(string)
		now := time.Now()

		switch action {
		case "CLUE_GIVEN":
			duration := now.Sub(actionStart).Seconds() * 1000
			team := msg["team"].(string)
			teamClueCount[team]++
			round := max(teamClueCount["RED"], teamClueCount["BLUE"])
			clue := msg["clue"].(map[string]any)
			history = append(history, historyEntry{
				Round: round, Team: team, Type: "clue",
				Detail:     fmt.Sprintf("%s (%v)", clue["word"], clue["count"]),
				DurationMs: duration,
			})
			actionStart = now
			fmt.Println("clue:", team, clue["word"], clue["count"])
		case "GUESS_GIVEN":
			duration := now.Sub(actionStart).Seconds() * 1000
			team := msg["team"].(string)
			round := max(teamClueCount["RED"], teamClueCount["BLUE"])
			result := ""
			if card, ok := msg["card"].(map[string]any); ok && card != nil {
				if agent, ok := card["agent"].(float64); ok {
					result = agentResult[agent]
				}
			}
			guess, _ := msg["guess"].(string)
			detail := guess
			if detail == "" {
				detail = "(pass)"
			}
			history = append(history, historyEntry{
				Round: round, Team: team, Type: "guess",
				Detail: detail, Result: result, DurationMs: duration,
			})
			actionStart = now
			fmt.Println("guess:", team, guess, result)
		case "GAME_END":
			fmt.Println("GAME_END, winning team:", msg["winning_team"])
			resp := post(fmt.Sprintf("/api/game/%s/log", gid), map[string]any{"entries": history})
			fmt.Println("saved:", resp)
			fmt.Println("done, gid=", gid)
			return
		}
	}
	log.Fatal("timed out waiting for game to finish")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
