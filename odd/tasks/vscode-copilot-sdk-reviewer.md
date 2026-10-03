# VS Code Copilot host-relay reviewer transport

Locator: `odd/tasks/vscode-copilot-sdk-reviewer.md` · Engram mirror: `odd/vscode-copilot-sdk-reviewer/tasks`
Branch: `feat/rdd-vscode-copilot-reviewer`

## Objective

Give the `vscode-copilot` runtime native RDD review through a host relay, like Pi: the gentle-ai Go binary freezes
evidence, materializes the reviewer prompt, and admits the result; VS Code Copilot Chat runs the reviewer in an
isolated, tool-less custom subagent (`runSubagent`) and submits the raw result with `--input`.

## Problem / Why

`vscode-copilot` has RDD dormant. Users with only GitHub Copilot Chat cannot run native review. The Copilot CLI is
blocked by org policy and the Copilot SDK bundles that same CLI runtime, so a subprocess transport is not viable.
No Node, no CLI: everything on the gentle-ai side is Go.

## Decisions

- 2026-10-03: SDK/Node subprocess adapter discarded (user: reviewer must run through the gentle-ai Go binary like
  OpenCode/Pi). Chosen: host relay.
- Gating: no env handshake (compiled manifest eligibility, like Claude/Codex). VS Code has no launcher process to
  set an env proof; an installer-set `terminal.integrated.env.*` handshake is unverified.
- Host-relay checks generalized from Pi-only equality to a shared "host relay transport" predicate.
- One generic reviewer agent (`gentle-reviewer.agent.md`) suffices: the prompt is opaque materialized bytes.

## Constraints

- RDD semantics untouched (lens selection, severities, refuter, correction, admission). Adapter/host output is
  advisory; only Go admission creates authority.
- Materialize prints raw prompt bytes; submission tokens follow the existing relay validation in
  `review_status_contract.go`.
- Risk (unverified): VS Code may normalize `tools: []` to "all tools" on some paths (DeepWiki contradictory). The
  reviewer body forbids tool use; organic proof must confirm zero tools.
- Artifacts in English. VS Code doubts resolved via DeepWiki `microsoft/vscode`.

## Delivery

- Strategy: `ask-on-risk`. Forecast ≈ 380–530 authored lines → chain strategy asked once before the first commit.

## Tasks

- [x] T1 — Go transport: vscode host-relay transport constant + shared host-relay predicate (review_transport_capability,
  review_provider_runtime, review_artifact, review_provider_role_capture), registered runtime, manifest exposure maps,
  RDD agent list, vscode capture-transport contract body (reviewassets/contract.go + review-ledger-contract.md),
  updated parity/count/digest tests, materialize→`--input` round-trip test. Route: delegated direct.
- [x] T2 — Assets/install: `internal/assets/vscode/agents/gentle-reviewer.agent.md` (tool-less, user-invocable false,
  agents []), vscode adapter agent dir wiring + install, tests. Route: delegated direct.
- [ ] T3 — Organic proof in VS Code Copilot Chat: positive capture and zero-tools confirmation. Pending: user env.

## Acceptance criteria

- `go test ./...` green (delegated), `go vet ./...` and `gofmt` clean.
- STATUS for `--agent vscode-copilot` returns capture inputs with `--materialize=true` and an `--input` submission.
- Non-vscode runtimes behave identically (existing tests unchanged except counts/digests).

## Progress / Evidence

- Exploration done (transport map, Pi/OpenCode relay map, DeepWiki VS Code subagent semantics). RDD mode: on (global).
- Discarded: Node SDK adapter (never committed).
- T1 (delegated writer, uncommitted): RED observed as compile failure of `internal/cli/review_vscode_host_relay_test.go`
  (undefined `reviewImmutableTransportVSCodeHostRelay` / `reviewImmutableTransportIsHostRelay`); GREEN after
  implementation. Added transport `vscode_copilot_host_relay`, shared predicate `reviewImmutableTransportIsHostRelay`
  (replaces Pi-only checks in review_provider_runtime/review_artifact/review_provider_role_capture; next_transition and
  status_contract already route through `reviewProviderHostRelayMaterializeRuntime`), registered runtime, manifest
  exposure, RDD agent list, vscode capture body + sequential reviewer group in reviewassets/contract.go, ledger prose.
  Pi handshake path unchanged. vscode manifest digest now
  `sha256:36912502645bad0505186814e3b892658dd72e1c3764ca8303b56d4f0b671436`. Verification results in the T1 handoff.
  Writer: gofmt/vet/build clean; 43 packages ok (internal/cli full run passed). Parent spot check:
  `go test ./internal/cli/ -run 'VSCode|HostRelay' -count=1` → 10 passed; reviewassets/capabilitymanifest/model → 277 passed.
  Open: shared ledger sentence "Tokens carrying `--agent` capture in process with no `--input`" is inaccurate for
  vscode materialize tokens (reword in T2).
- T2 (delegated writer, uncommitted): RED observed: compile failure of reviewassets/cli tests (undefined
  `VSCodeReviewerAgentName`) and `TestSubAgentsDirIsVSCodeUserPromptsFolder` failing (`EmbeddedSubAgentsDir() = ""`).
  GREEN after implementation. Asset `internal/assets/vscode/agents/gentle-reviewer.agent.md` (embedded via `all:vscode`)
  installs through the existing native-agent path: `NativeAgentManifest[vscode-copilot] = {gentle-reviewer.agent.md}`,
  adapter `SubAgentsDir` = VS Code user `prompts` folder, `EmbeddedSubAgentsDir` = `vscode/agents`,
  `Features.FileSubAgents` left false (no SDD sub-agents, manifest digest unchanged). `renderNativeAgent` returns the
  vscode reviewer verbatim (no CodeGraph/language/remote-auth injection). Ownership ledger, backup snapshots, user-file
  preservation, and idempotency come from the shared installer. `VSCodeReviewerAgentName` constant feeds both the
  contract `agentName` and the manifest; a cross-check test pins the asset name. Ledger sentence reworded for
  `--materialize=true` tokens. Checks: gofmt/vet/build clean; agents/components/assets/model ok; internal/cli 3138
  passed; update/recoverytrace 579 passed. Per-OS path test follows the existing runtime.GOOS pattern (linux branch
  observed locally; darwin/windows run in CI).
- Commits on fork `jagoqui/gentle-ai`: T1 `9b779cbf`, T2 `47ff4f88`. Parent spot check T2: vscode + reviewassets → 54 passed.
- RDD review (range main..47ff4f88): assessed medium (slice_budget_reached, 692 lines); consent granted; one lens
  (review-reliability) approved; acknowledged, lineage `review-4e880287a4e41236`, authority burned.
- Non-blocking follow-ups from review (separate later work):
  - R3-vscode-isolation-unproved: immutable-executor advertisement rests on `tools: []` + prose until T3 proves zero tools.
  - R3-vscode-eligibility-unconditional: no check that `gentle-reviewer` is installed / `runSubagent` available before
    STATUS offers the relay; no test for the absent-agent case.
  - R3-vscode-subagentsdir-activation: `SubAgentsDir` now returns the shared prompts folder; prove cleanup/uninstall
    paths never touch other user files there.

## Next step

T3: user builds the branch on Windows, runs `gentle-ai sync --agent vscode-copilot`, triggers a review in Copilot Chat
Agent mode, and confirms in Chat Debug View that the `gentle-reviewer` subagent ran with zero tools.
