package ollama

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	t.Setenv(BaseURLEnv, server.URL)
	t.Setenv(APIKeyEnv, "")

	return New(), server
}

func userRequest(content string) ChatRequest {
	return ChatRequest{
		Model:    "gemma4:e2b",
		Messages: []Message{{Role: RoleUser, Content: content}},
	}
}

func TestChatSendsNonStreamingRequest(t *testing.T) {
	var (
		gotPath   string
		gotMethod string
		gotAuth   string
		gotBody   map[string]any
	)

	chatClient, _ := newTestClient(t, func(writer http.ResponseWriter, req *http.Request) {
		gotPath = req.URL.Path
		gotMethod = req.Method
		gotAuth = req.Header.Get(authorizationHeader)
		require.NoError(t, json.NewDecoder(req.Body).Decode(&gotBody), "decode request")

		writer.Header().Set("Content-Type", "application/json")
		_, err := io.WriteString(writer, `{
			"model": "gemma4:e2b",
			"created_at": "2025-10-17T23:14:07.414671Z",
			"message": {"role": "assistant", "content": "Hello! How can I help you today?"},
			"done": true,
			"done_reason": "stop",
			"prompt_eval_count": 11,
			"eval_count": 18
		}`)
		require.NoError(t, err, "write response")
	})

	chatClient.apiKey = "secret"

	response, err := chatClient.Chat(context.Background(), userRequest("hello"))
	require.NoError(t, err, "chat")

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, chatPath, gotPath)
	assert.Equal(t, bearerPrefix+"secret", gotAuth)
	assert.Equal(t, "gemma4:e2b", gotBody["model"])
	assert.Equal(t, false, gotBody["stream"], "streaming is always disabled")

	messages, ok := gotBody["messages"].([]any)
	require.True(t, ok, "messages array")
	require.Len(t, messages, 1)
	assert.Equal(t, map[string]any{"role": "user", "content": "hello"}, messages[0])

	assert.Equal(t, "Hello! How can I help you today?", response.Message.Content)
	assert.Equal(t, RoleAssistant, response.Message.Role)
	assert.True(t, response.Done)
	assert.Equal(t, "stop", response.DoneReason)
	assert.Equal(t, 11, response.PromptEvalCount)
	assert.Equal(t, 18, response.EvalCount)
	assert.Equal(t, time.Date(2025, 10, 17, 23, 14, 7, 414671000, time.UTC), response.CreatedAt)
}

func TestChatOmitsAuthorizationWithoutAPIKey(t *testing.T) {
	var gotAuth string

	chatClient, _ := newTestClient(t, func(writer http.ResponseWriter, req *http.Request) {
		gotAuth = req.Header.Get(authorizationHeader)
		_, err := io.WriteString(writer, `{"message": {"role": "assistant", "content": "ok"}}`)
		require.NoError(t, err)
	})

	_, err := chatClient.Chat(context.Background(), userRequest("hello"))
	require.NoError(t, err, "chat")

	assert.Empty(t, gotAuth)
}

func TestChatReportsAPIError(t *testing.T) {
	chatClient, _ := newTestClient(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
		_, err := io.WriteString(writer, `{"error": "model 'nope' not found"}`)
		require.NoError(t, err)
	})

	_, err := chatClient.Chat(context.Background(), userRequest("hello"))
	require.Error(t, err, "chat")

	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusNotFound, apiErr.StatusCode)
	assert.Equal(t, "model 'nope' not found", apiErr.Message)
	assert.Equal(t, "ollama chat: 404: model 'nope' not found", err.Error())
}

func TestChatReportsPlainTextError(t *testing.T) {
	chatClient, _ := newTestClient(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadGateway)
		_, err := io.WriteString(writer, "  upstream unavailable  ")
		require.NoError(t, err)
	})

	_, err := chatClient.Chat(context.Background(), userRequest("hello"))
	require.Error(t, err, "chat")

	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "upstream unavailable", apiErr.Message)
}

func TestChatReportsEmptyErrorBody(t *testing.T) {
	chatClient, _ := newTestClient(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusInternalServerError)
	})

	_, err := chatClient.Chat(context.Background(), userRequest("hello"))
	require.Error(t, err, "chat")
	assert.Equal(t, "ollama chat: 500 Internal Server Error", err.Error())
}

func TestChatReportsBadJSON(t *testing.T) {
	chatClient, _ := newTestClient(t, func(writer http.ResponseWriter, _ *http.Request) {
		_, err := io.WriteString(writer, "definitely not json")
		require.NoError(t, err)
	})

	_, err := chatClient.Chat(context.Background(), userRequest("hello"))
	require.Error(t, err, "chat")
	assert.Contains(t, err.Error(), "decode response")
}

func TestChatRejectsInvalidRequests(t *testing.T) {
	chatClient := New()

	testCases := map[string]ChatRequest{
		"missing model": {Messages: []Message{{Role: RoleUser, Content: "hi"}}},
		"blank model":   {Model: "  ", Messages: []Message{{Role: RoleUser, Content: "hi"}}},
		"no messages":   {Model: "gemma4"},
		"missing role":  {Model: "gemma4", Messages: []Message{{Content: "hi"}}},
	}

	for name, request := range testCases {
		t.Run(name, func(t *testing.T) {
			response, err := chatClient.Chat(context.Background(), request)
			require.Error(t, err)
			assert.Zero(t, response, "no response on validation failure")
		})
	}
}

func TestChatHonorsContextCancellation(t *testing.T) {
	chatClient, _ := newTestClient(t, func(writer http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, err := io.WriteString(writer, `{"message": {"content": "late"}}`)
		assert.NoError(t, err)
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := chatClient.Chat(ctx, userRequest("hello"))
	require.Error(t, err, "chat")

	assert.ErrorIs(t, err, context.Canceled)
}

func TestNewOptions(t *testing.T) {
	t.Setenv(BaseURLEnv, "ollama.example.com/prefix/")
	t.Setenv(APIKeyEnv, "env-key")

	chatClient := New(WithBaseURL("https://cloud.example.com/"), WithAPIKey("opt-key"), WithTimeout(time.Second))

	assert.Equal(t, "https://cloud.example.com", chatClient.baseURL)
	assert.Equal(t, "https://cloud.example.com/api/chat", chatClient.endpoint)
	assert.Equal(t, "opt-key", chatClient.apiKey)
	assert.Equal(t, time.Second, chatClient.timeout)
}

func TestNewReadsEnvironment(t *testing.T) {
	t.Setenv(BaseURLEnv, "localhost:11434")
	t.Setenv(APIKeyEnv, "env-key")

	chatClient := New()

	assert.Equal(t, "http://localhost:11434", chatClient.baseURL)
	assert.Equal(t, "env-key", chatClient.apiKey)
	assert.Equal(t, DefaultTimeout, chatClient.timeout)
}

func TestNormalizeBaseURL(t *testing.T) {
	testCases := map[string]string{
		"":                               DefaultBaseURL,
		"   ":                            DefaultBaseURL,
		"localhost:11434":                "http://localhost:11434",
		"localhost:11434/":               "http://localhost:11434",
		"https://ollama.com":             "https://ollama.com",
		"https://ollama.com///":          "https://ollama.com",
		"http://[::1]:11434":             "http://[::1]:11434",
		"http://host/path/with/slashes/": "http://host/path/with/slashes",
	}

	for input, want := range testCases {
		assert.Equal(t, want, normalizeBaseURL(input), "input %q", input)
	}
}

func TestTruncateUTF8(t *testing.T) {
	assert.Equal(t, "abc", truncateUTF8("abc", 8), "short string is untouched")
	assert.Equal(t, "abc", truncateUTF8("abcdef", 3), "ascii cut")
	assert.Equal(t, "ab", truncateUTF8("abé", 3), "does not split a rune")
	assert.Equal(t, "", truncateUTF8("é", 1), "single multi-byte rune below limit")
}

func TestChatReportsLongPlainTextError(t *testing.T) {
	long := strings.Repeat("x", maxErrorBody+100)

	chatClient, _ := newTestClient(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadGateway)
		_, err := io.WriteString(writer, long)
		require.NoError(t, err)
	})

	_, err := chatClient.Chat(context.Background(), userRequest("hello"))
	require.Error(t, err, "chat")

	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Len(t, apiErr.Message, maxErrorBody)
}

func TestChatRequestValidate(t *testing.T) {
	require.NoError(t, userRequest("hi").validate(), "valid request")

	assert.Error(t, ChatRequest{}.validate(), "empty request")
	assert.Error(t, ChatRequest{Model: "m"}.validate(), "missing messages")
	assert.Error(t, ChatRequest{
		Model:    "m",
		Messages: []Message{{Role: RoleSystem}, {Role: ""}},
	}.validate(), "empty role in later message")
}

func TestChatRequestJSONShape(t *testing.T) {
	body, err := json.Marshal(ChatRequest{
		Model:    "gemma4",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
		Tools: []ToolDefinition{{
			Type: "function",
			Function: FunctionDefinition{
				Name:       "lookup",
				Parameters: json.RawMessage(`{"type":"object"}`),
			},
		}},
		Format:    json.RawMessage(`"json"`),
		Options:   map[string]any{"temperature": 0},
		KeepAlive: "5m",
	})
	require.NoError(t, err, "marshal request")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload), "unmarshal request")

	assert.Equal(t, "5m", payload["keep_alive"])
	assert.Equal(t, "json", payload["format"])
	assert.Equal(t, map[string]any{"temperature": float64(0)}, payload["options"])

	tools, ok := payload["tools"].([]any)
	require.True(t, ok, "tools array")
	require.Len(t, tools, 1)

	tool := tools[0].(map[string]any)
	assert.Equal(t, "function", tool["type"])
	assert.Equal(t, "lookup", tool["function"].(map[string]any)["name"])
}

func TestChatRequestOmitsEmptyOptionals(t *testing.T) {
	body, err := json.Marshal(userRequest("hi"))
	require.NoError(t, err, "marshal request")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload), "unmarshal request")

	for _, key := range []string{"tools", "format", "options", "keep_alive"} {
		assert.NotContains(t, payload, key)
	}
}
