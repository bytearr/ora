package match

func jaro(a, b []rune) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	window := max(len(a), len(b))/2 - 1
	if window < 0 {
		window = 0
	}
	am := make([]bool, len(a))
	bm := make([]bool, len(b))
	matches := 0
	for i := range a {
		lo := max(0, i-window)
		hi := min(len(b)-1, i+window)
		for j := lo; j <= hi; j++ {
			if !bm[j] && a[i] == b[j] {
				am[i], bm[j] = true, true
				matches++
				break
			}
		}
	}
	if matches == 0 {
		return 0
	}
	transpositions := 0
	k := 0
	for i := range a {
		if !am[i] {
			continue
		}
		for !bm[k] {
			k++
		}
		if a[i] != b[k] {
			transpositions++
		}
		k++
	}
	m := float64(matches)
	return (m/float64(len(a)) + m/float64(len(b)) + (m-float64(transpositions)/2)/m) / 3
}

func JaroWinkler(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	j := jaro(ra, rb)
	prefix := 0
	for prefix < min(4, len(ra), len(rb)) && ra[prefix] == rb[prefix] {
		prefix++
	}
	return j + float64(prefix)*0.1*(1-j)
}
