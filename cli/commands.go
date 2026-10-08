package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/sys/windows"

	"ora/core/config"
	"ora/core/discover"
	"ora/core/engine"
	"ora/core/index"
	"ora/core/match"
	"ora/mcp"
)

// NewRoot builds the terminal command tree.
func NewRoot() *cobra.Command {
	a := &app{v: viper.New()}
	root := &cobra.Command{
		Use:   "ora <query> [args...]",
		Short: "Start a Windows app by a short, possibly misspelled name",
		Long: "Start a Windows app by a short, possibly misspelled name.\n\n" +
			"The first argument is the query, the rest is passed to the app:\n" +
			"  ora code .\n" +
			"With --, all words before it form the query:\n" +
			"  ora pixel paint -- --debug",
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(a.v)
			if err != nil {
				return err
			}
			a.cfg = cfg
			a.eng = &engine.Engine{Cfg: cfg, Warn: a.warn, Debug: a.debug}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			q, extra := splitQuery(args, cmd.ArgsLenAtDash())
			if q == "" {
				return cmd.Help()
			}
			return a.runLaunch(q, extra)
		},
	}
	// Flags after the query belong to the launched app: `ora code --new-window`.
	root.Flags().SetInterspersed(false)
	pf := root.PersistentFlags()
	pf.BoolVar(&a.refresh, "refresh", false, "rebuild the index (all sources) before matching")
	pf.Float64("min-score", 0.86, "minimum score for auto-launch")
	pf.BoolVarP(&a.verbose, "verbose", "v", false, "print sources, timings and scores")
	_ = a.v.BindPFlag("min_score", pf.Lookup("min-score"))

	root.AddCommand(
		&cobra.Command{
			Use:   "index",
			Short: "Rebuild the index cache now",
			Args:  cobra.NoArgs,
			RunE:  func(*cobra.Command, []string) error { return a.runIndex() },
		},
		&cobra.Command{
			Use:   "list",
			Short: "Print the index",
			Args:  cobra.NoArgs,
			RunE:  func(*cobra.Command, []string) error { return a.runList() },
		},
		fileCmd(a),
		openCmd(a),
		&cobra.Command{
			Use:   "which <query...>",
			Short: "Print the winner, score, kind and target without launching",
			Args:  cobra.MinimumNArgs(1),
			RunE:  func(_ *cobra.Command, args []string) error { return a.runWhich(strings.Join(args, " ")) },
		},
		&cobra.Command{
			Use:   "mcp",
			Short: "Serve search, launch and reveal to agents over MCP on stdin/stdout",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return mcp.Run(cmd.Context(), a.eng)
			},
		},
		&cobra.Command{
			Use:   "alias <shortcut> <official name...>",
			Short: "Write an alias into config.yaml",
			Args:  cobra.MinimumNArgs(2),
			RunE: func(_ *cobra.Command, args []string) error {
				return a.runAlias(args[0], strings.Join(args[1:], " "))
			},
		},
	)
	return root
}

func fileCmd(a *app) *cobra.Command {
	var which bool
	cmd := &cobra.Command{
		Use:     "file <query...>",
		Aliases: []string{"f"},
		Short:   "Find any file by name via Everything and open it with its default app",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return a.runFile(strings.Join(args, " "), which)
		},
	}
	cmd.Flags().BoolVar(&which, "which", false, "print the winner, score and decision without opening")
	return cmd
}

func openCmd(a *app) *cobra.Command {
	var file bool
	cmd := &cobra.Command{
		Use:   "open <query...>",
		Short: "Show the match in Explorer instead of launching it",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return a.runOpen(strings.Join(args, " "), file)
		},
	}
	cmd.Flags().BoolVarP(&file, "file", "f", false, "search files and folders, not programs")
	return cmd
}

// openableFile reports a path the user typed, so ora can open that file
// with its default app. A bare name with no extension stays a program query.
func openableFile(q string) (string, bool) {
	p, st, ok := existingPath(q)
	if !ok || st.IsDir() {
		return "", false
	}
	return p, true
}

func openableDir(q string) (string, bool) {
	p, st, ok := existingPath(q)
	if !ok || !st.IsDir() {
		return "", false
	}
	return p, true
}

// existingPath accepts a typed path. A bare name with no extension stays a query.
func existingPath(q string) (string, os.FileInfo, bool) {
	if q == "" || (!strings.ContainsAny(q, `\/`) && !strings.Contains(q, ".")) {
		return "", nil, false
	}
	st, err := os.Stat(q)
	if err != nil {
		return "", nil, false
	}
	abs, err := filepath.Abs(q)
	if err != nil {
		return q, st, true
	}
	return abs, st, true
}

// splitQuery implements the argument rules: with a literal "--" the words
// before it form the query; without it the first word is the query.
func splitQuery(args []string, dashAt int) (string, []string) {
	for i, s := range args {
		if s == "--" {
			return strings.Join(args[:i], " "), args[i+1:]
		}
	}
	if dashAt > 0 {
		return strings.Join(args[:dashAt], " "), args[dashAt:]
	}
	if len(args) == 0 {
		return "", nil
	}
	return args[0], args[1:]
}

func (a *app) runLaunch(q string, extra []string) error {
	if p, ok := openableFile(q); ok {
		return a.launch(nil, fileEntry(p), extra)
	}
	ix, err := a.eng.Index(a.refresh)
	if err != nil {
		return err
	}
	return a.resolve(ix, a.eng.Search(ix, q), q, func(e index.Entry, score float64) string {
		return fmt.Sprintf("%-40s %-9s %.2f  %s", e.Name, e.Kind, score, describeTarget(e))
	}, func(e index.Entry) error {
		return a.launch(ix, e, extra)
	})
}

func (a *app) runOpen(q string, file bool) error {
	if p, ok := openableFile(q); ok {
		return a.reveal(fileEntry(p))
	}
	if p, ok := openableDir(q); ok {
		return a.reveal(discover.DirEntry(p))
	}
	label := func(e index.Entry, score float64) string {
		return fmt.Sprintf("%-40s %-9s %.2f  %s", e.Name, e.Kind, score, describeTarget(e))
	}
	act := func(e index.Entry) error { return a.reveal(e) }
	if file {
		ix, rs, err := a.rankPaths(q, true)
		if err != nil {
			return err
		}
		label = func(e index.Entry, score float64) string {
			return fmt.Sprintf("%.2f  %s", score, e.Target)
		}
		return a.resolve(ix, rs, q, label, act)
	}
	ix, err := a.eng.Index(a.refresh)
	if err != nil {
		return err
	}
	return a.resolve(ix, a.eng.Search(ix, q), q, label, act)
}

func fileEntry(p string) index.Entry {
	return index.Entry{Name: filepath.Base(p), Kind: index.KindFile, Source: index.SourceSearch, Target: p}
}

func (a *app) runFile(q string, which bool) error {
	if p, ok := openableFile(q); ok {
		if which {
			ix := &index.Index{Entries: []index.Entry{fileEntry(p)}}
			return a.printWhich(ix, []match.Result{{Index: 0, Score: match.ScoreExact}})
		}
		return a.launch(nil, fileEntry(p), nil)
	}
	ix, rs, err := a.rankFiles(q)
	if err != nil {
		return err
	}
	if which {
		return a.printWhich(ix, rs)
	}
	return a.resolve(ix, rs, q, func(e index.Entry, score float64) string {
		return fmt.Sprintf("%.2f  %s", score, e.Target)
	}, func(e index.Entry) error {
		return a.launch(ix, e, nil)
	})
}

func (a *app) rankFiles(q string) (*index.Index, []match.Result, error) {
	return a.rankPaths(q, false)
}

func (a *app) rankPaths(q string, folders bool) (*index.Index, []match.Result, error) {
	ix, rs, err := a.eng.SearchPaths(q, folders)
	if err != nil {
		return nil, nil, err
	}
	if len(rs) == 0 {
		what := "file"
		if folders {
			what = "file or folder"
		}
		return nil, nil, &ExitError{code: exitNoMatch, msg: fmt.Sprintf("no %s matches %q", what, q)}
	}
	return ix, rs, nil
}

// resolve applies the decision to ranked results: run act on the winner,
// ask with the picker, or print the closest entries.
func (a *app) resolve(ix *index.Index, rs []match.Result, q string, label func(index.Entry, float64) string, act func(index.Entry) error) error {
	d, rows := match.Decide(rs, a.cfg.MinScore, a.cfg.AmbiguityGap)
	if a.verbose {
		fmt.Fprintf(os.Stderr, "query %q -> %s\n", q, d)
		printRows(os.Stderr, ix, rs[:min(len(rs), match.PickerRows)])
	}

	switch d {
	case match.Launch:
		return act(ix.Entries[rows[0].Index])
	case match.Pick:
		more := countAbove(rs, match.PickerMin) - len(rows)
		if !Interactive() {
			printRows(os.Stdout, ix, rows)
			if more > 0 {
				fmt.Printf("... %d more, add a word to narrow\n", more)
			}
			return &ExitError{code: exitAmbiguous, msg: fmt.Sprintf("%q is ambiguous", q)}
		}
		labels := make([]string, len(rows))
		for i, r := range rows {
			labels[i] = label(ix.Entries[r.Index], r.Score)
		}
		title := fmt.Sprintf("Several matches for %q:", q)
		if more > 0 {
			title = fmt.Sprintf("Several matches for %q (%d more not shown, add a word to narrow):", q, more)
		}
		i, err := Pick(title, labels)
		if err != nil {
			return err
		}
		if i < 0 {
			return &ExitError{code: exitNoMatch}
		}
		return act(ix.Entries[rows[i].Index])
	default:
		fmt.Fprintf(os.Stderr, "no match for %q. Closest:\n", q)
		printRows(os.Stderr, ix, rows)
		return &ExitError{code: exitNoMatch}
	}
}

func countAbove(rs []match.Result, min float64) int {
	n := 0
	for _, r := range rs {
		if r.Score >= min {
			n++
		}
	}
	return n
}

func printRows(w io.Writer, ix *index.Index, rows []match.Result) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for i, r := range rows {
		e := ix.Entries[r.Index]
		fmt.Fprintf(tw, "%d\t%s\t%s\t%.2f\t%s\n", i+1, e.Name, e.Kind, r.Score, describeTarget(e))
	}
	tw.Flush()
}

func (a *app) runIndex() error {
	ix, err := a.eng.Index(true)
	if err != nil {
		return err
	}
	counts := map[index.Source]int{}
	for _, e := range ix.Entries {
		counts[e.Source]++
	}
	path, _ := config.CacheFile()
	fmt.Printf("%d entries (startmenu %d, registry %d, appsfolder %d, everything %d) -> %s\n",
		len(ix.Entries), counts[index.SourceStartMenu], counts[index.SourceRegistry],
		counts[index.SourceAppsFolder], counts[index.SourceEverything], path)
	return nil
}

func (a *app) runList() error {
	ix, err := a.eng.Index(a.refresh)
	if err != nil {
		return err
	}
	es := append([]index.Entry(nil), ix.Entries...)
	sort.SliceStable(es, func(i, j int) bool { return strings.ToLower(es[i].Name) < strings.ToLower(es[j].Name) })
	var buf strings.Builder
	tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tKIND\tSOURCE\tTARGET")
	for _, e := range es {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", e.Name, e.Kind, e.Source, describeTarget(e))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if !Interactive() {
		_, err := io.WriteString(os.Stdout, buf.String())
		if errors.Is(err, windows.ERROR_NO_DATA) || errors.Is(err, windows.ERROR_BROKEN_PIPE) {
			return nil // reader closed early, e.g. `ora list | Select -First 5`
		}
		return err
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	return Page(lines[0], lines[1:])
}

func (a *app) runWhich(q string) error {
	ix, err := a.eng.Index(a.refresh)
	if err != nil {
		return err
	}
	if len(ix.Entries) == 0 {
		return &ExitError{code: exitNoMatch, msg: "index is empty"}
	}
	return a.printWhich(ix, a.eng.Search(ix, q))
}

// printWhich shows the winner and the decision for ranked results without
// launching anything.
func (a *app) printWhich(ix *index.Index, rs []match.Result) error {
	d, rows := match.Decide(rs, a.cfg.MinScore, a.cfg.AmbiguityGap)
	best := ix.Entries[rs[0].Index]
	fmt.Printf("name      %s\nscore     %.3f\nkind      %s (%s)\ntarget    %s\ndecision  %s\n",
		best.Name, rs[0].Score, best.Kind, best.Source, describeTarget(best), d)
	if d != match.Launch || a.verbose {
		fmt.Println()
		printRows(os.Stdout, ix, rows)
		if more := countAbove(rs, match.PickerMin) - len(rows); d == match.Pick && more > 0 {
			fmt.Printf("... %d more, add a word to narrow\n", more)
		}
	}
	if d == match.NoMatch {
		return &ExitError{code: exitNoMatch}
	}
	return nil
}

func (a *app) runAlias(key, official string) error {
	path, err := config.ConfigFile()
	if err != nil {
		return err
	}
	if err := config.SetAlias(path, key, official); err != nil {
		return err
	}
	k, _ := config.NormalizeAliasKey(key)
	fmt.Printf("alias %s -> %s written to %s\n", k, official, path)

	if ix, _ := index.Load(); ix != nil {
		want := match.Compact(match.Normalize(official))
		for _, e := range ix.Entries {
			if match.Compact(match.Normalize(e.Name)) == want {
				return nil
			}
		}
		a.warn("no indexed app is named %q; the alias has no effect until one is (see `ora list`)", official)
	}
	return nil
}
