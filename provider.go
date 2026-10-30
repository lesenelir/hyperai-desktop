package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
	"github.com/anthropics/anthropic-sdk-go/v2/shared/constant"
)

// turn is a message of a conversation as a provider sees it.
type turn struct {
	Role    string `json:"role"` // "user" or "assistant"
	Content string `json:"content"`
}

// provider answers conversations with a language model.
type provider interface {
	// ID is the provider's, as its keys and settings are kept by: its
	// providerKind's, "demo" for the demo.
	ID() string
	// Name is what the settings and the composer's menu call the provider.
	Name() string
	// Models are the models the composer offers, the first by default.
	Models() []string
	// List returns the models the provider has, as its API lists them.
	List(ctx context.Context) ([]string, error)
	// Stream asks model to reply to turns, calling delta with each piece of
	// the reply as it comes. It returns once the reply ends, ctx is
	// canceled or the request fails.
	Stream(ctx context.Context, model string, turns []turn, delta func(string)) error
}

// providerKind is a service the Providers page offers, which replies come
// from once it has the user's API key: its ID, as the keys and settings
// are kept by; its name; the endpoint its API is at, which the user may
// change, as for a proxy; where its keys are made; the models the composer offers of it until the user chooses
// others, the first by default, as many of them as it lists; chat, which
// tells the models it lists that chat from the others, nil for all; and
// open, which returns the provider at an endpoint, for a key, offering
// models.
type providerKind struct {
	id, name string
	baseURL  string
	keyURL   string
	models   []string
	chat     func(model string) bool
	open     func(k *providerKind, baseURL, key string, models []string) provider
}

// providerKinds are the services HyperAI talks to, in the order the
// Providers page and the composer's menu list them. All but Anthropic
// take OpenAI's chat completions API, Google at an endpoint it keeps for
// that.
var providerKinds = []providerKind{
	{id: "openai", name: "OpenAI", baseURL: "https://api.openai.com/v1", keyURL: "https://platform.openai.com/api-keys",
		models: []string{"gpt-5", "gpt-5-mini"}, chat: openAIChat, open: openChatCompletions},
	{id: "anthropic", name: "Anthropic", baseURL: "https://api.anthropic.com", keyURL: "https://console.anthropic.com/settings/keys",
		models: []string{"claude-opus-5-5", "claude-sonnet-5-5", "claude-haiku-5-5"}, open: openAnthropic},
	{id: "google", name: "Google", baseURL: "https://generativelanguage.googleapis.com/v1beta/openai", keyURL: "https://aistudio.google.com/apikey",
		models: []string{"gemini-4-argon", "gemini-3.8-flash", "gemini-3.5-flash-lite"}, chat: geminiChat, open: openChatCompletions},
	{id: "openrouter", name: "OpenRouter", baseURL: "https://openrouter.ai/api/v1", keyURL: "https://openrouter.ai/keys",
		models: []string{"anthropic/claude-sonnet-5.5", "anthropic/claude-opus-5.5", "anthropic/claude-haiku-5.5"}, open: openChatCompletions},
	{id: "deepseek", name: "DeepSeek", baseURL: "https://api.deepseek.com/v1", keyURL: "https://platform.deepseek.com/api_keys",
		models: []string{"deepseek-chat", "deepseek-reasoner"}, open: openChatCompletions},
}

// providerKindOf returns the kind of id, nil for none.
func providerKindOf(id string) *providerKind {
	for i := range providerKinds {
		if providerKinds[i].id == id {
			return &providerKinds[i]
		}
	}
	return nil
}

// openAIChat tells OpenAI's models that chat from those that embed, speak,
// listen, draw or moderate, which its list holds too.
func openAIChat(model string) bool {
	for _, not := range []string{"embedding", "tts", "whisper", "transcribe", "audio", "realtime", "dall-e", "image",
		"moderation", "davinci", "babbage", "search", "computer-use", "sora"} {
		if strings.Contains(model, not) {
			return false
		}
	}
	return true
}

// geminiChat tells Google's models that chat from those openAIChat would
// not, and those that film, compose music, answer of a corpus, drive
// robots or talk live, which its list holds too.
func geminiChat(model string) bool {
	for _, not := range []string{"veo", "lyria", "aqa", "live", "robotics"} {
		if strings.Contains(model, not) {
			return false
		}
	}
	return openAIChat(model)
}

func openChatCompletions(k *providerKind, baseURL, key string, models []string) provider {
	return &openAI{id: k.id, name: k.name, baseURL: baseURL, apiKey: key, models: models, client: http.DefaultClient}
}

// openAI streams replies through OpenAI's chat completions API, as OpenAI,
// Google, OpenRouter and DeepSeek take it.
type openAI struct {
	id, name, baseURL, apiKey string
	models                    []string
	client                    *http.Client
}

func (p *openAI) ID() string       { return p.id }
func (p *openAI) Name() string     { return p.name }
func (p *openAI) Models() []string { return p.models }

func (p *openAI) List(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	res, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, statusError(res)
	}
	var list struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("reading the models: %w", err)
	}
	var ids []string
	for _, m := range list.Data {
		// Google's may name a model by its path in its own API,
		// "models/gemini-…", where chats name it bare.
		ids = append(ids, strings.TrimPrefix(m.ID, "models/"))
	}
	return ids, nil
}

func (p *openAI) Stream(ctx context.Context, model string, turns []turn, delta func(string)) error {
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
	// Names the app in OpenRouter's activity log; the others ignore it.
	req.Header.Set("X-Title", "HyperAI")
	res, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return statusError(res)
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

// statusError returns what a response that failed says, under its
// status: the message of its error, as OpenAI's API and those like it
// give it, else its body.
func statusError(res *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(res.Body, 4<<10))
	var body struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	msg := strings.TrimSpace(string(b))
	if json.Unmarshal(b, &body) == nil && body.Error.Message != "" {
		msg = body.Error.Message
	}
	return fmt.Errorf("%s: %s", res.Status, msg)
}

func openAnthropic(k *providerKind, baseURL, key string, models []string) provider {
	// Of the key and endpoint given, and not the ANTHROPIC_ variables the
	// SDK reads of the environment otherwise.
	client := anthropic.NewClient(option.WithoutEnvironmentDefaults(), option.WithAPIKey(key), option.WithBaseURL(baseURL))
	return &anthropicAPI{client: client, models: models}
}

// anthropicAPI streams replies from Claude, through Anthropic's Messages
// API and its SDK.
type anthropicAPI struct {
	client anthropic.Client
	models []string
}

func (p *anthropicAPI) ID() string       { return "anthropic" }
func (p *anthropicAPI) Name() string     { return "Anthropic" }
func (p *anthropicAPI) Models() []string { return p.models }

func (p *anthropicAPI) List(ctx context.Context) ([]string, error) {
	var ids []string
	pages := p.client.Models.ListAutoPaging(ctx, anthropic.ModelListParams{})
	for pages.Next() {
		ids = append(ids, pages.Current().ID)
	}
	if err := pages.Err(); err != nil {
		return nil, anthropicError(err)
	}
	return ids, nil
}

// anthropicFallbacks are the models whose declines Anthropic retries on
// another model of its choosing, in the same stream, where a request asks
// it to. Claude Haiku has none.
var anthropicFallbacks = []string{"claude-fable-5-1", "claude-opus-5-5", "claude-opus-5", "claude-sonnet-5-5"}

func (p *anthropicAPI) Stream(ctx context.Context, model string, turns []turn, delta func(string)) error {
	var msgs []anthropic.BetaMessageParam
	for _, t := range turns {
		block := anthropic.NewBetaTextBlock(t.Content)
		if t.Role == "assistant" {
			msgs = append(msgs, anthropic.BetaMessageParam{OfAssistant: &anthropic.BetaAssistantMessageParam{
				Content: []anthropic.BetaContentBlockParamUnion{block}}})
		} else {
			msgs = append(msgs, anthropic.NewBetaUserMessage(block))
		}
	}
	// Room for a long reply, which streaming lets take its time.
	params := anthropic.BetaMessageNewParams{Model: model, MaxTokens: 64000, Messages: msgs}
	if slices.Contains(anthropicFallbacks, model) {
		params.Betas = []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01}
		params.Fallbacks = anthropic.BetaFallbacksParamUnion{OfDefault: constant.ValueOf[constant.Default]()}
	}
	stream := p.client.Beta.Messages.NewStreaming(ctx, params)
	defer stream.Close()
	var refused *anthropic.BetaRefusalStopDetails
	for stream.Next() {
		switch ev := stream.Current().AsAny().(type) {
		case anthropic.BetaRawContentBlockDeltaEvent:
			if d, ok := ev.Delta.AsAny().(anthropic.BetaTextDelta); ok && d.Text != "" {
				delta(d.Text)
			}
		case anthropic.BetaRawMessageDeltaEvent:
			// Declined, by the model or its safeguards, and by its fallback
			// too where it has one.
			if ev.Delta.StopReason == anthropic.BetaStopReasonRefusal {
				refused = &ev.Delta.StopDetails
			}
		}
	}
	if err := stream.Err(); err != nil {
		return anthropicError(err)
	}
	if refused != nil {
		if refused.Explanation != "" {
			return fmt.Errorf("Claude declined to answer: %s", refused.Explanation)
		}
		return errors.New("Claude declined to answer.")
	}
	return nil
}

// anthropicError returns err as a reply shows it: what the API said, under
// its status, rather than the SDK's whole request.
func anthropicError(err error) error {
	var apiErr *anthropic.Error
	if !errors.As(err, &apiErr) {
		return err
	}
	var body struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(apiErr.RawJSON()), &body) != nil || body.Error.Message == "" {
		return err
	}
	// An error event of a stream that began, whose status is the stream's.
	if apiErr.StatusCode < 300 {
		return errors.New(body.Error.Message)
	}
	return fmt.Errorf("%d %s: %s", apiErr.StatusCode, http.StatusText(apiErr.StatusCode), body.Error.Message)
}
