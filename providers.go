package main

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"hyperai-desktop/locales"
)

// keysSecret is the secret of the system's credential store that keeps the
// providers' API keys, as JSON by the provider's ID: one secret, which the
// system asks to unlock once, rather than one for each.
const keysSecret = "api-keys"

// keyring keeps secrets, as mygo.Secrets does in the system's credential
// store.
type keyring interface {
	Get(key string) ([]byte, error)
	Set(key string, value []byte) error
	Delete(key string) error
}

// providerConf is what the user set of a provider but its key, as
// providers.json keeps it: its endpoint, "" for its kind's; the models the
// composer offers, which are Models where Custom, the user having added
// or removed one, and its kind's otherwise, as many of them as it lists;
// and Listed, the models it listed last that chat.
type providerConf struct {
	BaseURL string   `json:"baseURL,omitempty"`
	Custom  bool     `json:"custom,omitempty"`
	Models  []string `json:"models,omitempty"`
	Listed  []string `json:"listed,omitempty"`
}

// modelRef names a model of a provider, by the provider's ID.
type modelRef struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// providersFile is what providers.json holds, beside the chats: the
// model a new chat asks, and each provider's settings, by its ID.
type providersFile struct {
	Default   modelRef                 `json:"default,omitzero"`
	Providers map[string]*providerConf `json:"providers,omitempty"`
}

// loadConf reads the providers' settings kept in dir, if any.
func (a *app) loadConf(dir string) {
	a.confPath = filepath.Join(dir, "providers.json")
	b, err := os.ReadFile(a.confPath)
	if err != nil {
		return
	}
	var f providersFile
	if json.Unmarshal(b, &f) != nil {
		return
	}
	a.confs, a.defaultModel = f.Providers, f.Default
}

// saveConf keeps the providers' settings, where the app has a place for
// them (not in tests), on a goroutine, as save does the chats.
func (a *app) saveConf() {
	if a.confPath == "" {
		return
	}
	b, err := json.MarshalIndent(providersFile{Default: a.defaultModel, Providers: a.confs}, "", "  ")
	if err != nil {
		return
	}
	a.confSaver.save(a.confPath, b)
}

// conf returns the settings of the provider id, made where it has none.
func (a *app) conf(id string) *providerConf {
	if a.confs == nil {
		a.confs = map[string]*providerConf{}
	}
	c := a.confs[id]
	if c == nil {
		c = &providerConf{}
		a.confs[id] = c
	}
	return c
}

// endpoint returns where the API of kind is: the user's, else its own.
func (a *app) endpoint(kind *providerKind) string {
	if c := a.confs[kind.id]; c != nil && c.BaseURL != "" {
		return c.BaseURL
	}
	return kind.baseURL
}

// shownModels returns the models of kind the composer offers: those the
// user chose, else its own, but for those it no longer lists.
func (a *app) shownModels(kind *providerKind) []string {
	c := a.confs[kind.id]
	switch {
	case c == nil:
		return kind.models
	case c.Custom:
		return c.Models
	}
	if len(c.Listed) > 0 {
		listed := slices.DeleteFunc(slices.Clone(kind.models), func(m string) bool { return !slices.Contains(c.Listed, m) })
		if len(listed) > 0 {
			return listed
		}
	}
	return kind.models
}

// loadKeys reads the providers' API keys from k, which keeps them from
// then on, and asks the providers that have one.
func (a *app) loadKeys(k keyring) {
	a.keyring = k
	b, err := k.Get(keysSecret)
	switch {
	case err == nil:
		if json.Unmarshal(b, &a.keys) != nil {
			a.keys = nil
		}
	case !errors.Is(err, mygo.ErrSecretNotFound):
		a.keyError = locales.KeysUnread(err.Error())
	}
	a.setProviders()
}

// key returns the API key of the provider id, "" for none.
func (a *app) key(id string) string { return a.keys[id] }

// setKey keeps key as the API key of the provider id, or forgets it for
// "", asks the providers that have one from then on, and lists the
// models of the one keyed, which tells too whether the key serves. Where
// the credential store cannot keep it, the key serves until the app
// quits, and the Providers page says why.
func (a *app) setKey(id, key string) {
	keys := maps.Clone(a.keys)
	if keys == nil {
		keys = map[string]string{}
	}
	if key = strings.TrimSpace(key); key == "" {
		delete(keys, id)
	} else {
		keys[id] = key
	}
	a.keyError = ""
	if a.keyring != nil {
		var err error
		if len(keys) == 0 {
			err = a.keyring.Delete(keysSecret)
		} else {
			b, _ := json.Marshal(keys)
			err = a.keyring.Set(keysSecret, b)
		}
		if err != nil {
			a.keyError = locales.KeysUnkept(err.Error())
		}
	}
	a.keys = keys
	delete(a.listErrors, id)
	a.setProviders()
	if key != "" {
		a.listModels(id)
	}
}

// setEndpoint makes url where the API of the provider id is, its own for
// "", and lists its models there.
func (a *app) setEndpoint(id, url string) {
	url = strings.TrimRight(strings.TrimSpace(url), "/")
	if kind := providerKindOf(id); kind != nil && url == kind.baseURL {
		url = ""
	}
	a.conf(id).BaseURL = url
	a.saveConf()
	delete(a.listErrors, id)
	a.setProviders()
	a.listModels(id)
}

// setShown makes models those of the provider id the composer offers.
func (a *app) setShown(id string, models []string) {
	c := a.conf(id)
	c.Custom, c.Models = true, models
	a.saveConf()
	a.setProviders()
}

// addModel offers model of the provider id in the composer too, last.
func (a *app) addModel(id, model string) {
	model = strings.TrimSpace(model)
	shown := a.shownModels(providerKindOf(id))
	if model == "" || slices.Contains(shown, model) {
		return
	}
	a.setShown(id, append(slices.Clone(shown), model))
}

// removeModel offers model of the provider id in the composer no more.
func (a *app) removeModel(id, model string) {
	shown := a.shownModels(providerKindOf(id))
	a.setShown(id, slices.DeleteFunc(slices.Clone(shown), func(m string) bool { return m == model }))
}

// listTimeout is how long listing a provider's models takes at most.
const listTimeout = 20 * time.Second

// listModels asks the provider id for its models, on a goroutine where
// the app posts (app.post), and keeps those that chat, by name, for the
// Providers page to add of. The composer offers no more of its own models
// than it lists.
func (a *app) listModels(id string) {
	kind := providerKindOf(id)
	if kind == nil || a.key(id) == "" {
		return
	}
	// Opened of its own, as the composer offers none of its models where
	// the user removed them all.
	key, url := a.key(id), a.endpoint(kind)
	p := kind.open(kind, url, key, nil)
	if a.listing == nil {
		a.listing, a.listErrors = map[string]bool{}, map[string]string{}
	}
	a.listing[id] = true
	delete(a.listErrors, id)
	done := func(models []string, err error) {
		// What the key or the endpoint listed before it changed: the
		// listing of the new one is to come.
		if a.key(id) != key || a.endpoint(kind) != url {
			return
		}
		delete(a.listing, id)
		if err != nil {
			a.listErrors[id] = err.Error()
			return
		}
		if kind.chat != nil {
			models = slices.DeleteFunc(models, func(m string) bool { return !kind.chat(m) })
		}
		slices.Sort(models)
		a.conf(id).Listed = slices.Compact(models)
		a.saveConf()
		a.setProviders()
	}
	list := func() ([]string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), listTimeout)
		defer cancel()
		return p.List(ctx)
	}
	if a.post == nil {
		done(list())
		return
	}
	go func() {
		models, err := list()
		a.post(func() { done(models, err) })
	}()
}

// listUnlisted lists the models of the providers with a key that have
// listed none yet, as the app starts.
func (a *app) listUnlisted() {
	for _, kind := range providerKinds {
		if c := a.confs[kind.id]; c == nil || len(c.Listed) == 0 {
			a.listModels(kind.id)
		}
	}
}

// setProviders makes the providers with a key and a model those the
// composer asks, the fallback where none is. The model chosen stays where its
// provider offers it still; otherwise the chat's is chosen, or the
// default.
func (a *app) setProviders() {
	var provs []provider
	for i := range providerKinds {
		kind := &providerKinds[i]
		key, models := a.key(kind.id), a.shownModels(kind)
		if key != "" && len(models) > 0 {
			provs = append(provs, kind.open(kind, a.endpoint(kind), key, models))
		}
	}
	if len(provs) == 0 && a.fallback != nil {
		provs = []provider{a.fallback}
	}
	a.provs = provs
	if a.prov != nil {
		if p := a.providerOf(modelRef{a.prov.ID(), a.model}); p != nil {
			a.prov = p
			return
		}
	}
	a.chooseFor(a.current())
}

// providerTerms returns what the Providers page holds, as the settings'
// field finds it: each provider, by its name, and its rows.
func (a *app) providerTerms() []settingsTerm {
	var out []settingsTerm
	for _, kind := range providerKinds {
		out = append(out, settingsTerm{kind.name, terms(locales.ProviderTerms())})
	}
	return out
}

// providersPage builds the Providers page: a card of each provider, which
// takes its API key and, once it has one, where its API is and the models
// the composer offers of it; and where the keys are kept.
func (a *app) providersPage(c *ui.Context) {
	for i := range providerKinds {
		kind := &providerKinds[i]
		a.settingsCard(c, kind.name, func() {
			a.keyRow(c, kind)
			if a.key(kind.id) == "" {
				return
			}
			settingDivider(c)
			a.endpointRow(c, kind)
			settingDivider(c)
			a.modelsRow(c, kind)
			for _, model := range a.shownModels(kind) {
				settingDivider(c)
				a.modelRow(c, kind, model)
			}
		})
	}
	note := locales.KeysKept(platform())
	if a.keyError != "" {
		note = a.keyError
	}
	ui.Text(c, note).FontSize(12).LineHeight(1.5).TextColor(c.Theme().TextMuted).Padding(0, 4)
}

// keyRow builds the row of kind's API key: whether it has one, and the
// buttons that add, change and remove it, or, while it is being added or
// changed, the field that takes it.
func (a *app) keyRow(c *ui.Context, kind *providerKind) {
	about := locales.KeyNone()
	if a.keys[kind.id] != "" {
		about = locales.KeySet(keyEnd(a.keys[kind.id]))
	}
	a.settingRow(c, locales.KeyRow(), about, func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if a.editing == "key "+kind.id {
				if v, ok := a.editField(c, locales.KeyPlaceholder(), locales.KeyLabel(kind.name), true); ok {
					a.setKey(kind.id, v)
				}
				return
			}
			if a.keys[kind.id] != "" {
				if a.settingButton(c, "remove key "+kind.id, locales.KeyRemove(), false) {
					a.setKey(kind.id, "")
				}
				if a.settingButton(c, "change key "+kind.id, locales.KeyChange(), false) {
					a.edit("key "+kind.id, "")
				}
				return
			}
			if a.settingButton(c, "get key "+kind.id, locales.KeyGet(), false) {
				c.Services().OpenURL(kind.keyURL)
			}
			if a.settingButton(c, "add key "+kind.id, locales.KeyAdd(), false) {
				a.edit("key "+kind.id, "")
			}
		})
	})
}

// endpointRow builds the row of where kind's API is, which a proxy or a
// compatible service changes.
func (a *app) endpointRow(c *ui.Context, kind *providerKind) {
	url, custom := a.endpoint(kind), a.endpoint(kind) != kind.baseURL
	about := locales.EndpointDefault(url)
	if custom {
		about = url
	}
	a.settingRow(c, locales.Endpoint(), about, func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if a.editing == "endpoint "+kind.id {
				if v, ok := a.editField(c, kind.baseURL, locales.EndpointLabel(kind.name), false); ok {
					a.setEndpoint(kind.id, v)
				}
				return
			}
			if custom && a.settingButton(c, "reset endpoint "+kind.id, locales.EndpointReset(), false) {
				a.setEndpoint(kind.id, "")
			}
			if a.settingButton(c, "change endpoint "+kind.id, locales.KeyChange(), false) {
				a.edit("endpoint "+kind.id, url)
			}
		})
	})
}

// modelsRow builds the row over kind's models: how many the composer
// offers, of those it lists, or why it listed none; the button that lists
// them again, and the menu that adds one.
func (a *app) modelsRow(c *ui.Context, kind *providerKind) {
	listed := a.listedModels(kind)
	shown := a.shownModels(kind)
	about := locales.ModelsShown(len(shown))
	switch {
	case a.listing[kind.id]:
		about = locales.ModelsListing()
	case a.listErrors[kind.id] != "":
		about = locales.ModelsFailed(a.listErrors[kind.id])
	case len(listed) > 0:
		about = locales.ModelsShownListed(len(shown), len(listed))
	}
	a.settingRow(c, locales.Models(), about, func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if a.editing == "model "+kind.id {
				if v, ok := a.editField(c, locales.ModelPlaceholder(), locales.ModelLabel(kind.name), false); ok {
					a.addModel(kind.id, v)
				}
				return
			}
			if a.settingButton(c, "list models "+kind.id, locales.ModelsRefresh(), a.listing[kind.id]) {
				a.listModels(kind.id)
			}
			a.settingMenuButton(c, "add model "+kind.id, locales.ModelAdd(), func(m *ui.Menu) {
				a.addModelMenu(m, kind, listed, shown)
			})
		})
	})
}

// listedModels returns the models kind listed last.
func (a *app) listedModels(kind *providerKind) []string {
	if c := a.confs[kind.id]; c != nil {
		return c.Listed
	}
	return nil
}

// addModelMenu fills m with the models kind lists that the composer does
// not offer, which a choice adds, by their vendors where their names say
// it (OpenRouter's "anthropic/…"), and the item that adds another by its
// name.
func (a *app) addModelMenu(m *ui.Menu, kind *providerKind, listed, shown []string) {
	var vendors []string
	byVendor := map[string][]string{}
	var flat []string
	for _, model := range listed {
		if slices.Contains(shown, model) {
			continue
		}
		if vendor, _, ok := strings.Cut(model, "/"); ok {
			if byVendor[vendor] == nil {
				vendors = append(vendors, vendor)
			}
			byVendor[vendor] = append(byVendor[vendor], model)
		} else {
			flat = append(flat, model)
		}
	}
	add := func(m *ui.Menu, model string) {
		if m.Item(model).Chosen() {
			a.addModel(kind.id, model)
		}
	}
	for _, vendor := range vendors {
		m.Submenu(vendor, func(m *ui.Menu) {
			for _, model := range byVendor[vendor] {
				add(m, model)
			}
		})
	}
	for _, model := range flat {
		add(m, model)
	}
	if len(vendors)+len(flat) == 0 {
		m.Item(locales.ModelsNoneListed()).Disabled(true)
	}
	m.Separator()
	if m.Item(locales.ModelOther()).Chosen() {
		a.edit("model "+kind.id, "")
	}
}

// modelRow builds the row of a model of kind the composer offers, which
// says whether a new chat asks it, and the button that removes it.
func (a *app) modelRow(c *ui.Context, kind *providerKind, model string) {
	about := ""
	if a.defaultModel == (modelRef{kind.id, model}) {
		about = locales.ModelIsDefault()
	}
	a.settingRow(c, model, about, func() {
		if a.settingButton(c, "remove model "+kind.id+" "+model, locales.ModelRemove(), false) {
			a.removeModel(kind.id, model)
		}
	})
}

// edit shows the field of the row what, holding value.
func (a *app) edit(what, value string) { a.editing, a.editDraft = what, value }

// editField builds the field of the row being edited, which holds the edit's draft, hidden where secret, and its Cancel and
// Save buttons: it reports what was saved, with Save or Return, which
// ends the edit.
func (a *app) editField(c *ui.Context, placeholder, label string, secret bool) (string, bool) {
	field := ui.TextInput(c.Key(a.editing), &a.editDraft).Width(240).FontSize(13).
		Placeholder(placeholder).Label(label).AutoFocus()
	if secret {
		field.Password()
	}
	ok := strings.TrimSpace(a.editDraft) != ""
	if a.settingButton(c, "cancel "+a.editing, locales.Cancel(), false) {
		a.edit("", "")
		return "", false
	}
	if (a.settingButton(c, "save "+a.editing, locales.KeySave(), !ok) || field.Submitted()) && ok {
		v := a.editDraft
		a.edit("", "")
		return v, true
	}
	return "", false
}

// settingMenuButton builds a button of a settings row, keyed key and
// labeled label, as settingButton does, that opens the system's menu
// build fills.
func (a *app) settingMenuButton(c *ui.Context, key, label string, build func(m *ui.Menu)) {
	t := c.Theme()
	btn := ui.ButtonBase(c.Key(key)).Padding(5, 8, 5, 10).Gap(4).Radius(8).Background(a.pal.composer).
		Border(1, t.Border).AlignItems(ui.Center).Label(label)
	if btn.Hovered() {
		btn.Background(a.pal.fg.Alpha(0.04).Over(a.pal.composer))
	}
	btn.Children(func() {
		ui.Text(c, label).SingleLine()
		ui.Icon(c, chevronIcon).Size(14, 14).TextColor(t.TextMuted)
	})
	btn.Menu(build)
}

// keyEnd returns the last characters of key, which tell it from another
// without showing it.
func keyEnd(key string) string {
	r := []rune(key)
	return string(r[max(0, len(r)-4):])
}
