package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/viper"

	"ora/core/config"
	"ora/core/engine"
	"ora/core/index"
	"ora/core/match"
)

// fake ranks with the real engine over a fixture index and records what
// would have been launched, remembered or revealed.
type fake struct {
	*engine.Engine
	ix         *index.Index
	files      *index.Index
	filesErr   error
	launched   []index.Entry
	launchIx   []*index.Index
	args       [][]string
	remembered []index.Entry
	revealed   []index.Entry
}

func (f *fake) Index(bool) (*index.Index, error) { return f.ix, nil }

func (f *fake) SearchPaths(q string, _ bool) (*index.Index, []match.Result, error) {
	if f.filesErr != nil {
		return nil, nil, f.filesErr
	}
	cs := make([]match.Candidate, len(f.files.Entries))
	for i, e := range f.files.Entries {
		cs[i] = match.Candidate{Name: e.Name}
	}
	return f.files, match.Rank(q, cs), nil
}

func (f *fake) Launch(ix *index.Index, e index.Entry, extra []string) error {
	f.launched = append(f.launched, e)
	f.launchIx = append(f.launchIx, ix)
	f.args = append(f.args, extra)
	return nil
}

func (f *fake) Remember(e index.Entry) { f.remembered = append(f.remembered, e) }

func (f *fake) Reveal(e index.Entry) error {
	f.revealed = append(f.revealed, e)
	return nil
}

func newFake(t *testing.T) (*fake, *config.Config) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("LOCALAPPDATA", dir)
	if err := os.MkdirAll(filepath.Join(dir, "ora"), 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := "min_score: 0.86\nambiguity_gap: 0.12\neverything:\n  enabled: false\n"
	if err := os.WriteFile(filepath.Join(dir, "ora", "config.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(viper.New())
	if err != nil {
		t.Fatal(err)
	}
	return &fake{
		Engine: &engine.Engine{Cfg: cfg},
		ix: &index.Index{Entries: []index.Entry{
			{Name: "Visual Studio Code", Kind: index.KindShortcut, Source: index.SourceStartMenu, Target: `C:\lnk\Code.lnk`},
			{Name: "Discord", Kind: index.KindShortcut, Source: index.SourceStartMenu, Target: `C:\lnk\Discord.lnk`},
			{Name: "Disk Cleanup", Kind: index.KindShortcut, Source: index.SourceStartMenu, Target: `C:\lnk\cleanmgr.lnk`},
			{Name: "Calculator", Kind: index.KindStore, Source: index.SourceAppsFolder, Target: "Calc!App", AUMID: "Calc!App"},
		}},
		files: &index.Index{Entries: []index.Entry{
			{Name: "notes", Kind: index.KindFile, Source: index.SourceSearch, Target: `E:\Docs\notes.txt`},
			{Name: "Docs", Kind: index.KindFolder, Source: index.SourceSearch, Target: `E:\Docs`},
		}},
	}, cfg
}

func connect(t *testing.T, f *fake, cfg *config.Config) *sdk.ClientSession {
	t.Helper()
	ctx := context.Background()
	ct, st := sdk.NewInMemoryTransports()
	ss, err := New(f, cfg.MinScore, cfg.AmbiguityGap).Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cs.Close()
		ss.Wait()
	})
	return cs
}

// call returns the tool result; a tool error comes back as its text.
func call(t *testing.T, cs *sdk.ClientSession, name string, args map[string]any, out any) (errText string) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if res.IsError {
		var b strings.Builder
		for _, c := range res.Content {
			if tc, ok := c.(*sdk.TextContent); ok {
				b.WriteString(tc.Text)
			}
		}
		return b.String()
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("%s: %v in %s", name, err, raw)
	}
	return ""
}

func TestListsFourTools(t *testing.T) {
	f, cfg := newFake(t)
	res, err := connect(t, f, cfg).ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tl := range res.Tools {
		names = append(names, tl.Name)
	}
	if strings.Join(names, ",") != "launch,reveal,search_files,search_programs" {
		t.Fatalf("tools %v", names)
	}
}

func TestSearchProgramsReturnsIDsAndDoesNotLaunch(t *testing.T) {
	f, cfg := newFake(t)
	cs := connect(t, f, cfg)
	var r SearchResult
	if e := call(t, cs, "search_programs", map[string]any{"query": "discord"}, &r); e != "" {
		t.Fatal(e)
	}
	if r.Decision != "launch" || len(r.Rows) == 0 || len(r.Rows) > maxRows {
		t.Fatalf("result %+v", r)
	}
	top := r.Rows[0]
	if top.ID != f.ix.Entries[1].ID() || top.Name != "Discord" || top.Kind != "shortcut" || top.Target != `C:\lnk\Discord.lnk` {
		t.Fatalf("top row %+v", top)
	}
	if len(f.launched) != 0 || len(f.remembered) != 0 || len(f.revealed) != 0 {
		t.Fatal("search must not launch, remember or reveal")
	}
}

func TestAmbiguousQueryIsNotLaunched(t *testing.T) {
	f, cfg := newFake(t)
	cs := connect(t, f, cfg)
	var r SearchResult
	if e := call(t, cs, "search_programs", map[string]any{"query": "dis"}, &r); e != "" {
		t.Fatal(e)
	}
	if r.Decision != "pick" || len(r.Rows) < 2 {
		t.Fatalf("want pick with candidates, got %+v", r)
	}
	var d Done
	if e := call(t, cs, "launch", map[string]any{"id": "dis"}, &d); !strings.Contains(e, "unknown id") {
		t.Fatalf("a query is no id: %q", e)
	}
	if len(f.launched) != 0 || len(f.remembered) != 0 {
		t.Fatal("nothing may launch")
	}
}

func TestUnknownIDFails(t *testing.T) {
	f, cfg := newFake(t)
	cs := connect(t, f, cfg)
	var d Done
	for _, tool := range []string{"launch", "reveal"} {
		for _, id := range []string{"path:c:\\lnk\\nothing.lnk", "aumid:none!app", "Discord", ""} {
			if e := call(t, cs, tool, map[string]any{"id": id}, &d); e == "" {
				t.Errorf("%s %q succeeded", tool, id)
			}
		}
	}
	if len(f.launched)+len(f.remembered)+len(f.revealed) != 0 {
		t.Fatal("unknown ids must not reach the engine")
	}
}

func TestLaunchKnownIDCallsLaunchAndRemember(t *testing.T) {
	f, cfg := newFake(t)
	cs := connect(t, f, cfg)
	want := f.ix.Entries[1]
	var d Done
	if e := call(t, cs, "launch", map[string]any{"id": want.ID(), "args": []string{"--x", "y z"}}, &d); e != "" {
		t.Fatal(e)
	}
	if len(f.launched) != 1 || f.launched[0] != want || f.launchIx[0] != f.ix {
		t.Fatalf("launched %+v", f.launched)
	}
	if len(f.args[0]) != 2 || f.args[0][1] != "y z" {
		t.Fatalf("args %q", f.args[0])
	}
	if len(f.remembered) != 1 || f.remembered[0] != want {
		t.Fatalf("remembered %+v", f.remembered)
	}
	if d.ID != want.ID() || d.Name != "Discord" {
		t.Fatalf("result %+v", d)
	}
}

func TestLaunchStoreByAUMID(t *testing.T) {
	f, cfg := newFake(t)
	cs := connect(t, f, cfg)
	var d Done
	if e := call(t, cs, "launch", map[string]any{"id": "aumid:calc!app"}, &d); e != "" {
		t.Fatal(e)
	}
	if len(f.launched) != 1 || f.launched[0].Kind != index.KindStore {
		t.Fatalf("launched %+v", f.launched)
	}
}

func TestTypedPaths(t *testing.T) {
	f, cfg := newFake(t)
	cs := connect(t, f, cfg)
	dir := t.TempDir()
	file := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	var d Done
	if e := call(t, cs, "launch", map[string]any{"id": file}, &d); e != "" {
		t.Fatal(e)
	}
	if e := call(t, cs, "launch", map[string]any{"id": "path:" + strings.ToLower(file)}, &d); e != "" {
		t.Fatal(e)
	}
	if len(f.launched) != 2 || f.launched[0].Kind != index.KindFile || f.launchIx[0] != nil {
		t.Fatalf("launched %+v", f.launched)
	}
	if e := call(t, cs, "launch", map[string]any{"id": dir}, &d); !strings.Contains(e, "use reveal") {
		t.Fatalf("folder launch: %q", e)
	}
	if e := call(t, cs, "launch", map[string]any{"id": "report.txt"}, &d); e == "" {
		t.Fatal("relative paths depend on the server's cwd and must fail")
	}
	if e := call(t, cs, "reveal", map[string]any{"id": dir}, &d); e != "" {
		t.Fatal(e)
	}
	if len(f.revealed) != 1 || f.revealed[0].Kind != index.KindFolder || len(f.launched) != 2 {
		t.Fatalf("revealed %+v launched %d", f.revealed, len(f.launched))
	}
}

func TestRevealKnownIDDoesNotLaunch(t *testing.T) {
	f, cfg := newFake(t)
	cs := connect(t, f, cfg)
	var d Done
	if e := call(t, cs, "reveal", map[string]any{"id": f.ix.Entries[0].ID()}, &d); e != "" {
		t.Fatal(e)
	}
	if len(f.revealed) != 1 || f.revealed[0] != f.ix.Entries[0] || len(f.launched)+len(f.remembered) != 0 {
		t.Fatalf("revealed %+v launched %d", f.revealed, len(f.launched))
	}
}

func TestSearchFiles(t *testing.T) {
	f, cfg := newFake(t)
	cs := connect(t, f, cfg)
	var r SearchResult
	if e := call(t, cs, "search_files", map[string]any{"query": "notes", "folders": true}, &r); e != "" {
		t.Fatal(e)
	}
	if len(r.Rows) == 0 || r.Rows[0].ID != `path:e:\docs\notes.txt` || r.Rows[0].Kind != "file" {
		t.Fatalf("result %+v", r)
	}
	f.filesErr = engine.ErrNeedsEverything
	if e := call(t, cs, "search_files", map[string]any{"query": "notes"}, &r); !strings.Contains(e, "Everything") {
		t.Fatalf("everything down: %q", e)
	}
	if len(f.launched)+len(f.revealed) != 0 {
		t.Fatal("search must not open anything")
	}
}
