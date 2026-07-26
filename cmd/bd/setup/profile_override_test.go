package setup

import (
	"os"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/templates/agents"
)

// withProfileOverride sets the --profile override for one test and restores it.
func withProfileOverride(t *testing.T, p agents.Profile) {
	t.Helper()
	old := profileOverride
	t.Cleanup(func() { SetProfileOverride(old) })
	SetProfileOverride(p)
}

func TestProfileOverrideBeatsIntegrationDefault(t *testing.T) {
	withProfileOverride(t, agents.ProfilePointer)

	got := resolveProfile(agentsIntegration{name: "Full", profile: agents.ProfileFull})

	if got != agents.ProfilePointer {
		t.Fatalf("explicit --profile should win over the integration default, got %q", got)
	}
}

func TestNoProfileOverrideKeepsIntegrationDefault(t *testing.T) {
	withProfileOverride(t, "")

	if got := resolveProfile(agentsIntegration{name: "Minimal", profile: agents.ProfileMinimal}); got != agents.ProfileMinimal {
		t.Fatalf("without an override the integration profile must stand, got %q", got)
	}
	if got := resolveProfile(agentsIntegration{name: "Unset"}); got != agents.ProfileFull {
		t.Fatalf("an integration with no profile must still default to full, got %q", got)
	}
}

// Stickiness protects a file that already carries a richer profile — but only
// when nobody asked for something leaner. An explicit --profile is that ask.
func TestInstallAgentsProfileOverrideDowngradesDeliberately(t *testing.T) {
	env, stdout, _ := newFactoryTestEnv(t)
	if err := os.WriteFile(env.agentsPath, []byte(agents.RenderSection(agents.ProfileFull)), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	withProfileOverride(t, agents.ProfilePointer)

	integration := agentsIntegration{
		name:         "MinimalAgent",
		setupCommand: "bd setup minimalagent",
		profile:      agents.ProfileMinimal,
	}
	if err := installAgents(env, integration); err != nil {
		t.Fatalf("installAgents: %v", err)
	}

	data, err := readFileBytes(env.agentsPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "profile:pointer") {
		t.Fatalf("explicit --profile pointer should have been written: %q", content)
	}
	if strings.Contains(content, "profile:full") {
		t.Fatalf("full profile survived an explicit downgrade: %q", content)
	}
	if !strings.Contains(stdout.String(), "Replacing full profile with pointer") {
		t.Fatalf("a deliberate downgrade should be reported, got %q", stdout.String())
	}
}

// Without an override, a leaner integration must still not clobber a richer
// file — the behavior that predates this flag, now generalized over three
// profiles instead of two.
func TestInstallAgentsPreservesRicherProfileAgainstPointer(t *testing.T) {
	env, stdout, _ := newFactoryTestEnv(t)
	if err := os.WriteFile(env.agentsPath, []byte(agents.RenderSection(agents.ProfileMinimal)), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	withProfileOverride(t, "")

	integration := agentsIntegration{
		name:         "PointerAgent",
		setupCommand: "bd setup pointeragent",
		profile:      agents.ProfilePointer,
	}
	if err := installAgents(env, integration); err != nil {
		t.Fatalf("installAgents: %v", err)
	}

	data, err := readFileBytes(env.agentsPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "profile:minimal") {
		t.Fatalf("minimal should have been preserved against an unrequested pointer downgrade: %q", content)
	}
	if !strings.Contains(stdout.String(), "preserving") {
		t.Fatalf("expected the preserve notice, got %q", stdout.String())
	}
}
