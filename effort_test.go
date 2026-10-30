package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
	"github.com/egoist/mygo/ui"

	"hyperai-desktop/locales"
)

// OpenAI's and Google's models take efforts by their names; the others'
// none, but as their lists say.
func TestEffortRules(t *testing.T) {
	for _, c := range []struct {
		id, model string
		want      []string
	}{
		{"openai", "gpt-5", []string{"minimal", "low", "medium", "high"}},
		{"openai", "gpt-5-mini", []string{"minimal", "low", "medium", "high"}},
		{"openai", "gpt-5-chat-latest", nil},
		{"openai", "o3", []string{"low", "medium", "high"}},
		{"openai", "o4-mini", []string{"low", "medium", "high"}},
		{"openai", "gpt-4o", nil},
		{"google", "gemini-3.8-flash", []string{"low", "medium", "high"}},
		{"google", "gemini-2.5-pro", []string{"low", "medium", "high"}},
		{"google", "gemini-2.0-flash", nil},
		{"deepseek", "deepseek-reasoner", nil},
		{"ollama", "gpt-oss:20b", nil},
	} {
		if got := effortRules(c.id, c.model); !slices.Equal(got, c.want) {
			t.Errorf("effortRules(%q, %q) = %q, want %q", c.id, c.model, got, c.want)
		}
	}
}

// Anthropic's list says each model's efforts, which the providers' list
// keeps, listing once more those kept before; OpenRouter's, where a model
// takes reasoning.
func TestEffortsListed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		caps := func(levels ...string) string {
			on := func(l string) bool { return slices.Contains(levels, l) }
			return fmt.Sprintf(`{"effort":{"supported":%v,"low":{"supported":%v},"medium":{"supported":%v},"high":{"supported":%v},"xhigh":{"supported":%v},"max":{"supported":%v}}}`,
				len(levels) > 0, on("low"), on("medium"), on("high"), on("xhigh"), on("max"))
		}
		fmt.Fprintf(w, `{"data":[{"type":"model","id":"claude-opus-5-5","display_name":"Opus","created_at":"2026-01-01T00:00:00Z","capabilities":%s},`+
			`{"type":"model","id":"claude-haiku-4-5","display_name":"Haiku","created_at":"2026-01-01T00:00:00Z","capabilities":%s}],"has_more":false}`,
			caps("low", "medium", "high", "xhigh", "max"), caps())
	}))
	defer srv.Close()
	a := newApp(nil)
	a.keys = map[string]string{"anthropic": "k"}
	a.conf("anthropic").BaseURL = srv.URL
	a.conf("anthropic").Listed = []string{"claude-opus-5-5"} // kept before efforts were
	a.listUnlisted()
	c := a.confs["anthropic"]
	if !c.EffortsListed || !slices.Equal(c.Efforts["claude-opus-5-5"], []string{"low", "medium", "high", "xhigh", "max"}) ||
		c.Efforts["claude-haiku-4-5"] != nil {
		t.Errorf("efforts %v, listed %v", c.Efforts, c.EffortsListed)
	}
	if got := a.effortsFor(&anthropicAPI{id: "anthropic"}, "claude-opus-5-5"); len(got) != 5 {
		t.Errorf("effortsFor %q", got)
	}

	or := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[{"id":"anthropic/claude-opus-5.5","supported_parameters":["tools","reasoning"]},{"id":"meta/llama","supported_parameters":["tools"]}]}`)
	}))
	defer or.Close()
	p := &openAI{id: "openrouter", baseURL: or.URL, client: or.Client()}
	if _, err := p.List(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := p.ListedEfforts(); len(got) != 1 || !slices.Equal(got["anthropic/claude-opus-5.5"], []string{"low", "medium", "high"}) {
		t.Errorf("OpenRouter's %v", got)
	}
}

// The effort goes as each API takes it, and none where it is the model's
// own.
func TestEffortAsked(t *testing.T) {
	var body map[string]any
	ant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body = nil
		json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer ant.Close()
	ap := &anthropicAPI{client: anthropic.NewClient(option.WithAPIKey("k"), option.WithBaseURL(ant.URL), option.WithMaxRetries(0))}
	turns := []turn{{Role: "user", Content: "Hi"}}
	ap.Stream(withEffort(context.Background(), "xhigh"), "claude-opus-5-5", turns, func(chunk) {})
	if cfg, _ := body["output_config"].(map[string]any); cfg["effort"] != "xhigh" {
		t.Errorf("Anthropic's: %v", body["output_config"])
	}
	ap.Stream(context.Background(), "claude-opus-5-5", turns, func(chunk) {})
	if body["output_config"] != nil {
		t.Errorf("Anthropic's, of its own: %v", body["output_config"])
	}

	oa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body = nil
		json.NewDecoder(r.Body).Decode(&body)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer oa.Close()
	openai := &openAI{id: "openai", baseURL: oa.URL, client: oa.Client()}
	openai.Stream(withEffort(context.Background(), "minimal"), "gpt-5", turns, func(chunk) {})
	if body["reasoning_effort"] != "minimal" || body["reasoning"] != nil {
		t.Errorf("OpenAI's: %v", body)
	}
	openai.Stream(context.Background(), "gpt-5", turns, func(chunk) {})
	if _, ok := body["reasoning_effort"]; ok {
		t.Errorf("OpenAI's, of its own: %v", body)
	}
	router := &openAI{id: "openrouter", baseURL: oa.URL, client: oa.Client()}
	router.Stream(withEffort(context.Background(), "high"), "anthropic/claude-opus-5.5", turns, func(chunk) {})
	if r, _ := body["reasoning"].(map[string]any); r["effort"] != "high" || body["reasoning_effort"] != nil {
		t.Errorf("OpenRouter's: %v", body)
	}

	if s := copilotSession("gpt-5", "Be brief.", "high"); s.ReasoningEffort != "high" {
		t.Errorf("Copilot's: %q", s.ReasoningEffort)
	}
}

// reasoner is OpenAI, as far as efforts go: GPT-5 takes them, GPT-4o
// none; it keeps the effort each reply was asked with.
type reasoner struct {
	counter
	efforts []string
}

func (p *reasoner) ID() string       { return "openai" }
func (p *reasoner) Models() []string { return []string{"gpt-5", "gpt-4o"} }

func (p *reasoner) Stream(ctx context.Context, model string, turns []turn, delta func(chunk)) error {
	p.efforts = append(p.efforts, effortOf(ctx))
	return p.counter.Stream(ctx, model, turns, delta)
}

// The composer offers a model's efforts, by its own, where it takes any;
// the one chosen goes with what it sends, kept with the chat, and is the
// next new chat's; a model that takes none is asked with none.
func TestEffortPicker(t *testing.T) {
	p := &reasoner{}
	a := newApp(p)
	tt := ui.NewTester(a.view, 1120, 760)
	tt.Frame()
	if a.model != "gpt-5" || !tt.HasText(locales.EffortDefault()) {
		t.Fatalf("model %q; no effort in %q", a.model, tt.Texts())
	}
	if err := tt.Click(locales.EffortLabel()); err != nil {
		t.Fatal(err)
	}
	want := []string{locales.EffortDefault(), "-", locales.EffortMinimal(), locales.EffortLow(), locales.EffortMedium(), locales.EffortHigh()}
	if got := tt.Menu(); !slices.Equal(got, want) {
		t.Fatalf("menu %q, want %q", got, want)
	}
	if err := tt.ChooseMenuItem(locales.EffortHigh()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText(locales.EffortHigh()) {
		t.Errorf("the button shows %q", tt.Texts())
	}
	a.send("Think hard")
	first := a.current()
	if p.efforts[0] != "high" || first.Effort != "high" {
		t.Fatalf("asked %q, kept %q", p.efforts, first.Effort)
	}

	// A model of none: no button, asked with none, though high stays chosen.
	a.pick(p, "gpt-4o")
	tt.Frame()
	if tt.HasText(locales.EffortHigh()) || tt.HasText(locales.EffortDefault()) {
		t.Errorf("an effort for GPT-4o: %q", tt.Texts())
	}
	a.send("Again")
	if got := p.efforts[len(p.efforts)-1]; got != "" || first.Effort != "" {
		t.Errorf("GPT-4o asked with %q, kept %q", got, first.Effort)
	}

	// A new chat asks the one chosen last, on a model that takes it.
	a.newChat()
	a.pick(p, "gpt-5")
	tt.Advance(time.Second)
	if a.askedEffort() != "high" {
		t.Errorf("a new chat asks %q", a.askedEffort())
	}
	a.send("New")
	second := a.current()
	// Each chat its own, as it is shown again.
	a.effort = "low"
	a.send("Lower")
	a.open(fmt.Sprint(first.ID))
	tt.Frame()
	a.open(fmt.Sprint(second.ID))
	tt.Frame()
	if a.effort != "low" || second.Effort != "low" {
		t.Errorf("shown again, effort %q, kept %q", a.effort, second.Effort)
	}
}
