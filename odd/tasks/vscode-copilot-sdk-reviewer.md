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
- [x] T4 — Review follow-ups (user-authorized 2026-10-03): (a) vscode relay eligibility requires the installed
  `gentle-reviewer.agent.md` to exist and match the embedded asset bytes, else typed refusal naming
  `gentle-ai sync --agent vscode-copilot` (covers eligibility-unconditional + tamper side of isolation-unproved;
  runtime zero-tools proof stays T3); (b) audit every `SubAgentsDir` consumer and guarantee cleanup/uninstall/backup
  never touch non-managed files in the shared prompts folder, with tests. Route: delegated direct.
- [x] T5 — Second review follow-ups (user-authorized 2026-10-03): (a) install `gentle-reviewer.agent.md` into
  `~/.copilot/agents` (VS Code default user agent folder per `chat.agentFilesLocations`; independent of
  XDG_CONFIG_HOME/%APPDATA%), migrate the owned file out of the VS Code user prompts folder, gate checks the new path;
  (b) explicit `--materialize=true` capture for vscode-copilot re-checks the managed agent gate (`--input` submission
  stays ungated, Pi precedent). Route: delegated direct.
- [x] T6 — Third review follow-ups (user-authorized 2026-10-04): R3-002 positive vscode refuter/validator
  `--materialize=true` tests with the agent installed; R3-003 refusal guidance states the sync must be global
  (workspace-scoped installs are ignored). R3-001 (runtime zero-tools) is closed only by T3 evidence, not code.
  Route: delegated direct.
- [x] T7 — Claude config mixing advisory (user-authorized 2026-10-04): on vscode-copilot install/sync, when
  `~/.claude/CLAUDE.md`, `~/.claude/agents` or `~/.claude/skills` exist and VS Code settings do not disable them
  (`chat.useClaudeMd: false`, `chat.agentFilesLocations["~/.claude/agents"]: false`,
  `chat.agentSkillsLocations["~/.claude/skills"]: false`), report an advisory with the exact settings. Never edit
  the user's settings automatically. Route: delegated direct.
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
- T4 (delegated writer, uncommitted): RED observed: reviewassets compile failure (undefined
  `VSCodeReviewerAgentFileName`/`ManagedVSCodeReviewerAgent`) and cli compile failure (undefined
  `reviewVSCodeReviewerAgentHome` and gate symbols); after wiring the gate, 5 existing vscode tests failed until they
  installed the agent through the new seam (proves the gate bites). GREEN after implementation.
  (a) `review_vscode_reviewer_agent_gate.go`: vscode-copilot is eligible only when
  `SubAgentsDir(os.UserHomeDir())/gentle-reviewer.agent.md` is a regular file byte-identical to
  `reviewassets.ManagedVSCodeReviewerAgent` (the installer's own render); missing/modified/unverifiable refuse with
  typed guidance naming `gentle-ai sync --agent <caller identity>` (modified: delete first, since the installer
  preserves unowned files). Guidance survives the privacy scrubber (no path separators). Workspace scope not accepted:
  the workspace-root render lands under `<ws>/.config/Code/User/prompts`, which VS Code never discovers. Capture-time
  follows Pi (#4256): `reviewCaptureBoundRuntimeCapability` skips the agent gate for a bound `--input` capture; START,
  STATUS and the `--materialize` offer still require it.
  (b) SubAgentsDir audit: install backup (`run.go` backupTargets) and sync backup (`sync.go` syncBackupTargetsScoped)
  are ungated but scoped to `NativeAgentFileNames` + ledger; `sddSubAgentPaths` and the upgrade executor backup are
  gated on `SupportsSubAgents` (false for vscode); `InstallNativeAgents` is manifest/ledger scoped (no retired list for
  vscode); uninstall never resolves SubAgentsDir. No narrowing needed; guard tests added for install/sync, backup
  targets, gated consumers, complete/partial uninstall.
  Checks: gofmt clean; `go vet ./internal/...` clean; `go build ./...` ok; agents/components/model/reviewerprovider
  41 packages ok; `go test ./internal/cli/...` ok (402.9s).
- T5 (delegated writer, uncommitted): RED observed: `TestSubAgentsDirIsCopilotUserAgentsFolder` failed (SubAgentsDir
  = `<home>/.config/Code/User/prompts`); reviewassets compile failure (undefined `RelocatedNativeAgentPaths`); cli
  vet failure (undefined `reviewCaptureMaterializeRuntimeGate`), then with a no-op stub
  `TestVSCodeMaterializeCaptureRequiresTheManagedReviewerAgent` (missing/modified printed the prompt),
  `TestPiMaterializeCaptureIgnoresTheVSCodeReviewerAgentGate`, and both `TestVSCodePromptsFolder*` tests failed.
  GREEN after implementation.
  (a) vscode `SubAgentsDir` = `GlobalConfigDir/agents` = `~/.copilot/agents` on every OS (no XDG/APPDATA). Migration
  seam in reviewassets: `relocatedNativeAgentManifest[vscode-copilot] = {gentle-reviewer.agent.md}` with old dir
  `SystemPromptDir(home)`; `InstallNativeAgents` installs the new location first, then removes the old copy only when
  owned (ledger hash match or managed bytes, same rule as retired agents via `removeRetiredNativeAgent`), drops it from
  the old ledger and removes that ledger when empty; never creates a ledger; own journal; idempotent.
  `RelocatedNativeAgentPaths` feeds install (`run.go` backupTargets) and sync (`sync.go` syncBackupTargetsScoped)
  snapshots. Gate derives from `SubAgentsDir` (unchanged code); guidance now says "Copilot user agents folder".
  (b) `reviewCaptureMaterializeRuntimeGate` (vscode-only; reuses `reviewRuntimeWithImmutableTransport` refusal)
  runs on the `--materialize=true` branches of capture-result and capture-refuter/capture-validation; `--input` stays
  ungated; Pi/OpenCode/Claude/Codex unaffected. Misleading capability comment fixed.
  Checks: gofmt clean; `go vet ./internal/...` clean; `go build ./...` ok; agents/components/model/reviewerprovider
  41 packages ok; `go test ./internal/cli/...` 3159 passed; `go test ./internal/update/...` 479 passed.
- T6 (delegated writer, uncommitted): R3-003 RED observed: new `TestVSCodeReviewerAgentGuidanceNamesTheGlobalSyncScope`
  failed (guidance lacked the scope). GREEN: the remedy is now `` `gentle-ai sync --agent <caller id> --scope global` ``
  plus "(a workspace-scoped install is not used for review)"; `--scope global` is explicit so a
  `GENTLE_AI_INSTALL_SCOPE=workspace` env cannot redirect it; runtime id still from the caller (#2440); no path
  separators (privacy-gate test still green). R3-002: new `review_vscode_role_materialize_test.go` drives real
  vscode-copilot lineages (Pi fixtures `piRefuterReview`, `providerCorrectionReadyWithoutVerificationEvidence`) with the
  managed agent installed: STATUS renders `--agent=vscode-copilot --materialize=true` and a submission with
  `--agent=vscode-copilot --input={{value}}`; the rendered prelude prints non-empty bytes equal to the Go-materialized
  refuter/validator prompt (idempotent, no authority mutation); `--input` admits the refuter result and closes the
  validator capture as approved. No production change needed for R3-002 (passed first run); mutation check (agent not
  installed) made both fail at STATUS. Checks: gofmt clean; `go vet ./internal/cli/...` clean;
  `go test ./internal/cli/...` 3162 passed.
- T7 (delegated writer, uncommitted): RED observed: vscode package compile failure (undefined `ClaudeConfigMixing`,
  `ClaudeMixingSource`, ...), cli vet failure (undefined `SyncResult.Advisories`), then with field+render only, 3 cli
  tests failed (sync/install printed no advisory; settings-disabled case). GREEN after implementation.
  `internal/agents/vscode/claude_mixing.go`: pure `ClaudeConfigMixing(home, settingsJSONC)` checks
  `~/.claude/CLAUDE.md` (regular file), `~/.claude/agents` (has `*.md`), `~/.claude/skills` (non-empty); parses
  settings with `filemerge.UnmarshalJSONObject` (existing JSONC: comments + trailing commas); handled when
  `chat.useClaudeMd` is false, `chat.agentFilesLocations` has `~/.claude/agents` or `.claude/agents` = false,
  `chat.agentSkillsLocations` has `~/.claude/skills` = false; unparseable/missing settings = defaults.
  `ClaudeMixingAdvisory` renders only the unhandled settings. CLI: new non-blocking `Advisories []string` on
  `SyncResult`/`InstallResult`, rendered under `Advisories:` after manual actions (sync report, all paths; install via
  `RenderInstallManualActions`), set only when vscode-copilot is selected; settings path from
  `vscode.Adapter.SettingsPath`; read-only (never writes settings.json); no exit-code change. TUI install path not
  covered (CLI only). Checks: gofmt clean; `go vet ./internal/...` clean; agents/components 3228 passed (40 pkgs);
  `go test ./internal/cli/...` 3166 passed.

## Next step

T3: user builds the branch on Windows, runs `gentle-ai sync --agent vscode-copilot`, triggers a review in Copilot Chat
Agent mode, and confirms in Chat Debug View that the `gentle-reviewer` subagent ran with zero tools.

## T4 review (lineage `review-022f1c728effad50`, range main..cd1cd326)

Consent granted; review-reliability approved; acknowledged, authority burned. New non-blocking follow-ups:
- R3-vscode-subagentsdir-ignores-xdg: on Linux VS Code honors `XDG_CONFIG_HOME`; installer/gate pin `~/.config`.
- R3-vscode-isolation-advertised-unproved: still requires T3 runtime zero-tools proof.
- R3-vscode-materialize-ungated-at-capture: explicit `--materialize=true` capture bypasses the agent gate (STATUS offer is gated).

## T5 review (lineage `review-0bcc02c652c8e507`, range main..4befa9f1)

Consent granted; review-reliability approved; acknowledged, authority burned. Non-blocking follow-ups:
- R3-001: immutable-executor advertisement still awaits T3 runtime zero-tools proof.
- R3-002: no positive test for vscode refuter/validator `--materialize=true` with the agent installed.
- R3-003: refusal guidance should say the sync must be global (workspace-scoped installs are ignored by the gate).
