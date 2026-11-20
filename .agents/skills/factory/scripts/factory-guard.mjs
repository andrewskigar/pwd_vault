#!/usr/bin/env node
// Guard for factory repos, shared by Claude Code (PreToolUse hook), Google Antigravity
// (PreToolUse hook in hooks.json, run with --harness antigravity) and Pi (factory-guard.ts
// extension, which imports evaluate()). It blocks:
//  - force pushes, pushes to the base branch, and --no-verify (for every agent);
//  - file edits outside .factory/runs/*/reviews/ and git history changes by the read-only
//    reviewer agents (identified by the hook input's `agent_type` in Claude Code, or the
//    FACTORY_AGENT env var that pi-subagent.mjs sets for Pi sub-agents).
// It does nothing in repos without .factory/config.json, and allows anything it cannot parse.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const READ_ONLY_AGENTS = new Set(['standards-reviewer', 'spec-reviewer', 'security-reviewer', 'interface-explorer']);
const EDIT_TOOLS = new Set(['Write', 'Edit', 'MultiEdit', 'NotebookEdit']);

// Tool names per harness, mapped onto the Claude Code vocabulary evaluate() reasons about.
const TOOL_ALIASES = {
  bash: 'Bash',
  run_command: 'Bash',
  write: 'Write',
  edit: 'Edit',
  write_to_file: 'Write',
  replace_file_content: 'Edit',
  multi_replace_file_content: 'MultiEdit',
};

/**
 * Normalise a tool call from any harness into { tool_name, tool_input: { command, file_path }, cwd, agent_type }.
 * Antigravity's hook payload is only partly documented, so several field spellings are accepted.
 */
export function normalizeToolCall(raw = {}) {
  const name = raw.tool_name ?? raw.toolName ?? raw.tool ?? raw.name ?? '';
  const input = raw.tool_input ?? raw.toolInput ?? raw.input ?? raw.args ?? raw.arguments ?? raw.parameters ?? {};
  return {
    tool_name: TOOL_ALIASES[name] || name,
    tool_input: {
      command: input.command ?? input.CommandLine ?? input.commandLine ?? input.cmd,
      file_path: input.file_path ?? input.path ?? input.TargetFile ?? input.targetFile ?? input.AbsolutePath ?? input.notebook_path,
    },
    cwd: raw.cwd ?? input.Cwd ?? input.cwd,
    agent_type: raw.agent_type ?? raw.agentType ?? process.env.FACTORY_AGENT ?? null,
  };
}

function findFactoryRoot(startDir) {
  let dir = path.resolve(startDir || '.');
  for (;;) {
    if (fs.existsSync(path.join(dir, '.factory', 'config.json'))) return dir;
    const parent = path.dirname(dir);
    if (parent === dir) return null;
    dir = parent;
  }
}

function readBaseBranch(root) {
  try {
    return JSON.parse(fs.readFileSync(path.join(root, '.factory', 'config.json'), 'utf8')).base_branch || null;
  } catch {
    return null;
  }
}

/** Split a shell command into simple commands (on && || ; | and newlines), then into tokens. */
function simpleCommands(command) {
  return String(command)
    .split(/&&|\|\||;|\||\n/)
    .map((part) => part.trim().match(/"[^"]*"|'[^']*'|\S+/g) || [])
    .map((tokens) => tokens.map((t) => t.replace(/^["']|["']$/g, '')))
    .filter((tokens) => tokens.length > 0);
}

function gitSubcommand(tokens) {
  const gitIdx = tokens.findIndex((t) => t === 'git' || t.endsWith('/git') || t.endsWith('\\git.exe'));
  if (gitIdx < 0) return null;
  // Skip global options like -C <dir> or -c key=value.
  let i = gitIdx + 1;
  while (i < tokens.length && tokens[i].startsWith('-')) i += tokens[i] === '-C' || tokens[i] === '-c' ? 2 : 1;
  return { sub: tokens[i], args: tokens.slice(i + 1) };
}

/** Returns a denial reason, or null to allow. */
export function evaluate(rawInput, { projectDir } = {}) {
  const input = normalizeToolCall(rawInput);
  const root = findFactoryRoot(projectDir || input.cwd);
  if (!root) return null;
  const agent = input.agent_type || null;
  const readOnly = agent && READ_ONLY_AGENTS.has(agent);
  const toolInput = input.tool_input || {};

  if (input.tool_name === 'Bash') {
    const baseBranch = readBaseBranch(root);
    for (const tokens of simpleCommands(toolInput.command || '')) {
      const git = gitSubcommand(tokens);
      if (!git) continue;
      const { sub, args } = git;
      if (args.includes('--no-verify') && (sub === 'commit' || sub === 'push')) {
        return 'Factory guard: --no-verify skips the repository hooks; fix what the hook reports instead.';
      }
      if (sub === 'push') {
        if (args.some((a) => a === '--force' || a === '-f' || a.startsWith('--force-with-lease') || a.startsWith('--force-if-includes') || /^\+/.test(a))) {
          return 'Factory guard: force-pushing is not allowed from a factory run. Push new commits instead.';
        }
        if (baseBranch && args.some((a) => a === baseBranch || a.endsWith(`:${baseBranch}`) || a === `refs/heads/${baseBranch}`)) {
          return `Factory guard: pushing to the base branch '${baseBranch}' is not allowed; push the run branch and open a PR.`;
        }
      }
      if (readOnly && ['commit', 'push', 'reset', 'checkout', 'switch', 'stash', 'rebase', 'merge', 'cherry-pick', 'restore', 'clean', 'am', 'apply'].includes(sub)) {
        return `Factory guard: '${agent}' is a read-only reviewer and may not run 'git ${sub}'.`;
      }
    }
    return null;
  }

  if (readOnly && EDIT_TOOLS.has(input.tool_name)) {
    const target = toolInput.file_path || '';
    const resolved = path.resolve(input.cwd || root, target);
    const rel = path.relative(root, resolved).replace(/\\/g, '/');
    if (!/^\.factory\/runs\/[^/]+\/reviews\/[^/]+$/.test(rel)) {
      return `Factory guard: '${agent}' may only write its report under .factory/runs/<run-id>/reviews/, not '${rel}'.`;
    }
  }
  return null;
}

function main() {
  let raw = '';
  process.stdin.setEncoding('utf8');
  process.stdin.on('data', (chunk) => {
    raw += chunk;
  });
  process.stdin.on('end', () => {
    let input;
    try {
      input = JSON.parse(raw);
    } catch {
      process.exit(0);
    }
    const reason = evaluate(input, { projectDir: process.env.CLAUDE_PROJECT_DIR });
    if (reason && process.argv.includes('--harness') && process.argv.includes('antigravity')) {
      // Antigravity PreToolUse hooks return a decision object (antigravity.google/docs/hooks).
      process.stdout.write(JSON.stringify({ decision: 'deny', reason }));
      process.stderr.write(`${reason}
`);
    } else if (reason) {
      process.stdout.write(
        JSON.stringify({
          hookSpecificOutput: {
            hookEventName: 'PreToolUse',
            permissionDecision: 'deny',
            permissionDecisionReason: reason,
          },
        })
      );
    }
    process.exit(0);
  });
}

if (process.argv[1] && path.resolve(process.argv[1]) === path.resolve(fileURLToPath(import.meta.url))) {
  main();
}
