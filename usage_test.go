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

// Anthropic's stream counts the input apart, as the cache gave it, took it
// and neither: all of it is the input, and what it gave the cached.
func TestAnthropicUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		event := func(name, data string) { fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data) }
		event("message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"m","content":[],"stop_reason":null,`+
			`"usage":{"input_tokens":10,"cache_read_input_tokens":900,"cache_creation_input_tokens":90,"output_tokens":1}}}`)
		event("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		event("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}`)
		event("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":42}}`)
		event("message_stop", `{"type":"message_stop"}`)
	}))
	defer srv.Close()
	p := &anthropicAPI{client: anthropic.NewClient(option.WithAPIKey("key"), option.WithBaseURL(srv.URL), option.WithMaxRetries(0))}
	var got *usage
	err := p.Stream(context.Background(), "claude-haiku-5-5", []turn{{Role: "user", Content: "Hi"}}, func(ch chunk) {
		if ch.Usage != nil {
			got = ch.Usage
		}
	})
	if err != nil || got == nil || *got != (usage{Input: 1000, Cached: 900, Output: 42}) {
		t.Errorf("usage %+v, %v", got, err)
	}
}

// Chat completions are asked for the usage, which comes in a last chunk of
// no choices, its cached input as OpenAI counts it or DeepSeek; a server
// that will not take the asking is asked again without.
func TestOpenAIUsage(t *testing.T) {
	var bodies []map[string]any
	last := ""
	refuse := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		if refuse && body["stream_options"] != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":{"message":"Unrecognized request argument supplied: stream_options"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"Hi"}}]}`+"\n\n")
		if last != "" {
			fmt.Fprint(w, "data: "+last+"\n\n")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	p := &openAI{baseURL: srv.URL, client: srv.Client()}
	ask := func() (*usage, error) {
		var got *usage
		err := p.Stream(context.Background(), "m", []turn{{Role: "user", Content: "Hi"}}, func(ch chunk) {
			if ch.Usage != nil {
				got = ch.Usage
			}
		})
		return got, err
	}

	last = `{"choices":[],"usage":{"prompt_tokens":2006,"completion_tokens":300,"prompt_tokens_details":{"cached_tokens":1920}}}`
	if got, err := ask(); err != nil || got == nil || *got != (usage{Input: 2006, Cached: 1920, Output: 300}) {
		t.Errorf("OpenAI's: %+v, %v", got, err)
	}
	if opts, _ := bodies[0]["stream_options"].(map[string]any); opts["include_usage"] != true {
		t.Errorf("asked %v", bodies[0])
	}
	last = `{"choices":[],"usage":{"prompt_tokens":500,"completion_tokens":20,"prompt_cache_hit_tokens":448,"prompt_cache_miss_tokens":52}}`
	if got, err := ask(); err != nil || got == nil || *got != (usage{Input: 500, Cached: 448, Output: 20}) {
		t.Errorf("DeepSeek's: %+v, %v", got, err)
	}
	// None counted: none given.
	last = ""
	if got, err := ask(); err != nil || got != nil {
		t.Errorf("of none: %+v, %v", got, err)
	}
	// Refused, then asked without.
	refuse, bodies = true, nil
	if got, err := ask(); err != nil || got != nil || len(bodies) != 2 || bodies[1]["stream_options"] != nil {
		t.Errorf("refused: %+v, %v; asked %d times", got, err, len(bodies))
	}
}

// The usage goes through the batcher, after the text, as the stream ends.
func TestBatcherUsage(t *testing.T) {
	var got []chunk
	b := newBatcher(func(ch chunk) { got = append(got, ch) })
	b.add(chunk{Text: "Hi"})
	b.add(chunk{Usage: &usage{Input: 3, Output: 1}})
	b.close()
	var text string
	var used *usage
	for _, ch := range got {
		text += ch.Text
		if ch.Usage != nil {
			used = ch.Usage
		}
	}
	if text != "Hi" || used == nil || used.Output != 1 {
		t.Errorf("committed %+v", got)
	}
}

// A reply's usage shows in a card from the button under it: its tokens,
// how long its first one took and the whole of it, and how fast it came.
func TestUsageCard(t *testing.T) {
	m := &message{Role: "assistant", Text: "Hi", Elapsed: 11.1, FirstToken: 3.2,
		Usage: &usage{Input: 34037, Cached: 33024, Output: 1638}}
	var got []string
	for _, r := range usageRows(m) {
		got = append(got, r.label+"="+r.value)
	}
	want := "Input tokens=34,037 Cached input tokens=33,024 Output tokens=1,638 Time to first token=3.2s Duration=11.1s Output speed=207.3 tokens/s"
	if strings.Join(got, " ") != want {
		t.Errorf("rows\n%q\nwant\n%q", strings.Join(got, " "), want)
	}
	// A reply its provider did not count: its times alone.
	if rows := usageRows(&message{Elapsed: 2}); len(rows) != 1 || rows[0].label != locales.UsageDuration() {
		t.Errorf("uncounted: %v", rows)
	}

	a := newApp(&counter{})
	tt := ui.NewTester(a.view, 1120, 760)
	a.send("Hello")
	reply := a.current().Messages[1]
	// Timed as its first text came.
	if reply.FirstToken <= 0 || reply.FirstToken > reply.Elapsed {
		t.Errorf("first token after %vs of %vs", reply.FirstToken, reply.Elapsed)
	}
	reply.Usage = &usage{Input: 12, Output: 3}
	tt.Frame()
	if tt.HasText(locales.UsageInput()) {
		t.Fatal("the card shows before it is asked for")
	}
	if err := tt.Click(locales.UsageButton()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !tt.HasText(locales.UsageInput()) || !tt.HasText(locales.UsageDuration()) {
		t.Fatalf("no card in %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	tt.Advance(time.Second)
	if tt.HasText(locales.UsageInput()) {
		t.Error("the card stays after Escape")
	}
}
