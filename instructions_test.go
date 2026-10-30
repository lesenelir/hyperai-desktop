package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
	"github.com/egoist/mygo/ui"

	"hyperai-desktop/locales"
)

// The user's instructions go first in what a reply is asked of, every
// reply's, in the chats there were too, but in no chat kept; a title is
// asked for without them.
func TestInstructionsAsked(t *testing.T) {
	p := &namer{title: "Caching"}
	a := newApp(p)
	a.nameChats = true
	var asked [][]turn
	rec := &recorder{provider: p, asked: &asked}
	a.prov = rec
	a.send("How do I cache?")
	if len(asked) != 1 || asked[0][0].Role != "user" {
		t.Fatalf("without instructions: asked %+v", asked)
	}

	a.instructions = "  Reply in Chinese.  "
	th := a.current()
	a.send("And in Go?")
	if got := asked[len(asked)-1]; got[0].Role != "system" || got[0].Content != "Reply in Chinese." || got[1].Content != "How do I cache?" {
		t.Errorf("with instructions: asked %+v", got)
	}
	a.regenerate(th, 3)
	if got := asked[len(asked)-1]; got[0].Role != "system" {
		t.Errorf("regenerated without them: %+v", got)
	}
	for _, m := range th.Messages {
		if m.Role == "system" {
			t.Error("the instructions were kept in the chat")
		}
	}
	// A chat started with them is named as without.
	a.newChat()
	a.send("What is a goroutine?")
	if a.current().Title != "Caching" {
		t.Errorf("titled %q", a.current().Title)
	}
}

// recorder asks provider, keeping what the replies were asked of; the
// titles go to it unkept.
type recorder struct {
	provider
	asked *[][]turn
}

func (r *recorder) Stream(ctx context.Context, model string, turns []turn, delta func(chunk)) error {
	if !strings.HasPrefix(turns[0].Content, titlePrompt) {
		*r.asked = append(*r.asked, turns)
	}
	return r.provider.Stream(ctx, model, turns, delta)
}

// Each API is told the instructions as it takes them: Anthropic's as its
// system prompt, OpenAI's as a system message first, Copilot after its own.
func TestInstructionsAsProvidersTakeThem(t *testing.T) {
	turns := []turn{{Role: "system", Content: "Be brief."}, {Role: "user", Content: "Hi"}}

	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "text/event-stream")
		event := func(name, data string) { fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data) }
		event("message_start", `{"type":"message_start","message":{"id":"m","type":"message","role":"assistant","model":"m","content":[],"stop_reason":null,"usage":{"input_tokens":1,"output_tokens":0}}}`)
		event("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`)
		event("message_stop", `{"type":"message_stop"}`)
	}))
	defer srv.Close()
	ap := &anthropicAPI{client: anthropic.NewClient(option.WithAPIKey("k"), option.WithBaseURL(srv.URL), option.WithMaxRetries(0))}
	if err := ap.Stream(context.Background(), "claude-haiku-5-5", turns, func(chunk) {}); err != nil {
		t.Fatal(err)
	}
	system, _ := json.Marshal(body["system"])
	msgs, _ := json.Marshal(body["messages"])
	if string(system) != `[{"text":"Be brief.","type":"text"}]` || string(msgs) != `[{"content":[{"text":"Hi","type":"text"}],"role":"user"}]` {
		t.Errorf("Anthropic's: system %s, messages %s", system, msgs)
	}

	b, _ := json.Marshal(openAIMessages(turns))
	if string(b) != `[{"content":"Be brief.","role":"system"},{"content":"Hi","role":"user"}]` {
		t.Errorf("OpenAI's: %s", b)
	}

	sys, prompt, _ := copilotPrompt(turns)
	if sys != copilotSystem+"\n\nThe user's instructions:\n\nBe brief." || prompt != "Hi" {
		t.Errorf("Copilot's: system %q, prompt %q", sys, prompt)
	}
}

// The Instructions page's field sets them, kept at once, for the next
// start.
func TestInstructionsPage(t *testing.T) {
	dir := t.TempDir()
	a := newApp(&counter{})
	a.loadFrom(dir)
	t.Cleanup(func() { a.layoutSaver.wait() })
	tt := ui.NewTester(a.view, 1120, 760)
	a.openSettings()
	a.settingsPage = "instructions"
	tt.Frame()
	for _, s := range []string{locales.PageInstructions(), locales.InstructionsCard(), locales.InstructionsPlaceholder(), locales.InstructionsAbout()} {
		if !tt.HasText(s) {
			t.Fatalf("no %q in %q", s, tt.Texts())
		}
	}
	if err := tt.Click(locales.InstructionsPlaceholder()); err != nil {
		t.Fatal(err)
	}
	tt.Type("Reply in Chinese.")
	tt.Advance(time.Second)
	if a.instructions != "Reply in Chinese." || tt.HasText(locales.InstructionsPlaceholder()) {
		t.Fatalf("instructions %q", a.instructions)
	}
	a.layoutSaver.wait()
	again := newApp(&counter{})
	again.loadFrom(dir)
	if again.instructions != "Reply in Chinese." {
		t.Errorf("kept %q", again.instructions)
	}
}
