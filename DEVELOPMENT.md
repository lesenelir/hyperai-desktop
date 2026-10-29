# Development

See [docs/design.md](docs/design.md) for the design rules.

## Running

```sh
go tool mygo dev
```

No setup is needed. The development app (identifier `com.example.hyperaidesktop.dev`) keeps its data apart from the installed app's, on macOS in:

```
~/Library/Application Support/hyperai-desktop Dev/
```

Delete that folder to start from a clean state.

## API keys

A development build keeps the API keys **unencrypted** in `dev-keys.json` in that folder (mode 0600), not in the Keychain; a release build (`mygo build`) keeps them in the system's credential store.

This is because `mygo dev` signs ad hoc, so every rebuild has a new signature. The macOS Keychain treats each build as a different app, and asks for your password every time.

Use test keys with a spending limit while developing, and don't share `dev-keys.json`.

### Optional: trying the Keychain in development

Only needed when changing the code that uses the Keychain:

```sh
HYPERAI_DEV_KEYCHAIN=1 go tool mygo dev -sign "HyperAI Dev"
```

`-sign` is required, with a code signing certificate that stays the same, or every rebuild asks again. To create one (once per Mac):

1. Open Keychain Access and select **login** in the sidebar.
2. In the menu bar, choose **Keychain Access → Certificate Assistant → Create a Certificate…**
3. Set Name to `HyperAI Dev`, Identity Type to `Self-Signed Root` and Certificate Type to `Code Signing`, then click **Create → Continue → Done**.
4. Check it with `security find-identity -p codesigning`: `"HyperAI Dev"` should be listed. `CSSMERR_TP_NOT_TRUSTED` doesn't matter; don't add `-v`, which leaves the certificate out.

On the first launch, click **Always Allow** in both prompts (`codesign`'s, and the app's reading the keys); they won't show again. The certificate is for local development only, not for releases.

## Testing

```sh
go vet ./...
go test ./...
go test -race ./...    # required for changes to streaming or concurrency
GOOS=linux go vet ./... && GOOS=windows go vet ./...
```

Tests use `demo`, in `bench.go`, in place of a real provider. To test replies streaming in, replace `a.post` with a queue the test drains, as `TestChatsStreamAtOnce` does.

## Strings

The app's strings are in `locales/*.json`, with `en.json` the source language, where each string has a description under its `@` key. Add or change a string in all six languages (en, zh-Hans, ja, ko, fr, de), then run:

```sh
go tool mygo generate
```

Don't edit `locales/catalog_gen.go`, which it generates.
