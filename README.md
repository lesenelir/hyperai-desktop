<p align="center">
  <img src="resources/logo.svg" width="96" alt="HyperAI logo">
</p>

<h1 align="center">HyperAI</h1>

<p align="center">A desktop AI chat client with a native UI, written in Go.</p>

---

> [!WARNING]
> This is **not** the official HyperAI Desktop. It is still in development, and anything may change at any time.

HyperAI is a desktop app for [HyperAI](https://hyper.ai), the AI/ML resource site. It's built with [MyGo](https://github.com/egoist/mygo), so the UI is native rather than a web view.

## Status and roadmap

Only the first piece is done so far: a basic chat client. Next:

- **From chat to agent**: the chat will become an agent that can use tools and do multi-step work, not only answer.
- **HyperAI's content**: bring in what the HyperAI website offers, such as resources, news, datasets and notebooks, so you can browse them in the app and the agent can find and use them.
- **Working with Console and your workspace**: HyperAI Desktop will work together with HyperAI Console and with the Jupyter workspace inside your container.

Plans may change as development goes on.

## Features (current chat)

HyperAI keeps your chats in a sidebar, streams replies in as Markdown, and asks whichever model you choose.


- **Many providers**: OpenAI, Anthropic, Google, OpenRouter and DeepSeek with your own API key; Ollama and LM Studio running on your computer; GitHub Copilot through the Copilot CLI; and any other service that offers an OpenAI- or Anthropic-compatible API.
- **Rich replies**: Markdown, highlighted code, and TeX math rendered by [RaTeX](https://github.com/erweixin/RaTeX). A model's reasoning is shown folded above its reply.
- **Thinking effort**: choose how hard a model thinks, for models that support it.
- **Attachments**: pick, drag in or paste files.
- **Versions**: edit a question or ask for a reply again, and every version is kept.
- **Chat management**: chats are titled for you automatically, and you can pin, rename and search them.
- **Export**: save a chat as a PDF, a web page or JSON.
- **Custom instructions**: write instructions that every model is given.
- **Usage**: see each reply's tokens, how long it took and how fast it streamed.
- **Desktop touches**: a menu bar icon, a choice of Dock icons, light and dark themes, open at login, and keyboard shortcuts.
- **Six languages**: English, 简体中文, 日本語, 한국어, Français and Deutsch.

API keys are stored in the system's credential store (the Keychain on macOS).

## Getting started

You need Go (see `go.mod` for the version). Then:

```sh
git clone https://github.com/lesenelir/hyperai-desktop.git
cd hyperai-desktop
go tool mygo dev      # run a development build
go tool mygo build    # make a release build in build/
```

On first launch, open the **Providers** page and add an API key, or turn on a local server.

## Development

See [DEVELOPMENT.md](DEVELOPMENT.md) for running, testing and translating, and [docs/design.md](docs/design.md) for the design rules.
