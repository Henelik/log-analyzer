// Command ollama-chat sends a single prompt to the Ollama chat API and prints
// the assistant's reply.  It exists to exercise the ollama package by hand.
//
// Environment:
//
//	OLLAMA_HOST     base URL, default http://localhost:11434
//	OLLAMA_API_KEY  bearer token for Ollama Cloud
//	OLLAMA_MODEL    model name, overridden by -model
//
// Usage:
//
//	ollama-chat -model gemma4:e2b "why is the sky blue?"
//	echo "summarize this log" | ollama-chat -model gemma4:e2b
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Henelik/log-analyzer/ollama"
)

const (
	modelEnv = "OLLAMA_MODEL"

	defaultSystemPrompt = "You are a terse assistant. Answer in one paragraph."
)

func main() {
	var (
		model   = flag.String("model", os.Getenv(modelEnv), "model name (default $"+modelEnv+")")
		system  = flag.String("system", defaultSystemPrompt, "system prompt")
		timeout = flag.Duration("timeout", ollama.DefaultTimeout, "request timeout")
		baseURL = flag.String("url", "", "Ollama base URL (default $"+ollama.BaseURLEnv+" or "+ollama.DefaultBaseURL+")")
		verbose = flag.Bool("v", false, "print response timing and token counts to stderr")
	)

	flag.Parse()

	prompt, err := readPrompt(flag.Args())
	if err != nil {
		log.Fatal(err)
	}

	if *model == "" {
		log.Fatalf("no model: pass -model or set %s", modelEnv)
	}

	client := ollama.New(
		ollama.WithBaseURL(*baseURL),
		ollama.WithTimeout(*timeout),
	)

	messages := make([]ollama.Message, 0, 2)
	if strings.TrimSpace(*system) != "" {
		messages = append(messages, ollama.Message{Role: ollama.RoleSystem, Content: *system})
	}
	messages = append(messages, ollama.Message{Role: ollama.RoleUser, Content: prompt})

	start := time.Now()

	response, err := client.Chat(context.Background(), ollama.ChatRequest{
		Model:    *model,
		Messages: messages,
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(response.Message.Content)

	if *verbose {
		fmt.Fprintf(
			os.Stderr,
			"wall=%s model=%s prompt_tokens=%d eval_tokens=%d done_reason=%s\n",
			time.Since(start).Round(time.Millisecond),
			response.Model,
			response.PromptEvalCount,
			response.EvalCount,
			response.DoneReason,
		)
	}
}

// readPrompt joins the positional arguments, or reads stdin when none are
// given so the command composes with pipes.
func readPrompt(args []string) (string, error) {
	if len(args) > 0 {
		return strings.Join(args, " "), nil
	}

	info, err := os.Stdin.Stat()
	if err != nil {
		return "", fmt.Errorf("inspect stdin: %w", err)
	}

	if info.Mode()&os.ModeCharDevice != 0 {
		return "", fmt.Errorf("no prompt: pass it as an argument or on stdin")
	}

	body, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}

	prompt := strings.TrimSpace(string(body))
	if prompt == "" {
		return "", fmt.Errorf("empty prompt on stdin")
	}

	return prompt, nil
}
