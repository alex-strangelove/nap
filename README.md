# Nap

<img width="1200" alt="Nap" src="https://user-images.githubusercontent.com/42545625/202545409-eb53f92a-233a-4f78-b598-a59c65248ad3.png">

<sub><sub>z</sub></sub><sub>z</sub>z

Nap is a code snippet manager for your terminal. Create and access new snippets
quickly with the command-line interface or browse, manage, and organize them with the
text-user interface. Keep your code snippets safe, sound, and well-rested in your terminal.

<br />

<p align="center">
<img width="1000" src="https://user-images.githubusercontent.com/42545625/202577549-f2e0887a-b740-41f4-9408-c2f53673503f.gif" />
</p>

<br />

## Text-based User Interface

Launch the interactive interface:

```bash
nap
```

<img width="1000" src="https://user-images.githubusercontent.com/42545625/202768989-caf2ab62-b69d-4e2d-ac93-1517eab7f2ad.gif" />

<details>

<summary>Key Bindings</summary>

<br />

| Action | Key |
| :--- | :--- |
| Create a new snippet | <kbd>n</kbd> |
| Create `00-nap-cards.md` in the selected folder | <kbd>f</kbd> |
| Append a draft card from the selected snippet | <kbd>g</kbd> |
| Review the selected folder's flashcards | <kbd>F</kbd> |
| Edit selected snippet (in `$EDITOR`) | <kbd>e</kbd> |
| Copy selected snippet to clipboard | <kbd>c</kbd> |
| Paste clipboard to selected snippet | <kbd>p</kbd> |
| Delete selected snippet | <kbd>x</kbd> |
| Rename selected snippet | <kbd>r</kbd> |
| Set folder of selected snippet | <kbd>R</kbd> |
| Set language of selected snippet | <kbd>L</kbd> |
| Move to next pane | <kbd>l</kbd> |
| Move to previous pane | <kbd>h</kbd> |
| Find in the current file and highlight matches in preview | <kbd>/</kbd> |
| Search folders and file names | <kbd>S</kbd> |
| Search file contents globally | <kbd>s</kbd> |
| Move to the next search result or occurrence | <kbd>ctrl+j</kbd> |
| Move to the previous search result or occurrence | <kbd>ctrl+k</kbd> |
| Focus search results during content search | <kbd>ctrl+h</kbd> |
| Focus preview during content search | <kbd>ctrl+l</kbd> |
| Open the selected search match in Helix | <kbd>ctrl+e</kbd> |
| Toggle help | <kbd>?</kbd> |
| Quit application | <kbd>ctrl+c</kbd> |

</details>

## Command Line Interface

Create new snippets:

```bash
# Quick save an untitled snippet.
nap < main.go

# From a file, specify Notes/ folder and Go language.
nap Notes/FizzBuzz.go < main.go

# Save some code from the internet for later.
curl https://example.com/main.go | nap Notes/FizzBuzz.go

# Works great with GitHub gists
gh gist view 4ff8a6472247e6dd2315fd4038926522 | nap
```

<img width="600" src="https://user-images.githubusercontent.com/42545625/202767159-134d679f-490f-4ad2-8875-cda604aa7b13.gif" />

Output saved snippets:

```bash
# Fuzzy find a snippet by path or file contents.
nap fuzzy

# Write snippet to a file.
nap go/boilerplate > main.go

# Copy snippet to clipboard.
nap foobar | pbcopy
nap foobar | xclip
```

<img width="600" src="https://user-images.githubusercontent.com/42545625/202240249-d724fd73-2f90-4036-b9fc-6d2ccef982b3.gif" />

List snippets:

```bash
nap list
```
<img width="600" src="https://user-images.githubusercontent.com/42545625/202242653-1696dda6-2527-4c38-b673-74d67ad1517f.gif" />

Fuzzy find a snippet (with [Gum](https://github.com/charmbracelet/gum)).

```bash
nap $(nap list | gum filter)
```

<img width="600" src="https://user-images.githubusercontent.com/42545625/202240268-3a71fde6-73c3-4b0a-b129-f87ec1bb1b88.gif" />

## Installation

<!--

Use a package manager:

```bash
# macOS
brew install nap

# Arch
yay -S nap

# Nix
nix-env -iA nixpkgs.nap
```

-->

Install with Go:

```sh
go install github.com/alex-strangelove/nap/cmd/nap@main
```

Nap now requires **Go 1.24+**.

Build from source on Debian/Ubuntu:

```sh
make apt-deps
make build
```

`make apt-deps` installs the distro `golang-go` package. Make sure the installed Go toolchain is **1.24+**.

Or download a binary from the [releases](https://github.com/maaslalani/nap/releases).


## Customization

Nap is customized through a configuration file located at `NAP_CONFIG` (`$XDG_CONFIG_HOME/nap/config.yaml`).

```yaml
# Configuration
home: ~/.nap
default_language: go
flashcards_enabled: true
markdown_style: auto
theme: nord

# Colors
background: "0"
foreground: "7"
primary_color: "#AFBEE1"
primary_color_subdued: "#64708D"
green: "#527251"
bright_green: "#BCE1AF"
bright_red: "#E49393"
red: "#A46060"
black: "#373B41"
gray: "240"
white: "#FFFFFF"
search_highlight_color: "#FF8700"
```

The configuration file can be overridden through environment variables:

```bash
# Configuration
export NAP_CONFIG="~/.nap/config.yaml"
export NAP_HOME="~/.nap"
export NAP_DEFAULT_LANGUAGE="go"
export NAP_FLASHCARDS_ENABLED="false"
export NAP_MARKDOWN_STYLE="tokyo-night"
export NAP_THEME="nord"

# Colors
export NAP_PRIMARY_COLOR="#AFBEE1"
export NAP_RED="#A46060"
export NAP_GREEN="#527251"
export NAP_FOREGROUND="7"
export NAP_BACKGROUND="0"
export NAP_BLACK="#373B41"
export NAP_GRAY="240"
export NAP_WHITE="#FFFFFF"
```

Flashcards are enabled by default.

- Use <kbd>f</kbd> to scaffold a native Nap deck at `00-nap-cards.md`.
- Use <kbd>g</kbd> on a normal snippet to append a draft basic card to the folder's deck and jump to `00-nap-cards.md` for editing.
- Use <kbd>F</kbd> to review the selected folder's flashcards.
- Native `00-nap-cards.md` decks are reviewed inside Nap and store progress in a hidden sidecar state file next to the deck.
- Use <kbd>z</kbd> to reset Napcards progress for the selected folder.

Set `flashcards_enabled: false` or `NAP_FLASHCARDS_ENABLED=false` to turn the integration off. See [`docs/ROADMAP.md`](docs/ROADMAP.md) for the shipped foundation and planned next phases.

Markdown previews are rendered internally with Glamour. When `markdown_style` is `auto`, Nap derives `light` or `dark` from the configured Chroma `theme` so Markdown preview stays aligned with the rest of the app. Set `markdown_style` or `NAP_MARKDOWN_STYLE` to a built-in Glamour style such as `auto`, `dark`, `light`, `dracula`, or `tokyo-night` to override that behavior.

<br />

<p align="center">
  <img
    width="1000"
    alt="image"
    src="https://user-images.githubusercontent.com/42545625/202867429-5bcf8fae-5dd7-478c-b958-638aa5765d97.png"
  />
</p>

## License

[MIT](https://github.com/maaslalani/nap/blob/master/LICENSE)

## Feedback

I'd love to hear your feedback on improving `nap`.

Feel free to reach out via:
* [Email](mailto:maas@lalani.dev) 
* [Twitter](https://twitter.com/maaslalani)
* [GitHub issues](https://github.com/maaslalani/nap/issues/new)

---

<sub><sub>z</sub></sub><sub>z</sub>z
