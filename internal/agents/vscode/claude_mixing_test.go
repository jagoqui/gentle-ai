package vscode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type claudeHomeFixture struct {
	claudeMd bool
	agents   bool
	skills   bool
}

func writeClaudeHome(t *testing.T, fx claudeHomeFixture) string {
	t.Helper()
	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	mustWrite := func(path string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if fx.claudeMd {
		mustWrite(filepath.Join(claudeDir, "CLAUDE.md"))
	}
	if fx.agents {
		mustWrite(filepath.Join(claudeDir, "agents", "sdd-apply.md"))
	}
	if fx.skills {
		mustWrite(filepath.Join(claudeDir, "skills", "go-testing", "SKILL.md"))
	}
	return home
}

func findingSources(findings []ClaudeMixingFinding) []ClaudeMixingSource {
	sources := make([]ClaudeMixingSource, 0, len(findings))
	for _, f := range findings {
		sources = append(sources, f.Source)
	}
	return sources
}

func TestClaudeConfigMixing(t *testing.T) {
	all := claudeHomeFixture{claudeMd: true, agents: true, skills: true}
	allSources := []ClaudeMixingSource{ClaudeMixingClaudeMd, ClaudeMixingAgents, ClaudeMixingSkills}

	tests := []struct {
		name     string
		home     claudeHomeFixture
		settings string
		want     []ClaudeMixingSource
	}{
		{name: "no claude config yields nothing", home: claudeHomeFixture{}, settings: "", want: nil},
		{name: "claude md with default settings", home: claudeHomeFixture{claudeMd: true}, want: []ClaudeMixingSource{ClaudeMixingClaudeMd}},
		{name: "claude agents with default settings", home: claudeHomeFixture{agents: true}, want: []ClaudeMixingSource{ClaudeMixingAgents}},
		{name: "claude skills with default settings", home: claudeHomeFixture{skills: true}, want: []ClaudeMixingSource{ClaudeMixingSkills}},
		{name: "all sources with default settings", home: all, settings: "{}", want: allSources},
		{
			name: "all sources disabled in JSONC settings",
			home: all,
			settings: `{
  // keep Copilot isolated
  "chat.useClaudeMd": false,
  "chat.agentFilesLocations": { "~/.claude/agents": false, ".github/agents": true, },
  /* skills */
  "chat.agentSkillsLocations": { "~/.claude/skills": false },
}`,
			want: nil,
		},
		{name: "workspace relative agents key keeps home agents", home: claudeHomeFixture{agents: true}, settings: `{"chat.agentFilesLocations": {".claude/agents": false}}`, want: []ClaudeMixingSource{ClaudeMixingAgents}},
		{name: "workspace relative skills key keeps home skills", home: claudeHomeFixture{skills: true}, settings: `{"chat.agentSkillsLocations": {".claude/skills": false}}`, want: []ClaudeMixingSource{ClaudeMixingSkills}},
		{name: "claude md disabled only", home: all, settings: `{"chat.useClaudeMd": false}`, want: []ClaudeMixingSource{ClaudeMixingAgents, ClaudeMixingSkills}},
		{name: "enabled values are not handled", home: all, settings: `{"chat.useClaudeMd": true, "chat.agentFilesLocations": {"~/.claude/agents": true}, "chat.agentSkillsLocations": {"~/.claude/skills": "false"}}`, want: allSources},
		{name: "unparseable settings fall back to defaults", home: all, settings: `{"chat.useClaudeMd": false,,, nope`, want: allSources},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := writeClaudeHome(t, tt.home)
			got := findingSources(ClaudeConfigMixing(home, []byte(tt.settings)))
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("sources = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClaudeConfigMixingIgnoresEmptyClaudeDirs(t *testing.T) {
	home := t.TempDir()
	for _, dir := range []string{"agents", "skills"} {
		if err := os.MkdirAll(filepath.Join(home, ".claude", dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// A non-markdown file does not make the agents folder a source.
	if err := os.WriteFile(filepath.Join(home, ".claude", "agents", "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ClaudeConfigMixing(home, nil); len(got) != 0 {
		t.Fatalf("findings = %v, want none", got)
	}
}

func TestClaudeMixingAdvisoryListsOnlyUnhandledSettings(t *testing.T) {
	home := writeClaudeHome(t, claudeHomeFixture{claudeMd: true, skills: true})
	advisory := ClaudeMixingAdvisory(ClaudeConfigMixing(home, []byte(`{"chat.agentSkillsLocations": {"~/.claude/skills": false}}`)))
	if !strings.Contains(advisory, "VS Code Copilot also loads Claude Code configuration from ~/.claude") {
		t.Fatalf("advisory missing headline:\n%s", advisory)
	}
	if !strings.Contains(advisory, `"chat.useClaudeMd": false`) {
		t.Fatalf("advisory missing useClaudeMd setting:\n%s", advisory)
	}
	for _, unexpected := range []string{"chat.agentFilesLocations", "chat.agentSkillsLocations"} {
		if strings.Contains(advisory, unexpected) {
			t.Fatalf("advisory names handled/absent setting %q:\n%s", unexpected, advisory)
		}
	}
	if ClaudeMixingAdvisory(nil) != "" {
		t.Fatal("advisory without findings must be empty")
	}
}

// TestClaudeConfigMixingAcceptsEquivalentLocationKeys treats every spelling VS
// Code resolves to the user-home Claude folder as the disabling key: tilde and
// the absolute home path with either separator, each with or without a
// trailing separator, for both the agents and the skills setting.
func TestClaudeConfigMixingAcceptsEquivalentLocationKeys(t *testing.T) {
	sources := []struct {
		name    string
		home    claudeHomeFixture
		setting string
		folder  string
		source  ClaudeMixingSource
	}{
		{name: "agents", home: claudeHomeFixture{agents: true}, setting: settingAgentFilesLocations, folder: "agents", source: ClaudeMixingAgents},
		{name: "skills", home: claudeHomeFixture{skills: true}, setting: settingAgentSkillsLocations, folder: "skills", source: ClaudeMixingSkills},
	}
	variants := []struct {
		name string
		key  func(home, folder string) string
	}{
		{name: "tilde", key: func(_, folder string) string { return "~/.claude/" + folder }},
		{name: "tilde trailing slash", key: func(_, folder string) string { return "~/.claude/" + folder + "/" }},
		{name: "absolute home", key: func(home, folder string) string { return filepath.Join(home, ".claude", folder) }},
		{name: "absolute home trailing separator", key: func(home, folder string) string {
			return filepath.Join(home, ".claude", folder) + string(filepath.Separator)
		}},
		{name: "absolute home forward slashes", key: func(home, folder string) string {
			return filepath.ToSlash(filepath.Join(home, ".claude", folder))
		}},
		{name: "absolute home forward slashes trailing slash", key: func(home, folder string) string {
			return filepath.ToSlash(filepath.Join(home, ".claude", folder)) + "/"
		}},
		{name: "absolute home backslashes", key: func(home, folder string) string {
			return strings.ReplaceAll(filepath.ToSlash(filepath.Join(home, ".claude", folder)), "/", `\`)
		}},
		{name: "absolute home backslashes trailing backslash", key: func(home, folder string) string {
			return strings.ReplaceAll(filepath.ToSlash(filepath.Join(home, ".claude", folder)), "/", `\`) + `\`
		}},
	}
	for _, src := range sources {
		for _, variant := range variants {
			t.Run(src.name+"/"+variant.name, func(t *testing.T) {
				home := writeClaudeHome(t, src.home)
				settings := mixingLocationSettings(t, src.setting, variant.key(home, src.folder), false)
				if got := ClaudeConfigMixing(home, settings); len(got) != 0 {
					t.Fatalf("findings = %v, want none for key %q", findingSources(got), variant.key(home, src.folder))
				}
			})
		}
	}
}

// TestClaudeConfigMixingRejectsNonEquivalentLocationKeys keeps keys that do
// not name the user's Claude folder from silencing the advisory. Workspace
// relative keys resolve against the open workspace in VS Code, not the user
// home, so they leave <home>/.claude/<folder> loaded.
func TestClaudeConfigMixingRejectsNonEquivalentLocationKeys(t *testing.T) {
	for _, src := range []struct {
		name    string
		home    claudeHomeFixture
		setting string
		folder  string
		source  ClaudeMixingSource
	}{
		{name: "agents", home: claudeHomeFixture{agents: true}, setting: settingAgentFilesLocations, folder: "agents", source: ClaudeMixingAgents},
		{name: "skills", home: claudeHomeFixture{skills: true}, setting: settingAgentSkillsLocations, folder: "skills", source: ClaudeMixingSkills},
	} {
		for _, key := range []func(home string) string{
			func(string) string { return "~/.claude" },
			func(string) string { return "~/.github/" + src.folder },
			func(string) string { return ".claude/" + src.folder },
			func(string) string { return ".claude/" + src.folder + "/" },
			func(string) string { return "./.claude/" + src.folder },
			func(string) string { return `.claude\` + src.folder },
			func(home string) string { return filepath.Join(home, "other", ".claude", src.folder) },
			func(home string) string { return filepath.Join(filepath.Dir(home), ".claude", src.folder) },
		} {
			home := writeClaudeHome(t, src.home)
			settings := mixingLocationSettings(t, src.setting, key(home), false)
			got := findingSources(ClaudeConfigMixing(home, settings))
			if !reflect.DeepEqual(got, []ClaudeMixingSource{src.source}) {
				t.Fatalf("%s key %q: sources = %v, want %v", src.name, key(home), got, []ClaudeMixingSource{src.source})
			}
		}
	}
}

func mixingLocationSettings(t *testing.T, setting, key string, enabled bool) []byte {
	t.Helper()
	settings, err := json.Marshal(map[string]any{setting: map[string]any{key: enabled}})
	if err != nil {
		t.Fatal(err)
	}
	return settings
}
