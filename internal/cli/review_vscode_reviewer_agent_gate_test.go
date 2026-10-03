package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/vscode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// useVSCodeReviewerAgentHome points the vscode relay eligibility gate at a
// fresh temporary home for the rest of the test and returns the path the
// native installer writes the managed reviewer agent to inside it.
func useVSCodeReviewerAgentHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	previous := reviewVSCodeReviewerAgentHome
	reviewVSCodeReviewerAgentHome = func() (string, error) { return home, nil }
	t.Cleanup(func() { reviewVSCodeReviewerAgentHome = previous })
	return filepath.Join(vscode.NewAdapter().SubAgentsDir(home), reviewassets.VSCodeReviewerAgentFileName)
}

// installVSCodeReviewerAgentForTest installs the managed reviewer agent through
// the real native installer into a fresh gate home, so the vscode-copilot
// host relay is eligible for the rest of the test.
func installVSCodeReviewerAgentForTest(t *testing.T) string {
	t.Helper()
	path := useVSCodeReviewerAgentHome(t)
	home, _ := reviewVSCodeReviewerAgentHome()
	if _, err := reviewassets.InstallNativeAgents(home, vscode.NewAdapter(), reviewassets.InstallOptions{}); err != nil {
		t.Fatalf("install the vscode reviewer agent: %v", err)
	}
	return path
}

func TestVSCodeHostRelayEligibilityRequiresTheManagedReviewerAgent(t *testing.T) {
	t.Setenv(reviewPiHostRelayContractEnvironment, "")
	for _, test := range []struct {
		name    string
		arrange func(t *testing.T, path string)
		state   reviewVSCodeReviewerAgentState
	}{
		{name: "missing", arrange: func(t *testing.T, path string) {}, state: reviewVSCodeReviewerAgentMissing},
		{name: "tools line changed", arrange: func(t *testing.T, path string) {
			managed := mustReadFile(t, path)
			modified := strings.Replace(string(managed), "tools: []", "tools: ['codebase', 'runCommands']", 1)
			if modified == string(managed) {
				t.Fatal("the managed reviewer has no `tools: []` line to tamper with")
			}
			mustWriteFile(t, path, []byte(modified))
		}, state: reviewVSCodeReviewerAgentModified},
		{name: "trailing byte appended", arrange: func(t *testing.T, path string) {
			mustWriteFile(t, path, append(mustReadFile(t, path), '\n'))
		}, state: reviewVSCodeReviewerAgentModified},
		{name: "replaced by a directory", arrange: func(t *testing.T, path string) {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatal(err)
			}
		}, state: reviewVSCodeReviewerAgentModified},
	} {
		t.Run(test.name, func(t *testing.T) {
			var path string
			if test.state == reviewVSCodeReviewerAgentMissing {
				path = useVSCodeReviewerAgentHome(t)
			} else {
				path = installVSCodeReviewerAgentForTest(t)
			}
			test.arrange(t, path)

			if got := reviewVSCodeReviewerAgentStatus(); got != test.state {
				t.Fatalf("reviewer agent state = %q, want %q", got, test.state)
			}
			capability := reviewImmutableRuntimeCapability(model.AgentVSCodeCopilot)
			if capability.Eligible || capability.supportsImmutableReceiptReview() || reviewProviderHostRelayMaterializeRuntime(model.AgentVSCodeCopilot) {
				t.Fatalf("vscode-copilot without the managed reviewer agent = %#v, want ineligible", capability)
			}
			if slices.Contains(reviewTransportSupportedRuntimeIDs(), string(model.AgentVSCodeCopilot)) {
				t.Fatalf("supported runtimes %v name a refused vscode-copilot", reviewTransportSupportedRuntimeIDs())
			}
			_, err := reviewRuntimeWithImmutableTransport(string(model.AgentVSCodeCopilot))
			if err == nil {
				t.Fatal("want a typed refusal")
			}
			for _, want := range []string{"gentle-ai sync --agent vscode-copilot", reviewassets.VSCodeReviewerAgentFileName, string(test.state)} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("refusal %q does not name %q", err, want)
				}
			}
			if strings.Contains(err.Error(), "gentle-ai review mode disable") || strings.Contains(err.Error(), reviewPiHostRelayContractEnvironment) {
				t.Fatalf("refusal %q offers a remedy that cannot fix a missing reviewer agent", err)
			}
		})
	}
}

func TestVSCodeHostRelayEligibleWithTheInstalledManagedReviewerAgent(t *testing.T) {
	t.Setenv(reviewPiHostRelayContractEnvironment, "")
	installVSCodeReviewerAgentForTest(t)
	if got := reviewVSCodeReviewerAgentStatus(); got != reviewVSCodeReviewerAgentInstalled {
		t.Fatalf("reviewer agent state = %q, want installed", got)
	}
	capability := reviewImmutableRuntimeCapability(model.AgentVSCodeCopilot)
	if !capability.Eligible || capability.Transport != reviewImmutableTransportVSCodeHostRelay {
		t.Fatalf("vscode-copilot with the managed reviewer agent = %#v", capability)
	}
	if identity, err := reviewRuntimeWithImmutableTransport(string(model.AgentVSCodeCopilot)); err != nil || identity != model.AgentVSCodeCopilot {
		t.Fatalf("vscode-copilot = %q, %v", identity, err)
	}
	if reviewVSCodeReviewerAgentIsSoleMissingCondition(model.AgentVSCodeCopilot) {
		t.Fatal("an installed reviewer agent still reports the sync guidance")
	}
}

// TestVSCodeReviewerAgentGuidanceStaysScoped keeps the sync remedy on
// vscode-copilot alone and keeps every other runtime's refusal unchanged.
func TestVSCodeReviewerAgentGuidanceStaysScoped(t *testing.T) {
	t.Setenv(reviewPiHostRelayContractEnvironment, "")
	useVSCodeReviewerAgentHome(t)
	for _, runtime := range []model.AgentID{model.AgentKilocode, model.AgentPi, model.AgentID("unknown-runtime")} {
		_, err := reviewRuntimeWithImmutableTransport(string(runtime))
		if err == nil {
			t.Fatalf("%s: want a refusal", runtime)
		}
		if strings.Contains(err.Error(), "gentle-ai sync --agent vscode-copilot") {
			t.Fatalf("%s refusal leaks the vscode sync remedy: %v", runtime, err)
		}
	}
	for _, runtime := range []model.AgentID{model.AgentClaudeCode, model.AgentCodex} {
		if _, err := reviewRuntimeWithImmutableTransport(string(runtime)); err != nil {
			t.Fatalf("%s depends on the vscode reviewer agent: %v", runtime, err)
		}
	}
}

// TestVSCodeReviewerAgentGuidanceSurvivesTheFailureCausePrivacyGate pins the
// remedy the operator reads through the privacy gate every refusal crosses.
func TestVSCodeReviewerAgentGuidanceSurvivesTheFailureCausePrivacyGate(t *testing.T) {
	for _, state := range []reviewVSCodeReviewerAgentState{reviewVSCodeReviewerAgentMissing, reviewVSCodeReviewerAgentModified, reviewVSCodeReviewerAgentUnverifiable} {
		guidance := reviewVSCodeReviewerAgentGuidance(model.AgentVSCodeCopilot, state)
		if scrubbed := reviewScrubDefectReportField(guidance); scrubbed != guidance {
			t.Fatalf("the privacy gate rewrites the %s guidance:\n\tguidance %q\n\tscrubbed %q", state, guidance, scrubbed)
		}
		if !strings.Contains(guidance, "gentle-ai sync --agent vscode-copilot") {
			t.Fatalf("%s guidance lost the sync remedy: %q", state, guidance)
		}
	}
}

// TestVSCodeCaptureBoundEligibilitySkipsTheReviewerAgentGate follows Pi's
// capture-time precedent (#4256): a capture already carries its own frozen
// binding, so removing the reviewer agent mid-lineage never strands it.
func TestVSCodeCaptureBoundEligibilitySkipsTheReviewerAgentGate(t *testing.T) {
	useVSCodeReviewerAgentHome(t)
	capability := reviewCaptureBoundRuntimeCapability(model.AgentVSCodeCopilot)
	if !capability.Eligible || capability.Transport != reviewImmutableTransportVSCodeHostRelay {
		t.Fatalf("capture-bound vscode-copilot = %#v, want the compiled host relay", capability)
	}
	if identity, err := reviewCaptureRuntimeWithBoundTransport(string(model.AgentVSCodeCopilot)); err != nil || identity != model.AgentVSCodeCopilot {
		t.Fatalf("capture-bound vscode-copilot = %q, %v", identity, err)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
