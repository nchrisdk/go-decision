package decision

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// maxResponseSize bounds memory used while reading API responses.
const maxResponseSize = 32 << 20

// statusServiceOverloaded is the API's nonstandard HTTP status for overloads.
const statusServiceOverloaded = 529

var (
	// ErrRateLimited identifies a 429 response.
	ErrRateLimited = errors.New("decision: rate limited")
	// ErrOverloaded identifies a 529 response.
	ErrOverloaded = errors.New("decision: service overloaded")
)

// Client is a decision API client. The request and response formats follow
// the API type in the config; the client translates them as needed.
type Client struct {
	apiKey     string
	baseURL    string
	model      string
	adapter    Adapter
	httpClient *http.Client
}

// New creates a Client from the given config and http client. The client
// must not be nil.
func New(cfg Config, client *http.Client) (*Client, error) {
	dialect, err := newAdapter(cfg.APIType)
	if err != nil {
		return nil, err
	}

	if cfg.BaseURL == "" {
		return nil, errors.New("decision: no base URL set")
	}

	if client == nil {
		return nil, errors.New("decision: no http client set")
	}

	return &Client{
		apiKey:     cfg.APIKey,
		baseURL:    trimBaseURL(cfg.BaseURL),
		model:      cfg.Model,
		adapter:    dialect,
		httpClient: client,
	}, nil
}

// trimBaseURL normalizes a base URL for the adapter paths, which carry the
// version. A base URL that already ends in the version prefix would
// otherwise produce a doubled path, e.g. /v1/v1/decisions.
func trimBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(baseURL, "/")

	if trimmed, hasVersion := strings.CutSuffix(baseURL, "/v1"); hasVersion {
		return trimmed
	}

	return baseURL
}

// APIError is returned when the API answers with a non-2xx status.
type APIError struct {
	// StatusCode is the HTTP status code.
	StatusCode int
	// Message is the trimmed response body.
	Message string
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return fmt.Sprintf("decision: HTTP %d: %s", e.StatusCode, e.Message)
}

// Is lets callers identify rate limits and overloads without inspecting status codes.
func (e *APIError) Is(target error) bool {
	return (target == ErrRateLimited && e.StatusCode == http.StatusTooManyRequests) ||
		(target == ErrOverloaded && e.StatusCode == statusServiceOverloaded)
}

// Decide sends req to the API and returns the response.
// The model is the one from the client config.
func (c *Client) Decide(ctx context.Context, req *Request) (*Response, error) {
	if req == nil {
		return nil, errors.New("decision: request is nil")
	}

	payload, err := c.adapter.EncodeRequest(req, c.model)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("decision: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+c.adapter.Path(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("decision: create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("decision: do request: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	data, err := readResponse(resp)
	if err != nil {
		return nil, err
	}

	out, err := c.adapter.DecodeResponse(data)
	if err != nil {
		return nil, fmt.Errorf("decision: decode response: %w", err)
	}

	err = validateAnswers(req, out)
	if err != nil {
		return nil, err
	}

	return out, nil
}

func validateAnswers(request *Request, response *Response) error {
	for questionID := range request.Questions {
		if _, ok := response.Answers[questionID]; !ok {
			return fmt.Errorf("decision: response has no answer for %q", questionID)
		}
	}

	return nil
}

func readResponse(resp *http.Response) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if err != nil {
		return nil, fmt.Errorf("decision: read response: %w", err)
	}

	if len(data) > maxResponseSize {
		return nil, fmt.Errorf("decision: response exceeds %d bytes", maxResponseSize)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &APIError{StatusCode: resp.StatusCode, Message: strings.TrimSpace(string(data))}
	}

	return data, nil
}
