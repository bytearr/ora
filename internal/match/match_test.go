package match

import "testing"

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"Visual Studio Code":          "visual studio code",
		"Mozilla Firefox (x64 en-US)": "mozilla firefox en us",
		"7-Zip 64-bit":                "7 zip",
		"Microsoft® Word™":            "microsoft word",
		"PixelPaintStudio":            "pixel paint studio",
		"Python312":                   "python 312",
		"Notepad++":                   "notepad",
		"HTTPServer":                  "http server",
		"  OBS   Studio  ":            "obs studio",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestJaroWinkler(t *testing.T) {
	if s := JaroWinkler("discrod", "discord"); s < 0.9 {
		t.Errorf("discrod/discord = %.3f, want >= 0.9", s)
	}
	if s := JaroWinkler("abc", "abc"); s != 1 {
		t.Errorf("identical = %.3f", s)
	}
	if s := JaroWinkler("abc", "xyz"); s != 0 {
		t.Errorf("disjoint = %.3f", s)
	}
}

func TestScoreLevels(t *testing.T) {
	cases := []struct {
		q    string
		c    Candidate
		want float64
	}{
		{"discord", Candidate{Name: "Discord"}, ScoreExact},
		{"pixelpaintstudio", Candidate{Name: "Pixel Paint Studio"}, ScoreExact},
		{"pixel paint", Candidate{Name: "Pixel Paint Studio"}, ScorePrefix},
		{"vscod", Candidate{Name: "Visual Studio Code"}, ScoreAbbrev},
		{"vsc", Candidate{Name: "Visual Studio Code"}, ScoreAbbrev},
		{"studio", Candidate{Name: "OBS Studio"}, ScoreToken},
		{"code", Candidate{Name: "Visual Studio Code", Aliases: []string{"code"}}, ScoreAlias},
		{"python", Candidate{Name: "python", Generic: true}, ScoreExact * GenericPenalty},
		{"foo", Candidate{Name: "run", Parent: "Foo"}, ScoreParentToken},
		{"pixel paint", Candidate{Name: "Pixel Paint", Portable: true}, ScoreExact},
		{"pixel pai", Candidate{Name: "Pixel Paint", Portable: true}, ScorePrefix * PortablePenalty},
		{"discord", Candidate{Name: "Discord", Recent: true}, ScoreExact},
		{"discor", Candidate{Name: "Discord", Recent: true}, ScorePrefix + RecentBump},
	}
	for _, c := range cases {
		got := Score(c.q, c.c)
		if diff := got - c.want; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("Score(%q, %q) = %.4f, want %.4f", c.q, c.c.Name, got, c.want)
		}
	}
}

func TestFuzzyAliasCapped(t *testing.T) {
	s := Score("vscod", Candidate{Name: "Something Else", Aliases: []string{"vscode"}})
	if s > ScoreAliasMax+1e-9 || s < 0.9 {
		t.Errorf("fuzzy alias = %.4f, want in [0.9, %.2f]", s, ScoreAliasMax)
	}
}

func TestRecentCannotPassExact(t *testing.T) {
	s := Score("discor", Candidate{Name: "Discordx", Recent: true, Aliases: []string{"discor"}})
	if s >= ScoreExact {
		t.Errorf("recent alias = %.4f, must stay below exact", s)
	}
}

func TestGenericWithParentWord(t *testing.T) {
	s := Score("python 312", Candidate{Name: "python", Parent: "Python312", Generic: true})
	if s < ScorePrefix {
		t.Errorf("python 312 = %.4f, want >= %.2f", s, ScorePrefix)
	}
}

func TestDecide(t *testing.T) {
	const minScore, gap = 0.86, 0.12
	d, rows := Decide([]Result{{0, 1.0, false}, {1, 0.7, false}}, minScore, gap)
	if d != Launch || len(rows) != 1 || rows[0].Index != 0 {
		t.Errorf("clear winner: %v %v", d, rows)
	}
	d, _ = Decide([]Result{{0, 0.9, false}}, minScore, gap)
	if d != Launch {
		t.Errorf("single candidate: %v", d)
	}
	d, rows = Decide([]Result{{0, 0.9, false}, {1, 0.85, false}, {2, 0.4, false}}, minScore, gap)
	if d != Pick || len(rows) != 2 {
		t.Errorf("ambiguous: %v %v", d, rows)
	}
	d, rows = Decide([]Result{{0, 0.8, false}, {1, 0.1, false}}, minScore, gap)
	if d != Pick || len(rows) != 1 {
		t.Errorf("below min: %v %v", d, rows)
	}
	many := make([]Result, 12)
	for i := range many {
		many[i] = Result{i, 0.7, false}
	}
	if _, rows = Decide(many, minScore, gap); len(rows) != PickerRows {
		t.Errorf("picker rows = %d", len(rows))
	}
	low := []Result{{0, 0.5, false}, {1, 0.4, false}, {2, 0.3, false}, {3, 0.2, false}, {4, 0.1, false}, {5, 0.05, false}}
	d, rows = Decide(low, minScore, gap)
	if d != NoMatch || len(rows) != NoMatchRows {
		t.Errorf("no match: %v %v", d, rows)
	}
	if d, _ = Decide(nil, minScore, gap); d != NoMatch {
		t.Errorf("empty: %v", d)
	}
	d, _ = Decide([]Result{{0, 0.94, false}, {1, 0.82, false}}, minScore, gap)
	if d != Launch {
		t.Errorf("gap exactly 0.12 must launch: %v", d)
	}
	d, _ = Decide([]Result{{0, 0.90, false}, {1, 0.85, false}}, minScore, gap)
	if d != Pick {
		t.Errorf("small gap below decisive must pick: %v", d)
	}
	d, _ = Decide([]Result{{0, 1.0, false}, {1, 0.9, false}}, minScore, gap)
	if d != Launch {
		t.Errorf("unique exact over prefix must launch: %v", d)
	}
	d, _ = Decide([]Result{{0, 1.0, false}, {1, 1.0, false}}, minScore, gap)
	if d != Pick {
		t.Errorf("two exact must pick: %v", d)
	}
	d, _ = Decide([]Result{{0, 0.87, false}, {1, 0.81, true}}, minScore, gap)
	if d != Launch {
		t.Errorf("structural over fuzzy with half gap must launch: %v", d)
	}
	d, _ = Decide([]Result{{0, 0.87, false}, {1, 0.82, true}}, minScore, gap)
	if d != Pick {
		t.Errorf("fuzzy runner-up closer than FuzzyGap must pick: %v", d)
	}
	d, _ = Decide([]Result{{0, 0.90, true}, {1, 0.80, true}}, minScore, gap)
	if d != Pick {
		t.Errorf("fuzzy winner keeps the full gap: %v", d)
	}
}

func TestRankScenarios(t *testing.T) {
	const minScore, gap = 0.86, 0.12
	apps := []Candidate{
		{Name: "Discord"},
		{Name: "Discord PTB"},
		{Name: "Visual Studio Code"},
		{Name: "Visual Studio Installer"},
		{Name: "Pixel Paint Studio", Parent: "PixelPaintStudio", Portable: true},
		{Name: "python", Parent: "Python312", Portable: true, Generic: true},
		{Name: "python", Parent: "Python311", Portable: true, Generic: true},
		{Name: "Steam"},
		{Name: "Google Chrome"},
		{Name: "chromedriver", Parent: "Translator++", Portable: true},
		{Name: "lldb-vscode", Parent: "bin", Portable: true},
		{Name: "CPU-Z MSI"},
		{Name: "7z", Parent: "tools", Portable: true},
		{Name: "PyCharm Community Edition"},
		{Name: "Comfy Desktop"},
	}
	if s := Score("code", Candidate{Name: "Comfy Desktop"}); s >= ScoreToken {
		t.Errorf("code vs Comfy Desktop = %.3f, abbreviation needs an initial", s)
	}
	launch := map[string]string{
		"discord":          "Discord",
		"discrod":          "Discord",
		"vscod":            "Visual Studio Code",
		"pixelpaintstudio": "Pixel Paint Studio",
		"steam":            "Steam",
		"chrome":           "Google Chrome",
	}
	for q, want := range launch {
		rs := Rank(q, apps)
		d, rows := Decide(rs, minScore, gap)
		if d != Launch || apps[rows[0].Index].Name != want {
			t.Errorf("%q: decision %v, top %q (%.3f), want launch %q", q, d, apps[rs[0].Index].Name, rs[0].Score, want)
		}
	}
	if d, _ := Decide(Rank("python", apps), minScore, gap); d == Launch {
		t.Errorf("python must not auto-launch a random copy")
	}
	if d, _ := Decide(Rank("zzzzqqq", apps), minScore, gap); d != NoMatch {
		t.Errorf("garbage query: %v", d)
	}
}

// Candidates as built by `ora file` (name keeps the extension).
func TestRankFiles(t *testing.T) {
	const minScore, gap = 0.86, 0.12
	one := []Candidate{
		{Name: "Project Notes.cfg", Parent: "Configs"},
		{Name: "Project Notes Backup.txt", Parent: "Documents"},
	}
	d, rows := Decide(Rank("projectnotes.cfg", one), minScore, gap)
	if d != Launch || one[rows[0].Index].Parent != "Configs" {
		t.Errorf("exact file name: %v %v", d, rows)
	}
	copies := []Candidate{
		{Name: "Project Notes.cfg", Parent: "Configs"},
		{Name: "Project Notes.cfg", Parent: "Downloads"},
	}
	if d, _ := Decide(Rank("projectnotes", copies), minScore, gap); d != Pick {
		t.Errorf("identical names in two folders must ask: %v", d)
	}
	d, rows = Decide(Rank("downloads projectnotes", copies), minScore, gap)
	if d == NoMatch || copies[rows[0].Index].Parent != "Downloads" {
		t.Errorf("folder word must rank that copy first: %v %v", d, rows)
	}
}
