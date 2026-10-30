package main

import (
	"context"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"hyperai-desktop/locales"
)

// How hard a model thinks before it replies, where it takes being told:
// its effort, of efforts, as each provider names them. A model offers the
// composer the efforts it takes alone, and none where it takes none, as
// telling one that does not fails the reply. Which those are:
//
//   - Anthropic says, of each model, in its list (Capabilities.Effort);
//     OpenRouter (supported_parameters) and Copilot
//     (SupportedReasoningEfforts) too. Their lists keep them
//     (providerConf.Efforts), listed once more where kept before they were.
//   - OpenAI's and Google's lists do not: their reasoning models are known
//     by their names (effortRules).
//   - The others, DeepSeek, the servers on the user's computer and the
//     providers the user added, take none the app can be sure of.
//
// The effort goes to the provider in what the reply is asked with
// (withEffort): Anthropic's output_config.effort, OpenAI's and Google's
// reasoning_effort, OpenRouter's reasoning.effort, Copilot's session's
// ReasoningEffort. "", the model's own, sends none. A chat keeps the one it
// asked last, and a new chat asks the one chosen last (layout.json).

// efforts are the efforts a provider may name, least first.
var efforts = []string{"minimal", "low", "medium", "high", "xhigh", "max"}

// effortTitle returns what the composer calls effort, in the app's
// language: "" is Default.
func effortTitle(effort string) string {
	switch effort {
	case "minimal":
		return locales.EffortMinimal()
	case "low":
		return locales.EffortLow()
	case "medium":
		return locales.EffortMedium()
	case "high":
		return locales.EffortHigh()
	case "xhigh":
		return locales.EffortXhigh()
	case "max":
		return locales.EffortMax()
	}
	return locales.EffortDefault()
}

// listsEfforts reports whether the provider id's list says the efforts of
// its models.
func listsEfforts(id string) bool {
	return id == "anthropic" || id == "openrouter" || id == "copilot"
}

// effortRules returns the efforts of model of the provider id whose list
// does not say them, by its name: OpenAI's reasoning models, GPT-5's and
// the o series', and Google's thinking models, Gemini 2.5's and since.
func effortRules(id, model string) []string {
	switch id {
	case "openai":
		switch {
		case strings.HasPrefix(model, "gpt-5") && !strings.Contains(model, "-chat"):
			return []string{"minimal", "low", "medium", "high"}
		case len(model) > 1 && model[0] == 'o' && model[1] >= '1' && model[1] <= '9':
			return []string{"low", "medium", "high"}
		}
	case "google":
		for _, v := range []string{"gemini-2.5", "gemini-3", "gemini-4"} {
			if strings.HasPrefix(model, v) {
				return []string{"low", "medium", "high"}
			}
		}
	}
	return nil
}

// effortsFor returns the efforts model of p takes, least first, nil for
// none: as its list said them, where it says them, else by its name.
func (a *app) effortsFor(p provider, model string) []string {
	if p == nil {
		return nil
	}
	id := p.ID()
	if listsEfforts(id) {
		if c := a.confs[id]; c != nil {
			return c.Efforts[model]
		}
		return nil
	}
	return effortRules(id, model)
}

// askedEffort returns the effort the composer asks of its model: the one
// chosen, where the model takes it, else "", its own.
func (a *app) askedEffort() string {
	if slices.Contains(a.effortsFor(a.prov, a.model), a.effort) {
		return a.effort
	}
	return ""
}

// effortLister is a provider whose List says the efforts of the models it
// listed, by their IDs, once it returned.
type effortLister interface {
	ListedEfforts() map[string][]string
}

// orderEfforts returns of, the efforts a list said, in efforts' order,
// those it does not know left out.
func orderEfforts(of []string) []string {
	var out []string
	for _, e := range efforts {
		if slices.Contains(of, e) {
			out = append(out, e)
		}
	}
	return out
}

type effortKey struct{}

// withEffort returns ctx, which a reply is asked with, telling its
// provider effort, "" for none.
func withEffort(ctx context.Context, effort string) context.Context {
	if effort == "" {
		return ctx
	}
	return context.WithValue(ctx, effortKey{}, effort)
}

// effortOf returns the effort ctx tells, "" for none.
func effortOf(ctx context.Context) string {
	e, _ := ctx.Value(effortKey{}).(string)
	return e
}

// effortPicker builds, by the model's button, the one that chooses its
// effort, where it takes any: what it asks, as the model's button shows
// its model, over a menu of Default and the efforts it takes, the one
// chosen checked. The choice is the next new chat's too (layout.json).
func (a *app) effortPicker(c *ui.Context) {
	takes := a.effortsFor(a.prov, a.model)
	if len(takes) == 0 {
		return
	}
	t := c.Theme()
	asked := a.askedEffort()
	b := ui.ButtonBase(c.Key("effort")).Padding(5, 8).Gap(6).Radius(8).TextColor(t.TextMuted).Label(locales.EffortLabel())
	b = tooltip(c, b, locales.EffortLabel())
	if b.Hovered() {
		b.Background(a.pal.fg.Alpha(0.06)).TextColor(t.Text)
	}
	b.Children(func() {
		ui.Icon(c, effortIcon).Size(13, 13)
		ui.Text(c, effortTitle(asked)).FontSize(13)
		ui.Icon(c, chevronIcon).Size(12, 12)
	})
	b.Menu(func(m *ui.Menu) {
		choose := func(e string) {
			a.effort, a.lastEffort = e, e
			a.saveLayout()
		}
		if m.Item(locales.EffortDefault()).Checked(asked == "").Chosen() {
			choose("")
		}
		m.Separator()
		for _, e := range takes {
			if m.Item(effortTitle(e)).Checked(asked == e).Chosen() {
				choose(e)
			}
		}
	})
}
