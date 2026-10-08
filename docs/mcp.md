# ora mcp

`ora mcp` is an MCP server built into the ora binary. It speaks MCP over stdio and runs until the client closes stdin. It uses the same index, config and recent list as the terminal and the window. It needs no window and no hotkey, and it never spawns `ora` itself.

An agent searches first, then launches or reveals by the `id` a search returned. Nothing auto-launches from a query, and launch never fuzzy-matches.

## Tools

### search_programs

Input: `query` (string).

Ranks the indexed programs (Start Menu, registry, Store apps, Everything) like `ora <query>` does. Launches nothing.

Output:

```json
{
  "decision": "launch",
  "rows": [
    { "id": "path:c:\\...\\discord.lnk", "name": "Discord", "kind": "shortcut", "score": 1, "target": "C:\\...\\Discord.lnk" }
  ]
}
```

- `rows` holds at most 8 rows, best first.
- `id` is the id stored in `recent.json`: `path:<lowercase target>` or `aumid:<lowercase AUMID>`.
- `decision` uses the configured `min_score` and `ambiguity_gap`:
  - `launch`: one clear winner, the first row.
  - `pick`: several candidates. Choose one or ask the user.
  - `none`: no good match. The rows are only the closest names.

### search_files

Input: `query` (string), `folders` (bool, optional).

Finds files by name via Everything, like `ora file`. With `folders: true` it also finds folders, like `ora open -f`. Output is the same shape as above with kind `file` or `folder`. Opens nothing. If Everything is not running, the call fails with that error.

### launch

Input: `id` (string), `args` (string array, optional).

Starts the entry with exactly this id and puts it in the recent list.

- `args` go to the program.
- An existing absolute file path, with or without the `path:` prefix, is opened with its default app.
- Folders fail. Use `reveal` for them.
- Anything else, including a query such as `disc`, fails with `unknown id`.

### reveal

Input: `id` (string).

Shows the entry in Explorer. A folder is opened. Anything else is selected in its folder. It accepts the same ids and absolute paths as `launch`. Store apps have no file and fail. Launches nothing.

## Cursor

The installed `ora` must be on `PATH` (`go install .` puts it in `%USERPROFILE%\go\bin`). In `.cursor/mcp.json` or the global `mcp.json`:

```json
{
  "mcpServers": {
    "ora": {
      "command": "ora",
      "args": ["mcp"]
    }
  }
}
```

Errors from a tool come back as tool results with `isError`, so the agent sees the reason. Warnings go to stderr. stdout carries only MCP.
