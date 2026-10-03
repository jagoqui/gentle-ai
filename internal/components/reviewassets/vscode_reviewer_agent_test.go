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

// TestManagedVSCodeReviewerAgentIsTheInstalledRender pins the bytes the vscode
// relay eligibility gate compares against to the installer's own output, so
// the gate and the installer can never drift (any InstallOptions).
func TestManagedVSCodeReviewerAgentIsTheInstalledRender(t *testing.T) {
	adapter, err := agents.NewAdapter(model.AgentVSCodeCopilot)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	if _, err := InstallNativeAgents(home, adapter, InstallOptions{CodeGraphGuidanceMarkdown: "## CodeGraph\n\nUse codegraph_explore.\n"}); err != nil {
		t.Fatal(err)
	}
	installed, err := os.ReadFile(filepath.Join(adapter.SubAgentsDir(home), VSCodeReviewerAgentFileName))
	if err != nil {
		t.Fatal(err)
	}
	managed, err := ManagedVSCodeReviewerAgent(adapter)
	if err != nil {
		t.Fatalf("ManagedVSCodeReviewerAgent() error = %v", err)
	}
	if string(managed) != string(installed) {
		t.Fatalf("managed render differs from the installed bytes:\nmanaged %q\ninstalled %q", managed, installed)
	}
	if VSCodeReviewerAgentFileName != VSCodeReviewerAgentName+".agent.md" {
		t.Fatalf("VSCodeReviewerAgentFileName = %q", VSCodeReviewerAgentFileName)
	}

	claude, err := agents.NewAdapter(model.AgentClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ManagedVSCodeReviewerAgent(claude); err == nil {
		t.Fatal("ManagedVSCodeReviewerAgent accepted a non-vscode adapter")
	}
}

// TestInstallNativeAgentsPreservesVSCodePromptsFolderUserFiles proves the
// native installer touches only the managed reviewer and its ownership ledger
// in the shared VS Code user prompts folder: the gentle-ai instructions file,
// a user's own custom agent, and a user prompt stay byte-identical across a
// first install, a managed-file refresh, and an idempotent re-run.
func TestInstallNativeAgentsPreservesVSCodePromptsFolderUserFiles(t *testing.T) {
	adapter, err := agents.NewAdapter(model.AgentVSCodeCopilot)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	dir := adapter.SubAgentsDir(home)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	userFiles := map[string]string{
		"gentle-ai.instructions.md": "managed instructions owned by another component\n",
		"my-agent.agent.md":         "---\nname: my-agent\ntools: ['codebase']\n---\nmine\n",
		"notes.prompt.md":           "---\nmode: ask\n---\nnotes\n",
	}
	for name, content := range userFiles {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	assertUserFiles := func(stage string) {
		t.Helper()
		for name, want := range userFiles {
			got, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil || string(got) != want {
				t.Fatalf("%s: user file %s = %q, %v; want byte-identical %q", stage, name, got, err, want)
			}
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if _, user := userFiles[entry.Name()]; user {
				continue
			}
			if entry.Name() != VSCodeReviewerAgentFileName && entry.Name() != OwnershipLedgerFilename {
				t.Fatalf("%s: unexpected file %s in the prompts folder", stage, entry.Name())
			}
		}
	}

	if _, err := InstallNativeAgents(home, adapter, InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	assertUserFiles("first install")

	// A stale owned reviewer (an older managed render) is refreshed in place.
	ledger, err := os.ReadFile(filepath.Join(dir, OwnershipLedgerFilename))
	if err != nil {
		t.Fatal(err)
	}
	stale := []byte("---\nname: gentle-reviewer\ntools: []\n---\nolder managed body\n")
	if err := os.WriteFile(filepath.Join(dir, VSCodeReviewerAgentFileName), stale, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, OwnershipLedgerFilename), []byte(strings.Replace(string(ledger), installedHash(mustManagedVSCodeReviewer(t, adapter)), installedHash(stale), 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	refresh, err := InstallNativeAgents(home, adapter, InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !refresh.Changed {
		t.Fatalf("stale owned reviewer was not refreshed: %+v", refresh)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, VSCodeReviewerAgentFileName)); string(got) != string(mustManagedVSCodeReviewer(t, adapter)) {
		t.Fatalf("refreshed reviewer = %q", got)
	}
	assertUserFiles("managed refresh")

	again, err := InstallNativeAgents(home, adapter, InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if again.Changed {
		t.Fatalf("idempotent re-run changed files: %+v", again)
	}
	assertUserFiles("idempotent re-run")
}

func mustManagedVSCodeReviewer(t *testing.T, adapter agents.Adapter) []byte {
	t.Helper()
	managed, err := ManagedVSCodeReviewerAgent(adapter)
	if err != nil {
		t.Fatal(err)
	}
	return managed
}
