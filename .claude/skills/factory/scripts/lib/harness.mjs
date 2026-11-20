// Per-harness tool names for each agent access level. Agent sources declare one `access` level;
// the build and the Pi runner translate it into each harness's own tool vocabulary.
//
//   readonly       read and search only (interface-explorer)
//   report         read, run commands, write one report file (reviewers)
//   write          full code + shell access (slice-builder, pr-publisher)
//   orchestrating  write + may spawn helper sub-agents where the harness allows (triage, spec)

export const ACCESS_LEVELS = ['readonly', 'report', 'write', 'orchestrating'];

const AG_READ = ['view_file', 'list_dir', 'find_by_name', 'grep_search', 'search_web', 'read_url_content'];
const AG_WRITE = [...AG_READ, 'write_to_file', 'replace_file_content', 'multi_replace_file_content', 'run_command', 'manage_task'];

/** Google Antigravity tool names (antigravity.google/docs/hooks, /docs/subagents). */
export const ANTIGRAVITY_TOOLS = {
  readonly: AG_READ,
  report: [...AG_READ, 'write_to_file', 'run_command'],
  write: AG_WRITE,
  orchestrating: [...AG_WRITE, 'invoke_subagent', 'define_subagent', 'send_message', 'manage_subagents'],
};

/** Pi coding agent built-in tool names (pi docs: read, bash, edit, write, grep, find, ls). */
export const PI_TOOLS = {
  readonly: ['read', 'grep', 'find', 'ls'],
  report: ['read', 'grep', 'find', 'ls', 'bash', 'write'],
  write: ['read', 'bash', 'edit', 'write', 'grep', 'find', 'ls'],
  orchestrating: ['read', 'bash', 'edit', 'write', 'grep', 'find', 'ls'],
};

export function toolsFor(harness, access) {
  const table = harness === 'antigravity' ? ANTIGRAVITY_TOOLS : harness === 'pi' ? PI_TOOLS : null;
  if (!table) throw new Error(`Unknown harness '${harness}'`);
  const tools = table[access];
  if (!tools) throw new Error(`Unknown access level '${access}' (use ${ACCESS_LEVELS.join(', ')})`);
  return tools;
}

/** Pi prompt template: `/factory <args>` hands the request to the factory skill. */
export const PI_PROMPT_TEMPLATE = `---
description: Run the AI Software Factory (triage, spec, TDD slices, review, PR)
argument-hint: "[setup | status | list | resume [run-id] | [--afk] [--quick] <problem or #issue>]"
---
Use the \`factory\` skill (/skill:factory) and follow its SKILL.md for this request: $@
`;

/**
 * Pi extension that runs the shared factory guard on every tool call. `guardRelPath` is the guard
 * script relative to the extension file, so one template serves the pi package (pi/extensions →
 * .agents/skills), project installs (.pi/extensions → .agents/skills) and global installs
 * (~/.pi/agent/extensions → ~/.pi/agent/skills).
 */
export function piGuardExtension(guardRelPath) {
  return `// AI Software Factory guard for pi (generated; do not edit).
// Blocks force pushes, pushes to the base branch and --no-verify, and confines read-only reviewer
// sub-agents (FACTORY_AGENT, set by pi-subagent.mjs) to .factory/runs/*/reviews/.
// Inactive in repos without .factory/config.json.
const GUARD_URL = new URL(${JSON.stringify(guardRelPath)}, import.meta.url).href;

export default function factoryGuard(pi: any) {
  pi.on("tool_call", async (event: any, ctx: any) => {
    const { evaluate } = await import(GUARD_URL);
    const cwd = ctx?.cwd ?? process.cwd();
    const reason = evaluate(
      { tool_name: event?.toolName ?? event?.name, tool_input: event?.input ?? {}, cwd },
      { projectDir: cwd },
    );
    if (reason) return { block: true, reason };
  });
}
`;
}

/** Antigravity hooks.json entry (antigravity.google/docs/hooks) running the shared guard. */
export function antigravityHooks(guardCommand) {
  return {
    'factory-guard': {
      PreToolUse: [
        {
          matcher: 'run_command|write_to_file|replace_file_content|multi_replace_file_content',
          hooks: [{ type: 'command', command: guardCommand, timeout: 10 }],
        },
      ],
    },
  };
}

/** Minimal frontmatter reader for the generated agent prompt files (flat `key: value` lines). */
export function parseFrontmatter(markdown) {
  const text = markdown.replace(/\r\n/g, '\n');
  const match = text.match(/^---\n([\s\S]*?)\n---\n?/);
  if (!match) return { meta: {}, body: text };
  const meta = {};
  for (const line of match[1].split('\n')) {
    const kv = line.match(/^([\w-]+):\s*(.*)$/);
    if (!kv) continue;
    let value = kv[2].trim();
    if (value.startsWith('"')) {
      try {
        value = JSON.parse(value);
      } catch {
        // keep the raw value
      }
    }
    meta[kv[1]] = value;
  }
  return { meta, body: text.slice(match[0].length) };
}
