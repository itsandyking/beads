package agents

import (
	"strings"
	"testing"
)

func TestParseProfile(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Profile
	}{
		{"full", ProfileFull},
		{"minimal", ProfileMinimal},
		{"pointer", ProfilePointer},
		{"  Pointer  ", ProfilePointer},
		{"MINIMAL", ProfileMinimal},
	} {
		got, err := ParseProfile(tc.in)
		if err != nil {
			t.Fatalf("ParseProfile(%q) returned error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("ParseProfile(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	for _, bad := range []string{"", "tiny", "none", "point"} {
		if _, err := ParseProfile(bad); err == nil {
			t.Fatalf("ParseProfile(%q) accepted an unknown profile", bad)
		}
	}
}

func TestProfileRankOrdersByContentVolume(t *testing.T) {
	if !(ProfileRank(ProfilePointer) < ProfileRank(ProfileMinimal) &&
		ProfileRank(ProfileMinimal) < ProfileRank(ProfileFull)) {
		t.Fatalf("expected pointer < minimal < full, got %d/%d/%d",
			ProfileRank(ProfilePointer), ProfileRank(ProfileMinimal), ProfileRank(ProfileFull))
	}
	// An unknown profile must rank as full so unrecognized markers are treated
	// as high-information and never silently replaced with less.
	if ProfileRank(Profile("legacy")) != ProfileRank(ProfileFull) {
		t.Fatalf("unknown profile must rank as full")
	}
}

func TestPointerProfileRendersWithMarkers(t *testing.T) {
	section := RenderSection(ProfilePointer)

	if !strings.Contains(section, "profile:pointer") {
		t.Fatalf("pointer section must record its profile in the marker: %q", section)
	}
	if !strings.HasPrefix(section, "<!-- BEGIN BEADS INTEGRATION") {
		t.Fatalf("pointer section must open with the begin marker: %q", section)
	}
	if !strings.Contains(section, "<!-- END BEADS INTEGRATION -->") {
		t.Fatalf("pointer section must close with the end marker: %q", section)
	}
	if !strings.Contains(section, "bd prime") {
		t.Fatalf("pointer section must point at bd prime: %q", section)
	}
	for _, cmd := range []string{"bd ready", "bd show", "--claim", "bd close", "bd create"} {
		if !strings.Contains(section, cmd) {
			t.Fatalf("pointer section dropped essential command %q: %s", cmd, section)
		}
	}
}

func TestPointerProfileIsSmallestProfile(t *testing.T) {
	pointer := len(templateBody(ProfilePointer))
	minimal := len(templateBody(ProfileMinimal))
	full := len(templateBody(ProfileFull))

	if pointer >= minimal || minimal >= full {
		t.Fatalf("expected pointer < minimal < full by size, got %d/%d/%d", pointer, minimal, full)
	}
	// The profile only earns its keep if it is substantially leaner, not
	// marginally so. Minimal is ~2.7KB today; pointer should be well under half.
	if pointer > minimal/2 {
		t.Fatalf("pointer profile (%d bytes) is not materially smaller than minimal (%d bytes)", pointer, minimal)
	}
}

// TestPointerProfileDefersHostPolicy pins the reason this profile exists: it
// describes beads and stops. Session-close choreography and prohibitions on the
// host's own tools belong to the host, and duplicating them here is what the
// profile is meant to avoid.
func TestPointerProfileDefersHostPolicy(t *testing.T) {
	body := templateBody(ProfilePointer)

	for _, banned := range []string{
		"SESSION CLOSE",
		"Session Completion",
		"git push",
		"Do NOT",
		"do NOT",
		"Prohibited",
		"ALL task tracking",
		"CRITICAL",
		"MUST",
	} {
		if strings.Contains(body, banned) {
			t.Fatalf("pointer profile must not carry host policy, found %q in:\n%s", banned, body)
		}
	}
}

func TestPointerProfileRoundTripsThroughReplaceSection(t *testing.T) {
	content := "# Project\n\n" + RenderSection(ProfileFull) + "\n\ntail\n"

	replaced, changed, err := ReplaceSection(content, ProfilePointer)
	if err != nil {
		t.Fatalf("ReplaceSection returned error: %v", err)
	}
	if !changed {
		t.Fatalf("replacing full with pointer should report a change")
	}
	if !strings.Contains(replaced, "profile:pointer") {
		t.Fatalf("replaced content should carry the pointer marker: %q", replaced)
	}
	if strings.Contains(replaced, "profile:full") {
		t.Fatalf("replaced content still carries the full marker: %q", replaced)
	}
	if !strings.HasPrefix(replaced, "# Project\n") || !strings.HasSuffix(replaced, "tail\n") {
		t.Fatalf("surrounding content was not preserved: %q", replaced)
	}

	// Re-rendering the same profile is a no-op.
	again, changedAgain, err := ReplaceSection(replaced, ProfilePointer)
	if err != nil {
		t.Fatalf("second ReplaceSection returned error: %v", err)
	}
	if changedAgain {
		t.Fatalf("re-rendering an up-to-date pointer section should not report a change")
	}
	if again != replaced {
		t.Fatalf("re-rendering mutated content")
	}
}
