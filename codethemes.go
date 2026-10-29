package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// codeColors is a code theme in one appearance, as its authors publish
// it: its text, and the colors of the kinds of tokens, which show on the
// app's own surface, not the theme's background, so that a block sits
// with the conversation around it. They are hex, as the themes are, the
// one place in the app that is: they are other people's palettes, not
// tokens of this one (docs/design.md).
type codeColors struct {
	text, comment                  string
	keyword, str, number, function string
	typ, property, tag, operator   string
	deleted, inserted              string
}

// codeTheme is a theme the settings offer for code: its light side and
// its dark one, for the app's appearances. A theme with no light side, as
// Nord and Dracula, has no place here: its colors, made for its dark
// background, do not read on the light app's.
type codeTheme struct {
	key, title  string
	light, dark *codeColors
}

// codeThemes are the code themes, in their menu's order: Default, MyGo's
// GitHub colors, then the others.
var codeThemes = []codeTheme{
	{key: "", title: "Default"},
	{"one", "One", &codeColors{
		text: "#383a42", comment: "#a0a1a7",
		keyword: "#a626a4", str: "#50a14f", number: "#986801", function: "#4078f2",
		typ: "#c18401", property: "#e45649", tag: "#e45649", operator: "#0184bc",
		deleted: "#e45649", inserted: "#50a14f",
	}, &codeColors{
		text: "#abb2bf", comment: "#7f848e",
		keyword: "#c678dd", str: "#98c379", number: "#d19a66", function: "#61afef",
		typ: "#e5c07b", property: "#e06c75", tag: "#e06c75", operator: "#56b6c2",
		deleted: "#e06c75", inserted: "#98c379",
	}},
	{"solarized", "Solarized", &codeColors{
		text: "#657b83", comment: "#93a1a1",
		keyword: "#859900", str: "#2aa198", number: "#d33682", function: "#268bd2",
		typ: "#b58900", property: "#268bd2", tag: "#268bd2", operator: "#859900",
		deleted: "#dc322f", inserted: "#859900",
	}, &codeColors{
		text: "#839496", comment: "#586e75",
		keyword: "#859900", str: "#2aa198", number: "#d33682", function: "#268bd2",
		typ: "#b58900", property: "#268bd2", tag: "#268bd2", operator: "#859900",
		deleted: "#dc322f", inserted: "#859900",
	}},
	{"catppuccin", "Catppuccin", &codeColors{ // Latte
		text: "#4c4f69", comment: "#7c7f93",
		keyword: "#8839ef", str: "#40a02b", number: "#fe640b", function: "#1e66f5",
		typ: "#df8e1d", property: "#7287fd", tag: "#1e66f5", operator: "#04a5e5",
		deleted: "#d20f39", inserted: "#40a02b",
	}, &codeColors{ // Mocha
		text: "#cdd6f4", comment: "#9399b2",
		keyword: "#cba6f7", str: "#a6e3a1", number: "#fab387", function: "#89b4fa",
		typ: "#f9e2af", property: "#b4befe", tag: "#89b4fa", operator: "#89dceb",
		deleted: "#f38ba8", inserted: "#a6e3a1",
	}},
	{"gruvbox", "Gruvbox", &codeColors{
		text: "#3c3836", comment: "#928374",
		keyword: "#9d0006", str: "#79740e", number: "#8f3f71", function: "#427b58",
		typ: "#b57614", property: "#076678", tag: "#9d0006", operator: "#af3a03",
		deleted: "#9d0006", inserted: "#79740e",
	}, &codeColors{
		text: "#ebdbb2", comment: "#928374",
		keyword: "#fb4934", str: "#b8bb26", number: "#d3869b", function: "#8ec07c",
		typ: "#fabd2f", property: "#83a598", tag: "#fb4934", operator: "#fe8019",
		deleted: "#fb4934", inserted: "#b8bb26",
	}},
	{"tokyo-night", "Tokyo Night", &codeColors{ // Day
		text: "#3760bf", comment: "#848cb5",
		keyword: "#9854f1", str: "#587539", number: "#b15c00", function: "#2e7de9",
		typ: "#007197", property: "#387068", tag: "#f52a65", operator: "#006a83",
		deleted: "#f52a65", inserted: "#587539",
	}, &codeColors{
		text: "#c0caf5", comment: "#565f89",
		keyword: "#bb9af7", str: "#9ece6a", number: "#ff9e64", function: "#7aa2f7",
		typ: "#2ac3de", property: "#73daca", tag: "#f7768e", operator: "#89ddff",
		deleted: "#f7768e", inserted: "#9ece6a",
	}},
}

// codeThemeNamed returns the code theme of key, nil for none.
func codeThemeNamed(key string) *codeTheme {
	for i := range codeThemes {
		if codeThemes[i].key == key {
			return &codeThemes[i]
		}
	}
	return nil
}

// colors returns the theme's colors in the appearance dark, nil for
// Default, which is MyGo's.
func (ct *codeTheme) colors(dark bool) *codeColors {
	if ct == nil {
		return nil
	}
	if dark {
		return ct.dark
	}
	return ct.light
}

// editorTheme returns the colors as the Markdown's code takes them: over
// MyGo's for t, which give what a code block shows beyond its tokens, its
// background the app's.
func (cc *codeColors) editorTheme(t *ui.Theme) *editor.Theme {
	et := editor.DefaultTheme(t)
	et.Text = ui.Hex(cc.text)
	set := func(hex string, kinds ...editor.TokenKind) {
		for _, k := range kinds {
			et.Tokens[k].Color = ui.Hex(hex)
		}
	}
	set(cc.text, editor.TokenVariable, editor.TokenPunctuation)
	set(cc.comment, editor.TokenComment)
	set(cc.keyword, editor.TokenKeyword, editor.TokenMeta)
	set(cc.str, editor.TokenString, editor.TokenRegexp, editor.TokenCode, editor.TokenLink)
	set(cc.number, editor.TokenNumber, editor.TokenConstant)
	set(cc.function, editor.TokenFunction)
	set(cc.typ, editor.TokenType)
	set(cc.property, editor.TokenProperty, editor.TokenAttribute, editor.TokenHeading)
	set(cc.tag, editor.TokenTag)
	set(cc.operator, editor.TokenOperator, editor.TokenEscape)
	set(cc.deleted, editor.TokenDeleted, editor.TokenInvalid)
	set(cc.inserted, editor.TokenInserted)
	et.Added, et.Deleted = ui.Hex(cc.inserted), ui.Hex(cc.deleted)
	return et
}
