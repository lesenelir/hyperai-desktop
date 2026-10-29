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
// background, do not read on the light app's; nor one whose light side
// is too faint to read, as Ayu's, most of whose tokens are under 2.5:1.
type codeTheme struct {
	key, title  string
	group       int
	light, dark *codeColors
}

// The groups of codeThemes, which their menu draws a line between: MyGo's
// Default; the editors' own themes; and the themes people made for many
// editors.
const (
	defaultTheme = iota
	editorThemes
	communityThemes
)

// codeThemes are the code themes, in their menu's order, by group:
// Default, MyGo's GitHub colors; VS Code's, Xcode's and Atom's One; then
// the others, by name.
var codeThemes = []codeTheme{
	{key: "", title: "Default"},
	{"vscode", "VS Code", editorThemes, &codeColors{ // Light+
		text: "#000000", comment: "#008000",
		keyword: "#0000ff", str: "#a31515", number: "#098658", function: "#795e26",
		typ: "#267f99", property: "#001080", tag: "#800000", operator: "#000000",
		deleted: "#a31515", inserted: "#098658",
	}, &codeColors{ // Dark+
		text: "#d4d4d4", comment: "#6a9955",
		keyword: "#569cd6", str: "#ce9178", number: "#b5cea8", function: "#dcdcaa",
		typ: "#4ec9b0", property: "#9cdcfe", tag: "#569cd6", operator: "#d4d4d4",
		deleted: "#f44747", inserted: "#b5cea8",
	}},
	{"xcode", "Xcode", editorThemes, &codeColors{ // Default (Light)
		text: "#262626", comment: "#5d6c79",
		keyword: "#9b2393", str: "#c41a16", number: "#1c00cf", function: "#326d74",
		typ: "#0b4f79", property: "#326d74", tag: "#9b2393", operator: "#262626",
		deleted: "#c41a16", inserted: "#326d74",
	}, &codeColors{ // Default (Dark)
		text: "#dfdfe0", comment: "#6c7986",
		keyword: "#fc5fa3", str: "#fc6a5d", number: "#d0bf69", function: "#67b7a4",
		typ: "#5dd8ff", property: "#67b7a4", tag: "#fc5fa3", operator: "#dfdfe0",
		deleted: "#fc6a5d", inserted: "#67b7a4",
	}},
	{"one", "One", editorThemes, &codeColors{
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
	{"catppuccin", "Catppuccin", communityThemes, &codeColors{ // Latte
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
	{"everforest", "Everforest", communityThemes, &codeColors{ // Light
		text: "#5c6a72", comment: "#939f91",
		keyword: "#f85552", str: "#35a77c", number: "#df69ba", function: "#8da101",
		typ: "#dfa000", property: "#3a94c5", tag: "#f57d26", operator: "#f57d26",
		deleted: "#f85552", inserted: "#8da101",
	}, &codeColors{ // Dark
		text: "#d3c6aa", comment: "#859289",
		keyword: "#e67e80", str: "#83c092", number: "#d699b6", function: "#a7c080",
		typ: "#dbbc7f", property: "#7fbbb3", tag: "#e69875", operator: "#e69875",
		deleted: "#e67e80", inserted: "#a7c080",
	}},
	{"gruvbox", "Gruvbox", communityThemes, &codeColors{
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
	{"rose-pine", "Rosé Pine", communityThemes, &codeColors{ // Dawn
		text: "#575279", comment: "#9893a5",
		keyword: "#286983", str: "#ea9d34", number: "#907aa9", function: "#d7827e",
		typ: "#56949f", property: "#56949f", tag: "#56949f", operator: "#797593",
		deleted: "#b4637a", inserted: "#286983",
	}, &codeColors{ // Main
		text: "#e0def4", comment: "#6e6a86",
		keyword: "#31748f", str: "#f6c177", number: "#c4a7e7", function: "#ebbcba",
		typ: "#9ccfd8", property: "#9ccfd8", tag: "#9ccfd8", operator: "#908caa",
		deleted: "#eb6f92", inserted: "#31748f",
	}},
	{"solarized", "Solarized", communityThemes, &codeColors{
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
	{"tokyo-night", "Tokyo Night", communityThemes, &codeColors{ // Day
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
