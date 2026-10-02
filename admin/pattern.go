package admin

import (
	"regexp"
	"regexp/syntax"
	"strings"
)

// portablePattern accepts printable ASCII literals, classes/ranges, captures,
// alternation, greedy quantifiers and anchors. No wildcard, flags, shorthand
// classes, lookaround, backreferences or engine-specific escapes. See README.
var portableRepeat = regexp.MustCompile(`^\{[0-9]+(,[0-9]*)?\}$`)

func portablePattern(pattern string) (*regexp.Regexp, error) {
	if strings.Contains(pattern, "[:") {
		return nil, invalidDefinition()
	}
	inClass := false
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		if c < 32 || c > 126 {
			return nil, invalidDefinition()
		}
		if c == '\\' {
			i++
			if i >= len(pattern) || !strings.ContainsRune(`\.^$|?*+()[]{}`, rune(pattern[i])) {
				return nil, invalidDefinition()
			}
			continue
		}
		if c == '[' {
			if inClass {
				return nil, invalidDefinition()
			}
			inClass = true
		} else if c == ']' {
			if !inClass {
				return nil, invalidDefinition()
			}
			inClass = false
		}
		if !inClass && c == '{' {
			end := strings.IndexByte(pattern[i:], '}')
			if i == 0 || end < 0 || !portableRepeat.MatchString(pattern[i:i+end+1]) {
				return nil, invalidDefinition()
			}
			i += end
			continue
		}
		if !inClass && c == '}' {
			return nil, invalidDefinition()
		}
		if !inClass && ((c == '.') || (c == '(' && i+1 < len(pattern) && pattern[i+1] == '?') || (c == '?' && i > 0 && strings.ContainsRune("*+?}", rune(pattern[i-1])))) {
			return nil, invalidDefinition()
		}
	}
	tree, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return nil, invalidDefinition()
	}
	var supported func(*syntax.Regexp) bool
	supported = func(r *syntax.Regexp) bool {
		switch r.Op {
		case syntax.OpAnyChar, syntax.OpAnyCharNotNL, syntax.OpWordBoundary, syntax.OpNoWordBoundary, syntax.OpBeginLine, syntax.OpEndLine:
			return false
		}
		for _, sub := range r.Sub {
			if !supported(sub) {
				return false
			}
		}
		return true
	}
	if !supported(tree) {
		return nil, invalidDefinition()
	}
	return regexp.Compile(pattern)
}
