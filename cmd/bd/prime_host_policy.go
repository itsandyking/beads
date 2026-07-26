package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/steveyegge/beads/internal/config"
)

// Host-policy knobs for `bd prime`.
//
// Two of the sections prime injects are not statements about beads — they are
// statements about how the *host* should run a session: the session-close
// protocol, and the rule prohibiting the host's own task-tracking tools. On a
// host that supplies neither (a small local model, an older agent), those
// sections are load-bearing and stay on by default. On a host that already has
// its own close discipline and its own judgment about tool choice, they are a
// second voice giving orders, and duplicated instructions cost more than they
// buy.
//
// Both knobs default to the historical text. Nothing changes unless asked.

const (
	sessionCloseFull  = "full"
	sessionCloseBrief = "brief"
	sessionCloseOff   = "off"

	coreRulesDirective = "directive"
	coreRulesAdvisory  = "advisory"
)

// primeConfigString reads a string config key (stubbable for tests).
var primeConfigString = func(key string) string {
	return config.GetString(key)
}

// primeSessionCloseMode resolves the session-close verbosity: the --session-close
// flag wins, then the prime.session-close config key, then "full".
func primeSessionCloseMode() string {
	return resolvePrimeMode(primeSessionClose, "prime.session-close", sessionCloseFull,
		sessionCloseFull, sessionCloseBrief, sessionCloseOff)
}

// primeCoreRulesMode resolves the core-rules register: the --core-rules flag
// wins, then the prime.core-rules config key, then "directive".
func primeCoreRulesMode() string {
	return resolvePrimeMode(primeCoreRules, "prime.core-rules", coreRulesDirective,
		coreRulesDirective, coreRulesAdvisory)
}

// resolvePrimeMode picks flag, then config, then fallback, accepting only the
// listed values. An unrecognized value falls back rather than erroring: prime
// runs from session-start hooks, where a hard failure would cost the agent its
// whole context-recovery payload over a typo in a config file.
func resolvePrimeMode(flagVal, configKey, fallback string, valid ...string) string {
	for _, candidate := range []string{flagVal, primeConfigString(configKey)} {
		got := strings.ToLower(strings.TrimSpace(candidate))
		if got == "" {
			continue
		}
		for _, v := range valid {
			if got == v {
				return v
			}
		}
	}
	return fallback
}

// primeSessionCloseSection renders the close protocol at the resolved verbosity.
// checklist is the already-computed, context-aware list of close steps; note is
// an optional trailing caveat (empty in MCP mode). fenced selects the CLI
// payload's framing — a "CRITICAL / you MUST" lead-in and a fenced checklist —
// which the terser MCP payload has never carried. The returned string is either
// empty or ends in a blank line, so callers can concatenate unconditionally.
//
// In full mode the output matches what beads emitted before these knobs
// existed, for both payloads; the one difference is that an empty note no
// longer leaves a run of blank lines behind it.
func primeSessionCloseSection(checklist, note string, fenced bool) string {
	writeChecklist := func(sb *strings.Builder) {
		if fenced {
			sb.WriteString("```\n" + checklist + "\n```\n")
		} else {
			sb.WriteString(checklist + "\n")
		}
		if strings.TrimSpace(note) != "" {
			sb.WriteString("\n" + strings.TrimSpace(note) + "\n")
		}
	}

	var sb strings.Builder
	switch primeSessionCloseMode() {
	case sessionCloseOff:
		return ""
	case sessionCloseBrief:
		sb.WriteString("## Session close (beads)\n\n")
		sb.WriteString("Before reporting work complete:\n\n")
		writeChecklist(&sb)
		sb.WriteString("\nWhere your host has its own session-close discipline, follow that; this is the beads half of it.\n\n")
		return sb.String()
	default:
		sb.WriteString("# 🚨 SESSION CLOSE PROTOCOL 🚨\n\n")
		if fenced {
			sb.WriteString("**CRITICAL**: Before saying \"done\" or \"complete\", you MUST run this checklist:\n\n")
		}
		writeChecklist(&sb)
		sb.WriteString("\n")
		return sb.String()
	}
}

// primeTrackingRules renders the task-tracking and memory bullets of Core Rules.
// memoryDirective is the caller's own wording for the memory bullet, which
// differs between the MCP and CLI payloads; it is used only in directive mode.
func primeTrackingRules(memoryDirective string) string {
	if primeCoreRulesMode() == coreRulesAdvisory {
		return "- **Scope**: Track work in beads when it outlives this session, has blockers, or another agent may touch it — a session-scoped todo list holds none of those. Scratch tracking that dies with the conversation can stay in your host's own tool\n" +
			"- **Memory**: `bd remember` is for knowledge that must survive the session and reach other machines; knowledge your host already persists for you does not need a second home\n"
	}
	return "- **Default**: Use beads for ALL task tracking (`bd create`, `bd ready`, `bd close`)\n" +
		"- **Prohibited**: Do NOT use TodoWrite, TaskCreate, or markdown files for task tracking\n" +
		memoryDirective
}

// orderMemoryKeys returns memory keys in the order prime should emit them.
// With no focus text the order is alphabetical, which is what beads has always
// done and the only stable order available (the memory store keeps no
// timestamps). With focus text — an active bead's title, a branch name, the
// task at hand — keys are ordered by relevance to that text first.
//
// Ordering only matters when a cap is active, since an uncapped prime emits
// everything regardless. But that is exactly when it matters most: an
// alphabetical cap keeps whichever memories happen to sort first, which has
// nothing to do with the work in front of the agent.
func orderMemoryKeys(memories map[string]string, focus string) []string {
	keys := make([]string, 0, len(memories))
	for k := range memories {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	tokens := focusTokens(focus)
	if len(tokens) == 0 {
		return keys
	}

	scores := make(map[string]int, len(keys))
	for _, k := range keys {
		scores[k] = memoryRelevance(k, memories[k], tokens)
	}
	// Stable sort over the already-alphabetical slice: equal scores keep
	// alphabetical order, so the result is deterministic.
	sort.SliceStable(keys, func(i, j int) bool {
		return scores[keys[i]] > scores[keys[j]]
	})
	return keys
}

// memoryRelevance scores one memory against the focus tokens. A token hit in
// the key counts more than one in the body: keys are curated slugs, so a match
// there is a deliberate signal, while a body match can be incidental.
func memoryRelevance(key, body string, tokens map[string]struct{}) int {
	keyText := strings.ToLower(strings.ReplaceAll(key, "-", " "))
	bodyText := strings.ToLower(body)
	score := 0
	for tok := range tokens {
		if strings.Contains(keyText, tok) {
			score += 3
		}
		if strings.Contains(bodyText, tok) {
			score++
		}
	}
	return score
}

// focusTokens splits focus text into distinct lowercase tokens worth matching.
// Tokens shorter than three characters are dropped (they match everything), as
// are a few connectives that carry no signal in a bead title or branch name.
func focusTokens(focus string) map[string]struct{} {
	if strings.TrimSpace(focus) == "" {
		return nil
	}
	skip := map[string]struct{}{
		"the": {}, "and": {}, "for": {}, "with": {}, "from": {}, "into": {},
		"that": {}, "this": {}, "when": {}, "then": {}, "than": {}, "was": {},
		"are": {}, "not": {}, "but": {}, "its": {}, "our": {}, "all": {},
	}
	tokens := make(map[string]struct{})
	for _, field := range strings.FieldsFunc(strings.ToLower(focus), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	}) {
		if len(field) < 3 {
			continue
		}
		if _, bad := skip[field]; bad {
			continue
		}
		tokens[field] = struct{}{}
	}
	return tokens
}

// primeFocusNote describes an active focus for the elision banner, so a capped
// prime says why these memories and not others.
func primeFocusNote(focus string) string {
	trimmed := strings.TrimSpace(focus)
	if trimmed == "" {
		return ""
	}
	const maxLen = 60
	if len(trimmed) > maxLen {
		trimmed = trimmed[:maxLen] + "…"
	}
	return fmt.Sprintf("ordered by relevance to %q", trimmed)
}
