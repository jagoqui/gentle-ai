package reviewassets

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

func TestRuntimeContracts(t *testing.T) {
	for _, tc := range []struct {
		agent              model.AgentID
		required, excluded string
	}{
		{model.AgentPi, "`gentle_review_capture_group`", "gentle-ai review status"},
		{model.AgentClaudeCode, "gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --agent claude-code --next-transition", "gentle_review_capture_group"},
		{model.AgentOpenCode, "### OpenCode Concurrent Reviewer Group", "gentle_review_capture_group"},
		{model.AgentVSCodeCopilot, "gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --agent vscode-copilot --next-transition", "gentle_review_capture_group"},
	} {
		t.Run(string(tc.agent), func(t *testing.T) {
			got, err := ReviewExecutionContractFor(tc.agent)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(got, tc.required) || strings.Contains(got, tc.excluded) || strings.Contains(got, identityPlaceholder) {
				t.Fatalf("incorrect runtime contract for %s", tc.agent)
			}
		})
	}
	if _, err := ReviewExecutionContractFor(model.AgentKilocode); err == nil {
		t.Fatal("unsupported runtime accepted")
	}
}

// TestVSCodeCopilotContractRelaysOneSequentialSubagent pins the VS Code
// Copilot capture transport: materialize through the exact returned tokens,
// relay the bytes verbatim to one isolated runSubagent reviewer, and submit the
// raw result through --input, one slot at a time. Every other runtime keeps
// its own transport body and group wording.
func TestVSCodeCopilotContractRelaysOneSequentialSubagent(t *testing.T) {
	got, err := ReviewExecutionContractFor(model.AgentVSCodeCopilot)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"`--agent=vscode-copilot --materialize=true`",
		"`#tool:runSubagent`",
		"`agentName: \"gentle-reviewer\"`",
		"BOM-less UTF-8",
		"outside the repository worktree",
		"`submission` operation",
		"never add `--agent` or `--input` to a token list that did not return them",
		"### VS Code Copilot Reviewer Sequence (MANDATORY)",
		"relay them one at a time in provider order",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("vscode-copilot contract lacks %q", want)
		}
	}
	for _, excluded := range []string{
		"### Concurrent Reviewer Group",
		"invoke one OpenCode Task",
		"This runtime captures in process",
		captureStart, captureEnd,
	} {
		if strings.Contains(got, excluded) {
			t.Errorf("vscode-copilot contract carries %q", excluded)
		}
	}
	for _, agent := range []model.AgentID{model.AgentClaudeCode, model.AgentCodex, model.AgentOpenCode, model.AgentPi} {
		other, err := ReviewExecutionContractFor(agent)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(other, "#tool:runSubagent") || strings.Contains(other, "VS Code Copilot Reviewer Sequence") {
			t.Errorf("%s contract leaked the vscode-copilot relay", agent)
		}
	}
	for _, agent := range []model.AgentID{model.AgentClaudeCode, model.AgentCodex} {
		other, err := ReviewExecutionContractFor(agent)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(other, "### Concurrent Reviewer Group (MANDATORY)") {
			t.Errorf("%s contract lost its concurrent reviewer group", agent)
		}
	}
}

func TestRenderedLensAssets(t *testing.T) {
	for _, path := range []string{
		"claude/agents/review-risk.md", "cursor/agents/review-reliability.md",
		"kimi/agents/review-readability.md", "kiro/agents/review-resilience.md",
	} {
		t.Run(path, func(t *testing.T) {
			source := assets.MustRead(path)
			got, ok := RenderReviewerAsset(path, source)
			if !ok || !strings.HasPrefix(got, source[:strings.Index(source, "\n---\n")+5]) {
				t.Fatal("review frontmatter changed")
			}
			for _, want := range []string{"## Candidate-Causal Admission", "## Scope", "## Output", "subject_hash", "GENTLE_AI_REVIEW_BINDING"} {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q", want)
				}
			}
		})
	}
	source := "---\nname: jd-judge-a\n---\noriginal"
	if got, ok := RenderReviewerAsset("kiro/agents/jd-judge-a.md", source); ok || got != source {
		t.Fatal("Judgment Day unexpectedly rendered as review lens")
	}
	if got, ok := RenderReviewerAsset("claude/agents/review-refuter.md", source); ok || got != source {
		t.Fatal("refuter unexpectedly rendered as lens")
	}
}

func TestInspectionCommandsIndependent(t *testing.T) {
	first, second := InspectionCommands(), InspectionCommands()
	first[0] = "changed"
	if second[0] == "changed" {
		t.Fatal("inspection command slices share storage")
	}
}
