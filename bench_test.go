package main

import (
	"strconv"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// BenchmarkFrame measures a frame of a window with few and many chats in
// the sidebar, as every frame of a streaming reply builds it.
func BenchmarkFrame(b *testing.B) {
	for _, n := range []int{5, 500} {
		b.Run(strconv.Itoa(n)+"chats", func(b *testing.B) { benchFrame(b, n) })
	}
}

func benchFrame(b *testing.B, chats int) {
	a := newApp(&demo{})
	now := time.Now()
	for i := range chats {
		th := &thread{ID: newID(), Title: "Chat number " + strconv.Itoa(i), Model: "hyperai-demo", Updated: now.Add(-time.Duration(i) * 3 * time.Hour)}
		a.threads = append(a.threads, th)
	}
	a.selected = string(a.threads[0].ID)
	tt := ui.NewTester(a.view, 1120, 760)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		tt.Frame()
	}
}
