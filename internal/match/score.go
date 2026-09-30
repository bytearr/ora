package match

import (
	"sort"
	"strings"
)

const (
	ScoreExact         = 1.00
	ScoreAlias         = 0.95
	ScoreAliasMax      = 0.94
	ScorePrefix        = 0.90
	ScoreAbbrev        = 0.88
	ScoreToken         = 0.87
	ScoreParentToken   = 0.80
	PortablePenalty    = 0.80
	GenericPenalty     = 0.85
	FirstLetterPenalty = 0.90
	TokenFuzzyPenalty  = 0.90
	Decisive           = 0.95
	FuzzyGap           = 0.06
	RecentBump         = 0.03
	RecentCap          = 0.99
	PickerMin          = 0.55
	PickerRows         = 8
	NoMatchRows        = 5
)

type Candidate struct {
	Name     string
	Parent   string   // parent directory name, portable entries only
	Portable bool     // found by Everything, not installed
	Generic  bool     // stem occurs 3+ times among portable entries
	Recent   bool     // in the recent-launch list
	Aliases  []string // alias keys that point to this candidate
}

type Result struct {
	Index int
	Score float64
	// Fuzzy is true when the score comes from Jaro-Winkler rather than an
	// exact, alias, prefix, abbreviation or token match.
	Fuzzy bool
}

type query struct {
	norm    string
	compact string
	tokens  []string
}

func newQuery(s string) query {
	n := Normalize(s)
	return query{norm: n, compact: Compact(n), tokens: Tokens(n)}
}

type name struct {
	norm    string
	compact string
	tokens  []string
}

func newName(s string) name {
	n := Normalize(s)
	return name{norm: n, compact: Compact(n), tokens: Tokens(n)}
}

type score struct {
	v     float64
	fuzzy bool
}

// better returns the higher score; on a tie the structural one wins.
func better(a, b score) score {
	if b.v > a.v+eps || (b.v+eps >= a.v && a.fuzzy && !b.fuzzy) {
		return b
	}
	return a
}

func Score(q string, c Candidate) float64 {
	return scoreCandidate(newQuery(q), c).v
}

func scoreCandidate(q query, c Candidate) score {
	if q.compact == "" {
		return score{}
	}
	best := scoreName(q, newName(c.Name))
	if c.Portable && best.v < ScoreExact {
		best.v *= PortablePenalty
	}
	if c.Generic {
		best.v *= GenericPenalty
	}

	if c.Parent != "" {
		p := newName(c.Parent)
		for _, t := range append([]string{p.compact}, p.tokens...) {
			if t == q.norm || t == q.compact {
				best = better(best, score{v: ScoreParentToken})
			}
		}
		if len(q.tokens) >= 2 {
			best = better(best, scoreName(q, newName(c.Parent+" "+c.Name)))
			best = better(best, scoreName(q, newName(c.Name+" "+c.Parent)))
		}
	}

	for _, a := range c.Aliases {
		an := newName(a)
		if an.compact == "" {
			continue
		}
		if an.norm == q.norm || an.compact == q.compact {
			best = better(best, score{v: ScoreAlias})
			continue
		}
		s := scoreName(q, an)
		s.v = min(s.v, ScoreAliasMax)
		best = better(best, s)
	}

	if c.Recent {
		best.v = min(best.v+RecentBump, max(best.v, RecentCap))
	}
	return best
}

func scoreName(q query, n name) score {
	if n.compact == "" {
		return score{}
	}
	if q.norm == n.norm || q.compact == n.compact {
		return score{v: ScoreExact}
	}
	if strings.HasPrefix(n.norm, q.norm) || strings.HasPrefix(n.compact, q.compact) {
		return score{v: ScorePrefix}
	}
	if abbrev(q.compact, n.tokens) {
		return score{v: ScoreAbbrev}
	}
	for _, t := range n.tokens {
		if t == q.norm || t == q.compact {
			return score{v: ScoreToken}
		}
	}
	s := max(fuzzy(q.norm, n.norm), fuzzy(q.compact, n.compact))
	if len(n.tokens) > 1 {
		for _, t := range n.tokens {
			s = max(s, fuzzy(q.compact, t)*TokenFuzzyPenalty)
		}
	}
	return score{v: s, fuzzy: true}
}

// fuzzy is Jaro-Winkler with a penalty when the first letters differ and
// when one string is less than half as long as the other. Without them,
// unrelated names land around 0.8 and one-letter tokens around 0.7.
func fuzzy(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	s := JaroWinkler(a, b)
	if a[0] != b[0] {
		s *= FirstLetterPenalty
	}
	la, lb := float64(len([]rune(a))), float64(len([]rune(b)))
	if ratio := min(la, lb) / max(la, lb); ratio < 0.5 {
		s *= 2 * ratio
	}
	return s
}

// abbrev reports whether q splits into at least two non-empty parts that are
// prefixes of consecutive tokens, starting at the first token, with at least
// one part being a single initial. "vscod" = v|s|cod matches Visual Studio
// Code; "code" = co|de must not match Comfy Desktop.
func abbrev(q string, tokens []string) bool {
	if len(q) < 2 || len(tokens) < 2 {
		return false
	}
	var rec func(qi, ti, parts int, initial bool) bool
	rec = func(qi, ti, parts int, initial bool) bool {
		if qi == len(q) {
			return parts >= 2 && initial
		}
		if ti == len(tokens) {
			return false
		}
		t := tokens[ti]
		for k := min(len(t), len(q)-qi); k >= 1; k-- {
			if q[qi:qi+k] == t[:k] && rec(qi+k, ti+1, parts+1, initial || k == 1) {
				return true
			}
		}
		return false
	}
	return rec(0, 0, 0, false)
}

// Rank scores all candidates and returns them sorted by descending score.
// Ties keep the input order, which is the index source priority.
func Rank(q string, cs []Candidate) []Result {
	qq := newQuery(q)
	rs := make([]Result, 0, len(cs))
	for i, c := range cs {
		s := scoreCandidate(qq, c)
		rs = append(rs, Result{Index: i, Score: s.v, Fuzzy: s.fuzzy})
	}
	sort.SliceStable(rs, func(i, j int) bool { return rs[i].Score > rs[j].Score })
	return rs
}

type Decision int

const (
	NoMatch Decision = iota
	Launch
	Pick
)

func (d Decision) String() string {
	switch d {
	case Launch:
		return "launch"
	case Pick:
		return "picker"
	default:
		return "no match"
	}
}

const eps = 1e-9

// Decide returns the decision and the rows that go with it: one row for
// Launch, up to PickerRows for Pick, up to NoMatchRows for NoMatch.
func Decide(rs []Result, minScore, gap float64) (Decision, []Result) {
	if len(rs) == 0 {
		return NoMatch, nil
	}
	if launchable(rs, minScore, gap) {
		return Launch, rs[:1]
	}
	var rows []Result
	for _, r := range rs {
		if r.Score+eps < PickerMin || len(rows) == PickerRows {
			break
		}
		rows = append(rows, r)
	}
	if len(rows) > 0 {
		return Pick, rows
	}
	return NoMatch, rs[:min(NoMatchRows, len(rs))]
}

func launchable(rs []Result, minScore, gap float64) bool {
	best := rs[0]
	if best.Score+eps < minScore {
		return false
	}
	var second, secondStructural, secondFuzzy float64
	for _, r := range rs[1:] {
		second = max(second, r.Score)
		if r.Fuzzy {
			secondFuzzy = max(secondFuzzy, r.Score)
		} else {
			secondStructural = max(secondStructural, r.Score)
		}
	}
	switch {
	case best.Score-second+eps >= gap:
		return true
	case best.Score+eps >= Decisive && best.Score-second > eps:
		return true
	case !best.Fuzzy:
		// A structural match only needs half the gap against fuzzy noise.
		return best.Score-secondStructural+eps >= gap && best.Score-secondFuzzy+eps >= FuzzyGap
	}
	return false
}
