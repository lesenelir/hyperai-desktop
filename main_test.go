package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// The view runs without a window in tests.
func TestView(t *testing.T) {
	tt := ui.NewTester(view, 480, 360)
	if !tt.HasText("Hello, world!") {
		t.Errorf("texts %q", tt.Texts())
	}
}
