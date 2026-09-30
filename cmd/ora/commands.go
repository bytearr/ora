package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/sys/windows"

	"ora/internal/config"
	"ora/internal/discover"
	"ora/internal/index"
	"ora/internal/match"
	"ora/internal/ui"
)

func newRoot() *cobra.Command {
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
		&cobra.Command{
			Use:   "which <query...>",
			Short: "Print the winner, score, kind and target without launching",
			Args:  cobra.MinimumNArgs(1),
			RunE:  func(_ *cobra.Command, args []string) error { return a.runWhich(strings.Join(args, " ")) },
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

// openableFile reports a path the user typed, so ora can open that file
// with its default app. A bare name with no extension stays a program query.
func openableFile(q string) (string, bool) {
	if q == "" || (!strings.ContainsAny(q, `\/`) && !strings.Contains(q, ".")) {
		return "", false
	}
	st, err := os.Stat(q)
	if err != nil || st.IsDir() {
		return "", false
	}
	abs, err := filepath.Abs(q)
	if err != nil {
		return q, true
	}
	return abs, true
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
	ix, err := a.loadIndex(a.refresh)
	if err != nil {
		return err
	}
	return a.resolve(ix, a.rank(ix, q), q, extra, func(e index.Entry, score float64) string {
		return fmt.Sprintf("%-40s %-9s %.2f  %s", e.Name, e.Kind, score, describeTarget(e))
	})
}

func fileEntry(p string) index.Entry {
	return index.Entry{Name: filepath.Base(p), Kind: index.KindFile, Source: index.SourceSearch, Target: p}
}

// fileSearchLimit caps one live `ora file` query; Everything sorts by name,
// so a cap hit means the query is too broad.
const fileSearchLimit = 2000

func (a *app) runFile(q string, which bool) error {
	if p, ok := openableFile(q); ok {
		if which {
			ix := &index.Index{Entries: []index.Entry{fileEntry(p)}}
			return a.printWhich(ix, []match.Result{{Index: 0, Score: match.ScoreExact}})
		}
		return a.launch(nil, fileEntry(p), nil)
	}
	start := time.Now()
	paths, total, err := discover.SearchFiles(a.cfg.Everything, q, fileSearchLimit, a.warn)
	if errors.Is(err, discover.ErrEverythingNotRunning) {
		return fmt.Errorf("file search needs Everything (https://www.voidtools.com)")
	}
	if err != nil {
		return fmt.Errorf("everything: %w", err)
	}
	a.debug("file search: %d of %d results in %s", len(paths), total, time.Since(start).Round(time.Millisecond))
	if total > len(paths) && len(paths) >= fileSearchLimit {
		a.warn("%d files match, only the first %d (by name) are ranked; add a word to narrow it", total, len(paths))
	}
	if len(paths) == 0 {
		return &exitErr{code: exitNoMatch, msg: fmt.Sprintf("no file matches %q", q)}
	}
	ix := &index.Index{Entries: make([]index.Entry, len(paths))}
	cs := make([]match.Candidate, len(paths))
	for i, p := range paths {
		ix.Entries[i] = discover.FileEntry(p)
		cs[i] = match.Candidate{Name: ix.Entries[i].Name, Parent: ix.Entries[i].Parent}
	}
	if which {
		return a.printWhich(ix, match.Rank(q, cs))
	}
	return a.resolve(ix, match.Rank(q, cs), q, nil, func(e index.Entry, score float64) string {
		return fmt.Sprintf("%.2f  %s", score, e.Target)
	})
}

// resolve applies the decision to ranked results: launch the winner, ask
// with the picker, or print the closest entries.
func (a *app) resolve(ix *index.Index, rs []match.Result, q string, extra []string, label func(index.Entry, float64) string) error {
	d, rows := match.Decide(rs, a.cfg.MinScore, a.cfg.AmbiguityGap)
	if a.verbose {
		fmt.Fprintf(os.Stderr, "query %q -> %s\n", q, d)
		printRows(os.Stderr, ix, rs[:min(len(rs), match.PickerRows)])
	}

	switch d {
	case match.Launch:
		return a.launch(ix, ix.Entries[rows[0].Index], extra)
	case match.Pick:
		more := countAbove(rs, match.PickerMin) - len(rows)
		if !ui.Interactive() {
			printRows(os.Stdout, ix, rows)
			if more > 0 {
				fmt.Printf("... %d more, add a word to narrow\n", more)
			}
			return &exitErr{code: exitAmbiguous, msg: fmt.Sprintf("%q is ambiguous", q)}
		}
		labels := make([]string, len(rows))
		for i, r := range rows {
			labels[i] = label(ix.Entries[r.Index], r.Score)
		}
		title := fmt.Sprintf("Several matches for %q:", q)
		if more > 0 {
			title = fmt.Sprintf("Several matches for %q (%d more not shown, add a word to narrow):", q, more)
		}
		i, err := ui.Pick(title, labels)
		if err != nil {
			return err
		}
		if i < 0 {
			return &exitErr{code: exitNoMatch}
		}
		return a.launch(ix, ix.Entries[rows[i].Index], extra)
	default:
		fmt.Fprintf(os.Stderr, "no match for %q. Closest:\n", q)
		printRows(os.Stderr, ix, rows)
		return &exitErr{code: exitNoMatch}
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
	ix, err := a.build(true, nil)
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
	ix, err := a.loadIndex(a.refresh)
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
	if !ui.Interactive() {
		_, err := io.WriteString(os.Stdout, buf.String())
		if errors.Is(err, windows.ERROR_NO_DATA) || errors.Is(err, windows.ERROR_BROKEN_PIPE) {
			return nil // reader closed early, e.g. `ora list | Select -First 5`
		}
		return err
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	return ui.Page(lines[0], lines[1:])
}

func (a *app) runWhich(q string) error {
	ix, err := a.loadIndex(a.refresh)
	if err != nil {
		return err
	}
	if len(ix.Entries) == 0 {
		return &exitErr{code: exitNoMatch, msg: "index is empty"}
	}
	return a.printWhich(ix, a.rank(ix, q))
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
		return &exitErr{code: exitNoMatch}
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
