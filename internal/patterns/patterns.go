package patterns

import (
	"regexp"
	"strconv"
	"sync"

	"github.com/noamsto/resolved/internal/model"
)

var (
	urlRe   = regexp.MustCompile(`https?://github\.com/([\w.-]+)/([\w.-]+)/(issues|pull)/(\d+)`)
	shortRe = regexp.MustCompile(`\b([\w.-]+)/([\w.-]+)#(\d+)\b`)
	bareRe  = regexp.MustCompile(`#([1-9]\d*)\b`)
)

// Match is a reference found in a single piece of comment text. Col is the
// 0-based byte offset of the match within that text.
type Match struct {
	Kind       model.RefKind
	Owner      string
	Repo       string
	Number     int
	Type       model.RefType
	Col        int
	Raw        string
	Confidence model.Confidence
}

// Extract finds GitHub references in comment text. Full URLs win over short
// forms, which win over bare #n; overlapping spans are not double-counted.
// originOwner/originRepo resolve bare #n; if empty, bare refs are skipped.
func Extract(text, originOwner, originRepo string) []Match {
	consumed := make([]bool, len(text))
	free := func(s, e int) bool {
		for i := s; i < e && i < len(text); i++ {
			if consumed[i] {
				return false
			}
		}
		return true
	}
	mark := func(s, e int) {
		for i := s; i < e && i < len(text); i++ {
			consumed[i] = true
		}
	}

	var out []Match

	for _, m := range urlRe.FindAllStringSubmatchIndex(text, -1) {
		n, _ := strconv.Atoi(text[m[8]:m[9]])
		typ := model.TypeIssue
		if text[m[6]:m[7]] == "pull" {
			typ = model.TypePR
		}
		out = append(out, Match{
			Kind: model.KindURL, Owner: text[m[2]:m[3]], Repo: text[m[4]:m[5]],
			Number: n, Type: typ, Col: m[0], Raw: text[m[0]:m[1]], Confidence: model.ConfHigh,
		})
		mark(m[0], m[1])
	}

	for _, m := range shortRe.FindAllStringSubmatchIndex(text, -1) {
		if !free(m[0], m[1]) {
			continue
		}
		n, _ := strconv.Atoi(text[m[6]:m[7]])
		out = append(out, Match{
			Kind: model.KindShortForm, Owner: text[m[2]:m[3]], Repo: text[m[4]:m[5]],
			Number: n, Type: model.TypeIssue, Col: m[0], Raw: text[m[0]:m[1]], Confidence: model.ConfHigh,
		})
		mark(m[0], m[1])
	}

	if originOwner != "" && originRepo != "" {
		for _, m := range bareRe.FindAllStringSubmatchIndex(text, -1) {
			// Standalone tokens only: a real issue ref is written like
			// "TODO(#42)", "see #42", or "[#42]" — not glued into a word,
			// hex color, or format pattern.
			if m[0] > 0 {
				switch text[m[0]-1] {
				case ' ', '\t', '(', '[':
				default:
					continue
				}
			}
			if !free(m[0], m[1]) {
				continue
			}
			n, _ := strconv.Atoi(text[m[2]:m[3]])
			out = append(out, Match{
				Kind: model.KindBare, Owner: originOwner, Repo: originRepo,
				Number: n, Type: model.TypeIssue, Col: m[0], Raw: text[m[0]:m[1]], Confidence: model.ConfHigh,
			})
			mark(m[0], m[1])
		}
	}

	return out
}

var keywordReCache sync.Map // keyword string -> *regexp.Regexp

func keywordRe(kw string) *regexp.Regexp {
	if v, ok := keywordReCache.Load(kw); ok {
		return v.(*regexp.Regexp)
	}
	re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(kw) + `\b`)
	keywordReCache.Store(kw, re)
	return re
}

// DetectKeyword returns the first keyword (whole-word, case-insensitive) found
// in text, or "" if none.
func DetectKeyword(text string, keywords []string) string {
	for _, kw := range keywords {
		if keywordRe(kw).MatchString(text) {
			return kw
		}
	}
	return ""
}
