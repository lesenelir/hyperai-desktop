package main

import (
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
