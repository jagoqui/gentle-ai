package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/vscode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
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

// TestVSCodeReviewerAgentGateIgnoresTheAmbientHomeInTests proves the package
// TestMain pins the gate's home seam: a HOME/USERPROFILE that holds a valid
// managed reviewer agent must not make vscode-copilot eligible, so no test
// result depends on whether the developer machine synced the reviewer. Tests
// that need an installed agent opt in through installVSCodeReviewerAgentForTest.
func TestVSCodeReviewerAgentGateIgnoresTheAmbientHomeInTests(t *testing.T) {
	t.Setenv(reviewPiHostRelayContractEnvironment, "")
	ambientHome := t.TempDir()
	if _, err := reviewassets.InstallNativeAgents(ambientHome, vscode.NewAdapter(), reviewassets.InstallOptions{}); err != nil {
		t.Fatalf("install the vscode reviewer agent into the ambient home: %v", err)
	}
	ambientAgent := filepath.Join(vscode.NewAdapter().SubAgentsDir(ambientHome), reviewassets.VSCodeReviewerAgentFileName)
	if _, err := os.Stat(ambientAgent); err != nil {
		t.Fatalf("ambient reviewer agent not installed: %v", err)
	}
	t.Setenv("HOME", ambientHome)
	t.Setenv("USERPROFILE", ambientHome)

	if got := reviewVSCodeReviewerAgentStatus(); got == reviewVSCodeReviewerAgentInstalled {
		t.Fatal("the gate read the ambient HOME instead of the package's hermetic seam")
	}
	if capability := reviewImmutableRuntimeCapability(model.AgentVSCodeCopilot); capability.Eligible {
		t.Fatalf("vscode-copilot is eligible from the ambient HOME: %#v", capability)
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

// TestVSCodeReviewerAgentGuidanceNamesTheGlobalSyncScope keeps a
// workspace-scope operator out of a loop: the gate reads only the Copilot user
// agents folder, so the remedy pins `--scope global` explicitly (overriding
// GENTLE_AI_INSTALL_SCOPE) and says a workspace-scoped install is not used.
func TestVSCodeReviewerAgentGuidanceNamesTheGlobalSyncScope(t *testing.T) {
	for _, state := range []reviewVSCodeReviewerAgentState{reviewVSCodeReviewerAgentMissing, reviewVSCodeReviewerAgentModified, reviewVSCodeReviewerAgentUnverifiable} {
		guidance := reviewVSCodeReviewerAgentGuidance(model.AgentVSCodeCopilot, state)
		for _, want := range []string{"`gentle-ai sync --agent vscode-copilot --scope global`", "workspace-scoped install is not used for review"} {
			if !strings.Contains(guidance, want) {
				t.Fatalf("%s guidance %q does not name %q", state, guidance, want)
			}
		}
	}
}

// TestVSCodeCaptureBoundEligibilitySkipsTheReviewerAgentGate follows Pi's
// capture-time precedent (#4256) for the bound `--input` submission: a capture
// already carries its own frozen binding, so removing the reviewer agent
// mid-lineage never strands a reviewer result the host already produced. An
// explicit `--materialize=true` capture is gated separately
// (TestVSCodeMaterializeCaptureRequiresTheManagedReviewerAgent).
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

// TestVSCodeMaterializeCaptureRequiresTheManagedReviewerAgent closes the
// capture-time side of the installed-reviewer gate: an explicit
// `--materialize=true` capture (lens, refuter, and targeted validator) prints
// the reviewer prompt only while the managed `gentle-reviewer` agent is
// installed byte-identical, because that print is what the host hands to the
// subagent. A missing or modified agent refuses with the typed sync guidance
// and prints no prompt bytes, while the bound `--input` submission of a result
// the host already produced stays admitted (Pi precedent #4256).
func TestVSCodeMaterializeCaptureRequiresTheManagedReviewerAgent(t *testing.T) {
	path := installVSCodeReviewerAgentForTest(t)
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, "")
	repo, args, record, _ := newCandidateInspectionReview(t, "candidate\n", true)
	binding := piHostRelayCaptureBinding(t, repo, args, record)
	lens := record.State.SelectedLenses[0]
	agent := string(model.AgentVSCodeCopilot)
	managed := mustReadFile(t, path)
	roleBinding := []string{
		"--cwd", repo, "--repository-context", "provider-issued-context", "--lineage", record.State.LineageID,
		"--target", "provider-issued-target", "--expected-revision", record.State.CapturePhaseRevision,
		"--agent", agent, "--materialize=true",
	}

	for _, test := range []struct {
		name    string
		arrange func(t *testing.T)
		state   reviewVSCodeReviewerAgentState
	}{
		{name: "missing", arrange: func(t *testing.T) {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}, state: reviewVSCodeReviewerAgentMissing},
		{name: "modified", arrange: func(t *testing.T) {
			mustWriteFile(t, path, append(slices.Clone(managed), '\n'))
		}, state: reviewVSCodeReviewerAgentModified},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.arrange(t)
			t.Cleanup(func() { mustWriteFile(t, path, managed) })
			captures := map[string]func(io.Writer) error{
				"capture-result": func(w io.Writer) error {
					return RunReviewCaptureResult(append(slices.Clone(binding), "--agent", agent, "--materialize=true"), w)
				},
				"capture-refuter": func(w io.Writer) error { return RunReviewCaptureRefuter(slices.Clone(roleBinding), w) },
				"capture-validation": func(w io.Writer) error {
					return RunReviewCaptureValidation(append(slices.Clone(roleBinding), "--request-hash", strings.Repeat("a", 64)), w)
				},
			}
			for command, capture := range captures {
				var printed bytes.Buffer
				err := capture(&printed)
				if err == nil {
					t.Fatalf("%s --materialize=true with a %s reviewer agent printed the prompt", command, test.state)
				}
				for _, want := range []string{"gentle-ai sync --agent vscode-copilot", reviewassets.VSCodeReviewerAgentFileName, string(test.state)} {
					if !strings.Contains(err.Error(), want) {
						t.Fatalf("%s refusal %q does not name %q", command, err, want)
					}
				}
				if printed.Len() != 0 {
					t.Fatalf("%s refusal still printed %d prompt bytes", command, printed.Len())
				}
			}
		})
	}

	// With the managed agent back in place the same capture prints the prompt.
	var printed bytes.Buffer
	if err := RunReviewCaptureResult(append(slices.Clone(binding), "--agent", agent, "--materialize=true"), &printed); err != nil || printed.Len() == 0 {
		t.Fatalf("vscode materialize with the installed agent = %d bytes, %v", printed.Len(), err)
	}

	// The bound --input submission of an already-produced result stays
	// admitted with the agent missing: an in-flight lineage is never stranded.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	resultFile := filepath.Join(t.TempDir(), "vscode-result.json")
	if err := os.WriteFile(resultFile, admittedReviewerPayloadForTest(t, repo, record, lens, 0), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunReviewCaptureResult(append(slices.Clone(binding), "--agent", agent, "--input", resultFile), &output); err != nil {
		t.Fatalf("vscode --input submission without the reviewer agent refused: %v", err)
	}
	var terminal reviewLastEventClosureResult
	decodeStrictReviewJSON(t, output.Bytes(), &terminal)
	if terminal.Operation != "review/capture-result" || terminal.State != reviewtransaction.StateApproved {
		t.Fatalf("submitted vscode reviewer terminal result = %#v", terminal)
	}
}

// TestPiMaterializeCaptureIgnoresTheVSCodeReviewerAgentGate keeps the
// materialize gate scoped to vscode-copilot: Pi's capture-time eligibility
// still derives from the bound transaction alone (#4256), whatever the VS Code
// reviewer agent's state.
func TestPiMaterializeCaptureIgnoresTheVSCodeReviewerAgentGate(t *testing.T) {
	useVSCodeReviewerAgentHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, "")
	if err := reviewCaptureMaterializeRuntimeGate(model.AgentPi); err != nil {
		t.Fatalf("pi materialize depends on the vscode reviewer agent: %v", err)
	}
	for _, runtime := range []model.AgentID{model.AgentClaudeCode, model.AgentCodex, model.AgentOpenCode} {
		if err := reviewCaptureMaterializeRuntimeGate(runtime); err != nil {
			t.Fatalf("%s materialize depends on the vscode reviewer agent: %v", runtime, err)
		}
	}
	if err := reviewCaptureMaterializeRuntimeGate(model.AgentVSCodeCopilot); err == nil {
		t.Fatal("vscode-copilot materialize without the reviewer agent passed the gate")
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
