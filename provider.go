package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
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

// turn is a message of a conversation as a provider sees it, with the
// files sent with it.
type turn struct {
	Role    string
	Content string
	Files   []attachment
}

// chunk is a piece of a reply as it streams in: of its text, or of the
// reasoning the model did before it, which some models show.
type chunk struct {
	Text      string
	Reasoning string
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
	// the reply as it comes, of its text or its reasoning. It returns once
	// the reply ends, ctx is canceled or the request fails.
	Stream(ctx context.Context, model string, turns []turn, delta func(chunk)) error
}

// providerKind is a service the Providers page offers, which replies come
// from once it has the user's API key: its ID, as the keys and settings
// are kept by; its name; the endpoint its API is at, which the user may
// change, as for a proxy; where its keys are made; the models the composer offers of it until the user chooses
// others, the first by default, as many of them as it lists; chat, which
// tells the models it lists that chat from the others, nil for all; open,
// which returns the provider at an endpoint, for a key, offering models;
// local, of a server on the user's computer, which takes no key, offers
// the models it runs, and is asked once the user turns it on, its keyURL
// where it is downloaded; titleModel, the cheap model that names its chats
// (titles.go), "" for the chat's own; copilot, of GitHub Copilot, asked through its
// CLI as the user signed in (copilot.go), which openKind opens, not open; and,
// of one the user added, api, the API it takes (customAPIs).
type providerKind struct {
	id, name   string
	baseURL    string
	keyURL     string
	models     []string
	chat       func(model string) bool
	open       func(k *providerKind, baseURL, key string, models []string) provider
	local      bool
	copilot    bool
	api        string
	titleModel string
}

// providerKinds are the services HyperAI talks to, in the order the
// Providers page and the composer's menu list them, the servers on the
// user's computer last. All but Anthropic take OpenAI's chat completions
// API, Google at an endpoint it keeps for that.
var providerKinds = []providerKind{
	{id: "openai", name: "OpenAI", baseURL: "https://api.openai.com/v1", keyURL: "https://platform.openai.com/api-keys",
		models: []string{"gpt-5", "gpt-5-mini"}, chat: openAIChat, open: openChatCompletions, titleModel: "gpt-5-mini"},
	{id: "anthropic", name: "Anthropic", baseURL: "https://api.anthropic.com", keyURL: "https://console.anthropic.com/settings/keys",
		models: []string{"claude-opus-5-5", "claude-sonnet-5-5", "claude-haiku-5-5"}, open: openAnthropic, titleModel: "claude-haiku-5-5"},
	{id: "google", name: "Google", baseURL: "https://generativelanguage.googleapis.com/v1beta/openai", keyURL: "https://aistudio.google.com/apikey",
		models: []string{"gemini-4-argon", "gemini-3.8-flash", "gemini-3.5-flash-lite"}, chat: geminiChat, open: openChatCompletions,
		titleModel: "gemini-3.5-flash-lite"},
	{id: "openrouter", name: "OpenRouter", baseURL: "https://openrouter.ai/api/v1", keyURL: "https://openrouter.ai/keys",
		models: []string{"anthropic/claude-sonnet-5.5", "anthropic/claude-opus-5.5", "anthropic/claude-haiku-5.5"}, open: openChatCompletions,
		titleModel: "anthropic/claude-haiku-5.5"},
	{id: "deepseek", name: "DeepSeek", baseURL: "https://api.deepseek.com/v1", keyURL: "https://platform.deepseek.com/api_keys",
		models: []string{"deepseek-chat", "deepseek-reasoner"}, open: openChatCompletions, titleModel: "deepseek-chat"},
	{id: "copilot", name: "GitHub Copilot", copilot: true},
	{id: "ollama", name: "Ollama", baseURL: "http://localhost:11434/v1", keyURL: "https://ollama.com/download",
		chat: localChat, open: openChatCompletions, local: true},
	{id: "lmstudio", name: "LM Studio", baseURL: "http://localhost:1234/v1", keyURL: "https://lmstudio.ai/download",
		chat: localChat, open: openChatCompletions, local: true},
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

// localChat tells the models a server on the user's computer runs that
// chat from those that embed, which its list holds too.
func localChat(model string) bool { return !strings.Contains(model, "embed") }

func openChatCompletions(k *providerKind, baseURL, key string, models []string) provider {
	return &openAI{id: k.id, name: k.name, baseURL: baseURL, apiKey: key, models: models, client: http.DefaultClient}
}

// openAI streams replies through OpenAI's chat completions API, as OpenAI,
// Google, OpenRouter, DeepSeek and the providers the user adds take it.
type openAI struct {
	id, name, baseURL, apiKey string
	models                    []string
	client                    *http.Client
}

func (p *openAI) ID() string       { return p.id }
func (p *openAI) Name() string     { return p.name }
func (p *openAI) Models() []string { return p.models }

// authorize gives req the API key, where there is one: a server on the
// user's computer may take none.
func (p *openAI) authorize(req *http.Request) {
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
}

func (p *openAI) List(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	p.authorize(req)
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

func (p *openAI) Stream(ctx context.Context, model string, turns []turn, delta func(chunk)) error {
	body, err := json.Marshal(map[string]any{"model": model, "messages": openAIMessages(turns), "stream": true})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	p.authorize(req)
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
	// comments (": OPENROUTER PROCESSING") while the model warms up. Its
	// reasoning comes as reasoning_content (DeepSeek, Kimi, GLM, Qwen,
	// vLLM), as reasoning (OpenRouter, Ollama), or in <think> tags opening
	// its text (LM Studio, llama.cpp), which think takes out.
	var think thinkTags
	// end gives what think held back, as the reply ends.
	end := func() {
		if rest := think.flush(); rest != (chunk{}) {
			delta(rest)
		}
	}
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		data, ok := strings.CutPrefix(sc.Text(), "data:")
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)
		if data == "[DONE]" {
			end()
			return nil
		}
		var event struct {
			// Error is set when the request fails after the reply began.
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
			Choices []struct {
				Delta struct {
					Content          string      `json:"content"`
					ReasoningContent looseString `json:"reasoning_content"`
					Reasoning        looseString `json:"reasoning"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return fmt.Errorf("reading the reply: %w", err)
		}
		if event.Error != nil {
			return errors.New(event.Error.Message)
		}
		if len(event.Choices) == 0 {
			continue
		}
		d := event.Choices[0].Delta
		ch := think.feed(d.Content)
		ch.Reasoning = string(d.ReasoningContent) + string(d.Reasoning) + ch.Reasoning
		if ch != (chunk{}) {
			delta(ch)
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	end()
	return nil
}

// openAIMessages returns turns as chat completions takes them: a message's
// text as its content, or, with files, its parts, the files first and the
// text after them, as OpenAI's guide has them. An image goes as a data
// URL, a PDF as a file, as OpenAI and OpenRouter take them, and a text
// file in a part of text, as any takes it.
func openAIMessages(turns []turn) []map[string]any {
	msgs := make([]map[string]any, 0, len(turns))
	for _, t := range turns {
		if len(t.Files) == 0 {
			msgs = append(msgs, map[string]any{"role": t.Role, "content": t.Content})
			continue
		}
		var parts []map[string]any
		for _, f := range t.Files {
			switch f.Type {
			case typeText:
				parts = append(parts, map[string]any{"type": "text", "text": fileText(f)})
			case typePDF:
				parts = append(parts, map[string]any{"type": "file", "file": map[string]any{"filename": f.Name, "file_data": dataURL(f)}})
			default:
				parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]any{"url": dataURL(f)}})
			}
		}
		if t.Content != "" {
			parts = append(parts, map[string]any{"type": "text", "text": t.Content})
		}
		msgs = append(msgs, map[string]any{"role": t.Role, "content": parts})
	}
	return msgs
}

// looseString is a string of a reply's JSON, "" where the field holds
// another type, as one a service names as others do but fills otherwise.
type looseString string

func (s *looseString) UnmarshalJSON(b []byte) error {
	var v string
	if json.Unmarshal(b, &v) == nil {
		*s = looseString(v)
	}
	return nil
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
	// SDK reads of the environment otherwise; with no key, for a service
	// the user added that takes none.
	opts := []option.RequestOption{option.WithoutEnvironmentDefaults(), option.WithBaseURL(baseURL)}
	if key != "" {
		opts = append(opts, option.WithAPIKey(key))
	}
	return &anthropicAPI{id: k.id, name: k.name, client: anthropic.NewClient(opts...), models: models, compatible: k.added()}
}

// anthropicAPI streams replies from Claude, through Anthropic's Messages
// API and its SDK, or from a service the user added that takes it, which
// is compatible.
type anthropicAPI struct {
	id, name   string
	client     anthropic.Client
	models     []string
	compatible bool
}

func (p *anthropicAPI) ID() string       { return p.id }
func (p *anthropicAPI) Name() string     { return p.name }
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
// it to. Claude Haiku has none, nor do compatible services, which may not
// know the beta.
var anthropicFallbacks = []string{"claude-fable-5-1", "claude-opus-5-5", "claude-opus-5", "claude-sonnet-5-5"}

func (p *anthropicAPI) Stream(ctx context.Context, model string, turns []turn, delta func(chunk)) error {
	var msgs []anthropic.BetaMessageParam
	for _, t := range turns {
		if t.Role == "assistant" {
			msgs = append(msgs, anthropic.BetaMessageParam{OfAssistant: &anthropic.BetaAssistantMessageParam{
				Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock(t.Content)}}})
		} else {
			msgs = append(msgs, anthropic.NewBetaUserMessage(anthropicBlocks(t)...))
		}
	}
	// Room for a long reply, which streaming lets take its time.
	params := anthropic.BetaMessageNewParams{Model: model, MaxTokens: 64000, Messages: msgs}
	// The conversation is cached, as far as its last block: the next
	// question reads it back, its files too, which every question sends
	// again, at a tenth of the price or less, rather than all of it anew.
	// For five minutes from each question, as the default, which a reply
	// within them keeps; a compatible service may not take it.
	if !p.compatible {
		params.CacheControl = anthropic.NewBetaCacheControlEphemeralParam()
	}
	if !p.compatible && slices.Contains(anthropicFallbacks, model) {
		params.Betas = []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01}
		params.Fallbacks = anthropic.BetaFallbacksParamUnion{OfDefault: constant.ValueOf[constant.Default]()}
	}
	stream := p.client.Beta.Messages.NewStreaming(ctx, params)
	defer stream.Close()
	var refused *anthropic.BetaRefusalStopDetails
	for stream.Next() {
		switch ev := stream.Current().AsAny().(type) {
		case anthropic.BetaRawContentBlockDeltaEvent:
			switch d := ev.Delta.AsAny().(type) {
			case anthropic.BetaTextDelta:
				if d.Text != "" {
					delta(chunk{Text: d.Text})
				}
			// Of the services that think aloud, as DeepSeek's and Kimi's
			// reasoning models do, unasked.
			case anthropic.BetaThinkingDelta:
				if d.Thinking != "" {
					delta(chunk{Reasoning: d.Thinking})
				}
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

// anthropicBlocks returns the question t as the Messages API takes it: its
// files first, as Anthropic's guide has them, an image as one, a PDF as a
// document titled by its name, and a text file in a block of text; then
// its text, where it has any.
func anthropicBlocks(t turn) []anthropic.BetaContentBlockParamUnion {
	var blocks []anthropic.BetaContentBlockParamUnion
	for _, f := range t.Files {
		switch f.Type {
		case typeText:
			blocks = append(blocks, anthropic.NewBetaTextBlock(fileText(f)))
		case typePDF:
			doc := anthropic.NewBetaDocumentBlock(anthropic.BetaBase64PDFSourceParam{Data: base64.StdEncoding.EncodeToString(f.data)})
			doc.OfDocument.Title = anthropic.String(f.Name)
			blocks = append(blocks, doc)
		default:
			blocks = append(blocks, anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{
				Data: base64.StdEncoding.EncodeToString(f.data), MediaType: anthropic.BetaBase64ImageSourceMediaType(f.Type)}))
		}
	}
	if t.Content != "" || len(blocks) == 0 {
		blocks = append(blocks, anthropic.NewBetaTextBlock(t.Content))
	}
	return blocks
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
