# Agents

- To find, start or reveal apps and files, use the MCP server of this binary: `ora mcp`. The tool contract is in [docs/mcp.md](docs/mcp.md).
- Do not add a second MCP server or copy one from another tree. New tools go into the `mcp` package and call `core/engine`.
- Do not shell out to `ora` (or `ora.exe`) to launch apps. Search, then call `launch` or `reveal` with the returned id.
