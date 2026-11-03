// Package ollama calls the Ollama chat API.
//
// New builds a client and Chat sends one non-streaming request, returning the
// assistant's reply.  The client targets the local server at
// http://localhost:11434 by default and reads OLLAMA_HOST and OLLAMA_API_KEY to
// configure the base URL and the bearer token used by Ollama Cloud.
package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gofiber/fiber/v3/client"
)

const (
	// BaseURLEnv overrides the Ollama base URL.  Hosts without a scheme are
	// treated as plain HTTP, matching Ollama's own OLLAMA_HOST convention.
	BaseURLEnv = "OLLAMA_HOST"
	// APIKeyEnv holds the bearer token for Ollama Cloud requests.
	APIKeyEnv = "OLLAMA_API_KEY"

	// DefaultBaseURL is the local Ollama server.
	DefaultBaseURL = "http://localhost:11434"
	// DefaultTimeout bounds a single chat request, including time for the
	// server to load the model.
	DefaultTimeout = 5 * time.Minute

	chatPath            = "/api/chat"
	authorizationHeader = "Authorization"
	bearerPrefix        = "Bearer "
	maxErrorBody        = 512
)

// Client sends chat requests to an Ollama server.  It is safe for concurrent
// use once constructed.
type Client struct {
	http     *client.Client
	baseURL  string
	endpoint string
	apiKey   string
	timeout  time.Duration
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the Ollama base URL.  An empty URL is ignored.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		if baseURL != "" {
			c.baseURL = baseURL
		}
	}
}

// WithAPIKey sets the bearer token used for Ollama Cloud.  An empty key sends
// no Authorization header, allowing local server use.
func WithAPIKey(apiKey string) Option {
	return func(c *Client) {
		c.apiKey = apiKey
	}
}

// WithTimeout bounds each request.  A non-positive timeout disables the
// deadline.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		c.timeout = timeout
	}
}

// New returns a client configured from the environment and then opts.
func New(opts ...Option) *Client {
	c := &Client{
		http:    client.New(),
		baseURL: os.Getenv(BaseURLEnv),
		apiKey:  os.Getenv(APIKeyEnv),
		timeout: DefaultTimeout,
	}

	for _, opt := range opts {
		opt(c)
	}

	c.baseURL = normalizeBaseURL(c.baseURL)
	c.endpoint = c.baseURL + chatPath

	return c
}

// Chat sends a non-streaming chat request and returns the assistant's reply.
// The returned response is only meaningful when err is nil.
func (c *Client) Chat(ctx context.Context, request ChatRequest) (ChatResponse, error) {
	if err := request.validate(); err != nil {
		return ChatResponse{}, err
	}

	payload := chatPayload{ChatRequest: request, Stream: false}

	config := client.Config{
		Ctx:     ctx,
		Body:    payload,
		Timeout: c.timeout,
	}
	if c.apiKey != "" {
		config.Header = map[string]string{authorizationHeader: bearerPrefix + c.apiKey}
	}

	response, err := c.http.Post(c.endpoint, config)
	if err != nil {
		// Fiber reports cancellation and timeout as one error; surface the
		// caller's context cause when there is one.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ChatResponse{}, fmt.Errorf("ollama chat: %w", ctxErr)
		}

		return ChatResponse{}, fmt.Errorf("ollama chat: request failed: %w", err)
	}
	defer response.Close()

	if response.StatusCode() != http.StatusOK {
		return ChatResponse{}, newAPIError(response)
	}

	var chat ChatResponse
	if err := response.JSON(&chat); err != nil {
		return ChatResponse{}, fmt.Errorf("ollama chat: decode response: %w", err)
	}

	return chat, nil
}

// chatPayload pins stream to false.  ChatRequest omits the field so callers
// cannot accidentally request an NDJSON stream this package does not parse.
type chatPayload struct {
	ChatRequest
	Stream bool `json:"stream"`
}

// APIError reports a non-200 response from the Ollama server.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("ollama chat: %d %s", e.StatusCode, http.StatusText(e.StatusCode))
	}

	return fmt.Sprintf("ollama chat: %d: %s", e.StatusCode, e.Message)
}

// newAPIError extracts Ollama's {"error": "..."} body, falling back to the raw
// body for proxies and other servers that return plain text.
func newAPIError(response *client.Response) error {
	body := response.Body()

	var payload struct {
		Error string `json:"error"`
	}

	message := ""
	if err := json.Unmarshal(body, &payload); err == nil {
		message = strings.TrimSpace(payload.Error)
	}

	if message == "" {
		message = strings.TrimSpace(string(body))
		message = truncateUTF8(message, maxErrorBody)
	}

	return &APIError{StatusCode: response.StatusCode(), Message: message}
}

// normalizeBaseURL adds a scheme when missing and trims trailing slashes.
func normalizeBaseURL(baseURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return DefaultBaseURL
	}

	if !strings.Contains(baseURL, "://") {
		baseURL = "http://" + baseURL
	}

	return strings.TrimRight(baseURL, "/")
}

// truncateUTF8 shortens s to at most maxBytes without splitting a rune.
func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}

	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}

	return s[:maxBytes]
}
