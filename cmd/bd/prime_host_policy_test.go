package main

import (
	"fmt"
	"strings"
	"testing"
)

// stubPrimeHostPolicy isolates the host-policy globals for one test.
func stubPrimeHostPolicy(t *testing.T, sessionClose, coreRules string, cfg map[string]string) {
	t.Helper()
	oldClose, oldRules, oldCfg := primeSessionClose, primeCoreRules, primeConfigString
	t.Cleanup(func() {
		primeSessionClose, primeCoreRules, primeConfigString = oldClose, oldRules, oldCfg
	})
	primeSessionClose, primeCoreRules = sessionClose, coreRules
	primeConfigString = func(key string) string { return cfg[key] }
}

func TestPrimeModeResolutionPrefersFlagThenConfigThenDefault(t *testing.T) {
	// Nothing set: historical behavior.
	stubPrimeHostPolicy(t, "", "", nil)
	if got := primeSessionCloseMode(); got != sessionCloseFull {
		t.Fatalf("default session-close = %q, want %q", got, sessionCloseFull)
	}
	if got := primeCoreRulesMode(); got != coreRulesDirective {
		t.Fatalf("default core-rules = %q, want %q", got, coreRulesDirective)
	}

	// Config applies when the flag is absent.
	stubPrimeHostPolicy(t, "", "", map[string]string{
		"prime.session-close": "brief",
		"prime.core-rules":    "advisory",
	})
	if got := primeSessionCloseMode(); got != sessionCloseBrief {
		t.Fatalf("config session-close = %q, want brief", got)
	}
	if got := primeCoreRulesMode(); got != coreRulesAdvisory {
		t.Fatalf("config core-rules = %q, want advisory", got)
	}

	// An explicit flag beats config.
	stubPrimeHostPolicy(t, "off", "directive", map[string]string{
		"prime.session-close": "brief",
		"prime.core-rules":    "advisory",
	})
	if got := primeSessionCloseMode(); got != sessionCloseOff {
		t.Fatalf("flag session-close = %q, want off", got)
	}
	if got := primeCoreRulesMode(); got != coreRulesDirective {
		t.Fatalf("flag core-rules = %q, want directive", got)
	}

	// Case and surrounding space are tolerated.
	stubPrimeHostPolicy(t, "  BRIEF ", " Advisory ", nil)
	if got := primeSessionCloseMode(); got != sessionCloseBrief {
		t.Fatalf("mixed-case flag not normalized: %q", got)
	}
	if got := primeCoreRulesMode(); got != coreRulesAdvisory {
		t.Fatalf("mixed-case flag not normalized: %q", got)
	}
}

// A typo must not cost the agent its context-recovery payload: prime runs from
// session-start hooks, so an unusable value falls back instead of failing.
func TestPrimeModeResolutionFallsBackOnGarbage(t *testing.T) {
	stubPrimeHostPolicy(t, "quiet", "shouty", map[string]string{
		"prime.session-close": "silent",
		"prime.core-rules":    "loud",
	})
	if got := primeSessionCloseMode(); got != sessionCloseFull {
		t.Fatalf("garbage session-close = %q, want full", got)
	}
	if got := primeCoreRulesMode(); got != coreRulesDirective {
		t.Fatalf("garbage core-rules = %q, want directive", got)
	}
}

func TestSessionCloseFullPreservesLegacyText(t *testing.T) {
	stubPrimeHostPolicy(t, "full", "", nil)
	checklist := "[ ] 1. bd close <id>"
	note := "**Note:** No git remote configured."

	// MCP payload: unfenced, no CRITICAL lead-in — as it has always been.
	legacyMCP := "# 🚨 SESSION CLOSE PROTOCOL 🚨\n\n" + checklist + "\n\n"
	if got := primeSessionCloseSection(checklist, "", false); got != legacyMCP {
		t.Fatalf("MCP full mode drifted from legacy:\n got %q\nwant %q", got, legacyMCP)
	}

	// CLI payload: CRITICAL lead-in, fenced checklist, trailing note.
	legacyCLI := "# 🚨 SESSION CLOSE PROTOCOL 🚨\n\n" +
		"**CRITICAL**: Before saying \"done\" or \"complete\", you MUST run this checklist:\n\n" +
		"```\n" + checklist + "\n```\n\n" + note + "\n\n"
	if got := primeSessionCloseSection(checklist, note, true); got != legacyCLI {
		t.Fatalf("CLI full mode drifted from legacy:\n got %q\nwant %q", got, legacyCLI)
	}
}

func TestSessionCloseBriefDropsTheShouting(t *testing.T) {
	stubPrimeHostPolicy(t, "brief", "", nil)
	checklist := "[ ] 1. bd close <id>"

	got := primeSessionCloseSection(checklist, "", true)

	for _, banned := range []string{"🚨", "CRITICAL", "you MUST"} {
		if strings.Contains(got, banned) {
			t.Fatalf("brief mode still contains %q: %q", banned, got)
		}
	}
	if !strings.Contains(got, checklist) {
		t.Fatalf("brief mode dropped the checklist itself: %q", got)
	}
	if !strings.Contains(got, "its own session-close discipline") {
		t.Fatalf("brief mode should defer to the host's own discipline: %q", got)
	}
	if !strings.HasSuffix(got, "\n\n") {
		t.Fatalf("section must end with a blank line so callers can concatenate: %q", got)
	}
}

func TestSessionCloseOffEmitsNothing(t *testing.T) {
	stubPrimeHostPolicy(t, "off", "", nil)
	if got := primeSessionCloseSection("[ ] 1. bd close <id>", "note", true); got != "" {
		t.Fatalf("off mode must emit nothing, got %q", got)
	}
}

func TestTrackingRulesDirectiveIsUnchanged(t *testing.T) {
	stubPrimeHostPolicy(t, "", "directive", nil)
	memory := "- **Memory**: Use `bd remember` for persistent knowledge. Do NOT use MEMORY.md files.\n"

	got := primeTrackingRules(memory)

	want := "- **Default**: Use beads for ALL task tracking (`bd create`, `bd ready`, `bd close`)\n" +
		"- **Prohibited**: Do NOT use TodoWrite, TaskCreate, or markdown files for task tracking\n" +
		memory
	if got != want {
		t.Fatalf("directive rules drifted:\n got %q\nwant %q", got, want)
	}
}

// Advisory mode exists so beads stops countermanding tools the host owns. It
// must still say when to reach for beads — the point is to trade a prohibition
// for a criterion, not to go silent.
func TestTrackingRulesAdvisoryDropsProhibitions(t *testing.T) {
	stubPrimeHostPolicy(t, "", "advisory", nil)

	got := primeTrackingRules("- **Memory**: Do NOT use MEMORY.md files.\n")

	for _, banned := range []string{"Do NOT", "Prohibited", "ALL task tracking", "TodoWrite", "MEMORY.md"} {
		if strings.Contains(got, banned) {
			t.Fatalf("advisory rules still contain %q: %q", banned, got)
		}
	}
	for _, wanted := range []string{"outlives this session", "has blockers", "another agent", "bd remember"} {
		if !strings.Contains(got, wanted) {
			t.Fatalf("advisory rules dropped the criterion %q: %q", wanted, got)
		}
	}
}

func TestOrderMemoryKeysAlphabeticalWithoutFocus(t *testing.T) {
	memories := map[string]string{"zebra": "z", "alpha": "a", "mid": "m"}

	got := orderMemoryKeys(memories, "")

	want := []string{"alpha", "mid", "zebra"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("without focus, order must stay alphabetical: got %v want %v", got, want)
		}
	}
}

func TestOrderMemoryKeysRanksFocusMatchesFirst(t *testing.T) {
	memories := map[string]string{
		"aaa-unrelated":       "nothing to do with the task",
		"zzz-vision-pipeline": "how the vision pipeline resolves variant images",
		"mmm-partial":         "mentions the pipeline once",
	}

	got := orderMemoryKeys(memories, "fix vision pipeline colorway binding")

	if got[0] != "zzz-vision-pipeline" {
		t.Fatalf("key+body match should rank first, got %v", got)
	}
	if got[1] != "mmm-partial" {
		t.Fatalf("body-only match should outrank a non-match, got %v", got)
	}
	if got[2] != "aaa-unrelated" {
		t.Fatalf("non-matching memory should sink despite sorting first alphabetically, got %v", got)
	}
}

func TestOrderMemoryKeysTiesStayAlphabetical(t *testing.T) {
	memories := map[string]string{"b-none": "x", "a-none": "y", "c-none": "z"}

	got := orderMemoryKeys(memories, "entirely unrelated focus text")

	want := []string{"a-none", "b-none", "c-none"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("equal scores must preserve alphabetical order: got %v want %v", got, want)
		}
	}
}

func TestFocusTokensDropsNoiseTokens(t *testing.T) {
	tokens := focusTokens("Fix the DB and a  UI, for #42")

	for _, dropped := range []string{"the", "and", "for", "db", "ui", "a"} {
		if _, ok := tokens[dropped]; ok {
			t.Fatalf("token %q should have been dropped: %v", dropped, tokens)
		}
	}
	if _, ok := tokens["fix"]; !ok {
		t.Fatalf("expected 'fix' to survive: %v", tokens)
	}
	if len(focusTokens("   ")) != 0 {
		t.Fatalf("blank focus must yield no tokens")
	}
}

// The headline behavior: under a cap, focus decides which memories survive.
// Alphabetical order would keep the irrelevant one.
func TestRenderPrimeMemoriesFocusDecidesWhatSurvivesACap(t *testing.T) {
	memories := map[string]string{
		"aaa-irrelevant": "unrelated background",
		"zzz-relevant":   "the deploy pipeline wedges when the worker is SIGTERMed",
	}

	capped := renderPrimeMemories(memories, false, 1, 0, "deploy pipeline wedge")

	if !strings.Contains(capped, "### zzz-relevant\n") {
		t.Fatalf("focused cap should keep the relevant memory: %q", capped)
	}
	if strings.Contains(capped, "### aaa-irrelevant\n") {
		t.Fatalf("focused cap kept the irrelevant memory: %q", capped)
	}
	if !strings.Contains(capped, "showing 1 of 2, by relevance") {
		t.Fatalf("header should report relevance ordering: %q", capped)
	}
	if !strings.Contains(capped, `ordered by relevance to "deploy pipeline wedge"`) {
		t.Fatalf("banner should name the focus so the elision is explicable: %q", capped)
	}

	// Same cap, no focus: alphabetical wins and the relevant memory is lost.
	unfocused := renderPrimeMemories(memories, false, 1, 0, "")
	if !strings.Contains(unfocused, "### aaa-irrelevant\n") {
		t.Fatalf("without focus the alphabetical first entry should survive: %q", unfocused)
	}
	if !strings.Contains(unfocused, "showing 1 of 2, alphabetical") {
		t.Fatalf("unfocused header must still say alphabetical: %q", unfocused)
	}
}

func TestPrimeFocusNoteTruncatesLongFocus(t *testing.T) {
	if got := primeFocusNote("   "); got != "" {
		t.Fatalf("blank focus should produce no note, got %q", got)
	}
	long := strings.Repeat("x", 200)
	got := primeFocusNote(long)
	if !strings.Contains(got, "…") {
		t.Fatalf("an overlong focus should be elided in the banner: %q", got)
	}
	if len(got) > 120 {
		t.Fatalf("focus note should stay short, got %d chars: %q", len(got), got)
	}
}

func TestPrimeMemoryElisionNoteCombinesCapAndFocus(t *testing.T) {
	got := primeMemoryElisionNote(3, 0, "vision pipeline")
	if !strings.Contains(got, "capped by max-memories=3") {
		t.Fatalf("elision note lost the cap: %q", got)
	}
	if !strings.Contains(got, "ordered by relevance") {
		t.Fatalf("elision note lost the focus: %q", got)
	}

	bare := primeMemoryElisionNote(3, 0, "")
	if strings.Contains(bare, "relevance") {
		t.Fatalf("no focus means no relevance clause: %q", bare)
	}
	if bare != fmt.Sprintf("capped by max-memories=%d", 3) {
		t.Fatalf("unfocused note drifted from the legacy cap note: %q", bare)
	}
}
