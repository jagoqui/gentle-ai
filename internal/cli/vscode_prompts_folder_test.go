package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"maps"
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

// The VS Code relay reviewer installs into the Copilot user agents folder
// (SubAgentsDir, `~/.copilot/agents`), which may also hold the user's own
// custom agents. Earlier builds installed it into the VS Code user prompts
// folder (SystemPromptDir), which also holds gentle-ai.instructions.md and the
// user's own prompts. These tests drive every flow reachable for
// vscode-copilot that resolves either folder and prove only the managed
// reviewer (and its ownership ledger) is ever created, updated, snapshotted,
// or removed there: the owned prompts-folder reviewer is migrated away, and
// everything else stays put.

var vscodePromptsFolderUserFiles = map[string]string{
	"my-agent.agent.md": "---\nname: my-agent\ntools: ['codebase']\n---\nMy own agent.\n",
	"notes.prompt.md":   "---\nmode: ask\n---\nSummarize my notes.\n",
}

var vscodeAgentsFolderUserFiles = map[string]string{
	"my.agent.md": "---\nname: my\ntools: ['codebase']\n---\nMy own Copilot agent.\n",
}

// vscodeInstructionsUserContent seeds gentle-ai.instructions.md. That file is
// the adapter's SystemPromptFile: gentle-ai's routing guidance and persona
// steps merge managed sections into it through their own marker-scoped
// writers, independent of SubAgentsDir, so the tests only require the user's
// own content to survive there, not byte identity.
const vscodeInstructionsUserContent = "# My own Copilot instructions\n\nKeep answers short.\n"

// seedVSCodeFolders seeds the prompts folder with the user's files and the
// reviewer an earlier build installed there (owned through its ledger), and
// the Copilot agents folder with the user's own agent. It returns the prompts
// folder and the agents folder.
func seedVSCodeFolders(t *testing.T, home string) (string, string) {
	t.Helper()
	adapter := vscode.NewAdapter()
	prompts := adapter.SystemPromptDir(home)
	for name, content := range vscodePromptsFolderUserFiles {
		mustWriteFile(t, filepath.Join(prompts, name), []byte(content))
	}
	mustWriteFile(t, filepath.Join(prompts, "gentle-ai.instructions.md"), []byte(vscodeInstructionsUserContent))
	reviewer := []byte(assets.MustRead("vscode/agents/gentle-reviewer.agent.md"))
	sum := sha256.Sum256(reviewer)
	mustWriteFile(t, filepath.Join(prompts, reviewassets.VSCodeReviewerAgentFileName), reviewer)
	mustWriteFile(t, filepath.Join(prompts, reviewassets.OwnershipLedgerFilename),
		[]byte("{\n  \"version\": 1,\n  \"files\": {\n    \""+reviewassets.VSCodeReviewerAgentFileName+"\": \""+hex.EncodeToString(sum[:])+"\"\n  }\n}\n"))
	agentsDir := adapter.SubAgentsDir(home)
	for name, content := range vscodeAgentsFolderUserFiles {
		mustWriteFile(t, filepath.Join(agentsDir, name), []byte(content))
	}
	return prompts, agentsDir
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

func assertVSCodeAgentsFolderUserFilesUntouched(t *testing.T, dir, stage string) {
	t.Helper()
	for name, want := range vscodeAgentsFolderUserFiles {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(got) != want {
			t.Fatalf("%s: %s = %q, %v; want byte-identical %q", stage, name, got, err, want)
		}
	}
}

// assertVSCodeFolderHoldsOnly fails on any entry outside allowed.
func assertVSCodeFolderHoldsOnly(t *testing.T, dir string, allowed []string, stage string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !slices.Contains(allowed, entry.Name()) {
			t.Fatalf("%s: unexpected %s in %s", stage, entry.Name(), dir)
		}
	}
}

func TestVSCodePromptsFolderInstallAndSyncTouchOnlyTheManagedReviewer(t *testing.T) {
	for _, flow := range []string{"install", "sync"} {
		t.Run(flow, func(t *testing.T) {
			home := t.TempDir()
			prompts, agentsDir := seedVSCodeFolders(t, home)
			selection := model.Selection{Agents: []model.AgentID{model.AgentVSCodeCopilot}}
			for run := 0; run < 2; run++ {
				if flow == "install" {
					runInstallInjectionSteps(t, newTestInstallRuntime(t, home, selection))
				} else {
					runSyncInjectionSteps(t, home, selection)
				}
				assertVSCodePromptsFolderUserFilesUntouched(t, prompts, flow)
				assertVSCodeAgentsFolderUserFilesUntouched(t, agentsDir, flow)
			}
			reviewer, err := os.ReadFile(filepath.Join(agentsDir, reviewassets.VSCodeReviewerAgentFileName))
			if err != nil || string(reviewer) != assets.MustRead("vscode/agents/gentle-reviewer.agent.md") {
				t.Fatalf("%s: managed reviewer = %q, %v", flow, reviewer, err)
			}
			// The owned prompts-folder reviewer and its ledger migrated away.
			assertVSCodeFolderHoldsOnly(t, prompts, append(slices.Collect(maps.Keys(vscodePromptsFolderUserFiles)), "gentle-ai.instructions.md"), flow)
			assertVSCodeFolderHoldsOnly(t, agentsDir, append(slices.Collect(maps.Keys(vscodeAgentsFolderUserFiles)), reviewassets.VSCodeReviewerAgentFileName, reviewassets.OwnershipLedgerFilename), flow)
		})
	}
}

// TestVSCodePromptsFolderBackupTargetsNameOnlyManagedFiles proves the
// install and sync rollback snapshots (which restore exactly what they
// captured) name only managed files in the prompts and agents folders, never
// a user's custom agent or prompt, and include the prompts-folder reviewer and
// ledger the migration may remove.
func TestVSCodePromptsFolderBackupTargetsNameOnlyManagedFiles(t *testing.T) {
	home := t.TempDir()
	prompts, agentsDir := seedVSCodeFolders(t, home)
	selection := model.Selection{Agents: []model.AgentID{model.AgentVSCodeCopilot}}
	installTargets, err := backupTargets(home, "", ScopeGlobal, selection, planner.ResolvedPlan{Agents: selection.Agents})
	if err != nil {
		t.Fatal(err)
	}
	syncTargets, err := syncBackupTargetsScoped(home, "", ScopeGlobal, selection, resolveAdapters(selection.Agents))
	if err != nil {
		t.Fatal(err)
	}
	folders := map[string][]string{
		prompts:   {reviewassets.VSCodeReviewerAgentFileName, reviewassets.OwnershipLedgerFilename, "gentle-ai.instructions.md"},
		agentsDir: {reviewassets.VSCodeReviewerAgentFileName, reviewassets.OwnershipLedgerFilename},
	}
	for flow, targets := range map[string][]string{"install": installTargets, "sync": syncTargets} {
		for dir, managed := range folders {
			for _, target := range targets {
				if filepath.Dir(target) != dir {
					if strings.HasPrefix(target, dir+string(filepath.Separator)) {
						t.Fatalf("%s snapshots nested path %s", flow, target)
					}
					continue
				}
				if !slices.Contains(managed, filepath.Base(target)) {
					t.Fatalf("%s snapshots non-managed file %s", flow, target)
				}
			}
			for _, want := range []string{reviewassets.VSCodeReviewerAgentFileName, reviewassets.OwnershipLedgerFilename} {
				if !slices.Contains(targets, filepath.Join(dir, want)) {
					t.Fatalf("%s snapshot omits the managed %s in %s", flow, want, dir)
				}
			}
		}
	}
}

// TestVSCodeSubAgentsDirConsumersGatedOnSupportsSubAgentsStayInert covers the
// consumers that read SubAgentsDir only behind SupportsSubAgents (SDD
// sub-agent verification here, and the upgrade executor's backup list): VS
// Code keeps FileSubAgents off, so they resolve nothing in the agents folder.
func TestVSCodeSubAgentsDirConsumersGatedOnSupportsSubAgentsStayInert(t *testing.T) {
	adapter := vscode.NewAdapter()
	if adapter.SupportsSubAgents() {
		t.Fatal("vscode-copilot advertises file sub-agents; SDD sub-agent consumers would now target the shared agents folder")
	}
	if paths := sddSubAgentPaths(t.TempDir(), adapter); len(paths) != 0 {
		t.Fatalf("sddSubAgentPaths(vscode) = %v, want none", paths)
	}
}

// TestVSCodePromptsFolderUserFilesSurviveUninstall runs the complete and the
// vscode-only partial uninstall over installed prompts and agents folders: uninstall
// never enumerates SubAgentsDir, so the user files stay byte-identical and the
// managed reviewer is retained like every other native review agent.
func TestVSCodePromptsFolderUserFilesSurviveUninstall(t *testing.T) {
	for _, mode := range []string{"complete", "partial"} {
		t.Run(mode, func(t *testing.T) {
			home := t.TempDir()
			dir, agentsDir := seedVSCodeFolders(t, home)
			selection := model.Selection{Agents: []model.AgentID{model.AgentVSCodeCopilot}}
			runInstallInjectionSteps(t, newTestInstallRuntime(t, home, selection))
			reviewerPath := filepath.Join(agentsDir, reviewassets.VSCodeReviewerAgentFileName)
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
			assertVSCodeAgentsFolderUserFilesUntouched(t, agentsDir, mode+" uninstall")
			if got := mustReadFile(t, reviewerPath); string(got) != string(reviewer) {
				t.Fatalf("%s uninstall changed the managed reviewer", mode)
			}
		})
	}
}
