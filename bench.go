package main

import (
	"context"
	"math/rand/v2"
	"os"
	"strconv"
	"time"

	"github.com/egoist/mygo"
)

// benchReplies is how many replies HYPERAI_BENCH asks to measure, 0 when
// it is unset.
func benchReplies() int {
	n, _ := strconv.Atoi(os.Getenv("HYPERAI_BENCH"))
	return max(n, 0)
}

// bench sends n of the suggestions to the demo one after another, as the
// user would, then quits: a stream to measure the same way every time
// (docs/performance.md). It runs on a goroutine of its own, and touches
// the app only through Window.Update.
func (a *app) bench(n int) {
	time.Sleep(time.Second) // the window opens and settles
	for i := range n {
		done := make(chan struct{})
		a.win.Update(func() {
			a.onFinish = func() { close(done) }
			a.send(suggestions[i%len(suggestions)].prompt())
		})
		<-done
		time.Sleep(300 * time.Millisecond)
	}
	time.Sleep(time.Second) // the last reveal settles
	a.win.Update(mygo.App.Quit)
}

// demo is a pretend model, which HYPERAI_BENCH measures and the tests ask
// in place of a provider: its canned Markdown replies stream in at the
// uneven pace of a model when paced, at once otherwise (in tests). The
// app never offers it.
type demo struct {
	paced   bool
	replies int
	// rng paces the tokens; the global source when nil. A seeded one
	// streams the same every time (HYPERAI_BENCH).
	rng *rand.Rand
}

func (p *demo) intN(n int) int {
	if p.rng != nil {
		return p.rng.IntN(n)
	}
	return rand.IntN(n)
}

func (p *demo) ID() string                                 { return "demo" }
func (p *demo) Name() string                               { return "Demo" }
func (p *demo) Models() []string                           { return []string{"hyperai-demo"} }
func (p *demo) List(ctx context.Context) ([]string, error) { return p.Models(), nil }

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
		n := min(len(src)-i, 2+p.intN(7))
		for n < len(src)-i && src[i+n]&0xC0 == 0x80 {
			n++
		}
		if err := sleep(ctx, time.Duration(8+p.intN(30))*time.Millisecond); err != nil {
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
		"To talk to a real model, open **Settings → Providers** and add an API key of OpenAI, Anthropic, Google, OpenRouter or DeepSeek. " +
		"The key stays in your system's keychain, and the composer offers that provider's models from then on.\n",
}
