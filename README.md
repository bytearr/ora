# ora

Start an app by a short, possibly misspelled name, or open any file by name. Windows and Linux.

![ora opening Notepad](docs/demo.gif)

The same program is the search window and the terminal command. Left Win and left Alt open the window (on Linux: the shortcut you bind, see [Window on Linux](#window-on-linux)). Enter launches, Esc hides it.

## Examples

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
ora open E:\Tools      # open that folder
ora open -f Tools      # a folder match is opened, a file match is selected
```

A clear winner launches immediately. Several close matches open a picker: ↑/↓ move, Enter launches, `1`–`8` pick a row, Esc cancels. With no terminal, ora prints the rows and exits `2`.

## Window

`ora` without arguments opens the search window and returns; `ora --help` stays the help. The window keeps running in the background: the hotkey shows it, Esc hides it, Ctrl+Q quits it. The default hotkey is `LWin & LAlt` (left Win plus left Alt, nothing else).

`%APPDATA%\ora\config.yaml` takes four window settings, read once when the window starts:

```yaml
settings:
  hotkey: "LWin & LAlt"   # AutoHotkey v2 notation, e.g. "^!Space" or "<#a"
  monitor: primary        # or cursor
  theme: dark             # or light
  autostart: false        # true starts the window hidden when you sign in
```

`autostart: true` keeps an `ora` entry under `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` that runs `ora --autostart` through `conhost.exe --headless`, so no console flashes at sign-in. The window checks it at start and each time it opens, so a change needs no restart; it rewrites the entry if `ora.exe` moved, and `false` removes it.

### Window on Linux

The window is GTK 4. On compositors with wlr-layer-shell (KDE Plasma, sway, Hyprland, niri, COSMIC, ...) it is an overlay placed like on Windows; a click beside it hides it. On GNOME and X11 it is a normal undecorated window that hides when it loses focus.

Wayland lets no app grab keys itself, so `settings.hotkey` is offered to the desktop through the GlobalShortcuts portal (`xdg-desktop-portal`), which may ask you to confirm or pick another key; a two-key combo such as `LWin & LAlt` has no portal form, so the desktop asks for one. Where the portal is missing, bind a desktop shortcut to the command `ora`: a second `ora` shows the running window, a third hides it.

`monitor: primary` uses the first monitor, `cursor` the output that has the focus. `theme` and the accent color follow the settings above and the desktop's accent color. `autostart: true` writes `$XDG_CONFIG_HOME/autostart/io.github.bytearr.ora.desktop` (`false` removes it); the first start also adds an app entry to `~/.local/share/applications` so the desktop can find ora for the portal.

## Install

### Windows

Windows 10/11 (amd64, arm64 or 386). Building requires Go 1.26+.

```powershell
go install .             # -> %USERPROFILE%\go\bin\ora.exe
ora index                # first full index (Start Menu, registry, AppsFolder, Everything)
```

Portable programs and `ora file` use [Everything](https://www.voidtools.com/) (1.4.1+ or 1.5). If it is installed but not running, ora starts it in the background and tells you so on stderr (turn off with `everything.autostart: false`). Without Everything, the index is built from Start Menu, the registry, and Store apps, and `ora file` is unavailable.

The index is built from Start Menu shortcuts, the uninstall registry, Store apps, and portable `exe`, `ahk`, `cmd`, and `bat` files found by Everything (`everything.extensions`). Documents are not indexed; `ora file` searches them live instead.

### Linux

Current distributions (kernel 6.1+, e.g. Ubuntu 24.04+, Fedora 40+, Debian 13, Arch). Building requires Go 1.26+.

```sh
go install .             # -> ~/go/bin/ora
ora index                # first full index (desktop entries, AppImages, file index)
```

Programs come from the XDG desktop entries (`~/.local/share/applications`, `$XDG_DATA_DIRS/applications`, Flatpak and Snap exports), honoring `NoDisplay`, `Hidden`, `OnlyShowIn`/`NotShowIn` and `TryExec`. Portable programs are AppImages (`everything.extensions`) outside `/usr`, `/opt`, `/snap`, Flatpak and Nix.

Instead of Everything, ora keeps its own file name index, no root required: a full scan of `files.roots` (default: the home folder) cached in `files.idx`, rescanned when older than `files.rescan` (default `1h`). `ora mcp` keeps it current with inotify (`files.watch`, default `true`); on very large trees raise `fs.inotify.max_user_watches`. `everything.exclude_regex` defaults to caches, VCS and build folders.

The window needs GTK 4, gtk4-layer-shell and the gdk-pixbuf SVG loader (librsvg) at run time; building needs cgo and the headers:

```sh
sudo apt install libgtk-4-dev libgtk4-layer-shell-dev librsvg2-common   # Kali rolling
sudo dnf install gtk4-devel gtk4-layer-shell-devel librsvg2             # Fedora
sudo pacman -S gtk4 gtk4-layer-shell librsvg                            # Arch
```

Ubuntu 24.04 has no `libgtk4-layer-shell-dev`; its `libgtk-layer-shell-dev` is the GTK 3 library and does not fit. There, install `libgtk-4-dev librsvg2-common` and build [gtk4-layer-shell](https://github.com/wmww/gtk4-layer-shell) from source. Without its `gtk4-layer-shell-0` pkg-config file the window does not build, and neither does `go test ./gui`.

Without them, `go install -tags nogui .` (or `CGO_ENABLED=0`) builds the terminal command and `ora mcp` only; `ora` without arguments then prints the help.

Apps start through `gio launch` (falling back to the desktop entry's `Exec`), files open with `xdg-open`/`gio open`, and `ora open` asks the file manager over D-Bus (`org.freedesktop.FileManager1`) to select the item, else opens its folder.

### Shell completion

Shell completion for the current PowerShell profile:

```powershell
ora completion powershell >> $PROFILE
```

Open a new window after that. `ora completion` also prints bash, zsh, and fish.

## Commands

| Command | |
|---|---|
| `ora <query> [args...]` | match and launch; an existing file path is opened with its default app |
| `ora file <query...>` / `ora f` | find any file by name via Everything (Linux: ora's file index) and open it; several copies open the picker; `--which` only shows the result |
| `ora index` | rebuild the cache (all sources) |
| `ora list` | print the index; in a terminal paged 20 rows at a time (←/→, Home/End, q), piped output is the full list |
| `ora which <query>` | show winner, score, target and decision without launching |
| `ora alias <shortcut> <official name>` | add an alias to `config.yaml` |
| `ora open <query...>` | show the match in Explorer (Linux: the file manager) instead of launching it; a typed folder is opened; `-f` searches files and folders |

Flags before the query: `--refresh`, `--min-score`, `-v`.

Exit codes: `0` launched, `1` no match / cancelled, `2` ambiguous without TTY, `3` launch failed, `4` other error.

`ora mcp` serves search, launch and reveal to AI agents over MCP on stdin/stdout. Agents search first and launch only by the id they got. Tools and setup: [docs/mcp.md](docs/mcp.md).

## Files

- `%APPDATA%\ora\config.yaml` – settings and aliases. A missing file uses the built-in defaults; the first window start writes it with the window settings
- `%APPDATA%\ora\recent.json` – last 20 launches (the CLI records apps, the window also files and folders)
- `%LOCALAPPDATA%\ora\index.json` – index cache, 24 h TTL
- `%LOCALAPPDATA%\ora\gui.log` – window warnings and errors

On Linux the same files live in `$XDG_CONFIG_HOME/ora` (default `~/.config/ora`) and `$XDG_CACHE_HOME/ora` (default `~/.cache/ora`), plus `files.idx`, the file name index.

## License

[MIT](LICENSE).

## Development

```powershell
go vet ./...
go test ./...
```

Unit tests use fixtures and temp directories only (no live COM, Everything, registry or user profile), so they pass on any Windows machine. Tests that use Windows paths live in `*_windows_test.go`; the rest also pass on Linux, and `GOOS=windows go vet ./...` checks the Windows build from Linux.
