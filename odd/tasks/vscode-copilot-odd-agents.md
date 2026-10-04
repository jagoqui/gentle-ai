# VS Code Copilot ODD agents

Locator: `odd/tasks/vscode-copilot-odd-agents.md` · Engram mirror: `odd/vscode-copilot-odd-agents/tasks`
Branch: `feat/vscode-copilot-odd-agents` (stacked on `feat/rdd-vscode-copilot-reviewer`)

## Objective

Give `vscode-copilot` its own native ODD worker agents (explorer, verifier, bounded writer) so the orchestrator can
delegate through `#tool:runSubagent` with `agentName`, instead of relying on Claude Code agents that VS Code mixes in
from `~/.claude/agents`.

## Problem / Why

gentle-ai ships no ODD agents to VS Code. The vscode orchestrator renders `generic/orchestrator.md` ("native bounded
worker"), and the agents users see in Copilot are Claude Code files (Claude tool names) discovered by VS Code
defaults. SDD is retired: only ODD and RDD apply.

## Decisions

- 2026-10-04: no SDD agents (retired repo-wide). RDD needs no new agent: `gentle-reviewer` already relays lens,
  refuter and targeted validator.
- Mirror OpenCode's ODD trio (`gentle-ai-explore`, `gentle-ai-verify`, `gentle-ai-worker`), reusing their prompt
  bodies; permissions expressed as VS Code tool sets (DeepWiki microsoft/vscode `languageModelToolSets`):
  explore `read, search, codegraph/*`; verify `read, search, execute`; worker `read, search, edit, execute,
  codegraph/*`. All `agents: []`, `user-invocable: false`.
- Ship via `reviewassets.NativeAgentManifest` into `~/.copilot/agents`; keep `FileSubAgents` false (manifest digest
  unchanged).

## Constraints

- Other runtimes byte-identical. RDD semantics untouched. Artifacts in English.
- Unverified: exact membership of the `execute` tool set (terminal) and `codegraph/*` MCP syntax; organic proof needed.

## Tasks

- [x] U1 — Assets + install: three `.agent.md` under `internal/assets/vscode/agents/`, manifest entries, tests
  (relax the single-entry assertion). Route: delegated direct.
- [x] U2 — vscode orchestrator delegation wording naming the three agents via `#tool:runSubagent` `agentName`, plus
  RDD `vscodeCapture` wording covering `capture-refuter` / `capture-validation`; parity tests. Route: delegated direct.
- [ ] U3 — Organic proof in VS Code (agents discovered, tool sets honored). Pending: user env.

## Acceptance criteria

- `go test ./internal/...` green (delegated); gofmt/vet clean.
- `gentle-ai sync --agent vscode-copilot` writes the four managed agents to `~/.copilot/agents` with ownership ledger.
- vscode orchestrator names the agents; generic orchestrator for other runtimes unchanged.

## Delivery

- Strategy `ask-on-risk`; forecast ≈ 200–300 authored lines (single slice).

## Progress / Evidence

- Exploration done (native agent map, ODD roles, DeepWiki VS Code tool sets).
- U1 implemented (delegated writer, uncommitted): `internal/assets/vscode/agents/gentle-ai-{explore,verify,worker}.agent.md`
  (OpenCode bodies; `task`/`bash`/`find`/`gentle_review`/`mem_save`/runner-watchdog wording replaced with VS Code or
  neutral wording); manifest entries in `reviewassets.NativeAgentManifest[vscode-copilot]`; reviewer still verbatim,
  ODD trio gets codegraph-guidance + language-contract injection. Gating: ships unconditionally, like OpenCode's trio
  (ODD is every runtime's default; only review agents are RDD-gated). RED: `TestVSCodeODDAgentAssetsFrontmatter`,
  `TestVSCodeNativeAgentManifestShipsReviewerAndODDAgents`, `TestInstallNativeAgentsWritesVSCodeODDAgents` failed
  (assets missing / manifest single entry); GREEN after implementation. CLI prompts/agents-folder tests widened to the
  full managed set (still exact allowlists).
- U2 implemented (delegated writer, uncommitted): render-time substitution `replaceVSCodeDelegationRoute`
  (agentguidance/orchestrator.go) keeps `generic/orchestrator.md` byte-identical; capable variant replaces the neutral
  sentence, small variant gains the route; fails closed on anchor drift. `vscodeCapture` now covers
  `review.capture-result`, `review.capture-refuter`, `review.capture-validation`. RED:
  `TestRenderOrchestratorVSCodeNamesODDAgents` (undefined route), contract test lacking refuter/validation wording;
  GREEN after.
- Checks: `gofmt -l internal/` empty; `go vet ./internal/...` ok; `go build ./...` ok;
  `go test ./internal/agents/... ./internal/components/... ./internal/assets/... ./internal/model/...` ok;
  `go test ./internal/cli/...` ok (405s).
