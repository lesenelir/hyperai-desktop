package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"strings"
	"time"
)

// turn is a message of a conversation as a provider sees it.
type turn struct {
	Role    string `json:"role"` // "user" or "assistant"
	Content string `json:"content"`
}

// provider answers conversations with a language model.
type provider interface {
	// Name is what the sidebar calls the provider.
	Name() string
	// Models are the models the composer offers, the first by default.
	Models() []string
	// Stream asks model to reply to turns, calling delta with each piece of
	// the reply as it comes. It returns once the reply ends, ctx is
	// canceled or the request fails.
	Stream(ctx context.Context, model string, turns []turn, delta func(string)) error
}

// defaultModels are the models offered on OpenRouter unless HYPERAI_MODELS
// says otherwise, the first by default.
var defaultModels = []string{"anthropic/claude-sonnet-5.5", "anthropic/claude-opus-5.5", "anthropic/claude-haiku-5.5"}

// newProvider returns OpenRouter when OPENROUTER_API_KEY is set, and the
// demo otherwise. HYPERAI_MODELS lists the models to offer, by their
// OpenRouter IDs, comma-separated.
func newProvider() provider {
	key := os.Getenv("OPENROUTER_API_KEY")
	if key == "" {
		return &demo{paced: true}
	}
	var models []string
	for _, m := range strings.Split(os.Getenv("HYPERAI_MODELS"), ",") {
		if m = strings.TrimSpace(m); m != "" {
			models = append(models, m)
		}
	}
	if len(models) == 0 {
		models = defaultModels
	}
	return &openRouter{baseURL: "https://openrouter.ai/api/v1", apiKey: key, models: models, client: http.DefaultClient}
}

// openRouter streams replies from OpenRouter, through its OpenAI-compatible
// chat completions API.
type openRouter struct {
	baseURL, apiKey string
	models          []string
	client          *http.Client
}

func (p *openRouter) Name() string     { return "OpenRouter" }
func (p *openRouter) Models() []string { return p.models }

func (p *openRouter) Stream(ctx context.Context, model string, turns []turn, delta func(string)) error {
	body, err := json.Marshal(map[string]any{"model": model, "messages": turns, "stream": true})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	// Names the app in OpenRouter's activity log.
	req.Header.Set("X-Title", "HyperAI")
	res, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 4<<10))
		return fmt.Errorf("%s: %s", res.Status, strings.TrimSpace(string(msg)))
	}
	// The reply comes as server-sent events, a chunk of JSON per line, and
	// comments (": OPENROUTER PROCESSING") while the model warms up.
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		data, ok := strings.CutPrefix(sc.Text(), "data:")
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)
		if data == "[DONE]" {
			return nil
		}
		var chunk struct {
			// Error is set when the request fails after the reply began.
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return fmt.Errorf("reading the reply: %w", err)
		}
		if chunk.Error != nil {
			return errors.New(chunk.Error.Message)
		}
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			delta(chunk.Choices[0].Delta.Content)
		}
	}
	return sc.Err()
}

// demo is a pretend model whose canned Markdown replies stream in at the
// uneven pace of a model when paced, at once otherwise (in tests).
type demo struct {
	paced   bool
	replies int
}

func (p *demo) Name() string     { return "Demo" }
func (p *demo) Models() []string { return []string{"hyperai-demo"} }

func (p *demo) Stream(ctx context.Context, model string, turns []turn, delta func(string)) error {
	src := demoReplies[p.replies%len(demoReplies)]
	p.replies++
	if !p.paced {
		delta(src)
		return nil
	}
	if err := sleep(ctx, 600*time.Millisecond); err != nil { // thinking
		return err
	}
	for i := 0; i < len(src); {
		// Tokens of a few bytes, never splitting a UTF-8 sequence.
		n := min(len(src)-i, 2+rand.IntN(7))
		for n < len(src)-i && src[i+n]&0xC0 == 0x80 {
			n++
		}
		if err := sleep(ctx, time.Duration(8+rand.IntN(30))*time.Millisecond); err != nil {
			return err
		}
		delta(src[i : i+n])
		i += n
	}
	return nil
}

// sleep waits for d, or until ctx is canceled.
func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

var demoReplies = []string{
	"Here is a small **LRU cache** in Go, safe for concurrent use:\n\n" +
		"```go\ntype LRU struct {\n\tmu    sync.Mutex\n\tcap   int\n\titems map[string]*list.Element\n\torder *list.List\n}\n\n" +
		"func (c *LRU) Get(key string) (any, bool) {\n\tc.mu.Lock()\n\tdefer c.mu.Unlock()\n\tif e, ok := c.items[key]; ok {\n\t\tc.order.MoveToFront(e)\n\t\treturn e.Value.(*entry).value, true\n\t}\n\treturn nil, false\n}\n```\n\n" +
		"A few things to note:\n\n1. `container/list` keeps the order of use in O(1).\n2. The mutex makes `Get` a writer too, since it moves the entry.\n3. For heavy read traffic, consider **sharding** by key hash.\n",
	"## Plan\n\nI'd split the work into three steps:\n\n" +
		"| Step | What changes | Risk |\n|---|---|:-:|\n| 1 | Extract the `provider` interface | low |\n| 2 | Stream replies through `Window.Update` | medium |\n| 3 | Persist chats as JSON | low |\n\n" +
		"> **Tip:** keep slow I/O off the UI thread, and publish results with `Window.Update`.\n\n" +
		"Want me to start with step 1?",
	"This is **demo mode**: replies are canned, streamed token by token the way a model streams them.\n\n" +
		"To talk to a real model, put an [OpenRouter](https://openrouter.ai/keys) key in `.env`:\n\n" +
		"```sh\nOPENROUTER_API_KEY=sk-or-...\n```\n\n" +
		"- `HYPERAI_MODELS` lists the models to offer, by their OpenRouter IDs, comma-separated\n",
}
