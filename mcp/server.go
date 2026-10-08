// Package mcp serves ora to agents over MCP on stdin/stdout. Agents search,
// then launch or reveal by the id a search returned. Nothing here picks a
// winner or launches from a query.
package mcp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"ora/core/discover"
	"ora/core/engine"
	"ora/core/index"
	"ora/core/match"
)

// maxRows caps the rows of one search.
const maxRows = match.PickerRows

// Engine is the part of *engine.Engine the tools use.
type Engine interface {
	Index(force bool) (*index.Index, error)
	Search(ix *index.Index, query string) []match.Result
	SearchPaths(query string, folders bool) (*index.Index, []match.Result, error)
	Launch(ix *index.Index, entry index.Entry, extra []string) error
	Remember(entry index.Entry)
	Reveal(entry index.Entry) error
}

// ErrUnknownID is returned by launch and reveal for an id that is neither in
// the index nor an existing absolute path.
var ErrUnknownID = errors.New("unknown id")

type Row struct {
	ID     string  `json:"id" jsonschema:"pass this to launch or reveal"`
	Name   string  `json:"name"`
	Kind   string  `json:"kind"`
	Score  float64 `json:"score"`
	Target string  `json:"target"`
}

type SearchResult struct {
	Decision string `json:"decision" jsonschema:"launch: one clear winner; pick: several candidates, ask or choose; none: no good match"`
	Rows     []Row  `json:"rows"`
}

type SearchProgramsIn struct {
	Query string `json:"query" jsonschema:"app name, may be short or misspelled"`
}

type SearchFilesIn struct {
	Query   string `json:"query" jsonschema:"words that must occur in the file name"`
	Folders bool   `json:"folders,omitempty" jsonschema:"also return folders"`
}

type LaunchIn struct {
	ID   string   `json:"id" jsonschema:"id from search_programs or search_files, or an existing absolute file path"`
	Args []string `json:"args,omitempty" jsonschema:"arguments passed to the program"`
}

type RevealIn struct {
	ID string `json:"id" jsonschema:"id from a search, or an existing absolute path"`
}

type Done struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Target string `json:"target"`
}

type tools struct {
	eng      Engine
	minScore float64
	gap      float64
}

// New builds the server; minScore and gap are the configured min_score and
// ambiguity_gap used for the decision.
func New(eng Engine, minScore, gap float64) *sdk.Server {
	t := &tools{eng: eng, minScore: minScore, gap: gap}
	version := "dev"
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		version = bi.Main.Version
	}
	s := sdk.NewServer(&sdk.Implementation{Name: "ora", Version: version}, nil)
	sdk.AddTool(s, &sdk.Tool{
		Name:        "search_programs",
		Description: "Rank installed programs against a query. Returns up to 8 rows with ids and ora's decision. Launches nothing.",
	}, t.searchPrograms)
	sdk.AddTool(s, &sdk.Tool{
		Name:        "search_files",
		Description: "Find files (and folders if folders is true) by name via Everything. Returns up to 8 rows with ids and ora's decision. Opens nothing.",
	}, t.searchFiles)
	sdk.AddTool(s, &sdk.Tool{
		Name:        "launch",
		Description: "Start the program or open the file with this exact id. No fuzzy matching: unknown ids fail.",
	}, t.launch)
	sdk.AddTool(s, &sdk.Tool{
		Name:        "reveal",
		Description: "Show the entry with this exact id in Explorer: a folder is opened, anything else is selected in its folder. Launches nothing.",
	}, t.reveal)
	return s
}

// Run serves eng on stdin/stdout until the client disconnects.
func Run(ctx context.Context, eng *engine.Engine) error {
	return New(eng, eng.Cfg.MinScore, eng.Cfg.AmbiguityGap).Run(ctx, &sdk.StdioTransport{})
}

func (t *tools) searchPrograms(_ context.Context, _ *sdk.CallToolRequest, in SearchProgramsIn) (*sdk.CallToolResult, SearchResult, error) {
	q := strings.TrimSpace(in.Query)
	if q == "" {
		return nil, SearchResult{}, errors.New("query is empty")
	}
	ix, err := t.eng.Index(false)
	if err != nil {
		return nil, SearchResult{}, err
	}
	return nil, t.result(ix, t.eng.Search(ix, q)), nil
}

func (t *tools) searchFiles(_ context.Context, _ *sdk.CallToolRequest, in SearchFilesIn) (*sdk.CallToolResult, SearchResult, error) {
	q := strings.TrimSpace(in.Query)
	if q == "" {
		return nil, SearchResult{}, errors.New("query is empty")
	}
	ix, rs, err := t.eng.SearchPaths(q, in.Folders)
	if err != nil {
		return nil, SearchResult{}, err
	}
	return nil, t.result(ix, rs), nil
}

func (t *tools) result(ix *index.Index, rs []match.Result) SearchResult {
	d, _ := match.Decide(rs, t.minScore, t.gap)
	out := SearchResult{Decision: decision(d), Rows: []Row{}}
	for _, r := range rs {
		if len(out.Rows) == maxRows || r.Score <= 0 {
			break
		}
		e := ix.Entries[r.Index]
		out.Rows = append(out.Rows, Row{ID: e.ID(), Name: e.Name, Kind: string(e.Kind), Score: r.Score, Target: e.Target})
	}
	return out
}

func decision(d match.Decision) string {
	switch d {
	case match.Launch:
		return "launch"
	case match.Pick:
		return "pick"
	default:
		return "none"
	}
}

func (t *tools) launch(_ context.Context, _ *sdk.CallToolRequest, in LaunchIn) (*sdk.CallToolResult, Done, error) {
	ix, e, err := t.resolve(in.ID)
	if err != nil {
		return nil, Done{}, err
	}
	if e.Kind == index.KindFolder {
		return nil, Done{}, fmt.Errorf("%s is a folder; use reveal", e.Target)
	}
	if err := t.eng.Launch(ix, e, in.Args); err != nil {
		return nil, Done{}, err
	}
	t.eng.Remember(e)
	return nil, done(e), nil
}

func (t *tools) reveal(_ context.Context, _ *sdk.CallToolRequest, in RevealIn) (*sdk.CallToolResult, Done, error) {
	_, e, err := t.resolve(in.ID)
	if err != nil {
		return nil, Done{}, err
	}
	if err := t.eng.Reveal(e); err != nil {
		return nil, Done{}, err
	}
	return nil, done(e), nil
}

func done(e index.Entry) Done {
	return Done{ID: e.ID(), Name: e.Name, Kind: string(e.Kind), Target: e.Target}
}

// resolve finds the index entry with exactly this id, else an existing
// absolute path ("path:" prefix optional, as search_files returns it). The
// index is nil for paths.
func (t *tools) resolve(id string) (*index.Index, index.Entry, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, index.Entry{}, errors.New("id is empty")
	}
	ix, err := t.eng.Index(false)
	if err != nil {
		return nil, index.Entry{}, err
	}
	for _, e := range ix.Entries {
		if strings.EqualFold(e.ID(), id) {
			return ix, e, nil
		}
	}
	p := strings.TrimPrefix(id, "path:")
	if filepath.IsAbs(p) {
		if st, err := os.Stat(p); err == nil {
			if st.IsDir() {
				return nil, discover.DirEntry(p), nil
			}
			return nil, discover.FileEntry(p), nil
		}
	}
	return nil, index.Entry{}, fmt.Errorf("%w %q: use an id from search_programs or search_files", ErrUnknownID, id)
}
