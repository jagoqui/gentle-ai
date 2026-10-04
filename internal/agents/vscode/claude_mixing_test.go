package vscode

import (
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
		{name: "workspace relative agents key accepted", home: claudeHomeFixture{agents: true}, settings: `{"chat.agentFilesLocations": {".claude/agents": false}}`, want: nil},
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
