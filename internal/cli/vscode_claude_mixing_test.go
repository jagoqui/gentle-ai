package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/vscode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

const claudeMixingHeadline = "VS Code Copilot also loads Claude Code configuration from ~/.claude"

// seedClaudeCodeUserConfig leaves the Claude Code user config a prior
// claude-code install would leave behind.
func seedClaudeCodeUserConfig(t *testing.T, home string) {
	t.Helper()
	mustWriteFile(t, filepath.Join(home, ".claude", "CLAUDE.md"), []byte("# Claude orchestrator\n"))
	mustWriteFile(t, filepath.Join(home, ".claude", "agents", "sdd-apply.md"), []byte("agent\n"))
}

func TestVSCodeSyncReportsClaudeConfigMixingAdvisoryWithoutEditingSettings(t *testing.T) {
	home := convergenceTestHome(t)
	seedClaudeCodeUserConfig(t, home)
	settingsPath := vscode.NewAdapter().SettingsPath(home)

	if _, err := RunSync([]string{"--agent", "vscode-copilot"}); err != nil {
		t.Fatalf("first RunSync() error = %v", err)
	}
	before, err := os.ReadFile(settingsPath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}

	result, err := RunSync([]string{"--agent", "vscode-copilot"})
	if err != nil {
		t.Fatalf("RunSync() error = %v; the advisory must never fail the sync", err)
	}
	report := RenderSyncReport(result)
	for _, want := range []string{
		"Advisories:",
		claudeMixingHeadline,
		`"chat.useClaudeMd": false`,
		`"chat.agentFilesLocations": { "~/.claude/agents": false }`,
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("sync report missing %q:\n%s", want, report)
		}
	}
	if strings.Contains(report, "chat.agentSkillsLocations") {
		t.Fatalf("sync report names an absent source (~/.claude/skills):\n%s", report)
	}
	if strings.Contains(report, "Manual actions required") || strings.Contains(report, "preserved native agents were not updated") {
		t.Fatalf("advisory leaked into the manual-actions channel:\n%s", report)
	}

	after, err := os.ReadFile(settingsPath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("sync modified VS Code settings.json:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if bytes.Contains(after, []byte("chat.useClaudeMd")) {
		t.Fatalf("settings.json gained an advisory setting:\n%s", after)
	}
}

func TestVSCodeSyncOmitsClaudeMixingAdvisoryWhenSettingsDisableSources(t *testing.T) {
	home := convergenceTestHome(t)
	seedClaudeCodeUserConfig(t, home)
	if _, err := RunSync([]string{"--agent", "vscode-copilot"}); err != nil {
		t.Fatal(err)
	}
	settingsPath := vscode.NewAdapter().SettingsPath(home)
	current, err := os.ReadFile(settingsPath)
	if os.IsNotExist(err) {
		current, err = []byte("{}\n"), nil
	}
	if err != nil {
		t.Fatal(err)
	}
	// JSONC: a comment and a trailing comma, as VS Code allows.
	disabled := strings.Replace(string(current), "{", "{\n  // isolate Copilot\n  \"chat.useClaudeMd\": false,\n  \"chat.agentFilesLocations\": { \"~/.claude/agents\": false, },", 1)
	mustWriteFile(t, settingsPath, []byte(disabled))

	result, err := RunSync([]string{"--agent", "vscode-copilot"})
	if err != nil {
		t.Fatal(err)
	}
	if report := RenderSyncReport(result); strings.Contains(report, claudeMixingHeadline) {
		t.Fatalf("advisory reported although settings disable every source:\n%s", report)
	}
}

func TestClaudeCodeOnlySyncNeverReportsVSCodeClaudeMixingAdvisory(t *testing.T) {
	home := convergenceTestHome(t)
	seedClaudeCodeUserConfig(t, home)
	result, err := RunSync([]string{"--agent", "claude-code"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Advisories) != 0 {
		t.Fatalf("claude-code sync advisories = %v, want none", result.Advisories)
	}
	if report := RenderSyncReport(result); strings.Contains(report, "Advisories:") || strings.Contains(report, claudeMixingHeadline) {
		t.Fatalf("claude-code sync printed the vscode advisory:\n%s", report)
	}
}

func TestVSCodeInstallReportsClaudeConfigMixingAdvisory(t *testing.T) {
	home := convergenceTestHome(t)
	seedClaudeCodeUserConfig(t, home)
	result, err := RunInstall([]string{"--agent", "vscode-copilot", "--preset", "full-gentleman"}, system.DetectionResult{})
	if err != nil {
		t.Fatalf("RunInstall() error = %v; the advisory must never fail the install", err)
	}
	rendered := RenderInstallManualActions(result)
	if !strings.Contains(rendered, "Advisories:") || !strings.Contains(rendered, claudeMixingHeadline) {
		t.Fatalf("install output missing advisory:\n%s", rendered)
	}
	if strings.Contains(rendered, "Manual actions required") {
		t.Fatalf("advisory rendered as a manual action:\n%s", rendered)
	}
}
