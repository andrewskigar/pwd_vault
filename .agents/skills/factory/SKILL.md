---
name: factory
description: >-
  Orchestrate an end-to-end AI Software Factory run (triage → spec & test seams → tracer-bullet
  TDD slices → parallel review → pull request) across isolated sub-agents, using filesystem state
  in .factory/runs/. Works in new or existing repos with PHP, Python, Go, JavaScript/TypeScript or
  Rust backends, frontends, CLIs and monorepos. Use when the user invokes /factory, asks to run
  the software factory on a bug or feature, or wants to check or resume a factory run.
---

# AI Software Factory Orchestrator

You are the **Orchestrator**. Drive a problem from `TRIAGE` to `DONE` across isolated sub-agents while keeping your own context small.

## Locating the factory CLI

`SKILL_DIR` is the folder containing this `SKILL.md`:
- **Claude Code** substitutes it for you: `${CLAUDE_SKILL_DIR}`.
- **Pi** tells you where the skill lives.
- **Antigravity**: it is `.agents/skills/factory` in the workspace, or `~/.gemini/config/skills/factory` when installed globally.

Other usual locations are `.claude/skills/factory`, `~/.claude/skills/factory`, `~/.pi/agent/skills/factory`, and a plugin's `skills/factory`.

`FACTORY` below means `node "<SKILL_DIR>/scripts/factory-state.mjs"`. Run it from the **repository root**. After the first `status`, prefer the ready-made `command`, `claim_command`, `complete_command` and `verify_command` strings in `next_action`, which already carry the right path.

## Core rules

1. **Delegate.** Reading production code, writing tests and reviewing diffs happen inside stage sub-agents. You read `PROGRESS.md`/`status` output, run `FACTORY`, spawn sub-agents, and handle human checkpoints.
2. **The filesystem is the source of truth.** Every stage reads its inputs from `.factory/runs/<run-id>/` and writes its output there. Stages advance only through `FACTORY advance`, which checks each gate in code.
3. **Pass context, don't load it.** For each spawn, pass the run directory plus the `packs`, `components` and command strings from `next_action`. Don't read the packs yourself.

## Invocation

How the user calls this skill:
- **Claude Code** and **Antigravity:** `/factory …`.
- **Pi:** `/skill:factory …`, or `/factory …` through the bundled prompt template.

The model may also pick the skill up by its description.

- **`/factory setup`** → run `FACTORY setup` and show the detected config:
  - each component's `name`, `path`, `language` + `language_version`, `frameworks`, `project_type`, and `commands`;
  - `strictness` (greenfield/brownfield), `tracker`, `base_branch`, and `security_review`.

  Ask the user to confirm or correct it. Fix commands with `FACTORY setup --component <name> --test-cmd "<cmd>"` (likewise `--typecheck-cmd`, `--lint-cmd`, `--prepare-cmd`, `--scoped-lint-cmd "<cmd with {files}>"`; `none` disables a check). A component with `test: null` cannot pass the completion gate, so get a test command from the user.
- **`/factory status`** / **`list`** → `FACTORY list` or `FACTORY status <run-id>`; present the `PROGRESS.md` table. These are read-only.
- **`/factory resume [run-id]`** → `FACTORY status <run-id>` (default `latest`), then continue the loop below.
- **`/factory [--afk] [--quick] <problem or #issue>`** → start a run:
  ```bash
  FACTORY init "<short-slug>" --prompt "<full user request>" [--issue "<#id>"] [--afk] [--quick]
  ```
  - `--afk`: no human checkpoints unless hard-blocked.
  - `--quick`: for small, well-understood fixes. Skips SPEC; triage's acceptance criteria become a single ticket.
  - In git, `init` records the base commit, runs a **baseline** check (every component's test/typecheck/lint on the untouched code), then creates and switches to `factory/<slug>`. It refuses when:
    - **the working tree is dirty** (outside `.factory/`): ask whether to commit/stash first, or re-run with `--allow-dirty`;
    - **`factory/<slug>` already exists**: pick another slug or pass `--branch <name>`;
    - **baseline checks already fail** (common in existing projects): show the user the failing checks and their output, and offer:
      1. fix them first;
      2. `--accept-baseline`: those checks are then gated only on files the run changes (via the component's `scoped_commands`), or skipped where no scoped command exists;
      3. `--skip-baseline`.

      Never pick for the user in `interactive` mode. In `afk` mode, accept the baseline and record it with `FACTORY record-decision`.
  - To stay on the current feature branch: `--use-current-branch` (refused on the base branch).

## Sub-agents

Definitions are generated for each harness from one source. Each `next_action` that spawns also carries `prompt_file`, the same instructions as a plain Markdown file, for when a type isn't registered. Spawn the way your harness supports. **Parallel** means one call or turn that starts several sub-agents (reviewers; independent tickets):

- **Claude Code:** the Agent/Task tool with `subagent_type: "<name>"` (definitions in `.claude/agents/` or the plugin). For parallel work, put several Agent calls in one message.
- **Google Antigravity:** `invoke_subagent` with
  `Subagents: [{ "TypeName": "<name>", "Role": "<short role>", "Prompt": "<task>", "Workspace": "share" }]`.
  - Definitions live in `.agents/agents/<name>/agent.md` or `~/.gemini/config/agents/`.
  - Use `Workspace: "share"` so every sub-agent works on the same files and the run's branch.
  - For parallel work, list several entries in one call.
  - If a `TypeName` isn't registered, call `define_subagent` first (`name`, `description`, `system_prompt` = the body of `prompt_file`, `enable_write_tools` per the agent).
- **Pi:** Pi has no built-in sub-agents. Use one of:
  - **The bundled runner (always available):** run `node "<SKILL_DIR>/scripts/pi-subagent.mjs" run --agent <name> --task "<task>"` with your bash tool. Repeat `--agent … --task …` pairs to run several in parallel.
    - Each sub-agent is a separate, session-less `pi -p` process with its own context, the agent's instructions, and tools restricted to its access level.
    - It prints JSON with each agent's final report. `next_action.pi_runner` holds the exact base command.
    - Long slices can take many minutes; give the bash call a generous timeout, or run it in the background and wait for it.
  - **Pi's `subagent` extension, if it is installed:** call `subagent` with `{ agent, task, agentScope: "project" }`, or `{ tasks: [...] }` for parallel work. Definitions are in `.pi/agents/` (project, confirmed once) or `~/.pi/agent/agents/`.

Every task you pass must name the run directory `.factory/runs/<run-id>/`, plus whatever `next_action` provides: the ticket file, `components`, `packs` (absolute paths), `verify_command`, and the review round.

| Agent | Writes | Purpose |
| :--- | :--- | :--- |
| `triage-investigator` | `01-brief.md` | Reproduce/verify, check redundancy and `.out-of-scope/`, write the brief |
| `spec-architect` | `02-spec.md`, `tickets/*.md` | Pick seams per project type, slice vertical tickets (each naming its `**Component:**`) |
| `interface-explorer` | nothing (read-only) | Design It Twice proposals for spec-architect |
| `slice-builder` | code + one commit | One ticket, Red → Green TDD, commit with a test change |
| `standards-reviewer` | `reviews/round-N-standards.md` | Repo conventions, packs, craftsmanship, smells |
| `spec-reviewer` | `reviews/round-N-spec.md` | Requirements coverage, test validity, scope creep, full verification |
| `security-reviewer` | `reviews/round-N-security.md` | Security of the diff (only when `security_review` is on for the run) |
| `pr-publisher` | `05-pr.md` + PR | Final verify, push, open PR/MR or write the local PR doc |

## Orchestration loop

Repeat until `stage` is `DONE` or `status` is `WAITING_FOR_HUMAN`/`BLOCKED_NEEDS_HUMAN`:

1. Run `FACTORY status <run-id>` and read `next_action`.
2. Act on it:

- **`SPAWN_SUBAGENT`** (triage-investigator, spec-architect, pr-publisher): spawn `next_action.subagent` with the run directory, `.factory/config.json`, the `summary`, and any `packs`/`verify_command`. When it finishes, run `next_action.command` (`advance`) on the next status check.

- **`CLAIM_AND_SPAWN_SLICE_BUILDER`**:
  1. Run `claim_command`.
     - Parallel option: if `parallel_candidates` lists several tickets on disjoint components/files, you may claim and spawn several slice-builders at once. Each commits on the same branch, so only do this when they can't conflict.
  2. Spawn a **fresh** `slice-builder` with the ticket file, `components`, and `packs`.
  3. When it reports done, run `complete_command` with its commit SHA and RED→GREEN summary. The gate trusts nothing. It rejects the ticket unless:
     - the ticket is claimed;
     - every acceptance criterion is ticked;
     - the working tree is clean and the commit is new on the run branch;
     - the slice changed at least one test file (unless the ticket says `**Test change:** not-required`);
     - the ticket's component(s) pass their test/typecheck/lint, honouring the baseline.

     Never tick criteria or edit tickets yourself to get past it.
     - **Rejected:** the ticket stays claimed (`RESUME_OR_COMPLETE_TICKET`). Spawn a fresh slice-builder with the gate's error verbatim. After two consecutive rejections of the same ticket, stop and ask the human.
     - **"No test command configured":** ask the user for it, then `FACTORY setup --component <name> --test-cmd "<cmd>"`.
  4. When `frontier.all_done` is true, `advance` moves the run to REVIEW.

- **`SPAWN_PARALLEL_REVIEWERS`**: spawn every agent in `next_action.subagents` **in the same turn**, so their contexts stay independent. Pass the round number, `packs`, and `verify_command`. When all reports exist, `advance`:
  - all PASS → PR;
  - any FAIL → a remediation ticket is created from the findings and the run returns to IMPLEMENT, until `max_review_rounds`, then it is blocked for a human.

- **`WAIT_HUMAN_APPROVAL`** (interactive mode):
  - **TRIAGE, `needs-info`:** put the brief's questions to the user, update `CONTEXT.md` if terms crystallise, set the brief to `**Status:** ready-for-agent`, then `advance`.
  - **SPEC approval:** summarise the seams, the tickets with their components and `Blocked by` edges, and any accepted baseline failures. On confirmation, run `FACTORY approve <run-id> --note "Spec and seams approved"`.

- **`ASK_HUMAN`**: show `summary` and wait.

## References (in this skill folder)

- `references/precedence.md`: which rules win (repo docs > existing style > framework > language > universal).
- `references/seams.md`: seam catalogue for backend, frontend, cli, and library project types.
- `references/languages/`: `php.md`, `python.md`, `go.md`, `javascript.md` + `typescript.md`, `rust.md` (version-gated).
- `references/frameworks/`: `laravel.md`, `symfony.md`, `django.md`, `fastapi.md`, `react.md`, `next.md`, `vue.md`, `cli.md`.
- `agents/<name>.md`: sub-agent instructions (generated; the source is `src/agents/` in the factory template repo).
