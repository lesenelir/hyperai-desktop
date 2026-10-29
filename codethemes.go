package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// codeColors is a code theme in one appearance, as its authors publish
// it: its text, and the colors of the kinds of tokens it colors, each
// "#rrggbb" and " italic" or " bold" where it styles them. The others
// show in its text. They show on the app's own surface, not the theme's
// background, so that a block sits with the conversation around it. They
// are hex, as the themes are, the one place in the app that is: they are
// other people's palettes, not tokens of this one (docs/design.md).
//
// They are what each theme's own file gives the TextMate scopes that
// stand for the kinds, as docs/code-themes.md has it, with how they were
// chosen.
type codeColors struct {
	text   string
	tokens map[editor.TokenKind]string
}

// codeTheme is a theme the settings offer for code: its light side and
// its dark one, for the app's appearances.
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
// the others, by name. Each reads on the app's surfaces, in both
// appearances, as docs/code-themes.md measures it.
var codeThemes = []codeTheme{
	{key: "", title: "Default"},
	// VS Code's Light+ and Dark+ (microsoft/vscode, MIT).
	{"vscode", "VS Code", editorThemes, &codeColors{text: "#000000", tokens: map[editor.TokenKind]string{
		editor.TokenKeyword:   "#0000ff",
		editor.TokenOperator:  "#000000",
		editor.TokenString:    "#a31515",
		editor.TokenNumber:    "#098658",
		editor.TokenConstant:  "#0000ff",
		editor.TokenFunction:  "#795e26",
		editor.TokenType:      "#267f99",
		editor.TokenVariable:  "#001080",
		editor.TokenProperty:  "#001080",
		editor.TokenTag:       "#800000",
		editor.TokenAttribute: "#e50000",
		editor.TokenComment:   "#008000",
		editor.TokenRegexp:    "#811f3f",
		editor.TokenEscape:    "#ee0000",
		editor.TokenMeta:      "#0000ff",
		editor.TokenHeading:   "#800000 bold",
		editor.TokenCode:      "#800000",
		editor.TokenInserted:  "#098658",
		editor.TokenDeleted:   "#a31515",
		editor.TokenInvalid:   "#cd3131",
	}}, &codeColors{text: "#d4d4d4", tokens: map[editor.TokenKind]string{
		editor.TokenKeyword:   "#569cd6",
		editor.TokenOperator:  "#d4d4d4",
		editor.TokenString:    "#ce9178",
		editor.TokenNumber:    "#b5cea8",
		editor.TokenConstant:  "#569cd6",
		editor.TokenFunction:  "#dcdcaa",
		editor.TokenType:      "#4ec9b0",
		editor.TokenVariable:  "#9cdcfe",
		editor.TokenProperty:  "#9cdcfe",
		editor.TokenTag:       "#569cd6",
		editor.TokenAttribute: "#9cdcfe",
		editor.TokenComment:   "#6a9955",
		editor.TokenRegexp:    "#d16969",
		editor.TokenEscape:    "#d7ba7d",
		editor.TokenMeta:      "#569cd6",
		editor.TokenHeading:   "#569cd6 bold",
		editor.TokenCode:      "#ce9178",
		editor.TokenInserted:  "#b5cea8",
		editor.TokenDeleted:   "#ce9178",
		editor.TokenInvalid:   "#f44747",
	}}},
	// Xcode's Default (Light) and (Dark), from its bundle.
	{"xcode", "Xcode", editorThemes, &codeColors{text: "#262626", tokens: map[editor.TokenKind]string{
		editor.TokenKeyword:   "#9b2393",
		editor.TokenOperator:  "#9b2393",
		editor.TokenString:    "#c41a16",
		editor.TokenNumber:    "#1c00cf",
		editor.TokenConstant:  "#9b2393",
		editor.TokenFunction:  "#326d74",
		editor.TokenType:      "#3900a0",
		editor.TokenProperty:  "#326d74",
		editor.TokenTag:       "#9b2393",
		editor.TokenAttribute: "#815f03",
		editor.TokenComment:   "#5d6c79",
		editor.TokenRegexp:    "#c41a16",
		editor.TokenEscape:    "#1c00cf",
		editor.TokenMeta:      "#643820",
		editor.TokenLink:      "#0e0eff",
	}}, &codeColors{text: "#deddde", tokens: map[editor.TokenKind]string{
		editor.TokenKeyword:   "#fc5fa3",
		editor.TokenOperator:  "#fc5fa3",
		editor.TokenString:    "#fc6a5d",
		editor.TokenNumber:    "#d0bf69",
		editor.TokenConstant:  "#fc5fa3",
		editor.TokenFunction:  "#67b7a4",
		editor.TokenType:      "#d0a8ff",
		editor.TokenProperty:  "#67b7a4",
		editor.TokenTag:       "#fc5fa3",
		editor.TokenAttribute: "#bf8555",
		editor.TokenComment:   "#6c7986",
		editor.TokenRegexp:    "#fc6a5d",
		editor.TokenEscape:    "#d0bf69",
		editor.TokenMeta:      "#fd8f3f",
		editor.TokenLink:      "#5482ff",
	}}},
	// Atom's One, as One Light (akamud/vscode-theme-onelight) and One Dark Pro (Binaryify/OneDark-Pro), MIT.
	{"one", "One", editorThemes, &codeColors{text: "#383a42", tokens: map[editor.TokenKind]string{
		editor.TokenKeyword:   "#a626a4",
		editor.TokenOperator:  "#383a42",
		editor.TokenString:    "#50a14f",
		editor.TokenNumber:    "#986801",
		editor.TokenConstant:  "#986801",
		editor.TokenFunction:  "#4078f2",
		editor.TokenType:      "#c18401",
		editor.TokenVariable:  "#e45649",
		editor.TokenProperty:  "#e45649",
		editor.TokenTag:       "#e45649",
		editor.TokenAttribute: "#986801",
		editor.TokenComment:   "#a0a1a7 italic",
		editor.TokenRegexp:    "#0184bc",
		editor.TokenEscape:    "#0184bc",
		editor.TokenMeta:      "#a626a4",
		editor.TokenHeading:   "#e45649",
		editor.TokenCode:      "#50a14f",
		editor.TokenInserted:  "#50a14f",
		editor.TokenDeleted:   "#e45649",
	}}, &codeColors{text: "#abb2bf", tokens: map[editor.TokenKind]string{
		editor.TokenKeyword:   "#c678dd",
		editor.TokenOperator:  "#abb2bf",
		editor.TokenString:    "#98c379",
		editor.TokenNumber:    "#d19a66",
		editor.TokenConstant:  "#d19a66",
		editor.TokenFunction:  "#61afef",
		editor.TokenType:      "#e5c07b",
		editor.TokenVariable:  "#e06c75",
		editor.TokenProperty:  "#e06c75",
		editor.TokenTag:       "#e06c75",
		editor.TokenAttribute: "#d19a66",
		editor.TokenComment:   "#7f848e italic",
		editor.TokenRegexp:    "#e06c75",
		editor.TokenEscape:    "#56b6c2",
		editor.TokenMeta:      "#c678dd",
		editor.TokenHeading:   "#e06c75",
		editor.TokenInserted:  "#98c379",
		editor.TokenDeleted:   "#e06c75",
		editor.TokenInvalid:   "#ffffff",
	}}},
	// Gruvbox Light and Dark Medium (jdinhify/vscode-theme-gruvbox, MIT).
	{"gruvbox", "Gruvbox", communityThemes, &codeColors{text: "#3c3836", tokens: map[editor.TokenKind]string{
		editor.TokenKeyword:     "#9d0006",
		editor.TokenOperator:    "#427b58",
		editor.TokenString:      "#79740e",
		editor.TokenNumber:      "#8f3f71",
		editor.TokenConstant:    "#8f3f71",
		editor.TokenFunction:    "#b57614",
		editor.TokenType:        "#b57614",
		editor.TokenVariable:    "#076678",
		editor.TokenProperty:    "#076678",
		editor.TokenTag:         "#427b58",
		editor.TokenAttribute:   "#b57614",
		editor.TokenComment:     "#928374 italic",
		editor.TokenPunctuation: "#7c6f64",
		editor.TokenRegexp:      "#af3a03",
		editor.TokenEscape:      "#8f3f71",
		editor.TokenMeta:        "#af3a03",
		editor.TokenHeading:     "#af3a03 bold",
		editor.TokenLink:        "#8f3f71",
		editor.TokenCode:        "#427b58",
		editor.TokenInserted:    "#79740e",
		editor.TokenDeleted:     "#d65d0e",
		editor.TokenInvalid:     "#cc241d",
	}}, &codeColors{text: "#ebdbb2", tokens: map[editor.TokenKind]string{
		editor.TokenKeyword:     "#fb4934",
		editor.TokenOperator:    "#8ec07c",
		editor.TokenString:      "#b8bb26",
		editor.TokenNumber:      "#d3869b",
		editor.TokenConstant:    "#d3869b",
		editor.TokenFunction:    "#fabd2f",
		editor.TokenType:        "#fabd2f",
		editor.TokenVariable:    "#83a598",
		editor.TokenProperty:    "#83a598",
		editor.TokenTag:         "#8ec07c",
		editor.TokenAttribute:   "#fabd2f",
		editor.TokenComment:     "#928374 italic",
		editor.TokenPunctuation: "#a89984",
		editor.TokenRegexp:      "#fe8019",
		editor.TokenEscape:      "#d3869b",
		editor.TokenMeta:        "#fe8019",
		editor.TokenHeading:     "#fe8019 bold",
		editor.TokenLink:        "#d3869b",
		editor.TokenCode:        "#8ec07c",
		editor.TokenInserted:    "#b8bb26",
		editor.TokenDeleted:     "#d65d0e",
		editor.TokenInvalid:     "#cc241d",
	}}},
	// Kanagawa Lotus and Wave (metapho-re/kanagawa-vscode-theme, MIT).
	{"kanagawa", "Kanagawa", communityThemes, &codeColors{text: "#545464", tokens: map[editor.TokenKind]string{
		editor.TokenKeyword:     "#624c83",
		editor.TokenOperator:    "#77713f",
		editor.TokenString:      "#6f894e",
		editor.TokenNumber:      "#b35b79",
		editor.TokenConstant:    "#cc6d00",
		editor.TokenFunction:    "#4d699b",
		editor.TokenType:        "#597b75",
		editor.TokenVariable:    "#545464",
		editor.TokenProperty:    "#77713f",
		editor.TokenTag:         "#77713f",
		editor.TokenAttribute:   "#624c83",
		editor.TokenComment:     "#716e61",
		editor.TokenPunctuation: "#4e8ca2",
		editor.TokenRegexp:      "#836f4a",
		editor.TokenEscape:      "#6693bf",
		editor.TokenMeta:        "#624c83",
		editor.TokenHeading:     "#6f894e",
		editor.TokenLink:        "#6693bf",
		editor.TokenInserted:    "#6e915f",
		editor.TokenDeleted:     "#d7474b",
		editor.TokenInvalid:     "#e82424",
	}}, &codeColors{text: "#dcd7ba", tokens: map[editor.TokenKind]string{
		editor.TokenKeyword:     "#957fb8",
		editor.TokenOperator:    "#e6c384",
		editor.TokenString:      "#98bb6c",
		editor.TokenNumber:      "#d27e99",
		editor.TokenConstant:    "#ffa066",
		editor.TokenFunction:    "#7e9cd8",
		editor.TokenType:        "#7aa89f",
		editor.TokenVariable:    "#dcd7ba",
		editor.TokenProperty:    "#e6c384",
		editor.TokenTag:         "#e6c384",
		editor.TokenAttribute:   "#957fb8",
		editor.TokenComment:     "#727169",
		editor.TokenPunctuation: "#9cabca",
		editor.TokenRegexp:      "#c0a36e",
		editor.TokenEscape:      "#7fb4ca",
		editor.TokenMeta:        "#957fb8",
		editor.TokenHeading:     "#98bb6c",
		editor.TokenLink:        "#7fb4ca",
		editor.TokenInserted:    "#76946a",
		editor.TokenDeleted:     "#c34043",
		editor.TokenInvalid:     "#e82424",
	}}},
	// Night Owl Light and Night Owl (sdras/night-owl-vscode-theme, MIT).
	{"night-owl", "Night Owl", communityThemes, &codeColors{text: "#403f53", tokens: map[editor.TokenKind]string{
		editor.TokenKeyword:   "#994cc3 italic",
		editor.TokenOperator:  "#0c969b",
		editor.TokenString:    "#c96765",
		editor.TokenNumber:    "#aa0982",
		editor.TokenConstant:  "#4876d6",
		editor.TokenFunction:  "#4876d6 italic",
		editor.TokenType:      "#4876d6",
		editor.TokenVariable:  "#4876d6",
		editor.TokenProperty:  "#0c969b",
		editor.TokenTag:       "#994cc3",
		editor.TokenAttribute: "#4876d6 italic",
		editor.TokenComment:   "#989fb1 italic",
		editor.TokenRegexp:    "#5ca7e4",
		editor.TokenEscape:    "#aa0982",
		editor.TokenMeta:      "#994cc3 italic",
		editor.TokenHeading:   "#4876d6",
		editor.TokenLink:      "#ff869a",
		editor.TokenCode:      "#0c969b",
		editor.TokenInvalid:   "#c96765",
	}}, &codeColors{text: "#d6deeb", tokens: map[editor.TokenKind]string{
		editor.TokenKeyword:   "#c792ea italic",
		editor.TokenOperator:  "#7fdbca",
		editor.TokenString:    "#ecc48d",
		editor.TokenNumber:    "#f78c6c",
		editor.TokenConstant:  "#82aaff",
		editor.TokenFunction:  "#82aaff italic",
		editor.TokenType:      "#c5e478",
		editor.TokenVariable:  "#c5e478",
		editor.TokenProperty:  "#baebe2",
		editor.TokenTag:       "#caece6",
		editor.TokenAttribute: "#c5e478 italic",
		editor.TokenComment:   "#637777 italic",
		editor.TokenRegexp:    "#5ca7e4",
		editor.TokenEscape:    "#f78c6c",
		editor.TokenMeta:      "#c792ea italic",
		editor.TokenHeading:   "#82b1ff",
		editor.TokenLink:      "#ff869a",
		editor.TokenCode:      "#80cbc4",
		editor.TokenInserted:  "#036a07",
		editor.TokenDeleted:   "#ff0000",
		editor.TokenInvalid:   "#ffffff",
	}}},
	// Tokyo Night Light and Tokyo Night (enkia/tokyo-night-vscode-theme, MIT).
	{"tokyo-night", "Tokyo Night", communityThemes, &codeColors{text: "#343b59", tokens: map[editor.TokenKind]string{
		editor.TokenKeyword:     "#65359d",
		editor.TokenOperator:    "#006c86",
		editor.TokenString:      "#385f0d",
		editor.TokenNumber:      "#965027",
		editor.TokenConstant:    "#965027",
		editor.TokenFunction:    "#2959aa",
		editor.TokenType:        "#343b58",
		editor.TokenVariable:    "#343b58",
		editor.TokenProperty:    "#0f4b6e",
		editor.TokenTag:         "#8c4351",
		editor.TokenAttribute:   "#65359d",
		editor.TokenComment:     "#888b94",
		editor.TokenPunctuation: "#006c86",
		editor.TokenRegexp:      "#3e6968",
		editor.TokenEscape:      "#363c4d",
		editor.TokenMeta:        "#33635c",
		editor.TokenHeading:     "#343b58",
		editor.TokenLink:        "#33635c",
		editor.TokenInserted:    "#449dab",
		editor.TokenDeleted:     "#914c54",
		editor.TokenInvalid:     "#942f2f",
	}}, &codeColors{text: "#a9b1d6", tokens: map[editor.TokenKind]string{
		editor.TokenKeyword:     "#bb9af7",
		editor.TokenOperator:    "#89ddff",
		editor.TokenString:      "#9ece6a",
		editor.TokenNumber:      "#ff9e64",
		editor.TokenConstant:    "#ff9e64",
		editor.TokenFunction:    "#7aa2f7",
		editor.TokenType:        "#c0caf5",
		editor.TokenVariable:    "#c0caf5",
		editor.TokenProperty:    "#7dcfff",
		editor.TokenTag:         "#f7768e",
		editor.TokenAttribute:   "#bb9af7",
		editor.TokenComment:     "#51597d",
		editor.TokenPunctuation: "#89ddff",
		editor.TokenRegexp:      "#b4f9f8",
		editor.TokenEscape:      "#89ddff",
		editor.TokenMeta:        "#73daca",
		editor.TokenHeading:     "#c0caf5",
		editor.TokenLink:        "#73daca",
		editor.TokenInserted:    "#449dab",
		editor.TokenDeleted:     "#914c54",
		editor.TokenInvalid:     "#ff5370",
	}}},
	// Vitesse Light and Dark (antfu/vscode-theme-vitesse, MIT).
	{"vitesse", "Vitesse", communityThemes, &codeColors{text: "#393a34", tokens: map[editor.TokenKind]string{
		editor.TokenKeyword:     "#1e754f",
		editor.TokenOperator:    "#ab5959",
		editor.TokenString:      "#b56959",
		editor.TokenNumber:      "#2f798a",
		editor.TokenConstant:    "#1e754f",
		editor.TokenFunction:    "#59873a",
		editor.TokenType:        "#2e8f82",
		editor.TokenVariable:    "#b07d48",
		editor.TokenProperty:    "#b07d48",
		editor.TokenTag:         "#1e754f",
		editor.TokenAttribute:   "#b07d48",
		editor.TokenComment:     "#a0ada0",
		editor.TokenPunctuation: "#999999",
		editor.TokenRegexp:      "#ab5e3f",
		editor.TokenEscape:      "#a65e2b",
		editor.TokenMeta:        "#1e754f",
		editor.TokenHeading:     "#1c6b48 bold",
		editor.TokenCode:        "#1c6b48",
		editor.TokenInserted:    "#22863a",
		editor.TokenDeleted:     "#b31d28",
		editor.TokenInvalid:     "#b31d28 italic",
	}}, &codeColors{text: "#cfcbbf", tokens: map[editor.TokenKind]string{
		editor.TokenKeyword:     "#4d9375",
		editor.TokenOperator:    "#cb7676",
		editor.TokenString:      "#c98a7d",
		editor.TokenNumber:      "#4c9a91",
		editor.TokenConstant:    "#4d9375",
		editor.TokenFunction:    "#80a665",
		editor.TokenType:        "#5da994",
		editor.TokenVariable:    "#bd976a",
		editor.TokenProperty:    "#bd976a",
		editor.TokenTag:         "#4d9375",
		editor.TokenAttribute:   "#bd976a",
		editor.TokenComment:     "#6a776a",
		editor.TokenPunctuation: "#666666",
		editor.TokenRegexp:      "#c4704f",
		editor.TokenEscape:      "#c99076",
		editor.TokenMeta:        "#4d9375",
		editor.TokenHeading:     "#4d9375 bold",
		editor.TokenCode:        "#4d9375",
		editor.TokenInserted:    "#85e89d",
		editor.TokenDeleted:     "#fdaeb7",
		editor.TokenInvalid:     "#fdaeb7 italic",
	}}},
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
// background the app's, and its prose's styles (bold headings and strong
// text, italic emphasis); the kinds the theme does not color in its text.
func (cc *codeColors) editorTheme(t *ui.Theme) *editor.Theme {
	et := editor.DefaultTheme(t)
	et.Text = ui.Hex(cc.text)
	for k := range et.Tokens {
		et.Tokens[k].Color = ui.Color{}
	}
	for k, v := range cc.tokens {
		hex, style, _ := strings.Cut(v, " ")
		et.Tokens[k].Color = ui.Hex(hex)
		et.Tokens[k].Italic = style == "italic"
		et.Tokens[k].Bold = style == "bold" || k == editor.TokenHeading
	}
	if c, ok := cc.tokens[editor.TokenInserted]; ok {
		et.Added = ui.Hex(strings.Fields(c)[0])
	}
	if c, ok := cc.tokens[editor.TokenDeleted]; ok {
		et.Deleted = ui.Hex(strings.Fields(c)[0])
	}
	return et
}
