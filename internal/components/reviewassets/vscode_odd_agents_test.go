package reviewassets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// vscodeODDAgents pins the VS Code ODD worker trio: the agentName the
// orchestrator routes to through `#tool:runSubagent` and the VS Code tool sets
// each one is limited to.
var vscodeODDAgents = []struct {
	name  string
	tools string
}{
	{"gentle-ai-explore", "tools: ['read', 'search', 'codegraph/*']"},
	{"gentle-ai-verify", "tools: ['read', 'search', 'execute']"},
	{"gentle-ai-worker", "tools: ['read', 'search', 'edit', 'execute', 'codegraph/*']"},
}

func frontmatterLines(t *testing.T, path, content string) []string {
	t.Helper()
	if !strings.HasPrefix(content, "---\n") {
		t.Fatalf("%s has no frontmatter", path)
	}
	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
		t.Fatalf("%s frontmatter is not closed", path)
	}
	return strings.Split(content[4:4+end], "\n")
}

// TestVSCodeODDAgentAssetsFrontmatter pins each embedded ODD agent's identity
// and tool-set grant: none may delegate, none is user-invocable, and only the
// worker may edit.
func TestVSCodeODDAgentAssetsFrontmatter(t *testing.T) {
	for _, agent := range vscodeODDAgents {
		t.Run(agent.name, func(t *testing.T) {
			path := "vscode/agents/" + agent.name + ".agent.md"
			content, err := assets.Read(path)
			if err != nil {
				t.Fatalf("embedded VS Code ODD agent missing: %v", err)
			}
			lines := frontmatterLines(t, path, content)
			for _, want := range []string{
				"name: " + agent.name,
				agent.tools,
				"agents: []",
				"user-invocable: false",
				"disable-model-invocation: false",
			} {
				if !containsName(lines, want) {
					t.Errorf("%s frontmatter lacks %q", path, want)
				}
			}
			if !slices.ContainsFunc(lines, func(line string) bool { return strings.HasPrefix(line, "description: ") }) {
				t.Errorf("%s frontmatter lacks a description", path)
			}
			if agent.name != "gentle-ai-worker" && strings.Contains(agent.tools, "'edit'") {
				t.Errorf("%s grants edit", path)
			}
			for _, excluded := range []string{"SDD", "sdd-", "opencode", "OpenCode", "`task`", "gentle_review"} {
				if strings.Contains(content, excluded) {
					t.Errorf("%s carries runtime-foreign or retired wording %q", path, excluded)
				}
			}
		})
	}
}

// TestVSCodeNativeAgentManifestShipsReviewerAndODDAgents pins the Copilot
// agents the installer manages: the relay reviewer plus the ODD trio, and no
// other runtime installs a VS Code agent file.
func TestVSCodeNativeAgentManifestShipsReviewerAndODDAgents(t *testing.T) {
	want := []string{VSCodeReviewerAgentFileName}
	for _, agent := range vscodeODDAgents {
		want = append(want, agent.name+".agent.md")
	}
	got := NativeAgentManifest[model.AgentVSCodeCopilot]
	if !slices.Equal(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(want))) {
		t.Fatalf("vscode-copilot native agents = %v, want %v", got, want)
	}
	for runtime, names := range NativeAgentManifest {
		if runtime == model.AgentVSCodeCopilot {
			continue
		}
		for _, name := range names {
			if strings.HasSuffix(name, ".agent.md") {
				t.Errorf("%s installs VS Code agent file %s", runtime, name)
			}
		}
	}
}

// TestInstallNativeAgentsWritesVSCodeODDAgents proves the ODD trio lands in
// `~/.copilot/agents` next to the verbatim reviewer, keeps its tool-set grant,
// receives the injected guidance and language contract, is recorded in the
// ownership ledger, and that a second install is a no-op.
func TestInstallNativeAgentsWritesVSCodeODDAgents(t *testing.T) {
	adapter, err := agents.NewAdapter(model.AgentVSCodeCopilot)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	dir := adapter.SubAgentsDir(home)
	opts := InstallOptions{CodeGraphGuidanceMarkdown: "## CodeGraph\n\nUse codegraph_explore.\n"}

	first, err := InstallNativeAgents(home, adapter, opts)
	if err != nil {
		t.Fatalf("InstallNativeAgents() error = %v", err)
	}
	if !first.Changed || len(first.Skipped) != 0 {
		t.Fatalf("first install = %+v, want changed without skips", first)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	wantNames := []string{OwnershipLedgerFilename, VSCodeReviewerAgentFileName}
	for _, agent := range vscodeODDAgents {
		wantNames = append(wantNames, agent.name+".agent.md")
	}
	if !slices.Equal(slices.Sorted(slices.Values(names)), slices.Sorted(slices.Values(wantNames))) {
		t.Fatalf("agents folder = %v, want %v", names, wantNames)
	}

	if got, _ := os.ReadFile(filepath.Join(dir, VSCodeReviewerAgentFileName)); string(got) != assets.MustRead(vscodeReviewerAsset) {
		t.Fatal("the relay reviewer is no longer installed verbatim")
	}

	for _, agent := range vscodeODDAgents {
		path := filepath.Join(dir, agent.name+".agent.md")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		content := string(data)
		lines := frontmatterLines(t, path, content)
		for _, want := range []string{"name: " + agent.name, agent.tools, "agents: []", "user-invocable: false"} {
			if !containsName(lines, want) {
				t.Errorf("installed %s frontmatter lacks %q", agent.name, want)
			}
		}
		for _, want := range []string{"<!-- gentle-ai:codegraph-guidance -->", "<!-- gentle-ai:agent-language-contract -->", "Use codegraph_explore."} {
			if !strings.Contains(content, want) {
				t.Errorf("installed %s lacks injected %q", agent.name, want)
			}
		}
	}

	raw, err := os.ReadFile(filepath.Join(dir, OwnershipLedgerFilename))
	if err != nil {
		t.Fatal(err)
	}
	var ledger struct {
		Files map[string]string `json:"files"`
	}
	if err := json.Unmarshal(raw, &ledger); err != nil {
		t.Fatal(err)
	}
	for _, name := range wantNames[1:] {
		if ledger.Files[name] == "" {
			t.Errorf("ownership ledger does not record %s", name)
		}
	}

	second, err := InstallNativeAgents(home, adapter, opts)
	if err != nil {
		t.Fatalf("second InstallNativeAgents() error = %v", err)
	}
	if second.Changed || len(second.Files) != 0 {
		t.Fatalf("second install = %+v, want no change", second)
	}
}
