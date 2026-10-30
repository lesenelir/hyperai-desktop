package main

import (
	"github.com/egoist/mygo/ui"

	"hyperai-desktop/locales"
)

// The model the composer asks: a chat's, the model it asked last, as it
// is shown, and a new chat's the default, which the Default model page
// chooses, or the composer's menu, whichever chose last. The composer's
// menu chooses another for the chat shown until another is.

// choose makes model of p the one the composer asks.
func (a *app) choose(p provider, model string) { a.prov, a.model = p, model }

// pick makes model of p, which the user chose in the composer's menu, the
// one it asks, and the default, which new chats ask from then on.
func (a *app) pick(p provider, model string) {
	a.choose(p, model)
	a.defaultModel = modelRef{p.ID(), model}
	a.saveConf()
}

// providerOf returns the provider asked that offers ref's model, nil for
// none.
func (a *app) providerOf(ref modelRef) provider {
	for _, p := range a.provs {
		if p.ID() == ref.Provider {
			for _, m := range p.Models() {
				if m == ref.Model {
					return p
				}
			}
		}
	}
	return nil
}

// chooseFor chooses the model th asked last, where it is still offered,
// and the default otherwise, or for a new chat (nil): the one chosen on
// the Default model page, else the first provider's first model.
func (a *app) chooseFor(th *thread) {
	a.modelFor = a.selected
	if len(a.provs) == 0 {
		a.choose(nil, "")
		return
	}
	if th != nil && th.Model != "" {
		for _, p := range a.provs {
			// Of the chat's provider, or of any, for chats kept before
			// they kept it.
			if (th.Provider == "" || th.Provider == p.ID()) && a.providerOf(modelRef{p.ID(), th.Model}) != nil {
				a.choose(p, th.Model)
				return
			}
		}
	}
	ref := a.defaultRef()
	a.choose(a.providerOf(ref), ref.Model)
}

// defaultRef returns the model a new chat asks.
func (a *app) defaultRef() modelRef {
	if a.providerOf(a.defaultModel) != nil {
		return a.defaultModel
	}
	return modelRef{a.provs[0].ID(), a.provs[0].Models()[0]}
}

// followChat chooses the model of the chat shown, once it is shown,
// however it came to be.
func (a *app) followChat() {
	if a.selected != a.modelFor {
		a.chooseFor(a.current())
	}
}

// modelMenu fills m with the models offered, each provider's under its
// name, where there are several, the one checked tells checked, and a
// choice calls chosen.
func (a *app) modelMenu(m *ui.Menu, checked func(p provider, model string) bool, chosen func(p provider, model string)) {
	for i, p := range a.provs {
		if len(a.provs) > 1 {
			if i > 0 {
				m.Separator()
			}
			m.Item(p.Name()).Disabled(true)
		}
		for _, model := range p.Models() {
			if m.Item(model).Checked(checked(p, model)).Chosen() {
				chosen(p, model)
			}
		}
	}
}

// modelTerms returns what the Default model page holds, as the settings'
// field finds it.
func (a *app) modelTerms() []settingsTerm {
	return []settingsTerm{{locales.DefaultModelRow(), terms(locales.DefaultModelTerms())}}
}

// modelPage builds the Default model page: the model a new chat asks, of
// those the providers offer, or, where none has a key, the button that
// adds one.
func (a *app) modelPage(c *ui.Context) {
	a.settingsCard(c, locales.DefaultModelCard(), func() {
		if len(a.provs) == 0 {
			a.settingRow(c, locales.DefaultModelRow(), locales.DefaultModelNone(), func() {
				if a.settingButton(c, "add a provider", locales.ModelAddProvider(), false) {
					a.settingsPage = "providers"
				}
			})
			return
		}
		ref := a.defaultRef()
		a.settingRow(c, locales.DefaultModelRow(), locales.DefaultModelAbout(), func() {
			titles := func() []string {
				var out []string
				for _, p := range a.provs {
					out = append(out, p.Models()...)
				}
				return out
			}
			a.popUpButton(c, locales.DefaultModelRow(), ref.Model, titles, func(m *ui.Menu) {
				a.modelMenu(m, func(p provider, model string) bool { return ref == modelRef{p.ID(), model} },
					func(p provider, model string) {
						a.defaultModel = modelRef{p.ID(), model}
						a.saveConf()
						// A new chat shown asks it at once.
						if a.current() == nil {
							a.choose(p, model)
						}
					})
			})
		})
	})
}
