package cli

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// The VS Code Copilot host relay carries the provider roles (refuter and
// targeted validator) through the same generalized host-relay checks as Pi:
// with the managed reviewer agent installed, STATUS renders a
// `--materialize=true` prelude that prints exactly the Go-materialized role
// prompt, plus a submission descriptor that returns the host-run result
// through `--input`. The refused states live in
// review_vscode_reviewer_agent_gate_test.go.

// vscodeHostRelayRoleInput renders the negotiated vscode-copilot STATUS for a
// lineage and returns its single host-relay collect input after checking the
// materialize prelude and the `--input` submission descriptor shape.
func vscodeHostRelayRoleInput(t *testing.T, repo, lineage, reason, operation, slot, schema string) ReviewTransitionInput {
	t.Helper()
	agent := string(model.AgentVSCodeCopilot)
	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", lineage, "--contract", ReviewIntegrationContractV2,
		"--agent", agent, "--next-transition",
	}, &output); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if err := status.Validate(); err != nil {
		t.Fatalf("vscode host relay %s STATUS is invalid: %v", slot, err)
	}
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
		status.NextTransition.ReasonCode != reason || status.NextTransition.Collect == nil ||
		len(status.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("vscode host relay %s transition = %#v", slot, status.NextTransition)
	}
	input := status.NextTransition.Collect.Inputs[0]
	if input.CaptureOperation != "review."+operation || input.Schema != schema || input.ProviderTask != nil {
		t.Fatalf("vscode host relay %s input = %#v", slot, input)
	}
	tokens := map[string]string{}
	for _, argument := range input.Arguments {
		tokens[argument.Name] = argument.Token
	}
	if tokens["agent"] != "--agent="+agent || tokens["materialize"] != "--materialize=true" || tokens["execute"] != "" {
		t.Fatalf("vscode host relay %s arguments = %#v", slot, input.Arguments)
	}
	wantSubmissionTokens := make([]string, 0, len(input.Arguments))
	for _, argument := range input.Arguments {
		if argument.Name != "materialize" {
			wantSubmissionTokens = append(wantSubmissionTokens, argument.Token)
		}
	}
	wantSubmissionTokens = append(wantSubmissionTokens, "--input="+reviewSubmissionValuePlaceholder)
	if input.Submission == nil || input.Submission.OperationToken != operation ||
		!slices.Equal(input.Submission.ArgumentTokens, wantSubmissionTokens) ||
		!slices.Contains(input.Submission.ArgumentTokens, "--agent="+agent) ||
		input.Submission.Value == nil || input.Submission.Value.Slot != slot || input.Submission.Value.Schema != schema ||
		input.Submission.Value.SubstitutionLocation != len(wantSubmissionTokens)-1 {
		t.Fatalf("vscode host relay %s submission = %#v, want tokens %v", slot, input.Submission, wantSubmissionTokens)
	}
	return input
}

// vscodeHostRelayMaterialize runs the STATUS-rendered materialize prelude
// exactly as issued and returns the printed prompt bytes.
func vscodeHostRelayMaterialize(t *testing.T, repo, operation string, input ReviewTransitionInput) []byte {
	t.Helper()
	prelude := []string{operation, "--cwd=" + repo}
	for _, argument := range input.Arguments {
		prelude = append(prelude, argument.Token)
	}
	var printed bytes.Buffer
	if err := RunReview(prelude, &printed); err != nil {
		t.Fatalf("vscode %s --materialize=true with the installed reviewer agent refused: %v", operation, err)
	}
	if printed.Len() == 0 {
		t.Fatalf("vscode %s --materialize=true printed no prompt bytes", operation)
	}
	return printed.Bytes()
}

// vscodeHostRelaySubmit substitutes the descriptor's {{value}} slot with a
// raw host-run result file and submits it through --input.
func vscodeHostRelaySubmit(t *testing.T, repo string, input ReviewTransitionInput, resultFile string) []byte {
	t.Helper()
	submit := append([]string{input.Submission.OperationToken, "--cwd", repo}, input.Submission.ArgumentTokens...)
	for index := range submit {
		submit[index] = strings.ReplaceAll(submit[index], reviewSubmissionValuePlaceholder, resultFile)
	}
	var captured bytes.Buffer
	if err := RunReview(submit, &captured); err != nil {
		t.Fatalf("vscode %s --input submission refused: %v", input.Submission.OperationToken, err)
	}
	return captured.Bytes()
}

func TestVSCodeHostRelayRefuterMaterializesTheRolePromptAndSubmitsThroughInput(t *testing.T) {
	installVSCodeReviewerAgentForTest(t)
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, "")
	repo, store, record, _ := piRefuterReview(t)

	input := vscodeHostRelayRoleInput(t, repo, record.State.LineageID, "provider_refuter_required",
		"capture-refuter", "provider_refuter", reviewRefuterSchemaID)
	printed := vscodeHostRelayMaterialize(t, repo, "capture-refuter", input)
	request, err := reviewProviderNewRefuterRequest(t.Context(), repo, store.Dir, record.State, record.State.CapturePhaseRevision)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(printed, request.Invocation.Prompt()) {
		t.Fatal("vscode refuter materialized bytes diverged from the Go-materialized refuter request")
	}
	if again := vscodeHostRelayMaterialize(t, repo, "capture-refuter", input); !bytes.Equal(printed, again) {
		t.Fatal("repeated vscode refuter materialization changed the prompt bytes")
	}
	if current, err := store.Load(); err != nil || recordHasAdmittedRole(current.State, reviewtransaction.CompactRoleRefuter) {
		t.Fatalf("vscode refuter materialize mutated compact authority: %#v, %v", current, err)
	}

	vscodeHostRelaySubmit(t, repo, input, writeReviewCLIRawInput(t, piRefuterRawResult(t, repo, store, record)))
	final, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !recordHasAdmittedRole(final.State, reviewtransaction.CompactRoleRefuter) {
		t.Fatal("vscode refuter --input submission did not admit a refuter result")
	}
}

func TestVSCodeHostRelayValidationMaterializesTheRolePromptAndSubmitsThroughInput(t *testing.T) {
	installVSCodeReviewerAgentForTest(t)
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, "")
	repo, lineage, request := providerCorrectionReadyWithoutVerificationEvidence(t)
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	input := vscodeHostRelayRoleInput(t, repo, lineage, "targeted_validation_required",
		"capture-validation", "provider_targeted_validator", reviewValidatorSchemaID)
	if input.ValidationRequest == nil || input.ValidationRequest.RequestHash != request.RequestHash {
		t.Fatalf("vscode host relay validation request = %#v, want hash %q", input.ValidationRequest, request.RequestHash)
	}
	printed := vscodeHostRelayMaterialize(t, repo, "capture-validation", input)
	correction, err := reviewProviderTargetedValidatorCorrection(t.Context(), repo, record.State)
	if err != nil {
		t.Fatal(err)
	}
	native, err := reviewProviderNewTargetedValidatorRequest(t.Context(), repo, record.State, record.State.CapturePhaseRevision, correction)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(printed, native.Invocation.Prompt()) {
		t.Fatal("vscode validator materialized bytes diverged from the Go-materialized request")
	}
	if again := vscodeHostRelayMaterialize(t, repo, "capture-validation", input); !bytes.Equal(printed, again) {
		t.Fatal("repeated vscode validator materialization changed the prompt bytes")
	}
	if current, err := store.Load(); err != nil || recordHasAdmittedRole(current.State, reviewtransaction.CompactRoleTargetedValidator) {
		t.Fatalf("vscode validator materialize mutated compact authority: %#v, %v", current, err)
	}

	captured := vscodeHostRelaySubmit(t, repo, input, writeReviewCLIRawInput(t, providerTargetedValidationPayload(t, request)))
	var closure reviewLastEventClosureResult
	decodeStrictReviewJSON(t, captured, &closure)
	if closure.Schema != reviewLastEventClosureSchema || closure.Operation != "review/capture-validation" ||
		closure.LineageID != lineage || closure.State != reviewtransaction.StateApproved {
		t.Fatalf("vscode validator terminal capture = %#v", closure)
	}
	assertApprovedCompactAuthorityBurned(t, store, lineage)
}
