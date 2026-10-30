package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	copilot "github.com/github/copilot-sdk/go"
	"github.com/github/copilot-sdk/go/rpc"

	"hyperai-desktop/locales"
)

// GitHub Copilot is asked through the Copilot CLI, which GitHub's Copilot
// SDK runs and talks to: the user installs it, and signs in to GitHub,
// through HyperAI's GitHub OAuth App or the CLI's own sign-in. Each reply
// is a session of its own, in the SDK's empty mode, with no tools, told
// the conversation so far in its system message.

// githubClientID is the client ID of HyperAI's GitHub OAuth App, with
// Device Flow on, which signs the user in to GitHub for Copilot: "" until
// it has one, when the Copilot CLI's own sign-in alone serves.
var githubClientID = ""

// githubURL is where GitHub signs users in, and pollUnit what its device
// flow's interval counts: a test server, and milliseconds, in tests.
var (
	githubURL = "https://github.com"
	pollUnit  = time.Second
)

// copilotInstallURL is where the Copilot CLI's installation is told.
const copilotInstallURL = "https://github.com/github/copilot-cli#installation"

// copilotRuntime is what HyperAI asks of GitHub Copilot, through the
// Copilot CLI: copilotSDK, or a fake in tests. Its methods may be called
// on any goroutine.
type copilotRuntime interface {
	// User returns the GitHub login it asks Copilot as, "" where it is
	// signed in as none.
	User(ctx context.Context) (string, error)
	// Models returns the models the user's Copilot offers.
	Models(ctx context.Context) ([]string, error)
	// Stream asks model to reply to prompt, with files, told system, as
	// provider's Stream does.
	Stream(ctx context.Context, model, system, prompt string, files []attachment, delta func(chunk)) error
	// Close stops the CLI, if it runs.
	Close()
}

// copilotState is the app's of GitHub Copilot: the CLI found, "" for none,
// and where it keeps its state for HyperAI's sign-in; newRuntime, which
// makes the runtime asked, rt, for the CLI and a token, "" for the CLI's
// own sign-in; the GitHub login it asks as, once known, and what checking
// it last said; and the sign-in under way, nil for none.
type copilotState struct {
	cli        string
	dir        string
	newRuntime func(cli, dir, token string) copilotRuntime
	rt         copilotRuntime
	user       string
	checking   bool
	err        string
	login      *deviceLogin
}

// copilotSystem is what Copilot is told, in place of the CLI's agent's.
const copilotSystem = "You are a helpful assistant, chatting with the user in HyperAI, a desktop chat app. " +
	"Reply in Markdown."

// copilotPrompt returns what a session of Copilot is told of turns: the
// conversation before the last of them in its system message, and the
// last as its prompt, with the images and PDFs sent with it as files. Text
// files are written in where they were sent; the others sent before, which
// a session is not told again, are named.
func copilotPrompt(turns []turn) (system, prompt string, files []attachment) {
	if len(turns) == 0 {
		return copilotSystem, "", nil
	}
	var b strings.Builder
	b.WriteString(copilotSystem)
	if len(turns) > 1 {
		b.WriteString("\n\nThe conversation so far, before the user's message:")
		for _, t := range turns[:len(turns)-1] {
			who := "User"
			if t.Role == "assistant" {
				who = "Assistant"
			}
			text, _ := copilotText(t)
			b.WriteString("\n\n" + who + ": " + text)
		}
	}
	prompt, files = copilotText(turns[len(turns)-1])
	return b.String(), prompt, files
}

// copilotText returns t as Copilot is told it: its text files written in
// before its text, and its other files apart, each named where it was.
func copilotText(t turn) (text string, files []attachment) {
	var b strings.Builder
	for _, f := range t.Files {
		if f.Type == typeText {
			b.WriteString(fileText(f) + "\n\n")
		} else {
			fmt.Fprintf(&b, "<file name=%q type=%q/>\n\n", f.Name, f.Type)
			files = append(files, f)
		}
	}
	b.WriteString(t.Content)
	return b.String(), files
}

// copilotProvider asks GitHub Copilot through rt.
type copilotProvider struct {
	rt     copilotRuntime
	models []string
}

func (p *copilotProvider) ID() string       { return "copilot" }
func (p *copilotProvider) Name() string     { return "GitHub Copilot" }
func (p *copilotProvider) Models() []string { return p.models }

func (p *copilotProvider) List(ctx context.Context) ([]string, error) { return p.rt.Models(ctx) }

func (p *copilotProvider) Stream(ctx context.Context, model string, turns []turn, delta func(chunk)) error {
	system, prompt, files := copilotPrompt(turns)
	return p.rt.Stream(ctx, model, system, prompt, files, delta)
}

// copilotSDK is the Copilot CLI at cli, which the SDK starts as it is
// first asked, signed in with token, or, for "", the CLI's own sign-in,
// keeping its state in dir.
type copilotSDK struct {
	cli, dir, token string
	mu              sync.Mutex
	client          *copilot.Client
}

func newCopilotSDK(cli, dir, token string) copilotRuntime {
	return &copilotSDK{cli: cli, dir: dir, token: token}
}

// start returns the client, starting the CLI the first time.
func (r *copilotSDK) start(ctx context.Context) (*copilot.Client, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.client != nil {
		return r.client, nil
	}
	opts := &copilot.ClientOptions{
		Connection: copilot.StdioConnection{Path: r.cli, Env: cliEnv(r.cli)},
		// No tools, nor the computer's context: replies, alone.
		Mode:          copilot.ModeEmpty,
		BaseDirectory: r.dir,
		LogLevel:      "error",
		ClientInfo:    &copilot.ClientInfo{ApplicationName: "HyperAI"},
	}
	if r.token != "" {
		opts.GitHubToken = r.token
	} else {
		opts.UseLoggedInUser = copilot.Bool(true)
	}
	c := copilot.NewClient(opts)
	if err := c.Start(ctx); err != nil {
		return nil, fmt.Errorf("starting the Copilot CLI: %w", err)
	}
	r.client = c
	return c, nil
}

func (r *copilotSDK) User(ctx context.Context) (string, error) {
	c, err := r.start(ctx)
	if err != nil {
		return "", err
	}
	st, err := c.GetAuthStatus(ctx)
	if err != nil || !st.IsAuthenticated {
		return "", err
	}
	if st.Login != nil && *st.Login != "" {
		return *st.Login, nil
	}
	return "GitHub", nil
}

func (r *copilotSDK) Models(ctx context.Context) ([]string, error) {
	c, err := r.start(ctx)
	if err != nil {
		return nil, err
	}
	models, err := c.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, m := range models {
		// Those the user's plan or organization has off.
		if m.Policy != nil && m.Policy.State != "" && m.Policy.State != "enabled" {
			continue
		}
		ids = append(ids, m.ID)
	}
	return ids, nil
}

func (r *copilotSDK) Stream(ctx context.Context, model, system, prompt string, files []attachment, delta func(chunk)) error {
	c, err := r.start(ctx)
	if err != nil {
		return err
	}
	s, err := c.CreateSession(ctx, copilotSession(model, system))
	if err != nil {
		return err
	}
	// The session goes with its reply: HyperAI keeps the chat.
	defer func() {
		s.Disconnect()
		c.DeleteSession(context.Background(), s.SessionID)
	}()
	done := make(chan error, 1)
	end := func(err error) {
		select {
		case done <- err:
		default:
		}
	}
	var used usage
	off := s.On(func(ev copilot.SessionEvent) {
		switch d := ev.Data.(type) {
		case *copilot.AssistantMessageDeltaData:
			if d.DeltaContent != "" {
				delta(chunk{Text: d.DeltaContent})
			}
		case *copilot.AssistantUsageData:
			// Of each call to the model, summed: the input with what the
			// cache gave, where it counted that apart.
			in, cached, out := deref(d.InputTokens), deref(d.CacheReadTokens), deref(d.OutputTokens)
			if cached > in {
				in += cached
			}
			used.Input, used.Cached, used.Output = used.Input+in, used.Cached+cached, used.Output+out
			u := used
			delta(chunk{Usage: &u})
		case *copilot.AssistantReasoningDeltaData:
			if d.DeltaContent != "" {
				delta(chunk{Reasoning: d.DeltaContent})
			}
		case *copilot.SessionErrorData:
			end(errors.New(d.Message))
		case *copilot.SessionIdleData:
			end(nil)
		}
	})
	defer off()
	// Images and PDFs as blobs of their bytes, which the CLI takes as the
	// files of the message.
	var blobs []copilot.Attachment
	for _, f := range files {
		data, name := base64.StdEncoding.EncodeToString(f.data), f.Name
		blobs = append(blobs, &copilot.AttachmentBlob{Data: &data, MIMEType: f.Type, DisplayName: &name})
	}
	if _, err := s.Send(ctx, copilot.MessageOptions{Prompt: prompt, Attachments: blobs}); err != nil {
		return err
	}
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		s.Abort(context.Background())
		return ctx.Err()
	}
}

// copilotSession returns the configuration of a session that asks model,
// told system, for a reply: streamed, with no tools, which empty mode wants
// said, as an empty list rather than none; were it to ask for one, it may
// not.
func copilotSession(model, system string) *copilot.SessionConfig {
	return &copilot.SessionConfig{
		ClientName:     "HyperAI",
		Model:          model,
		Streaming:      copilot.Bool(true),
		SystemMessage:  &copilot.SystemMessageConfig{Mode: "replace", Content: system},
		AvailableTools: []string{},
		OnPermissionRequest: func(copilot.PermissionRequest, copilot.PermissionInvocation) (rpc.PermissionDecision, error) {
			return &rpc.PermissionDecisionReject{}, nil
		},
	}
}

func (r *copilotSDK) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.client != nil {
		r.client.Stop()
		r.client = nil
	}
}

// copilotNames are the names the Copilot CLI's executable goes by.
func copilotNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"copilot.exe", "copilot.cmd"}
	}
	return []string{"copilot"}
}

// cliDirs are where the Copilot CLI's installers put it, and node, which
// npm's runs on, beside the PATH's: an app the Finder opens has not the
// shell's PATH.
func cliDirs() []string {
	home, _ := os.UserHomeDir()
	dirs := []string{"/opt/homebrew/bin", "/usr/local/bin",
		filepath.Join(home, ".npm-global", "bin"), filepath.Join(home, ".local", "bin"),
		filepath.Join(home, ".volta", "bin"), filepath.Join(home, ".bun", "bin")}
	if nvm, _ := filepath.Glob(filepath.Join(home, ".nvm", "versions", "node", "*", "bin")); len(nvm) > 0 {
		dirs = append(dirs, nvm...)
	}
	if appData := os.Getenv("APPDATA"); appData != "" {
		dirs = append(dirs, filepath.Join(appData, "npm"))
	}
	return dirs
}

// findCopilotCLI returns where the Copilot CLI is: COPILOT_CLI_PATH, on
// the PATH, or where its installers put it; "" where it is in none.
func findCopilotCLI() string {
	if p := os.Getenv("COPILOT_CLI_PATH"); p != "" {
		return p
	}
	for _, name := range copilotNames() {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	for _, dir := range cliDirs() {
		for _, name := range copilotNames() {
			p := filepath.Join(dir, name)
			if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
				return p
			}
		}
	}
	return ""
}

// cliEnv returns the environment the CLI at cli runs in: the app's, its
// PATH with the CLI's directory and cliDirs, for one npm installed, which
// runs on node.
func cliEnv(cli string) []string {
	path := append([]string{filepath.Dir(cli)}, cliDirs()...)
	if p := os.Getenv("PATH"); p != "" {
		path = append(path, p)
	}
	env := []string{"PATH=" + strings.Join(path, string(os.PathListSeparator))}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(kv), "PATH=") {
			env = append(env, kv)
		}
	}
	return env
}

// deviceLogin is a sign-in to GitHub through its device flow, which the
// user ends in their browser: the code they enter, where, and what HyperAI
// polls by, until it expires.
type deviceLogin struct {
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	DeviceCode      string `json:"device_code"`
	Interval        int    `json:"interval"`
	ExpiresIn       int    `json:"expires_in"`
	cancel          context.CancelFunc
}

// githubPost posts form to GitHub's path, decoding its JSON answer into
// v.
func githubPost(ctx context.Context, path string, form url.Values, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, githubURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return statusError(res)
	}
	return json.NewDecoder(res.Body).Decode(v)
}

// startDeviceLogin asks GitHub for a code the user enters to sign in.
func startDeviceLogin(ctx context.Context) (*deviceLogin, error) {
	var d deviceLogin
	if err := githubPost(ctx, "/login/device/code", url.Values{"client_id": {githubClientID}, "scope": {"read:user"}}, &d); err != nil {
		return nil, err
	}
	if d.DeviceCode == "" {
		return nil, errors.New("GitHub gave no code")
	}
	return &d, nil
}

// wait polls GitHub until the user has entered d's code, returning the
// token it gives, or until they decline, the code expires or ctx ends.
func (d *deviceLogin) wait(ctx context.Context) (string, error) {
	interval := time.Duration(max(d.Interval, 1)) * pollUnit
	ctx, cancel := context.WithTimeout(ctx, time.Duration(max(d.ExpiresIn, 60))*pollUnit)
	defer cancel()
	for {
		if err := sleep(ctx, interval); err != nil {
			return "", err
		}
		var res struct {
			AccessToken string `json:"access_token"`
			Error       string `json:"error"`
			Description string `json:"error_description"`
		}
		form := url.Values{"client_id": {githubClientID}, "device_code": {d.DeviceCode},
			"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}}
		if err := githubPost(ctx, "/login/oauth/access_token", form, &res); err != nil {
			return "", err
		}
		switch res.Error {
		case "":
			return res.AccessToken, nil
		case "authorization_pending":
		case "slow_down":
			interval += 5 * pollUnit
		default:
			if res.Description != "" {
				return "", errors.New(res.Description)
			}
			return "", errors.New(res.Error)
		}
	}
}

// copilotSignedIn reports whether HyperAI asks Copilot as a user: with
// its own sign-in's token, or the CLI's, which the user chose.
func (a *app) copilotSignedIn() bool {
	c := a.confs["copilot"]
	return a.key("copilot") != "" || c != nil && c.On
}

// copilotRuntime returns the runtime Copilot is asked through, made for
// the CLI and the sign-in as they are.
func (a *app) copilotRuntime() copilotRuntime {
	if a.copilot.rt == nil {
		dir := a.copilot.dir
		if a.key("copilot") == "" {
			// The CLI's own, where its sign-in is.
			if home, err := os.UserHomeDir(); err == nil {
				dir = filepath.Join(home, ".copilot")
			}
		}
		a.copilot.rt = a.copilot.newRuntime(a.copilot.cli, dir, a.key("copilot"))
	}
	return a.copilot.rt
}

// resetCopilot stops the runtime, as the CLI or the sign-in changes, for
// the next to be made of them, on a goroutine, as the CLI may take a while
// to stop.
func (a *app) resetCopilot() {
	if rt := a.copilot.rt; rt != nil {
		a.copilot.rt = nil
		go rt.Close()
	}
}

// closeCopilot stops the runtime as the app quits.
func (a *app) closeCopilot() {
	if a.copilot.rt != nil {
		a.copilot.rt.Close()
		a.copilot.rt = nil
	}
}

// background runs f on a goroutine, and then done with what it returned
// where the app posts (app.post), at once in tests.
func background[T any](a *app, f func() (T, error), done func(T, error)) {
	if a.post == nil {
		done(f())
		return
	}
	go func() {
		v, err := f()
		a.post(func() { done(v, err) })
	}()
}

// checkCopilot asks the runtime who it is signed in as, and, with
// cliSignIn, makes the CLI's sign-in the one HyperAI asks as where it is
// signed in.
func (a *app) checkCopilot(cliSignIn bool) {
	if a.copilot.cli == "" {
		return
	}
	if cliSignIn {
		a.resetCopilot()
	}
	rt := a.copilotRuntime()
	a.copilot.checking, a.copilot.err = true, ""
	background(a, func() (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), listTimeout)
		defer cancel()
		return rt.User(ctx)
	}, func(user string, err error) {
		if rt != a.copilot.rt {
			return // of a runtime since stopped
		}
		a.copilot.checking, a.copilot.user = false, user
		switch {
		case err != nil:
			a.copilot.err = err.Error()
		case user == "" && cliSignIn:
			a.copilot.err = locales.CopilotCLISignedOut()
		case user == "" && a.copilotSignedIn():
			a.copilot.err = locales.CopilotSignedOut()
		case cliSignIn:
			a.conf("copilot").On = true
			a.saveConf()
			a.setProviders()
			a.listModels("copilot")
		}
	})
}

// signInCopilot signs in to GitHub through its device flow: it shows the
// code to enter, and once the user has, keeps the token as Copilot's key.
func (a *app) signInCopilot() {
	ctx, cancel := context.WithCancel(context.Background())
	a.copilot.err = ""
	a.copilot.login = &deviceLogin{cancel: cancel}
	login := a.copilot.login
	background(a, func() (*deviceLogin, error) { return startDeviceLogin(ctx) }, func(d *deviceLogin, err error) {
		if a.copilot.login != login {
			return // canceled
		}
		if err != nil {
			a.copilot.login, a.copilot.err = nil, err.Error()
			cancel()
			return
		}
		d.cancel = cancel
		a.copilot.login = d
		background(a, func() (string, error) { return d.wait(ctx) }, func(token string, err error) {
			if a.copilot.login != d {
				return
			}
			a.copilot.login = nil
			cancel()
			if err != nil {
				a.copilot.err = err.Error()
				return
			}
			a.resetCopilot()
			a.setKey("copilot", token)
			a.checkCopilot(false)
		})
	})
}

// cancelCopilotLogin ends the sign-in under way.
func (a *app) cancelCopilotLogin() {
	if d := a.copilot.login; d != nil {
		a.copilot.login = nil
		if d.cancel != nil {
			d.cancel()
		}
	}
}

// signOutCopilot asks Copilot as no one: forgets HyperAI's token, or the
// choice of the CLI's sign-in, which the CLI keeps.
func (a *app) signOutCopilot() {
	a.resetCopilot()
	a.copilot.user, a.copilot.err = "", ""
	if c := a.confs["copilot"]; c != nil && c.On {
		c.On = false
		a.saveConf()
	}
	a.setKey("copilot", "")
}

// findCLI looks for the Copilot CLI again, as after it is installed.
func (a *app) findCLI() {
	if cli := findCopilotCLI(); cli != a.copilot.cli {
		a.resetCopilot()
		a.copilot.cli = cli
		a.setProviders()
		a.listModels("copilot")
	}
}

// copilotRows builds the rows of Copilot's card: where its CLI is, or how
// to install it; and, once it is found, the GitHub account it asks as,
// with the buttons that sign in and out, or the code to sign in with.
func (a *app) copilotRows(c *ui.Context) {
	cli := a.copilot.cli
	about := locales.CopilotCLIMissing()
	if cli != "" {
		about = homeRelative(cli)
	}
	a.settingRow(c, locales.CopilotCLI(), about, func() {
		if cli != "" {
			return
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if a.settingButton(c, "install copilot", locales.CopilotInstall(), false) {
				c.Services().OpenURL(copilotInstallURL)
			}
			if a.settingButton(c, "find copilot", locales.CopilotFind(), false) {
				a.findCLI()
			}
		})
	})
	if cli == "" {
		return
	}
	settingDivider(c)
	login := a.copilot.login
	signedIn := a.copilotSignedIn()
	switch {
	case login != nil && login.UserCode == "":
		about = locales.CopilotChecking()
	case login != nil:
		about = locales.CopilotCode(login.UserCode)
	case a.copilot.checking:
		about = locales.CopilotChecking()
	case a.copilot.err != "":
		about = a.copilot.err
	case signedIn && a.copilot.user != "" && a.key("copilot") == "":
		about = locales.CopilotSignedInCLI(a.copilot.user)
	case signedIn && a.copilot.user != "":
		about = locales.CopilotSignedIn(a.copilot.user)
	case signedIn:
		about = locales.CopilotSignedInUnknown()
	case githubClientID == "":
		about = locales.CopilotSignInCLI()
	default:
		about = locales.CopilotSignInAbout()
	}
	a.settingRow(c, locales.CopilotAccount(), about, func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			switch {
			case login != nil:
				if login.UserCode != "" {
					if a.settingButton(c, "copy copilot code", locales.CopilotCopyCode(), false) {
						c.Services().WriteClipboard(login.UserCode)
					}
					if a.settingButton(c, "open github", locales.CopilotOpenGitHub(), false) {
						c.Services().WriteClipboard(login.UserCode)
						c.Services().OpenURL(login.VerificationURI)
					}
				}
				if a.settingButton(c, "cancel copilot sign-in", locales.Cancel(), false) {
					a.cancelCopilotLogin()
				}
			case signedIn:
				if a.dangerButton(c, "sign out of copilot", locales.CopilotSignOut()) {
					a.askRemove("copilot", true)
				}
			default:
				if a.settingButton(c, "use copilot cli sign-in", locales.CopilotUseCLI(), a.copilot.checking) {
					a.checkCopilot(true)
				}
				if githubClientID != "" && a.settingButton(c, "sign in to copilot", locales.CopilotSignIn(), false) {
					a.signInCopilot()
				}
			}
		})
	})
}

// deref returns what n points at, 0 for nil.
func deref(n *int64) int64 {
	if n == nil {
		return 0
	}
	return *n
}
