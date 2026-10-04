package vscode

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
)

// ClaudeMixingSource names one piece of Claude Code user configuration that
// VS Code Copilot Chat loads by default alongside its own configuration.
type ClaudeMixingSource string

const (
	// ClaudeMixingClaudeMd is ~/.claude/CLAUDE.md, attached while
	// chat.useClaudeMd is enabled (VS Code default: true).
	ClaudeMixingClaudeMd ClaudeMixingSource = "claude-md"
	// ClaudeMixingAgents is ~/.claude/agents, discovered through
	// chat.agentFilesLocations unless that entry is set to false.
	ClaudeMixingAgents ClaudeMixingSource = "claude-agents"
	// ClaudeMixingSkills is ~/.claude/skills, discovered through
	// chat.agentSkillsLocations unless that entry is set to false.
	ClaudeMixingSkills ClaudeMixingSource = "claude-skills"
)

// ClaudeMixingFinding is one Claude Code source that is present on disk and
// not disabled in the VS Code user settings. Setting is the exact JSON line
// that disables it.
type ClaudeMixingFinding struct {
	Source  ClaudeMixingSource
	Path    string
	Setting string
}

const (
	settingUseClaudeMd          = "chat.useClaudeMd"
	settingAgentFilesLocations  = "chat.agentFilesLocations"
	settingAgentSkillsLocations = "chat.agentSkillsLocations"
)

// ClaudeConfigMixing reports the Claude Code sources under <home>/.claude that
// VS Code Copilot Chat would mix into its context given the VS Code user
// settings.json content. settingsJSONC may be empty, missing or unparseable:
// such settings are treated as VS Code defaults (every source enabled). It
// only reads; it never writes the settings file.
func ClaudeConfigMixing(home string, settingsJSONC []byte) []ClaudeMixingFinding {
	settings, err := filemerge.UnmarshalJSONObject(settingsJSONC)
	if err != nil {
		settings = map[string]any{}
	}
	claudeDir := filepath.Join(home, ".claude")
	var findings []ClaudeMixingFinding

	claudeMd := filepath.Join(claudeDir, "CLAUDE.md")
	if isRegularFile(claudeMd) && !isFalse(settings[settingUseClaudeMd]) {
		findings = append(findings, ClaudeMixingFinding{
			Source:  ClaudeMixingClaudeMd,
			Path:    claudeMd,
			Setting: `"chat.useClaudeMd": false`,
		})
	}

	agentsDir := filepath.Join(claudeDir, "agents")
	if dirHasMarkdown(agentsDir) && !locationDisabled(settings[settingAgentFilesLocations], home, "agents") {
		findings = append(findings, ClaudeMixingFinding{
			Source:  ClaudeMixingAgents,
			Path:    agentsDir,
			Setting: `"chat.agentFilesLocations": { "~/.claude/agents": false }`,
		})
	}

	skillsDir := filepath.Join(claudeDir, "skills")
	if dirNonEmpty(skillsDir) && !locationDisabled(settings[settingAgentSkillsLocations], home, "skills") {
		findings = append(findings, ClaudeMixingFinding{
			Source:  ClaudeMixingSkills,
			Path:    skillsDir,
			Setting: `"chat.agentSkillsLocations": { "~/.claude/skills": false }`,
		})
	}
	return findings
}

// ClaudeMixingAdvisory renders the operator advisory for findings, or "" when
// there is nothing to report. It lists only the settings still needed.
func ClaudeMixingAdvisory(findings []ClaudeMixingFinding) string {
	if len(findings) == 0 {
		return ""
	}
	lines := make([]string, 0, len(findings))
	for _, finding := range findings {
		lines = append(lines, "    "+finding.Setting)
	}
	return "VS Code Copilot also loads Claude Code configuration from ~/.claude, which can conflict with the " +
		"vscode-copilot review contract. To keep Copilot isolated, add to your VS Code user settings.json " +
		"(merge into existing objects; gentle-ai never edits this file):\n" +
		strings.Join(lines, ",\n")
}

func isFalse(value any) bool {
	b, ok := value.(bool)
	return ok && !b
}

// locationDisabled reports whether a chat.*Locations map disables the Claude
// folder <home>/.claude/<folder> under a spelling VS Code resolves to the user
// home: `~/.claude/<folder>` or the absolute home path, with either separator
// and with or without a trailing one. Workspace-relative keys such as
// `.claude/<folder>` resolve against the open workspace, not the user home, so
// they never count. Keys are compared exactly after separator normalization
// and cleaning.
func locationDisabled(value any, home, folder string) bool {
	locations, ok := value.(map[string]any)
	if !ok {
		return false
	}
	equivalent := []string{"~/.claude/" + folder}
	if strings.TrimSpace(home) != "" {
		equivalent = append(equivalent, normalizeLocationKey(filepath.Join(home, ".claude", folder)))
	}
	for key, enabled := range locations {
		if !isFalse(enabled) {
			continue
		}
		normalized := normalizeLocationKey(key)
		for _, want := range equivalent {
			if normalized == want {
				return true
			}
		}
	}
	return false
}

// normalizeLocationKey turns a settings location key into slash form without
// trailing separators, so `C:\Users\me\.claude\agents\` and
// `C:/Users/me/.claude/agents` compare equal on every host.
func normalizeLocationKey(key string) string {
	slashed := strings.ReplaceAll(strings.TrimSpace(key), `\`, "/")
	if slashed == "" {
		return ""
	}
	return path.Clean(slashed)
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func dirHasMarkdown(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			return true
		}
	}
	return false
}

func dirNonEmpty(dir string) bool {
	entries, err := os.ReadDir(dir)
	return err == nil && len(entries) > 0
}
