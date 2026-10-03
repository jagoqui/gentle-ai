// Package reviewassets owns runtime review contract rendering independently of SDD.
package reviewassets

import (
	"context"
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/capabilitymanifest"
	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewerprovider"
)

const identityPlaceholder = "{{GENTLE_AI_RUNTIME_AGENT_ID}}"
const captureStart = "<!-- reviewer-capture-transport:start -->"
const captureEnd = "<!-- reviewer-capture-transport:end -->"

const openCodeGroup = "### OpenCode Concurrent Reviewer Group (MANDATORY)\n\n" +
	"When one fresh `collect.inputs` set contains multiple distinct independent `review.capture-result` reviewer slots, emit one grouped OpenCode `task` tool-call response with one foreground task per input in provider order. For canonical 4R, preserve `review-risk`, `review-resilience`, `review-readability`, `review-reliability` order.\n\n" +
	"Each task copies its own `provider_task.agent` exactly as `subagent_type` and its own `provider_task.prompt` exactly as `prompt`. Do not parse, reconstruct, fence, or append text to either value. Do not set a `background` flag. Do not wait between launches; wait for every foreground task result. Completion order is not authority: shared Go admission/election owns reduction and semantics. The final admitted capture owns reduction and closure. On `approved`, authority is already burned: do not FINALIZE or issue a trailing STATUS. On `correction_required`, continue only through exact bound STATUS and the provider-issued `review.capture-correction-plan` binding. After a malformed or nonterminal capture, reconcile through exact bound STATUS and retry only an identically reoffered slot."

// OpenCodeConcurrentReviewerGroupContract is the runtime-specific group wording.
const OpenCodeConcurrentReviewerGroupContract = openCodeGroup

const concurrentGroup = "### Concurrent Reviewer Group (MANDATORY)\n\n" +
	"When one fresh `collect.inputs` set contains multiple distinct independent `review.capture-result` reviewer slots, launch every returned capture operation concurrently in provider order: start all without waiting between launches, then wait for every result. For canonical 4R, preserve `review-risk`, `review-resilience`, `review-readability`, `review-reliability` order.\n\n" +
	"Each launch runs only its own provider-issued `review.capture-result` argument tokens exactly as returned. Completion order is not authority: shared Go admission/election owns reduction and semantics. The final admitted capture owns reduction and closure. On `approved`, authority is already burned: do not FINALIZE or issue a trailing STATUS. On `correction_required`, continue only through exact bound STATUS and the provider-issued `review.capture-correction-plan` binding. After a malformed or nonterminal capture, reconcile through exact bound STATUS and retry only an identically reoffered slot."
const compiledCapture = "For each returned `review.capture-result` input, run its exact capture operation once with its argument tokens exactly as returned. " +
	"This runtime captures in process: those tokens carry `--agent` and no `--input`, and running them makes Go materialize the immutable reviewer context, run its own locked-down reviewer on it, and admit the result. " +
	"Never assemble a reviewer prompt, never launch a lens subagent, and never add `--input` to a returned token list. " +
	"Each of those rebuilds the returned command into the relay form and moves the complete candidate evidence onto the parent for every lens, to reach a result the returned command already produces carrying nothing. " +
	"An empty, malformed, schema-invalid, or incomplete result is handled by the recovery rule below, exactly as a relayed one is."

// VSCodeReviewerAgentName is the VS Code Copilot custom agent the vscode
// capture transport relays to; the embedded vscode/agents asset declares the
// same name and NativeAgentManifest installs it.
const VSCodeReviewerAgentName = "gentle-reviewer"

// VSCodeReviewerAgentFileName is the file the native installer writes into the
// VS Code user prompts folder (agents.Adapter.SubAgentsDir) for the reviewer.
const VSCodeReviewerAgentFileName = VSCodeReviewerAgentName + ".agent.md"

const vscodeCapture = "For each returned `review.capture-result` input, relay one isolated reviewer through VS Code Copilot Chat. " +
	"First run the input's exact capture operation in the terminal with its argument tokens exactly as returned; those tokens carry `--agent=vscode-copilot --materialize=true`, so the run only prints the raw Go-materialized reviewer prompt bytes and captures nothing. " +
	"Then invoke the `#tool:runSubagent` tool once with `agentName: \"" + VSCodeReviewerAgentName + "\"`, passing those printed bytes verbatim as its `prompt`. " +
	"Write that subagent's single final message verbatim, as BOM-less UTF-8, to a scratch file outside the repository worktree, then run the input's exact `submission` operation and argument tokens with that file path substituted only into its `--input` value. " +
	"Never edit, summarize, fence, or reconstruct the prompt or the result, and never add `--agent` or `--input` to a token list that did not return them. " +
	"The reviewer result must remain the reviewer's raw JSON object; `inspection.status`/`inspection.reason` are the only admission-completeness signal."

const vscodeGroup = "### VS Code Copilot Reviewer Sequence (MANDATORY)\n\n" +
	"When one fresh `collect.inputs` set contains multiple distinct independent `review.capture-result` reviewer slots, relay them one at a time in provider order: materialize, run one `runSubagent` reviewer, and submit each slot before starting the next. For canonical 4R, preserve `review-risk`, `review-resilience`, `review-readability`, `review-reliability` order.\n\n" +
	"Each relay runs only its own provider-issued `review.capture-result` argument tokens and submission tokens exactly as returned. Submission order is not authority: shared Go admission/election owns reduction and semantics. The final admitted capture owns reduction and closure. On `approved`, authority is already burned: do not FINALIZE or issue a trailing STATUS. On `correction_required`, continue only through exact bound STATUS and the provider-issued `review.capture-correction-plan` binding. After a malformed or nonterminal capture, reconcile through exact bound STATUS and retry only an identically reoffered slot."

// ReviewExecutionContractFor returns the runtime-bound provider-owned contract.
func ReviewExecutionContractFor(agent model.AgentID) (string, error) {
	manifest, err := capabilitymanifest.ForAgent(agent)
	if err != nil || !manifest.Advertises(capabilitymanifest.ContractReviewTransportV1) {
		return "", fmt.Errorf("%s does not render the review execution contract", agent)
	}
	return strings.ReplaceAll(ContractFor(agent), identityPlaceholder, string(agent)), nil
}

// ContractFor renders the shared review contract for an agent's transport.
func ContractFor(agent model.AgentID) string {
	if agent == model.AgentPi {
		return strings.TrimSpace(assets.MustRead("skills/_shared/review-ledger-contract-pi.md"))
	}
	contract := selectTransport(strings.TrimSpace(assets.MustRead("skills/_shared/review-ledger-contract.md")), agent)
	switch {
	case agent == model.AgentOpenCode:
		rendered := contract + "\n\n" + openCodeGroup
		if major, err := opencode.DetectRuntimeMajor(context.Background()); err == nil && major == opencode.RuntimeV2 {
			rendered = strings.NewReplacer("`task`", "`subagent`", "`subagent_type`", "`agent`").Replace(rendered)
		}
		return rendered
	case agent == model.AgentVSCodeCopilot:
		// The host relay is sequential: each slot materializes, runs one
		// isolated subagent, and submits before the next one starts.
		return contract + "\n\n" + vscodeGroup
	case reviewerprovider.RegisteredRuntime(agent):
		return contract + "\n\n" + concurrentGroup
	}
	return contract
}

func selectTransport(contract string, agent model.AgentID) string {
	start := strings.Index(contract, captureStart)
	end := strings.Index(contract, captureEnd)
	if start < 0 || end < start {
		return contract
	}
	body := strings.TrimSpace(contract[start+len(captureStart) : end])
	switch {
	case reviewerprovider.CapturesInProcess(agent):
		body = compiledCapture
	case agent == model.AgentVSCodeCopilot:
		body = vscodeCapture
	}
	return contract[:start] + body + contract[end+len(captureEnd):]
}
