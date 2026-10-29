# Code themes

Which themes color code in HyperAI, why those, and how to add one. The
themes are in [codethemes.go](../codethemes.go); the setting is the Code
theme row of Appearance ([design.md](design.md), Settings pages).

## What a theme is here

A theme colors the tokens of code blocks and nothing else. A block keeps
the app's surface, hairline and header, so that it sits with the
conversation; the theme gives its light side in the light app and its dark
side in the dark one. Code in a line keeps the accent.

So a theme's colors show on the app's surface, not on the background its
authors made them for: `#fffdfc` in the light app, the warm `#241e1f` in
the dark. Whether they read there is what decides which themes the app
offers.

## Where the colors come from

Each theme's own file, never memory:

- The themes [Shiki](https://shiki.style) bundles, as its `tm-themes`
  package has them (1.10.10), each from its authors' repository, all MIT.
  That is the set the VS Code world highlights code with, and what "the
  usual themes" means here.
- Tokyo Night Light and Night Owl Light, which Shiki does not bundle, from
  their authors' repositories (enkia/tokyo-night-vscode-theme,
  sdras/night-owl-vscode-theme). Tokyo Night Light's file says
  `"type": "dark"`; it is light.
- Xcode's Default (Light) and (Dark), from the `.xccolortheme` files in
  Xcode's bundle (`DVTUserInterfaceKit.framework/…/FontAndColorThemes`),
  as Apple publishes them nowhere else. Its text is black and white at
  85%, which the app takes over its surfaces.

A VS Code theme colors TextMate scopes; MyGo's highlighter has kinds of
tokens. Each kind takes the color of the first of these scopes the theme
colors, the rule with the longest matching selector winning, the later of
equals, as VS Code resolves them (rules for descendants, `a b`, aside):

| Kind | Scopes |
| --- | --- |
| Keyword | `keyword`, `storage.type`, `storage` |
| Operator | `keyword.operator` |
| String | `string.quoted`, `string` |
| Number | `constant.numeric` |
| Constant | `constant.language`, `constant` |
| Function | `entity.name.function`, `support.function` |
| Type | `entity.name.type`, `support.type`, `entity.name.class` |
| Variable | `variable.other`, `variable` |
| Property | `variable.other.property`, `variable.other.object.property`, `support.type.property-name`, `meta.object-literal.key` |
| Tag, Attribute | `entity.name.tag`; `entity.other.attribute-name` |
| Comment | `comment` |
| Punctuation | `punctuation` |
| Regexp, Escape | `string.regexp`; `constant.character.escape` |
| Meta | `meta.preprocessor`, `keyword.control.directive`, `meta.decorator` |
| Heading, Link, Code | `markup.heading`, `entity.name.section`; `markup.underline.link`; `markup.inline.raw`, `markup.raw` |
| Inserted, Deleted, Invalid | `markup.inserted`; `markup.deleted`; `invalid.illegal`, `invalid` |

The text is the theme's `editor.foreground`. A kind the theme does not
color shows in its text; italic and bold come with the color.

## What a theme must do

1. **A light side and a dark one**, of one family. A theme with no light
   side cannot follow the light app: its colors, made for a dark
   background, do not read on a light one.
2. **Text at 4.5:1** on the app's surface, both sides: WCAG's AA for text.
3. **Keywords, strings, functions, types and numbers at 3:1**: WCAG's
   floor for what is not body text. Color is not all that tells them
   apart, the code's words are, but under 3:1 they read as faded.
4. **Comments at 2.3:1**: faint by design in every theme, but under this
   they are gone (Min Light's are 1.74).
5. **Fits the design**: calm, so no neon (design.md's north star); not
   Default again; not a brand's copy of another.

`TestCodeThemesRead` holds every theme to 2–4, on the palettes' own
surfaces.

## The themes, measured

The lower of each theme's two sides, on the app's surfaces.

| Theme | Sides | Text | Weakest of the five | Comments | |
| --- | --- | --- | --- | --- | --- |
| VS Code | Light+, Dark+ | 11.1 | 4.52 types | 4.92 | offered |
| Xcode | Default (Light), (Dark) | 12.2 | 5.70 keywords | 3.68 | offered |
| One | One Light, One Dark Pro | 7.7 | 3.15 types | 2.54 | offered |
| Gruvbox | Light, Dark Medium | 11.4 | 3.72 functions | 3.62 | offered |
| Kanagawa | Lotus, Wave | 7.3 | 3.86 strings | 3.34 | offered |
| Night Owl | Light, Dark | 10.1 | 3.69 strings | 2.61 | offered |
| Tokyo Night | Light, Night | 7.8 | 5.96 numbers | 2.40 | offered |
| Vitesse | Light, Dark | 10.1 | 3.86 types | 2.31 | offered |
| GitHub | Light, Dark Default | 13.9 | 4.98 functions | 4.48 | is Default |
| Solarized | Light, Dark | **4.39** | 3.11 strings | 2.64 | text under 4.5 |
| Catppuccin | Latte, Mocha | 7.9 | **2.58** types | 3.90 | Latte too faint |
| Slack | Ochin, Dark | 13.1 | **2.49** functions | 4.92 | Dark is Dark+ |
| Everforest | Light, Dark | 5.5 | **2.26** strings | 2.72 | Light too faint |
| Rosé Pine | Dawn, Main | 7.2 | **2.21** strings | 2.94 | Dawn too faint |
| Material | Lighter, Default | **2.55** | 2.25 strings | 2.55 | Lighter too faint |
| Ayu | Light, Dark | 6.2 | **2.10** functions | **2.19** | Light too faint |
| Min | Light, Dark | 6.5 | 4.54 numbers | **1.74** | comments gone |

Dark alone, so not offered: Nord, Dracula (and Soft), Monokai, Houston,
Poimandres, Vesper, Andromeeda, Plastic, Red, Aurora X; and Synthwave '84
and Laserwave, neon besides. Tokyo Night is enkia's: its Light, not
folke's Day.

## Adding one

Take its colors from its own files by the scopes above, measure it on
both surfaces, and add it to `codeThemes` in its group (editors' own, or
the others by name) with where its colors came from.
`TestCodeThemesRead` fails a theme that does not read.
