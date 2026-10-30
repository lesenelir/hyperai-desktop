package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"math/rand/v2"
	"net"
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

// devKeysEnv, set, has a development build keep the keys in the
// credential store, as the app does, to try that (DEVELOPMENT.md).
const devKeysEnv = "HYPERAI_DEV_KEYCHAIN"

// appKeyring returns where the app keeps the API keys: the system's
// credential store, but for a development build a file of the development
// app's data directory, which file names, unless devKeysEnv is set. A
// rebuilt app is another to the macOS Keychain, which would ask to let
// each build read the keys: contributors would have to sign their builds
// (DEVELOPMENT.md).
func appKeyring() (k keyring, file string) {
	if !mygo.IsDev() || os.Getenv(devKeysEnv) != "" {
		return mygo.Secrets, ""
	}
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return mygo.Secrets, ""
	}
	return devKeyring(dir)
}

// devKeyring returns where a development build keeps the keys, its data
// directory being dir: the file there, unless devKeysEnv is set.
func devKeyring(dir string) (k keyring, file string) {
	if os.Getenv(devKeysEnv) != "" {
		return mygo.Secrets, ""
	}
	file = filepath.Join(dir, "dev-keys.json")
	return fileKeyring{file}, file
}

// fileKeyring keeps secrets in the file at path, as JSON by key, which
// the user alone reads and writes: unencrypted, for development only.
type fileKeyring struct{ path string }

func (k fileKeyring) read() (map[string]string, error) {
	b, err := os.ReadFile(k.path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	secrets := map[string]string{}
	if err := json.Unmarshal(b, &secrets); err != nil {
		return nil, err
	}
	return secrets, nil
}

// write writes secrets whole, or none, through a file beside it.
func (k fileKeyring) write(secrets map[string]string) error {
	b, err := json.MarshalIndent(secrets, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(k.path), 0o700); err != nil {
		return err
	}
	tmp := k.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, k.path)
}

func (k fileKeyring) Get(key string) ([]byte, error) {
	secrets, err := k.read()
	if err != nil {
		return nil, err
	}
	v, ok := secrets[key]
	if !ok {
		return nil, mygo.ErrSecretNotFound
	}
	return []byte(v), nil
}

func (k fileKeyring) Set(key string, value []byte) error {
	secrets, err := k.read()
	if err != nil {
		return err
	}
	secrets[key] = string(value)
	return k.write(secrets)
}

func (k fileKeyring) Delete(key string) error {
	secrets, err := k.read()
	if err != nil {
		return err
	}
	if _, ok := secrets[key]; !ok {
		return nil
	}
	delete(secrets, key)
	return k.write(secrets)
}

// providerConf is what the user set of a provider but its key, as
// providers.json keeps it: its endpoint, "" for its kind's; the models the
// composer offers, which are Models where Custom, the user having added
// or removed one, and its kind's otherwise, as many of them as it lists;
// Listed, the models it listed last that chat; and, of a server on the
// user's computer, whether they turned it on.
type providerConf struct {
	BaseURL string   `json:"baseURL,omitempty"`
	Custom  bool     `json:"custom,omitempty"`
	Models  []string `json:"models,omitempty"`
	Listed  []string `json:"listed,omitempty"`
	On      bool     `json:"on,omitempty"`
}

// customProvider is a provider the user added, as providers.json keeps
// it: its ID, as its key and settings are kept by, its name and the API
// it takes. Its endpoint is its providerConf's.
type customProvider struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	API  string `json:"api"`
}

// customAPIs are the APIs a provider the user adds may take: OpenAI's
// chat completions, or Anthropic's Messages.
var customAPIs = []string{"openai", "anthropic"}

// customKind returns the kind of the provider the user added, id, named
// name: one that takes api (customAPIs), OpenAI's for one it does not
// know, at the endpoint the user gives, with a key or without, and offers
// the models it lists until the user chooses.
func customKind(id, name, api string) *providerKind {
	if api == "anthropic" {
		return &providerKind{id: id, name: name, api: api, open: openAnthropic}
	}
	return &providerKind{id: id, name: name, api: "openai", open: openChatCompletions}
}

// apiName returns what the Providers page calls api, of customAPIs.
func apiName(api string) string {
	if api == "anthropic" {
		return locales.ProviderAnthropic()
	}
	return locales.ProviderOpenAI()
}

// apiAbout returns what the Providers page says of api, of customAPIs.
func apiAbout(api string) string {
	if api == "anthropic" {
		return locales.ProviderAnthropicAbout()
	}
	return locales.ProviderOpenAIAbout()
}

// added reports whether the user added kind, which has no endpoint until
// they give it.
func (k *providerKind) added() bool { return k.api != "" }

// modelRef names a model of a provider, by the provider's ID.
type modelRef struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// providersFile is what providers.json holds, beside the chats: the
// model a new chat asks, each provider's settings, by its ID, and the
// providers the user added, in order.
type providersFile struct {
	Default   modelRef                 `json:"default,omitzero"`
	Providers map[string]*providerConf `json:"providers,omitempty"`
	Custom    []customProvider         `json:"custom,omitempty"`
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
	a.copilot.dir = filepath.Join(dir, "copilot")
	a.custom = nil
	for _, p := range f.Custom {
		a.custom = append(a.custom, customKind(p.ID, p.Name, p.API))
	}
}

// saveConf keeps the providers' settings, where the app has a place for
// them (not in tests), on a goroutine, as save does the chats.
func (a *app) saveConf() {
	if a.confPath == "" {
		return
	}
	f := providersFile{Default: a.defaultModel, Providers: a.confs}
	for _, kind := range a.custom {
		f.Custom = append(f.Custom, customProvider{kind.id, kind.name, kind.api})
	}
	b, err := json.MarshalIndent(f, "", "  ")
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

// kinds returns the kinds of provider the Providers page and the
// composer's menu list, in that order: HyperAI's, then the user's.
func (a *app) kinds() []*providerKind {
	kinds := make([]*providerKind, 0, len(providerKinds)+len(a.custom))
	for i := range providerKinds {
		kinds = append(kinds, &providerKinds[i])
	}
	return append(kinds, a.custom...)
}

// kindOf returns the kind of the provider id, HyperAI's or the user's,
// nil for none.
func (a *app) kindOf(id string) *providerKind {
	if kind := providerKindOf(id); kind != nil {
		return kind
	}
	for _, kind := range a.custom {
		if kind.id == id {
			return kind
		}
	}
	return nil
}

// ready reports whether the provider of kind can be asked: it has a key;
// of those the user added, which a local server may serve without one, an
// endpoint; or, of a server on the user's computer, the user turned it on.
func (a *app) ready(kind *providerKind) bool {
	switch {
	case kind.added():
		return a.endpoint(kind) != ""
	case kind.local:
		c := a.confs[kind.id]
		return c != nil && c.On
	case kind.copilot:
		return a.copilot.cli != "" && a.copilotSignedIn()
	}
	return a.key(kind.id) != ""
}

// openKind returns the provider of kind at url, for key, offering models.
func (a *app) openKind(kind *providerKind, url, key string, models []string) provider {
	if kind.copilot {
		return &copilotProvider{rt: a.copilotRuntime(), models: models}
	}
	return kind.open(kind, url, key, models)
}

// setOn turns on, or off, the server on the user's computer id, and lists
// the models it runs once it is on.
func (a *app) setOn(id string, on bool) {
	a.conf(id).On = on
	a.saveConf()
	delete(a.listErrors, id)
	a.setProviders()
	a.listModels(id)
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
	case len(kind.models) == 0:
		// Of a provider the user added, or a server on their computer,
		// which have none of their own.
		return c.Listed
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
// models of the one keyed, which tells too whether the key serves, and of
// one the user added, which may serve without. Where
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
	a.listModels(id)
}

// setEndpoint makes url where the API of the provider id is, its own for
// "", and lists its models there.
func (a *app) setEndpoint(id, url string) {
	url = strings.TrimRight(strings.TrimSpace(url), "/")
	if kind := a.kindOf(id); kind != nil && url == kind.baseURL {
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
	shown := a.shownModels(a.kindOf(id))
	if model == "" || slices.Contains(shown, model) {
		return
	}
	a.setShown(id, append(slices.Clone(shown), model))
}

// removeModel offers model of the provider id in the composer no more.
func (a *app) removeModel(id, model string) {
	shown := a.shownModels(a.kindOf(id))
	a.setShown(id, slices.DeleteFunc(slices.Clone(shown), func(m string) bool { return m == model }))
}

// listTimeout is how long listing a provider's models takes at most.
const listTimeout = 20 * time.Second

// listModels asks the provider id for its models, on a goroutine where
// the app posts (app.post), and keeps those that chat, by name, for the
// Providers page to add of. The composer offers no more of its own models
// than it lists.
func (a *app) listModels(id string) {
	kind := a.kindOf(id)
	if kind == nil || !a.ready(kind) {
		return
	}
	// Opened of its own, as the composer offers none of its models where
	// the user removed them all.
	key, url := a.key(id), a.endpoint(kind)
	p := a.openKind(kind, url, key, nil)
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
			// Where nothing answers, as a server not running.
			var dial *net.OpError
			if kind.local && errors.As(err, &dial) && dial.Op == "dial" {
				a.listErrors[id] = locales.LocalUnreachable(url, kind.name)
			}
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

// listUnlisted lists the models of the providers that can be asked that
// have listed none yet, as the app starts.
func (a *app) listUnlisted() {
	for _, kind := range a.kinds() {
		if c := a.confs[kind.id]; c == nil || len(c.Listed) == 0 {
			a.listModels(kind.id)
		}
	}
}

// setProviders makes the providers that can be asked and have a model
// those the composer asks, the fallback where none is. The model chosen stays where its
// provider offers it still; otherwise the chat's is chosen, or the
// default.
func (a *app) setProviders() {
	var provs []provider
	for _, kind := range a.kinds() {
		if models := a.shownModels(kind); a.ready(kind) && len(models) > 0 {
			provs = append(provs, a.openKind(kind, a.endpoint(kind), a.key(kind.id), models))
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
	for _, kind := range a.kinds() {
		out = append(out, settingsTerm{kind.name, terms(locales.ProviderTerms())})
	}
	return out
}

// providersPage builds the Providers page: a card of each provider, which
// takes its API key, or, of a server on the user's computer, turns it on,
// and then shows where its API is and the models the composer offers of
// it; the card of each the user added, which takes where its API is
// first, and a key, which it may need none of; the button that adds
// another; and where the keys are kept.
func (a *app) providersPage(c *ui.Context) {
	for _, kind := range a.kinds() {
		a.settingsCard(c, kind.name, func() {
			switch {
			case kind.added():
				a.addedRow(c, kind)
				settingDivider(c)
				a.endpointRow(c, kind)
				settingDivider(c)
				a.keyRow(c, kind)
			case kind.local:
				a.localRow(c, kind)
			case kind.copilot:
				a.copilotRows(c)
			default:
				a.keyRow(c, kind)
			}
			if !a.ready(kind) {
				return
			}
			if !kind.added() && !kind.copilot {
				settingDivider(c)
				a.endpointRow(c, kind)
			}
			settingDivider(c)
			a.modelsRow(c, kind)
			for _, model := range a.shownModels(kind) {
				settingDivider(c)
				a.modelRow(c, kind, model)
			}
		})
	}
	a.addProviderCard(c)
	note := locales.KeysKept(platform())
	if a.keysFile != "" {
		note = locales.KeysKeptFile(platform(), homeRelative(a.keysFile))
	}
	if a.keyError != "" {
		note = a.keyError
	}
	ui.Text(c, note).FontSize(12).LineHeight(1.5).TextColor(c.Theme().TextMuted).Padding(0, 4)
}

// localRow builds the row that turns on the server on the user's
// computer of kind, with, while it is off, the button that opens where it
// is downloaded.
func (a *app) localRow(c *ui.Context, kind *providerKind) {
	on := a.ready(kind)
	about := locales.LocalOff()
	if on {
		about = locales.LocalOn()
	}
	a.settingRow(c, locales.LocalRow(), about, func() {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			if !on && a.settingButton(c, "download "+kind.id, locales.LocalDownload(), false) {
				c.Services().OpenURL(kind.keyURL)
			}
			if ui.Switch(c.Key("local "+kind.id), &on).Label(locales.LocalSwitch(kind.name)).Changed() {
				a.setOn(kind.id, on)
			}
		})
	})
}

// addProviderCard builds the button whose menu adds a provider of the
// user's, of the API chosen, or, once one is, the card of the field that
// takes its name.
func (a *app) addProviderCard(c *ui.Context) {
	api, adding := strings.CutPrefix(a.editing, "add provider ")
	if !adding {
		ui.Row(c).Children(func() {
			a.settingMenuButton(c, "add provider", locales.ProviderAdd(), func(m *ui.Menu) {
				for _, api := range customAPIs {
					if m.Item(apiName(api)).Chosen() {
						a.edit("add provider "+api, "")
					}
				}
			})
		})
		return
	}
	a.settingsCard(c, locales.ProviderNew(apiName(api)), func() {
		a.settingRow(c, locales.ProviderName(), locales.ProviderNameAbout(), func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if v, ok := a.editField(c, locales.ProviderName(), locales.ProviderName(), false); ok {
					a.addProvider(v, api)
				}
			})
		})
	})
}

// addedRow builds the first row of the card of kind, which the user
// added: what API it takes, and the buttons that rename and remove it,
// or, while it is being renamed, the field that takes its name.
func (a *app) addedRow(c *ui.Context, kind *providerKind) {
	a.settingRow(c, apiName(kind.api), apiAbout(kind.api), func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if a.editing == "name "+kind.id {
				if v, ok := a.editField(c, locales.ProviderName(), locales.ProviderName(), false); ok {
					a.renameProvider(kind.id, v)
				}
				return
			}
			if a.settingButton(c, "rename provider "+kind.id, locales.ProviderRename(), false) {
				a.edit("name "+kind.id, kind.name)
			}
			if a.dangerButton(c, "remove provider "+kind.id, locales.ProviderRemove()) {
				a.askRemove(kind.id, false)
			}
		})
	})
}

// addProvider adds a provider of the user's named name, last, which
// takes api (customAPIs), and shows the field of its endpoint, which it
// needs first.
func (a *app) addProvider(name, api string) {
	id := ""
	for id == "" || a.kindOf(id) != nil {
		id = fmt.Sprintf("custom-%08x", rand.Uint32())
	}
	a.custom = append(a.custom, customKind(id, strings.TrimSpace(name), api))
	a.saveConf()
	a.edit("endpoint "+id, "")
}

// renameProvider names name the provider id the user added.
func (a *app) renameProvider(id, name string) {
	a.kindOf(id).name = strings.TrimSpace(name)
	a.saveConf()
	a.setProviders()
}

// removeProvider removes the provider id the user added, with its
// settings and its key; a new chat asks another where it asked it.
func (a *app) removeProvider(id string) {
	a.custom = slices.DeleteFunc(a.custom, func(kind *providerKind) bool { return kind.id == id })
	delete(a.confs, id)
	delete(a.listing, id)
	delete(a.listErrors, id)
	if a.defaultModel.Provider == id {
		a.defaultModel = modelRef{}
	}
	a.saveConf()
	if a.key(id) != "" {
		a.setKey(id, "")
		return
	}
	a.setProviders()
}

// keyRow builds the row of kind's API key: whether it has one, and the
// buttons that add, change and remove it, or, while it is being added or
// changed, the field that takes it.
func (a *app) keyRow(c *ui.Context, kind *providerKind) {
	about := locales.KeyNone()
	if kind.added() {
		about = locales.KeyOptional()
	}
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
				if a.settingButton(c, "change key "+kind.id, locales.KeyChange(), false) {
					a.edit("key "+kind.id, "")
				}
				if a.dangerButton(c, "remove key "+kind.id, locales.KeyRemove()) {
					a.askRemove(kind.id, true)
				}
				return
			}
			if kind.keyURL != "" && a.settingButton(c, "get key "+kind.id, locales.KeyGet(), false) {
				c.Services().OpenURL(kind.keyURL)
			}
			if a.settingButton(c, "add key "+kind.id, locales.KeyAdd(), false) {
				a.edit("key "+kind.id, "")
			}
		})
	})
}

// endpointRow builds the row of where kind's API is, which a proxy or a
// compatible service changes, and which the user gives of one they added.
func (a *app) endpointRow(c *ui.Context, kind *providerKind) {
	url := a.endpoint(kind)
	changed := !kind.added() && url != kind.baseURL
	about := locales.EndpointDefault(url)
	switch {
	case url == "":
		about = locales.EndpointNone()
	case changed || kind.added():
		about = url
	}
	a.settingRow(c, locales.Endpoint(), about, func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if a.editing == "endpoint "+kind.id {
				placeholder := kind.baseURL
				switch kind.api {
				case "openai":
					placeholder = "http://localhost:8000/v1"
				case "anthropic":
					placeholder = "https://api.example.com/anthropic"
				}
				if v, ok := a.editField(c, placeholder, locales.EndpointLabel(kind.name), false); ok {
					a.setEndpoint(kind.id, v)
				}
				return
			}
			if changed && a.settingButton(c, "reset endpoint "+kind.id, locales.EndpointReset(), false) {
				a.setEndpoint(kind.id, "")
			}
			if url == "" {
				if a.settingButton(c, "add endpoint "+kind.id, locales.EndpointAdd(), false) {
					a.edit("endpoint "+kind.id, "")
				}
				return
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
