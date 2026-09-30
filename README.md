# ora

Start a Windows app by a short, possibly misspelled name, or open any file by name.

```text
ora discord            # launch
ora discrod            # typo, still Discord
ora vscod              # abbreviation of Visual Studio Code
ora "pixel paint"     # multi-word query must be quoted
ora code .             # query "code", "." goes to the app
ora pixel paint -- -v # with --, words before it are the query
ora -- list            # an app named like a subcommand
ora .\notes.txt        # open that file with its default app
ora f notes.cfg        # find a file anywhere and open it
ora f docs notes       # extra words may match the folder
ora alias vsc "Visual Studio Code"  # then `ora vsc` hits that app
ora open discord       # show that program in Explorer, don't launch it
ora open -f notes.cfg  # show that file in its folder
```

A clear winner launches immediately. Several close matches open a picker: ↑/↓ move, Enter launches, `1`–`8` pick a row, Esc cancels. With no terminal, ora prints the rows and exits `2`.

Design, scoring rules and trade-offs: [PLAN.md](PLAN.md).

## Install

Windows 10/11 (amd64, arm64 or 386). Building requires Go 1.26+.

```powershell
go install ./cmd/ora     # -> %USERPROFILE%\go\bin\ora.exe
ora index                # first full index (Start Menu, registry, AppsFolder, Everything)
```

Portable programs and `ora file` use [Everything](https://www.voidtools.com/) (1.4.1+ or 1.5). If it is installed but not running, ora starts it in the background and tells you so on stderr (turn off with `everything.autostart: false`). Without Everything, the index is built from Start Menu, the registry, and Store apps, and `ora file` is unavailable.

The index is built from Start Menu shortcuts, the uninstall registry, Store apps, and portable `exe`, `ahk`, `cmd`, and `bat` files found by Everything (`everything.extensions`). Documents are not indexed; `ora file` searches them live instead.

Shell completion for the current PowerShell profile:

```powershell
ora completion powershell >> $PROFILE
```

Open a new window after that. `ora completion` also prints bash, zsh, and fish.

## Commands

| Command | |
|---|---|
| `ora <query> [args...]` | match and launch; an existing file path is opened with its default app |
| `ora file <query...>` / `ora f` | find any file by name via Everything and open it; several copies open the picker; `--which` only shows the result |
| `ora index` | rebuild the cache (all sources) |
| `ora list` | print the index; in a terminal paged 20 rows at a time (←/→, Home/End, q), piped output is the full list |
| `ora which <query>` | show winner, score, target and decision without launching |
| `ora alias <shortcut> <official name>` | add an alias to `config.yaml` |
| `ora open <query...>` | show the match in Explorer instead of launching it; `-f` searches files like `ora file` |

Flags before the query: `--refresh`, `--min-score`, `-v`.

Exit codes: `0` launched, `1` no match / cancelled, `2` ambiguous without TTY, `3` launch failed, `4` other error.

## Files

- `%APPDATA%\ora\config.yaml` – settings and aliases (defaults in PLAN.md)
- `%APPDATA%\ora\recent.json` – last 20 app launches (files are not recorded)
- `%LOCALAPPDATA%\ora\index.json` – index cache, 24 h TTL

## License

[MIT](LICENSE).

## Development

```powershell
go vet ./...
go test ./...
```

Unit tests use fixtures and temp directories only (no live COM, Everything, registry or user profile), so they pass on any Windows machine.
