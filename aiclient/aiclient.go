// Package aiclient implements the simple interface for communicating with the
// AI service, mainly saying 'hey, join this game as an AI'.
package aiclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/bcspragu/Codenames/codenames"
)

type Client struct {
	secret   string
	endpoint string
	http     *http.Client
}

func New(secret, endpoint string) *Client {
	return &Client{
		secret:   secret,
		endpoint: endpoint,
		http:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) JoinGame(gID codenames.GameID, team, role, backend string) (codenames.RobotID, error) {
	body := struct {
		GameID  string `json:"game_id"`
		Team    string `json:"team"`
		Role    string `json:"role"`
		Backend string `json:"backend,omitempty"`
	}{string(gID), team, role, backend}

	endpoint := c.endpoint + "/join"
	req, err := http.NewRequest(http.MethodPost, endpoint, toBody(body))
	if err != nil {
		return "", fmt.Errorf("failed to form request: %w", err)
	}
	req.Header.Set("Authorization", c.secret)

	var resp struct {
		RobotID string `json:"robot_id"`
		Success bool   `json:"success"`
	}
	if err := c.do(req, &resp); err != nil {
		return "", fmt.Errorf("failed to request AI join a game: %w", err)
	}
	return codenames.RobotID(resp.RobotID), nil
}

type Backends struct {
	Backends []string `json:"backends"`
	Default  string   `json:"default"`
}

func (c *Client) Backends() (*Backends, error) {
	req, err := http.NewRequest(http.MethodGet, c.endpoint+"/backends", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to form request: %w", err)
	}
	req.Header.Set("Authorization", c.secret)

	var resp Backends
	if err := c.do(req, &resp); err != nil {
		return nil, fmt.Errorf("failed to fetch AI backends: %w", err)
	}
	return &resp, nil
}

// ReasoningEntry mirrors the ai-server's internal reasoningLogEntry shape —
// one record of why an AI backend picked a given clue or guess.
type ReasoningEntry struct {
	Timestamp string           `json:"timestamp"`
	GameID    codenames.GameID `json:"game_id"`
	Round     int              `json:"round"`
	Team      codenames.Team   `json:"team"`
	Role      codenames.Role   `json:"role"`
	Backend   string           `json:"backend"`
	Action    string           `json:"action"`
	Detail    string           `json:"detail"`
	Reasoning string           `json:"reasoning"`
	Error     string           `json:"error,omitempty"`
}

// GetReasoning fetches the logged AI reasoning entries for a single game.
// Intended for an admin-only view — never call this on behalf of a
// player-facing request.
func (c *Client) GetReasoning(gID codenames.GameID) ([]ReasoningEntry, error) {
	req, err := http.NewRequest(http.MethodGet, c.endpoint+"/reasoning?game_id="+url.QueryEscape(string(gID)), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to form request: %w", err)
	}
	req.Header.Set("Authorization", c.secret)

	var resp struct {
		Entries []ReasoningEntry `json:"entries"`
	}
	if err := c.do(req, &resp); err != nil {
		return nil, fmt.Errorf("failed to fetch AI reasoning: %w", err)
	}
	return resp.Entries, nil
}

func (c *Client) do(req *http.Request, resp any) error {
	httpResp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		return handleError(httpResp)
	}

	if resp != nil {
		if err := json.NewDecoder(httpResp.Body).Decode(resp); err != nil {
			return fmt.Errorf("failed to decode response body: %w", err)
		}
	}

	return nil
}

func toBody(req any) io.Reader {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(req); err != nil {
		return &errReader{err: err}
	}
	return &buf
}

type httpError struct {
	statusCode int
	body       string
	err        error
}

func (h *httpError) Error() string {
	if h.err != nil {
		return fmt.Sprintf("[%d] failed to handle error: %v", h.statusCode, h.err)
	}
	return fmt.Sprintf("[%d] error from server: %s", h.statusCode, h.body)
}

func handleError(resp *http.Response) error {
	dat, err := io.ReadAll(resp.Body)
	if err != nil {
		return &httpError{
			statusCode: resp.StatusCode,
			err:        fmt.Errorf("failed to read error response body: %w", err),
		}
	}

	return &httpError{
		statusCode: resp.StatusCode,
		body:       string(dat),
	}
}

type errReader struct {
	err error
}

func (e *errReader) Read(_ []byte) (int, error) {
	return 0, e.err
}
