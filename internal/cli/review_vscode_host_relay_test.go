package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// The VS Code Copilot host relay mirrors the Pi relay: Go materializes the
// opaque reviewer prompt, the host runs one isolated `runSubagent` reviewer on
// those bytes, and submits the raw result through --input. Unlike Pi it has
// no launcher process that could export an env handshake, so its eligibility
// is the compiled manifest alone (like Claude Code and Codex).

func TestVSCodeCopilotHostRelayIsEligibleWithoutAnyHandshake(t *testing.T) {
	t.Setenv(reviewPiHostRelayContractEnvironment, "")
	capability := reviewImmutableRuntimeCapability(model.AgentVSCodeCopilot)
	if !capability.Eligible || capability.Transport != reviewImmutableTransportVSCodeHostRelay ||
		!capability.supportsImmutableReceiptReview() {
		t.Fatalf("vscode-copilot capability = %#v, want eligible %q", capability, reviewImmutableTransportVSCodeHostRelay)
	}
	if !reviewImmutableTransportIsHostRelay(capability.Transport) || !reviewProviderHostRelayMaterializeRuntime(model.AgentVSCodeCopilot) {
		t.Fatal("vscode-copilot transport is not classified as a host relay")
	}
	if reviewProviderCaptureRuntime(model.AgentVSCodeCopilot) {
		t.Fatal("vscode-copilot must not capture in process")
	}
	if _, err := reviewProviderAdapterFor(reviewerprovider.Contract{RequiredCapabilities: []string{reviewerprovider.TransportCapability}}, model.AgentVSCodeCopilot); err == nil || !strings.Contains(err.Error(), "host-mediated") {
		t.Fatalf("vscode-copilot adapter refusal = %v, want host-mediated", err)
	}
	if !slices.Contains(reviewTransportSupportedRuntimeIDs(), string(model.AgentVSCodeCopilot)) {
		t.Fatalf("supported runtimes %v omit vscode-copilot", reviewTransportSupportedRuntimeIDs())
	}

	// Pi keeps its exact handshake gate and its own transport identity.
	if pi := reviewImmutableRuntimeCapability(model.AgentPi); pi.Eligible || pi.supportsImmutableReceiptReview() {
		t.Fatalf("pi without the handshake became eligible: %#v", pi)
	}
	if reviewPiRelayHandshakeIsSoleMissingCondition(model.AgentVSCodeCopilot) {
		t.Fatal("the pi handshake guidance leaked to vscode-copilot")
	}
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	if pi := reviewImmutableRuntimeCapability(model.AgentPi); !pi.Eligible || pi.Transport != reviewImmutableTransportPiHostRelay {
		t.Fatalf("pi with the handshake = %#v, want %q", pi, reviewImmutableTransportPiHostRelay)
	}
}

func TestNegotiatedStatusRendersVSCodeHostRelayMaterializeCaptureInput(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, "")
	repo, _, record, _ := newCandidateInspectionReview(t, "candidate\n", true)
	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", record.State.LineageID, "--contract", ReviewIntegrationContractV2,
		"--agent", string(model.AgentVSCodeCopilot), "--next-transition",
	}, &output); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if err := status.Validate(); err != nil {
		t.Fatalf("vscode host relay STATUS is invalid: %v", err)
	}
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
		status.NextTransition.ReasonCode != "reviewer_results_required" || status.NextTransition.Collect == nil ||
		len(status.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("vscode host relay transition = %#v", status.NextTransition)
	}
	input := status.NextTransition.Collect.Inputs[0]
	if input.CaptureOperation != "review.capture-result" {
		t.Fatalf("vscode host relay capture operation = %q", input.CaptureOperation)
	}
	tokens := map[string]string{}
	for _, argument := range input.Arguments {
		tokens[argument.Name] = argument.Token
	}
	if tokens["agent"] != "--agent="+string(model.AgentVSCodeCopilot) || tokens["materialize"] != "--materialize=true" {
		t.Fatalf("vscode host relay capture arguments = %#v", input.Arguments)
	}
	wantTokens := make([]string, 0, len(input.Arguments))
	for _, argument := range input.Arguments {
		if argument.Name != "materialize" {
			wantTokens = append(wantTokens, argument.Token)
		}
	}
	wantTokens = append(wantTokens, "--input={{value}}")
	if input.Submission == nil || input.Submission.OperationToken != "capture-result" ||
		!slices.Equal(input.Submission.ArgumentTokens, wantTokens) || input.Submission.Value == nil ||
		input.Submission.Value.Slot != "reviewer_result" || input.Submission.Value.Schema != reviewReviewerSchemaID ||
		input.Submission.Value.SubstitutionLocation != len(wantTokens)-1 {
		t.Fatalf("vscode host relay submission = %#v, want tokens %v", input.Submission, wantTokens)
	}
}

func TestReviewCaptureResultVSCodeMaterializesAndSubmitsThroughInput(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, "")
	repo, args, record, _ := newCandidateInspectionReview(t, "candidate\n", true)
	binding := piHostRelayCaptureBinding(t, repo, args, record)
	handle := args[slices.Index(args, "--repository-context")+1]
	lens := record.State.SelectedLenses[0]
	agent := string(model.AgentVSCodeCopilot)

	var materialized bytes.Buffer
	if err := RunReviewCaptureResult(append(slices.Clone(binding), "--agent", agent, "--materialize=true"), &materialized); err != nil {
		t.Fatalf("vscode materialize refused: %v", err)
	}
	var native bytes.Buffer
	if err := RunReview([]string{
		"lens-context", "--cwd", repo, "--repository-context", handle,
		"--lineage", record.State.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--expected-revision", record.State.CapturePhaseRevision, "--lens", lens,
	}, &native); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(materialized.Bytes(), native.Bytes()) {
		t.Fatal("vscode materialized bytes diverged from the Go-materialized lens context")
	}

	// Execution without --materialize or --input has no in-process reviewer.
	err := RunReviewCaptureResult(append(slices.Clone(binding), "--agent", agent), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "has no in-process reviewer to run") {
		t.Fatalf("vscode bare execution refusal = %v", err)
	}

	resultFile := filepath.Join(t.TempDir(), "vscode-result.json")
	if err := os.WriteFile(resultFile, admittedReviewerPayloadForTest(t, repo, record, lens, 0), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunReviewCaptureResult(append(slices.Clone(binding), "--agent", agent, "--input", resultFile), &output); err != nil {
		t.Fatalf("vscode --input submission refused: %v", err)
	}
	var terminal reviewLastEventClosureResult
	decodeStrictReviewJSON(t, output.Bytes(), &terminal)
	if terminal.Operation != "review/capture-result" || terminal.State != reviewtransaction.StateApproved {
		t.Fatalf("submitted vscode reviewer terminal result = %#v", terminal)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, record.State.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	assertApprovedCompactAuthorityBurned(t, store, record.State.LineageID)
}

func TestReviewCaptureResultRefusesNonRelayRuntimeWhereVSCodeRelays(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, "")
	repo, args, record, _ := newCandidateInspectionReview(t, "candidate\n", true)
	binding := piHostRelayCaptureBinding(t, repo, args, record)
	lens := record.State.SelectedLenses[0]
	resultFile := filepath.Join(t.TempDir(), "result.json")
	if err := os.WriteFile(resultFile, admittedReviewerPayloadForTest(t, repo, record, lens, 0), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		argv []string
		want string
	}{
		{
			name: "opencode cannot submit as a host relay",
			argv: append(slices.Clone(binding), "--agent", string(model.AgentOpenCode), "--input", resultFile),
			want: "only a compiled host-relay runtime may submit its raw reviewer result",
		},
		{
			name: "claude-code cannot submit as a host relay",
			argv: append(slices.Clone(binding), "--agent", string(model.AgentClaudeCode), "--input", resultFile),
			want: "only a compiled host-relay runtime may submit its raw reviewer result",
		},
		{
			name: "opencode cannot materialize",
			argv: append(slices.Clone(binding), "--agent", string(model.AgentOpenCode), "--materialize=true"),
			want: "printing the Go-materialized provider task is the host-relay form",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := RunReviewCaptureResult(test.argv, io.Discard)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("refusal = %v, want %q", err, test.want)
			}
		})
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, record.State.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.Load()
	if err != nil || recordHasAdmittedRole(current.State, reviewtransaction.CompactRoleLens) {
		t.Fatalf("a refused relay submission mutated compact authority: %v", err)
	}
}
