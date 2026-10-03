package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/vscode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/reviewassets"
	componentuninstall "github.com/gentleman-programming/gentle-ai/v4/internal/components/uninstall"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/planner"
)

// The VS Code adapter's SubAgentsDir is the shared VS Code user prompts
// folder: it also holds gentle-ai.instructions.md and the user's own custom
// agents and prompts. These tests drive every flow reachable for
// vscode-copilot that resolves SubAgentsDir and prove only the managed
// reviewer (and its ownership ledger) is ever created, updated, snapshotted,
// or removed there.

var vscodePromptsFolderUserFiles = map[string]string{
	"my-agent.agent.md": "---\nname: my-agent\ntools: ['codebase']\n---\nMy own agent.\n",
	"notes.prompt.md":   "---\nmode: ask\n---\nSummarize my notes.\n",
}

// vscodeInstructionsUserContent seeds gentle-ai.instructions.md. That file is
// the adapter's SystemPromptFile: gentle-ai's routing guidance and persona
// steps merge managed sections into it through their own marker-scoped
// writers, independent of SubAgentsDir, so the tests only require the user's
// own content to survive there, not byte identity.
const vscodeInstructionsUserContent = "# My own Copilot instructions\n\nKeep answers short.\n"

func seedVSCodePromptsFolder(t *testing.T, home string) string {
	t.Helper()
	dir := vscode.NewAdapter().SubAgentsDir(home)
	for name, content := range vscodePromptsFolderUserFiles {
		mustWriteFile(t, filepath.Join(dir, name), []byte(content))
	}
	mustWriteFile(t, filepath.Join(dir, "gentle-ai.instructions.md"), []byte(vscodeInstructionsUserContent))
	return dir
}

func assertVSCodePromptsFolderUserFilesUntouched(t *testing.T, dir, stage string) {
	t.Helper()
	for name, want := range vscodePromptsFolderUserFiles {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(got) != want {
			t.Fatalf("%s: %s = %q, %v; want byte-identical %q", stage, name, got, err, want)
		}
	}
	instructions, err := os.ReadFile(filepath.Join(dir, "gentle-ai.instructions.md"))
	if err != nil || !strings.Contains(string(instructions), vscodeInstructionsUserContent) {
		t.Fatalf("%s: gentle-ai.instructions.md lost the user's own content: %v", stage, err)
	}
}

func TestVSCodePromptsFolderInstallAndSyncTouchOnlyTheManagedReviewer(t *testing.T) {
	for _, flow := range []string{"install", "sync"} {
		t.Run(flow, func(t *testing.T) {
			home := t.TempDir()
			dir := seedVSCodePromptsFolder(t, home)
			selection := model.Selection{Agents: []model.AgentID{model.AgentVSCodeCopilot}}
			for run := 0; run < 2; run++ {
				if flow == "install" {
					runInstallInjectionSteps(t, newTestInstallRuntime(t, home, selection))
				} else {
					runSyncInjectionSteps(t, home, selection)
				}
				assertVSCodePromptsFolderUserFilesUntouched(t, dir, flow)
			}
			reviewer, err := os.ReadFile(filepath.Join(dir, reviewassets.VSCodeReviewerAgentFileName))
			if err != nil || string(reviewer) != assets.MustRead("vscode/agents/gentle-reviewer.agent.md") {
				t.Fatalf("%s: managed reviewer = %q, %v", flow, reviewer, err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if _, user := vscodePromptsFolderUserFiles[entry.Name()]; user || entry.Name() == "gentle-ai.instructions.md" {
					continue
				}
				if entry.Name() != reviewassets.VSCodeReviewerAgentFileName && entry.Name() != reviewassets.OwnershipLedgerFilename {
					t.Fatalf("%s wrote unexpected %s into the prompts folder", flow, entry.Name())
				}
			}
		})
	}
}

// TestVSCodePromptsFolderBackupTargetsNameOnlyManagedFiles proves the
// install and sync rollback snapshots (which restore exactly what they
// captured) name only managed files in the shared prompts folder, never a
// user's custom agent or prompt.
func TestVSCodePromptsFolderBackupTargetsNameOnlyManagedFiles(t *testing.T) {
	home := t.TempDir()
	dir := seedVSCodePromptsFolder(t, home)
	selection := model.Selection{Agents: []model.AgentID{model.AgentVSCodeCopilot}}
	installTargets, err := backupTargets(home, "", ScopeGlobal, selection, planner.ResolvedPlan{Agents: selection.Agents})
	if err != nil {
		t.Fatal(err)
	}
	syncTargets, err := syncBackupTargetsScoped(home, "", ScopeGlobal, selection, resolveAdapters(selection.Agents))
	if err != nil {
		t.Fatal(err)
	}
	managed := []string{reviewassets.VSCodeReviewerAgentFileName, reviewassets.OwnershipLedgerFilename, "gentle-ai.instructions.md"}
	for flow, targets := range map[string][]string{"install": installTargets, "sync": syncTargets} {
		inFolder := 0
		for _, target := range targets {
			if filepath.Dir(target) != dir {
				if strings.HasPrefix(target, dir+string(filepath.Separator)) {
					t.Fatalf("%s snapshots nested prompts-folder path %s", flow, target)
				}
				continue
			}
			inFolder++
			if !slices.Contains(managed, filepath.Base(target)) {
				t.Fatalf("%s snapshots non-managed prompts-folder file %s", flow, target)
			}
		}
		for _, want := range []string{reviewassets.VSCodeReviewerAgentFileName, reviewassets.OwnershipLedgerFilename} {
			if !slices.Contains(targets, filepath.Join(dir, want)) {
				t.Fatalf("%s snapshot omits the managed %s; targets in folder = %d", flow, want, inFolder)
			}
		}
	}
}

// TestVSCodeSubAgentsDirConsumersGatedOnSupportsSubAgentsStayInert covers the
// consumers that read SubAgentsDir only behind SupportsSubAgents (SDD
// sub-agent verification here, and the upgrade executor's backup list): VS
// Code keeps FileSubAgents off, so they resolve nothing in the prompts folder.
func TestVSCodeSubAgentsDirConsumersGatedOnSupportsSubAgentsStayInert(t *testing.T) {
	adapter := vscode.NewAdapter()
	if adapter.SupportsSubAgents() {
		t.Fatal("vscode-copilot advertises file sub-agents; SDD sub-agent consumers would now target the shared prompts folder")
	}
	if paths := sddSubAgentPaths(t.TempDir(), adapter); len(paths) != 0 {
		t.Fatalf("sddSubAgentPaths(vscode) = %v, want none", paths)
	}
}

// TestVSCodePromptsFolderUserFilesSurviveUninstall runs the complete and the
// vscode-only partial uninstall over an installed prompts folder: uninstall
// never enumerates SubAgentsDir, so the user files stay byte-identical and the
// managed reviewer is retained like every other native review agent.
func TestVSCodePromptsFolderUserFilesSurviveUninstall(t *testing.T) {
	for _, mode := range []string{"complete", "partial"} {
		t.Run(mode, func(t *testing.T) {
			home := t.TempDir()
			dir := seedVSCodePromptsFolder(t, home)
			selection := model.Selection{Agents: []model.AgentID{model.AgentVSCodeCopilot}}
			runInstallInjectionSteps(t, newTestInstallRuntime(t, home, selection))
			reviewerPath := filepath.Join(dir, reviewassets.VSCodeReviewerAgentFileName)
			reviewer := mustReadFile(t, reviewerPath)

			svc, err := componentuninstall.NewService(home, t.TempDir(), "dev")
			if err != nil {
				t.Fatal(err)
			}
			if mode == "complete" {
				_, err = svc.CompleteUninstall()
			} else {
				_, err = svc.PartialUninstall([]model.AgentID{model.AgentVSCodeCopilot}, nil)
			}
			if err != nil {
				t.Fatalf("%s uninstall: %v", mode, err)
			}
			assertVSCodePromptsFolderUserFilesUntouched(t, dir, mode+" uninstall")
			if got := mustReadFile(t, reviewerPath); string(got) != string(reviewer) {
				t.Fatalf("%s uninstall changed the managed reviewer", mode)
			}
		})
	}
}
