package classifier

import (
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/jinishshah00/sentinelflow/internal/shared"
)

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

type NB struct {
	alpha       float64
	labelCounts map[shared.Severity]int
	tokenCounts map[shared.Severity]map[string]int
	totalDocs   int
	vocab       map[string]struct{}
	totalTokens map[shared.Severity]int
}

// New creates a Naive Bayes classifier with Laplace smoothing alpha.
func New(alpha float64) *NB {
	return &NB{
		alpha:       alpha,
		labelCounts: make(map[shared.Severity]int),
		tokenCounts: map[shared.Severity]map[string]int{
			shared.SeverityLow:    {},
			shared.SeverityMedium: {},
			shared.SeverityHigh:   {},
		},
		vocab:       make(map[string]struct{}),
		totalTokens: make(map[shared.Severity]int),
	}
}

// Tokenize produces simple lowercase tokens from text.
func Tokenize(s string) []string {
	s = strings.ToLower(s)
	s = nonAlnum.ReplaceAllString(s, " ")
	fields := strings.Fields(s)
	return fields
}

func (nb *NB) addToken(y shared.Severity, tok string, count int) {
	if tok == "" {
		return
	}
	if _, ok := nb.tokenCounts[y][tok]; !ok {
		nb.tokenCounts[y][tok] = 0
	}
	nb.tokenCounts[y][tok] += count
	nb.totalTokens[y] += count
	nb.vocab[tok] = struct{}{}
}

// Train on labeled events (uses description, labels, event type, severity hint).
func (nb *NB) Train(data []shared.LabeledEvent) {
	for _, d := range data {
		nb.labelCounts[d.Y]++
		nb.totalDocs++

		// event_type
		for _, t := range Tokenize(d.EventType) {
			nb.addToken(d.Y, t, 1)
		}
		// description
		for _, t := range Tokenize(d.Description) {
			nb.addToken(d.Y, t, 1)
		}
		// labels
		for _, l := range d.Labels {
			for _, t := range Tokenize(l) {
				nb.addToken(d.Y, t, 1)
			}
		}
		// severity hint
		for _, t := range Tokenize(d.SeverityHint) {
			nb.addToken(d.Y, t, 1)
		}
	}
}

// Predict returns predicted severity, confidence (0..1), and top reason tokens.
func (nb *NB) Predict(ev shared.Event) (shared.Severity, float64, []string) {
	classes := []shared.Severity{shared.SeverityLow, shared.SeverityMedium, shared.SeverityHigh}

	// Build tokens for this event
	var toks []string
	toks = append(toks, Tokenize(ev.EventType)...)
	toks = append(toks, Tokenize(ev.Description)...)
	for _, l := range ev.Labels {
		toks = append(toks, Tokenize(l)...)
	}
	toks = append(toks, Tokenize(ev.SeverityHint)...)

	// Count frequencies
	freq := map[string]int{}
	for _, t := range toks {
		freq[t]++
	}

	// Compute log scores
	logScores := make(map[shared.Severity]float64)
	totalDocs := float64(nb.totalDocs)
	vocabSize := float64(len(nb.vocab))
	for _, c := range classes {
		// prior
		prior := (float64(nb.labelCounts[c]) + nb.alpha) / (totalDocs + nb.alpha*float64(len(classes)))
		score := math.Log(prior)

		// likelihood
		den := float64(nb.totalTokens[c]) + nb.alpha*vocabSize
		for tok, count := range freq {
			num := float64(nb.tokenCounts[c][tok]) + nb.alpha
			score += float64(count) * math.Log(num/den)
		}
		logScores[c] = score
	}

	// softmax for confidence
	maxLog := -math.MaxFloat64
	for _, c := range classes {
		if logScores[c] > maxLog {
			maxLog = logScores[c]
		}
	}
	sum := 0.0
	probs := make(map[shared.Severity]float64)
	for _, c := range classes {
		p := math.Exp(logScores[c] - maxLog)
		probs[c] = p
		sum += p
	}
	for _, c := range classes {
		probs[c] /= sum
	}

	// pick best
	best := classes[0]
	for _, c := range classes[1:] {
		if probs[c] > probs[best] {
			best = c
		}
	}

	// reasons: top tokens by P(tok|best)
	type kv struct {
		Tok string
		P   float64
	}
	var contrib []kv
	den := float64(nb.totalTokens[best]) + nb.alpha*vocabSize
	for tok := range freq {
		p := (float64(nb.tokenCounts[best][tok]) + nb.alpha) / den
		contrib = append(contrib, kv{Tok: tok, P: p})
	}
	sort.Slice(contrib, func(i, j int) bool { return contrib[i].P > contrib[j].P })
	reasons := []string{}
	for i := 0; i < len(contrib) && i < 3; i++ {
		reasons = append(reasons, contrib[i].Tok)
	}

	return best, probs[best], reasons
}
