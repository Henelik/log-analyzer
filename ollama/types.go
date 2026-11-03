package ollama

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Role identifies the author of a chat message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is one turn in a chat conversation.
type Message struct {
	Role      Role       `json:"role"`
	Content   string     `json:"content"`
	Images    []string   `json:"images,omitempty"`
	Thinking  string     `json:"thinking,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// ToolDefinition describes a function the model may call.
type ToolDefinition struct {
	Type     string             `json:"type"`
	Function FunctionDefinition `json:"function"`
}

// FunctionDefinition is the JSON schema of a callable tool.
type FunctionDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// ToolCall is a function invocation requested by the model.
type ToolCall struct {
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction names the requested function and carries its JSON arguments.
type ToolCallFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Arguments   json.RawMessage `json:"arguments,omitempty"`
}

// ChatRequest is the body sent to POST /api/chat.  Chat always disables
// streaming, so the response is a single JSON object.
type ChatRequest struct {
	Model     string           `json:"model"`
	Messages  []Message        `json:"messages"`
	Tools     []ToolDefinition `json:"tools,omitempty"`
	Format    json.RawMessage  `json:"format,omitempty"`
	Options   map[string]any   `json:"options,omitempty"`
	KeepAlive string           `json:"keep_alive,omitempty"`
}

// ChatResponse is a completed, non-streaming chat response.  Durations are in
// nanoseconds, as reported by Ollama.
type ChatResponse struct {
	Model              string    `json:"model"`
	CreatedAt          time.Time `json:"created_at"`
	Message            Message   `json:"message"`
	Done               bool      `json:"done"`
	DoneReason         string    `json:"done_reason"`
	TotalDuration      int64     `json:"total_duration"`
	LoadDuration       int64     `json:"load_duration"`
	PromptEvalCount    int       `json:"prompt_eval_count"`
	PromptEvalDuration int64     `json:"prompt_eval_duration"`
	EvalCount          int       `json:"eval_count"`
	EvalDuration       int64     `json:"eval_duration"`
}

// validate rejects requests Ollama would reject without spending a round trip.
func (r ChatRequest) validate() error {
	if strings.TrimSpace(r.Model) == "" {
		return fmt.Errorf("ollama chat: model is required")
	}

	if len(r.Messages) == 0 {
		return fmt.Errorf("ollama chat: at least one message is required")
	}

	for i, message := range r.Messages {
		if strings.TrimSpace(string(message.Role)) == "" {
			return fmt.Errorf("ollama chat: message %d: role is required", i)
		}
	}

	return nil
}
