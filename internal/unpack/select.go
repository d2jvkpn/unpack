package unpack

import (
	"fmt"
	"regexp"
	"strings"
)

func selectorMatches(selector string, name string) bool {
	var (
		pattern *regexp.Regexp
		err     error
	)

	if selector == name {
		return true
	}
	if strings.HasPrefix(name, selector+"/") {
		return true
	}
	pattern, err = globToRegexp(selector)
	if err != nil {
		return false
	}
	return pattern.MatchString(name)
}

// globToRegexp translates a shell-style glob into an anchored regexp. Unlike
// path.Match, '*' and '?' cross the '/' separator, matching the default
// wildcard behavior of unzip and (with --wildcards) GNU tar.
func globToRegexp(pattern string) (*regexp.Regexp, error) {
	var (
		builder strings.Builder
		runes   []rune
	)

	runes = []rune(pattern)
	builder.WriteString("^")
	for i := 0; i < len(runes); i++ {
		switch c := runes[i]; c {
		case '*':
			builder.WriteString(".*")
		case '?':
			builder.WriteString(".")
		case '[':
			var (
				end   int
				class string
			)

			end = i + 1
			if end < len(runes) && runes[end] == '!' {
				end++
			}
			for end < len(runes) && runes[end] != ']' {
				end++
			}
			if end >= len(runes) {
				builder.WriteString(regexp.QuoteMeta(string(c)))
				continue
			}
			class = string(runes[i+1 : end])
			if strings.HasPrefix(class, "!") {
				builder.WriteString("[^" + class[1:] + "]")
			} else {
				builder.WriteString("[" + class + "]")
			}
			i = end
		default:
			builder.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	builder.WriteString("$")
	return regexp.Compile(builder.String())
}

func filterPlans(plans []plannedEntry, selectors []string) ([]plannedEntry, error) {
	var (
		matched  []bool
		filtered []plannedEntry
	)

	matched = make([]bool, len(selectors))
	filtered = make([]plannedEntry, 0, len(plans))
	for _, plan := range plans {
		name := archiveMatchName(plan)
		for i, selector := range selectors {
			if selectorMatches(selector, name) {
				matched[i] = true
				filtered = append(filtered, plan)
				break
			}
		}
	}
	for i, selector := range selectors {
		if !matched[i] {
			return nil, fmt.Errorf("selector %q matched no entries", selector)
		}
	}
	return filtered, nil
}

func archiveMatchName(plan plannedEntry) string {
	if plan.TopLevelDir != "" && strings.HasPrefix(plan.ArchiveName, plan.TopLevelDir+"/") {
		return strings.TrimPrefix(plan.ArchiveName, plan.TopLevelDir+"/")
	}
	return plan.ArchiveName
}
