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

// frontmatterTools parses the `tools:` list from frontmatter lines, so grant
// checks read the asset itself rather than this file's expectation table.
func frontmatterTools(t *testing.T, path string, lines []string) []string {
	t.Helper()
	for _, line := range lines {
		value, ok := strings.CutPrefix(line, "tools:")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
			t.Fatalf("%s tools is not an inline list: %q", path, value)
		}
		var tools []string
		for _, item := range strings.Split(strings.Trim(value, "[]"), ",") {
			if item = strings.Trim(strings.TrimSpace(item), `'"`); item != "" {
				tools = append(tools, item)
			}
		}
		return tools
	}
	t.Fatalf("%s frontmatter has no tools line", path)
	return nil
}

// vscodeAgentsWithoutCodeGraph lists the ODD agents that carry no CodeGraph
// tool set and therefore must not receive CodeGraph guidance.
var vscodeAgentsWithoutCodeGraph = []string{"gentle-ai-verify"}

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
			tools := frontmatterTools(t, path, lines)
			if got, want := slices.Contains(tools, "edit"), agent.name == "gentle-ai-worker"; got != want {
				t.Errorf("%s edit grant = %v, want %v (tools %v)", path, got, want, tools)
			}
			if agent.name == "gentle-ai-explore" && slices.Contains(tools, "execute") {
				t.Errorf("%s grants execute (tools %v)", path, tools)
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
		if !strings.Contains(content, "<!-- gentle-ai:agent-language-contract -->") {
			t.Errorf("installed %s lacks the injected language contract", agent.name)
		}
		withoutCodeGraph := slices.Contains(vscodeAgentsWithoutCodeGraph, agent.name)
		if withoutCodeGraph && slices.Contains(frontmatterTools(t, path, lines), "codegraph/*") {
			t.Fatalf("%s is exempt from CodeGraph guidance but grants codegraph/*", agent.name)
		}
		for _, marker := range []string{"<!-- gentle-ai:codegraph-guidance -->", "Use codegraph_explore."} {
			if got := strings.Contains(content, marker); got == withoutCodeGraph {
				t.Errorf("installed %s contains %q = %v, want %v", agent.name, marker, got, !withoutCodeGraph)
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

// TestVSCodeODDExplorerAllowsDirectReads pins that the explorer uses CodeGraph
// first only for structural questions and never needs a CodeGraph failure
// before reading parent-named files or doing literal lookups.
func TestVSCodeODDExplorerAllowsDirectReads(t *testing.T) {
	for _, name := range []string{"gentle-ai-explore", "gentle-ai-worker"} {
		path := "vscode/agents/" + name + ".agent.md"
		content := assets.MustRead(path)
		for _, forbidden := range []string{
			"Do not use that fallback before CodeGraph is unavailable or fails",
			"If CodeGraph reports that it is unavailable or fails, then use",
		} {
			if strings.Contains(content, forbidden) {
				t.Errorf("%s gates direct reads behind a CodeGraph failure: %q", path, forbidden)
			}
		}
	}
	path := "vscode/agents/gentle-ai-explore.agent.md"
	content := assets.MustRead(path)
	for _, want := range []string{
		"For structural questions (architecture, call flow, dependencies, impact), use CodeGraph first when it is available",
		"Read and search parent-named files or literal lookups directly",
		"If CodeGraph tools are unavailable, proceed with `read` and `search` without retrying CodeGraph.",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("%s lacks %q", path, want)
		}
	}
}
