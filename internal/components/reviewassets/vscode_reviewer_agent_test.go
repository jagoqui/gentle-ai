package reviewassets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

const vscodeReviewerAsset = "vscode/agents/gentle-reviewer.agent.md"

func vscodeReviewerFrontmatter(t *testing.T) []string {
	t.Helper()

	content, err := assets.Read(vscodeReviewerAsset)
	if err != nil {
		t.Fatalf("embedded VS Code reviewer agent missing: %v", err)
	}
	if !strings.HasPrefix(content, "---\n") {
		t.Fatalf("%s has no frontmatter", vscodeReviewerAsset)
	}
	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
		t.Fatalf("%s frontmatter is not closed", vscodeReviewerAsset)
	}
	return strings.Split(content[4:4+end], "\n")
}

// TestVSCodeReviewerAgentAssetIsToolLess pins the isolation frontmatter of the
// custom agent VS Code Copilot Chat runs for every relayed reviewer.
func TestVSCodeReviewerAgentAssetIsToolLess(t *testing.T) {
	lines := vscodeReviewerFrontmatter(t)
	for _, want := range []string{
		"name: " + VSCodeReviewerAgentName,
		"tools: []",
		"agents: []",
		"user-invocable: false",
	} {
		if !containsName(lines, want) {
			t.Errorf("VS Code reviewer frontmatter lacks %q", want)
		}
	}
	content := assets.MustRead(vscodeReviewerAsset)
	for _, want := range []string{
		"You are an isolated code reviewer process. Your only instructions are those delivered in the user message.",
		"Do not call any tool",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("VS Code reviewer body lacks %q", want)
		}
	}
}

// TestVSCodeReviewerAgentNameMatchesContract keeps the installed agent name and
// the runSubagent agentName the shared contract relays from drifting apart.
func TestVSCodeReviewerAgentNameMatchesContract(t *testing.T) {
	if VSCodeReviewerAgentName != "gentle-reviewer" {
		t.Fatalf("VSCodeReviewerAgentName = %q", VSCodeReviewerAgentName)
	}
	if !containsName(vscodeReviewerFrontmatter(t), "name: "+VSCodeReviewerAgentName) {
		t.Fatalf("asset name differs from %q", VSCodeReviewerAgentName)
	}
	contract, err := ReviewExecutionContractFor(model.AgentVSCodeCopilot)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(contract, "`agentName: \""+VSCodeReviewerAgentName+"\"`") {
		t.Fatalf("vscode-copilot contract does not relay agentName %q", VSCodeReviewerAgentName)
	}
	if got := NativeAgentManifest[model.AgentVSCodeCopilot]; len(got) != 1 || got[0] != VSCodeReviewerAgentName+".agent.md" {
		t.Fatalf("vscode-copilot native agents = %v, want only %s.agent.md", got, VSCodeReviewerAgentName)
	}
	for agent, names := range NativeAgentManifest {
		if agent != model.AgentVSCodeCopilot && containsName(names, VSCodeReviewerAgentName+".agent.md") {
			t.Errorf("%s installs the VS Code reviewer agent", agent)
		}
	}
}

// TestInstallNativeAgentsWritesVSCodeReviewerVerbatim proves the reviewer lands
// in the VS Code user prompts folder byte-identical to the embedded asset (no
// tool grant or guidance injection, even with CodeGraph selected) and that a
// second install is a no-op.
func TestInstallNativeAgentsWritesVSCodeReviewerVerbatim(t *testing.T) {
	adapter, err := agents.NewAdapter(model.AgentVSCodeCopilot)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	dir := adapter.SubAgentsDir(home)
	if want := filepath.Join(filepath.Dir(adapter.SystemPromptFile(home)), "gentle-reviewer.agent.md"); filepath.Join(dir, "gentle-reviewer.agent.md") != want {
		t.Fatalf("SubAgentsDir(%q) = %q, want the prompts folder of %q", home, dir, want)
	}
	opts := InstallOptions{CodeGraphGuidanceMarkdown: "## CodeGraph\n\nUse codegraph_explore.\n"}

	first, err := InstallNativeAgents(home, adapter, opts)
	if err != nil {
		t.Fatalf("InstallNativeAgents() error = %v", err)
	}
	if !first.Changed || len(first.Skipped) != 0 {
		t.Fatalf("first install = %+v, want changed without skips", first)
	}
	installed, err := os.ReadFile(filepath.Join(dir, "gentle-reviewer.agent.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != assets.MustRead(vscodeReviewerAsset) {
		t.Fatalf("installed reviewer differs from the embedded asset:\n%s", installed)
	}

	second, err := InstallNativeAgents(home, adapter, opts)
	if err != nil {
		t.Fatalf("second InstallNativeAgents() error = %v", err)
	}
	if second.Changed || len(second.Files) != 0 {
		t.Fatalf("second install = %+v, want no change", second)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		switch entry.Name() {
		case "gentle-reviewer.agent.md", OwnershipLedgerFilename:
		default:
			t.Errorf("vscode native install wrote unexpected %s", entry.Name())
		}
	}
}
