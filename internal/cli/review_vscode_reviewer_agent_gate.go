package cli

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/capabilitymanifest"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/vscode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// reviewVSCodeReviewerAgentHome resolves the home whose VS Code user prompts
// folder holds the managed relay reviewer. A global `gentle-ai sync` installs
// it there; tests replace this seam with a temporary home.
//
// Only the user-profile location is accepted. A workspace-scoped sync passes
// the workspace root to the same adapter resolver, which yields
// `<workspace>/.config/Code/User/prompts` (or the per-OS equivalent): VS Code
// Copilot Chat never discovers custom agents there, so accepting that file
// would advertise a reviewer `runSubagent` cannot reach.
var reviewVSCodeReviewerAgentHome = os.UserHomeDir

// reviewVSCodeReviewerAgentState is the observed state of the managed relay
// reviewer agent. Its values reach the operator inside the refusal cause.
type reviewVSCodeReviewerAgentState string

const (
	reviewVSCodeReviewerAgentInstalled    reviewVSCodeReviewerAgentState = "installed"
	reviewVSCodeReviewerAgentMissing      reviewVSCodeReviewerAgentState = "missing"
	reviewVSCodeReviewerAgentModified     reviewVSCodeReviewerAgentState = "modified"
	reviewVSCodeReviewerAgentUnverifiable reviewVSCodeReviewerAgentState = "unverifiable"
)

// reviewVSCodeReviewerAgentStatus reports whether the VS Code user prompts
// folder holds exactly the managed tool-less reviewer the installer writes:
// one Lstat, one read, and one byte comparison against the installer's own
// render (reviewassets.ManagedVSCodeReviewerAgent), so the gate and the
// installer cannot drift. Anything other than a regular file with identical
// bytes (an edited tools line, a symlink, a directory) is "modified": the
// relay's isolation rests on those exact bytes until organic proof lands.
func reviewVSCodeReviewerAgentStatus() reviewVSCodeReviewerAgentState {
	home, err := reviewVSCodeReviewerAgentHome()
	if err != nil || strings.TrimSpace(home) == "" {
		return reviewVSCodeReviewerAgentUnverifiable
	}
	adapter := vscode.NewAdapter()
	managed, err := reviewassets.ManagedVSCodeReviewerAgent(adapter)
	if err != nil {
		return reviewVSCodeReviewerAgentUnverifiable
	}
	path := filepath.Join(adapter.SubAgentsDir(home), reviewassets.VSCodeReviewerAgentFileName)
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return reviewVSCodeReviewerAgentMissing
	}
	if err != nil {
		return reviewVSCodeReviewerAgentUnverifiable
	}
	if !info.Mode().IsRegular() || info.Size() != int64(len(managed)) {
		return reviewVSCodeReviewerAgentModified
	}
	installed, err := os.ReadFile(path)
	if err != nil {
		return reviewVSCodeReviewerAgentUnverifiable
	}
	if !bytes.Equal(installed, managed) {
		return reviewVSCodeReviewerAgentModified
	}
	return reviewVSCodeReviewerAgentInstalled
}

// reviewVSCodeReviewerAgentIsSoleMissingCondition reports whether
// vscode-copilot's refusal is caused by nothing but the managed reviewer
// agent: the compiled manifest already advertises the executor. Like the Pi
// handshake helper it only selects the cause the operator reads;
// reviewImmutableRuntimeCapability stays the only admission authority.
func reviewVSCodeReviewerAgentIsSoleMissingCondition(agent model.AgentID) bool {
	if agent != model.AgentVSCodeCopilot {
		return false
	}
	manifest, err := capabilitymanifest.ForAgent(agent)
	if err != nil || !manifest.Advertises(capabilitymanifest.ContractImmutableReviewExecutorV1) {
		return false
	}
	return reviewVSCodeReviewerAgentStatus() != reviewVSCodeReviewerAgentInstalled
}

// reviewVSCodeReviewerAgentGuidance names the runnable remedy for the refused
// runtime the caller declared (the sync command repeats that caller-supplied
// identity rather than a compiled constant, per issue #2440). The installer
// preserves a reviewer file it does not own, so a modified file must be
// removed before sync can reinstall the managed one. The prose carries no
// path separators: it crosses reviewScrubDefectReportField, which would
// redact anything shaped like a path.
func reviewVSCodeReviewerAgentGuidance(agent model.AgentID, state reviewVSCodeReviewerAgentState) string {
	prefix := "; " + string(agent) + " is eligible only while the managed `" + reviewassets.VSCodeReviewerAgentFileName +
		"` reviewer agent in the VS Code user prompts folder is "
	sync := "`gentle-ai sync --agent " + string(agent) + "`"
	switch state {
	case reviewVSCodeReviewerAgentModified:
		return prefix + "byte-identical to the installed one, and it is modified; delete that file, run " + sync + ", and re-run"
	case reviewVSCodeReviewerAgentUnverifiable:
		return prefix + "installed, and it is unverifiable; run " + sync + " and re-run"
	default:
		return prefix + "installed, and it is missing; run " + sync + " and re-run"
	}
}
