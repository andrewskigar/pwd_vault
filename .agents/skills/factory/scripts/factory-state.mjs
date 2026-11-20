#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import {
  SKILL_DIR,
  canonicalLanguage,
  detectComponents,
  detectStrictness,
  packsFor,
} from './lib/detect.mjs';
import { captureBaseline, isTestFile, runVerification, selectComponents } from './lib/verify.mjs';

export { detectComponents, SKILL_DIR } from './lib/detect.mjs';
export { normalizeCommandForPlatform, runVerification, captureBaseline } from './lib/verify.mjs';

/**
 * Run git with an argument array (no shell, so quoting is identical on Windows and POSIX).
 * Returns trimmed stdout, or null when git fails or is not installed.
 */
function git(cwd, args) {
  try {
    return execFileSync('git', args, {
      cwd,
      stdio: ['ignore', 'pipe', 'ignore'],
      encoding: 'utf8',
    }).trim();
  } catch {
    return null;
  }
}

/**
 * Run git and throw a readable error (including git's stderr) when it fails.
 */
function gitOrThrow(cwd, args, context) {
  try {
    return execFileSync('git', args, {
      cwd,
      stdio: ['ignore', 'pipe', 'pipe'],
      encoding: 'utf8',
    }).trim();
  } catch (err) {
    const stderr = String(err.stderr || err.message || '').trim();
    throw new Error(`${context}: git ${args.join(' ')} failed. ${stderr}`);
  }
}

function isGitRepo(cwd) {
  return git(cwd, ['rev-parse', '--is-inside-work-tree']) === 'true';
}

function localBranchExists(cwd, name) {
  return git(cwd, ['rev-parse', '--verify', '--quiet', `refs/heads/${name}`]) !== null;
}

function branchExists(cwd, name) {
  return (
    localBranchExists(cwd, name) ||
    git(cwd, ['rev-parse', '--verify', '--quiet', `refs/remotes/origin/${name}`]) !== null
  );
}

/**
 * Write a file via temp-file + rename so a crash mid-write never leaves truncated JSON.
 */
function writeFileAtomic(filePath, content) {
  const tmpPath = `${filePath}.${process.pid}.tmp`;
  fs.writeFileSync(tmpPath, content, 'utf8');
  fs.renameSync(tmpPath, filePath);
}

/**
 * Detect the branch PRs should target. A configured branch is kept only if it actually exists,
 * so a stale default (e.g. 'main' in a 'master' repo) is corrected on the next setup.
 */
export function detectBaseBranch(rootDir, configuredBranch = null) {
  if (!isGitRepo(rootDir)) return configuredBranch ?? 'main';
  if (configuredBranch && branchExists(rootDir, configuredBranch)) return configuredBranch;

  const originHead = git(rootDir, ['symbolic-ref', '--quiet', 'refs/remotes/origin/HEAD']);
  if (originHead) return originHead.replace('refs/remotes/origin/', '');

  for (const candidate of ['main', 'master', 'trunk', 'develop']) {
    if (branchExists(rootDir, candidate)) return candidate;
  }
  return git(rootDir, ['branch', '--show-current']) || configuredBranch || 'main';
}

/**
 * Slugify a string for run/ticket filenames.
 */
export function slugify(text) {
  return String(text || 'run')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 48) || 'run';
}

const CONFIG_DEFAULTS = {
  version: 2,
  tracker: 'local',
  base_branch: 'main',
  default_mode: 'interactive',
  max_review_rounds: 3,
  strictness: 'greenfield',
  security_review: 'auto',
  ticket_verification: 'full',
  baseline: 'check',
};

function normalizeComponent(component) {
  const base = {
    name: component.name || 'root',
    path: component.path || '.',
    language: canonicalLanguage(component.language) || 'generic',
    language_version: component.language_version ?? null,
    frameworks: component.frameworks || [],
    project_type: component.project_type || 'unknown',
    ...component,
  };
  base.language = canonicalLanguage(base.language) || 'generic';
  base.commands = { prepare: null, test: null, typecheck: null, lint: null, ...(component.commands || {}) };
  base.scoped_commands = { test: null, typecheck: null, lint: null, ...(component.scoped_commands || {}) };
  if (base.extensions === undefined) base.extensions = null;
  const extra = (component.extra_packs || []).map((p) => toPosix(String(p)));
  base.packs = [...new Set([...packsFor(base), ...extra])];
  return base;
}

/**
 * Accept both config v2 (components) and v1 (single top-level language + *_cmd fields).
 * A v1 config becomes one component at the repo root, so old installs keep working.
 */
export function normalizeConfig(raw) {
  if (!raw) return null;
  if (Array.isArray(raw.components)) {
    return { ...CONFIG_DEFAULTS, ...raw, version: 2, components: raw.components.map(normalizeComponent) };
  }
  const { language, language_reference: _ref, test_cmd, typecheck_cmd, lint_cmd, ...rest } = raw;
  const component = normalizeComponent({
    name: 'root',
    path: '.',
    language: language || 'generic',
    commands: { test: test_cmd ?? null, typecheck: typecheck_cmd ?? null, lint: lint_cmd ?? null },
  });
  return { ...CONFIG_DEFAULTS, ...rest, version: 2, components: [component] };
}

const languageFamily = (language) => (language === 'typescript' ? 'javascript' : language);

/** User-set values win (including `false` = disabled); null means "detect". */
function mergeCommandSet(existing = {}, detected = {}) {
  const merged = { ...detected };
  for (const [key, value] of Object.entries(existing)) {
    if (value !== null && value !== undefined) merged[key] = value;
    else if (!(key in merged)) merged[key] = null;
  }
  return merged;
}

function mergeComponents(existing, detected) {
  const used = new Set();
  const merged = detected.map((found) => {
    const prior = existing.find(
      (e) => !used.has(e) && e.path === found.path && languageFamily(e.language) === languageFamily(found.language)
    );
    if (!prior) return normalizeComponent(found);
    used.add(prior);
    return normalizeComponent({
      ...found,
      name: prior.name,
      extra_packs: prior.extra_packs,
      extensions: prior.extensions ?? found.extensions,
      commands: mergeCommandSet(prior.commands, found.commands),
      scoped_commands: mergeCommandSet(prior.scoped_commands, found.scoped_commands),
    });
  });
  for (const prior of existing) {
    if (used.has(prior)) continue;
    const hasCommands = Object.values(prior.commands || {}).some(Boolean);
    if (prior.language === 'generic' && detected.length > 0 && !hasCommands) continue;
    merged.push(normalizeComponent(prior));
  }
  return merged;
}

const COMMAND_OPTIONS = {
  prepareCmd: ['commands', 'prepare'],
  testCmd: ['commands', 'test'],
  typecheckCmd: ['commands', 'typecheck'],
  lintCmd: ['commands', 'lint'],
  scopedTestCmd: ['scoped_commands', 'test'],
  scopedTypecheckCmd: ['scoped_commands', 'typecheck'],
  scopedLintCmd: ['scoped_commands', 'lint'],
};

function applyComponentOptions(components, options) {
  const commandKeys = Object.keys(COMMAND_OPTIONS).filter((k) => options[k] !== undefined && options[k] !== null);
  if (commandKeys.length === 0 && !options.language) return components;

  let target;
  if (options.component) {
    target = components.find((c) => c.name === options.component);
    if (!target) {
      throw new Error(
        `Unknown component '${options.component}'. Configured components: ${components.map((c) => c.name).join(', ')}.`
      );
    }
  } else if (components.length === 1) {
    target = components[0];
  } else {
    throw new Error(
      `This repo has several components (${components.map((c) => c.name).join(', ')}); pass --component <name> ` +
        `to say which one the command/language option applies to.`
    );
  }

  for (const key of commandKeys) {
    const [group, step] = COMMAND_OPTIONS[key];
    const value = options[key];
    target[group][step] = value === 'none' || value === false ? false : String(value);
  }
  if (options.language) {
    target.language = canonicalLanguage(options.language);
    target.packs = normalizeComponent({ ...target, packs: undefined }).packs;
  }
  return components;
}

function parseSecurityOption(value) {
  if (value === undefined || value === null) return undefined;
  if (value === true || value === 'true' || value === 'on') return true;
  if (value === false || value === 'false' || value === 'off') return false;
  return 'auto';
}

/**
 * Detect every component (language, version, frameworks, project type, installed tools) and write
 * .factory/config.json. Values the user already set are kept; only nulls are re-detected.
 */
export function setupFactory(rootDir, options = {}) {
  const factoryDir = path.join(rootDir, '.factory');
  fs.mkdirSync(path.join(factoryDir, 'runs'), { recursive: true });

  const configPath = path.join(factoryDir, 'config.json');
  const existing = fs.existsSync(configPath)
    ? normalizeConfig(JSON.parse(fs.readFileSync(configPath, 'utf8')))
    : null;

  let components = mergeComponents(existing?.components || [], detectComponents(rootDir));
  if (components.length === 0) {
    components = [normalizeComponent({ name: 'root', path: '.', language: 'generic' })];
  }
  components = applyComponentOptions(components, options);

  let tracker = options.tracker ?? existing?.tracker ?? null;
  if (!tracker) {
    const trackerDocPath = path.join(rootDir, 'docs', 'agents', 'issue-tracker.md');
    if (fs.existsSync(trackerDocPath)) {
      const content = fs.readFileSync(trackerDocPath, 'utf8').toLowerCase();
      if (content.includes('github') || content.includes('gh issue')) tracker = 'github';
      else if (content.includes('gitlab') || content.includes('glab')) tracker = 'gitlab';
      else tracker = 'local';
    } else {
      const remote = git(rootDir, ['remote', 'get-url', 'origin']) || '';
      if (remote.includes('github.com')) tracker = 'github';
      else if (remote.includes('gitlab')) tracker = 'gitlab';
      else tracker = 'local';
    }
  }

  const config = {
    ...CONFIG_DEFAULTS,
    ...(existing || {}),
    version: 2,
    tracker,
    base_branch: options.baseBranch ?? detectBaseBranch(rootDir, existing?.base_branch ?? null),
    default_mode: options.mode ?? existing?.default_mode ?? 'interactive',
    max_review_rounds: Number(options.maxReviewRounds ?? existing?.max_review_rounds ?? 3),
    strictness: options.strictness ?? existing?.strictness ?? detectStrictness(rootDir),
    security_review: parseSecurityOption(options.securityReview) ?? existing?.security_review ?? 'auto',
    ticket_verification: options.ticketVerification ?? existing?.ticket_verification ?? 'full',
    baseline: options.baseline ?? existing?.baseline ?? 'check',
    components,
    updated_at: new Date().toISOString(),
  };

  fs.writeFileSync(configPath, JSON.stringify(config, null, 2) + '\n', 'utf8');
  return config;
}

/**
 * Load .factory/config.json (normalised to v2) or initialize it if missing.
 */
export function loadConfig(rootDir) {
  const configPath = path.join(rootDir, '.factory', 'config.json');
  if (!fs.existsSync(configPath)) {
    return setupFactory(rootDir, {});
  }
  return normalizeConfig(JSON.parse(fs.readFileSync(configPath, 'utf8')));
}

/** Whether this run gets the third (security) review axis. */
function resolveSecurityReview(config) {
  if (config.security_review === true) return true;
  if (config.security_review === false) return false;
  return (config.components || []).some((c) => c.project_type === 'backend');
}

/** Components a ticket targets: its **Component:** field, the only component, or all of them. */
function ticketComponents(ticket, config) {
  if (ticket.components && ticket.components.length > 0) return ticket.components;
  return (config.components || []).length === 1 ? [config.components[0].name] : ['all'];
}

/** Absolute paths of the packs an agent must read for the given components. */
export function packPathsFor(config, componentNames, rootDir = process.cwd()) {
  const components = selectComponents(config, componentNames);
  const rel = ['references/precedence.md'];
  for (const c of components) for (const p of c.packs || []) if (!rel.includes(p)) rel.push(p);
  rel.push('references/seams.md');
  // Built-in packs live in the skill; extra_packs may point at files in the repo itself.
  return rel.map((p) => toPosix(p.startsWith('references/') ? path.join(SKILL_DIR, p) : path.resolve(rootDir, p)));
}

function toPosix(p) {
  return p.replace(/\\/g, '/');
}

/**
 * How to invoke this CLI. Relative when the skill lives inside the repo (portable, and matches
 * the permission rule the installer writes), absolute otherwise (global or plugin install).
 */
function cliCommand(rootDir = process.cwd(), scriptName = 'factory-state.mjs') {
  const script = path.join(SKILL_DIR, 'scripts', scriptName);
  const rel = path.relative(rootDir, script);
  if (!rel.startsWith('..') && !path.isAbsolute(rel)) return `node ${toPosix(rel)}`;
  return `node "${toPosix(script)}"`;
}
const promptFile = (agent) => toPosix(path.join(SKILL_DIR, 'agents', `${agent}.md`));

/**
 * Resolve a runId ("latest" or explicit slug/id) to its directory and state.json.
 */
export function resolveRunDir(rootDir, runId) {
  const runsRoot = path.join(rootDir, '.factory', 'runs');
  if (!fs.existsSync(runsRoot)) {
    throw new Error(`No factory runs found in ${runsRoot}. Run 'factory-state init' first.`);
  }

  if (!runId || runId === 'latest') {
    const entries = fs
      .readdirSync(runsRoot, { withFileTypes: true })
      .filter((d) => d.isDirectory() && fs.existsSync(path.join(runsRoot, d.name, 'state.json')))
      .map((d) => {
        const st = JSON.parse(fs.readFileSync(path.join(runsRoot, d.name, 'state.json'), 'utf8'));
        return { name: d.name, updated: st.updated_at || st.created_at || '' };
      })
      .sort((a, b) => b.updated.localeCompare(a.updated));

    if (entries.length === 0) {
      throw new Error(`No factory runs found in ${runsRoot}.`);
    }
    return path.join(runsRoot, entries[0].name);
  }

  const exactPath = path.join(runsRoot, runId);
  if (fs.existsSync(exactPath)) return exactPath;

  // Allow matching by suffix slug
  const matches = fs
    .readdirSync(runsRoot, { withFileTypes: true })
    .filter((d) => d.isDirectory() && (d.name === runId || d.name.endsWith(`-${runId}`)));

  if (matches.length === 1) {
    return path.join(runsRoot, matches[0].name);
  }
  throw new Error(`Factory run '${runId}' not found in ${runsRoot}.`);
}

/**
 * Parse a single ticket markdown file from .factory/runs/<runId>/tickets/<file>.md.
 */
export function parseTicketFile(filePath) {
  const raw = fs.readFileSync(filePath, 'utf8');
  const fileName = path.basename(filePath);
  const idMatch =
    raw.match(/^#\s+([0-9a-zA-Z_-]+)\s*:\s*(.+)$/m) ||
    fileName.match(/^([0-9]+)-(.+)\.md$/);

  const id = idMatch ? idMatch[1].trim() : fileName.replace(/\.md$/, '');
  const title = idMatch ? idMatch[2].trim() : fileName.replace(/\.md$/, '');

  const statusMatch = raw.match(/^\*\*Status:\*\*\s*(.+)$/im) || raw.match(/^Status:\s*(.+)$/im);
  const statusRaw = statusMatch ? statusMatch[1].trim().toLowerCase() : 'ready-for-agent';

  const blockedMatch =
    raw.match(/^\*\*Blocked by:\*\*\s*(.+)$/im) || raw.match(/^Blocked by:\s*(.+)$/im);
  const blockedRaw = blockedMatch ? blockedMatch[1].trim() : 'None';

  let blockedBy = [];
  if (blockedRaw && !/^(none|n\/a|-|nothing)/i.test(blockedRaw)) {
    blockedBy = blockedRaw
      .split(/[,;]+/)
      .map((tok) => {
        const m = tok.trim().match(/^#?([0-9]{2,}|[0-9a-zA-Z_-]+)/);
        return m ? m[1] : tok.trim();
      })
      .filter(Boolean);
  }

  const seamMatch = raw.match(/^\*\*Seam:\*\*\s*(.+)$/im) || raw.match(/^Seam:\s*(.+)$/im);
  const seam = seamMatch ? seamMatch[1].trim() : null;

  const commitMatch = raw.match(/^\*\*Commit:\*\*\s*(.+)$/im) || raw.match(/^Commit:\s*(.+)$/im);
  const commit = commitMatch ? commitMatch[1].trim() : null;

  const componentMatch =
    raw.match(/^\*\*Components?:\*\*\s*(.+)$/im) || raw.match(/^Components?:\s*(.+)$/im);
  const components = componentMatch
    ? componentMatch[1]
        .split(/[,;]+/)
        .map((c) => c.trim().replace(/`/g, ''))
        .filter(Boolean)
    : [];

  const testChangeMatch =
    raw.match(/^\*\*Test change:\*\*\s*(.+)$/im) || raw.match(/^Test change:\s*(.+)$/im);
  const testChangeRequired = !(testChangeMatch && /^(not[- ]required|none|no\b|optional)/i.test(testChangeMatch[1].trim()));

  const unchecked = [...raw.matchAll(/^\s*-\s*\[\s\]\s+(.+)$/gm)].map((m) => m[1].trim());
  const checked = [...raw.matchAll(/^\s*-\s*\[[xX]\]\s+(.+)$/gm)].map((m) => m[1].trim());

  let normalizedStatus = 'ready-for-agent';
  if (['done', 'resolved', 'completed', 'closed'].includes(statusRaw)) {
    normalizedStatus = 'done';
  } else if (['claimed', 'in-progress', 'in_progress', 'active'].includes(statusRaw)) {
    normalizedStatus = 'claimed';
  }

  return {
    id,
    title,
    file: fileName,
    path: filePath,
    status: normalizedStatus,
    blocked_by: blockedBy,
    seam,
    commit,
    components,
    test_change_required: testChangeRequired,
    criteria_total: unchecked.length + checked.length,
    criteria_checked: checked.length,
    criteria_unchecked: unchecked,
  };
}

/**
 * Load all tickets in a run directory and verify there are no dependency cycles.
 */
export function loadTickets(runDir) {
  const ticketsDir = path.join(runDir, 'tickets');
  if (!fs.existsSync(ticketsDir)) return [];

  const files = fs
    .readdirSync(ticketsDir)
    .filter((f) => f.endsWith('.md'))
    .sort();

  const tickets = files.map((f) => parseTicketFile(path.join(ticketsDir, f)));
  const byId = new Map(tickets.map((t) => [t.id, t]));

  // Detect circular dependencies
  for (const t of tickets) {
    const visited = new Set();
    const stack = [...t.blocked_by];
    while (stack.length > 0) {
      const depId = stack.pop();
      if (depId === t.id) {
        throw new Error(`Circular ticket dependency detected involving ticket '${t.id}'.`);
      }
      if (!visited.has(depId)) {
        visited.add(depId);
        const depTicket = byId.get(depId);
        if (depTicket) stack.push(...depTicket.blocked_by);
      }
    }
  }

  return tickets;
}

/**
 * Reject ticket graphs that would stall the run later: duplicate IDs, or `Blocked by`
 * edges pointing at tickets that do not exist (those can never become unblocked).
 */
export function validateTicketGraph(tickets, config = null) {
  const errors = [];
  const componentNames = (config?.components || []).map((c) => c.name);
  if (componentNames.length > 1) {
    for (const t of tickets) {
      if (t.components.length === 0) {
        errors.push(
          `ticket '${t.id}' (${t.file}) has no '**Component:**' line; this repo has several components (${componentNames.join(', ')}), so each ticket must name the one(s) it changes, or 'all'`
        );
        continue;
      }
      const unknown = t.components.filter((c) => c !== 'all' && !componentNames.includes(c));
      if (unknown.length > 0) {
        errors.push(`ticket '${t.id}' (${t.file}) names unknown component(s): ${unknown.join(', ')} (known: ${componentNames.join(', ')})`);
      }
    }
  }
  const fileById = new Map();
  for (const t of tickets) {
    if (fileById.has(t.id)) {
      errors.push(`duplicate ticket id '${t.id}' in ${fileById.get(t.id)} and ${t.file}`);
    } else {
      fileById.set(t.id, t.file);
    }
  }
  for (const t of tickets) {
    const unknown = t.blocked_by.filter((depId) => !fileById.has(depId));
    if (unknown.length > 0) {
      errors.push(`ticket '${t.id}' (${t.file}) is blocked by unknown ticket id(s): ${unknown.join(', ')}`);
    }
  }
  if (errors.length > 0) {
    throw new Error(`Stage Gate Failed (SPEC -> IMPLEMENT): ${errors.join('; ')}.`);
  }
}

/**
 * Compute the ticket frontier (unblocked, claimed, blocked, done).
 */
export function computeFrontier(tickets) {
  const doneIds = new Set(tickets.filter((t) => t.status === 'done').map((t) => t.id));

  const unblocked = [];
  const claimed = [];
  const blocked = [];
  const done = [];

  for (const t of tickets) {
    const openBlockers = t.blocked_by.filter((depId) => !doneIds.has(depId));
    if (t.status === 'done') {
      done.push(t);
    } else if (t.status === 'claimed') {
      claimed.push({ ...t, open_blockers: openBlockers });
    } else if (openBlockers.length === 0) {
      unblocked.push(t);
    } else {
      blocked.push({ ...t, open_blockers: openBlockers });
    }
  }

  return {
    total: tickets.length,
    unblocked,
    claimed,
    blocked,
    done,
    all_done: tickets.length > 0 && done.length === tickets.length,
  };
}

/**
 * Compute the next deterministic orchestrator instruction for the run.
 */
export function computeNextAction(state, frontier, runDir, config = null) {
  const cfg = config || { components: [] };
  const rootDir = path.resolve(runDir, '..', '..', '..');
  const CLI = cliCommand(rootDir);
  // Pi has no native sub-agents; this runner starts each one as an isolated `pi -p` process.
  const PI_RUNNER = `${cliCommand(rootDir, 'pi-subagent.mjs')} run`;
  const packsFor = (names) => (cfg.components?.length ? packPathsFor(cfg, names, rootDir) : []);
  const advance = (summary) => ({
    action: 'ADVANCE_STAGE',
    command: `${CLI} advance ${state.run_id}`,
    summary,
  });
  const spawn = (subagent, summary, extra = {}) => ({
    action: 'SPAWN_SUBAGENT',
    subagent,
    prompt_file: promptFile(subagent),
    pi_runner: PI_RUNNER,
    summary,
    ...extra,
  });

  if (state.status === 'COMPLETED' || state.stage === 'DONE') {
    return { action: 'NONE', summary: 'Factory run is COMPLETED.' };
  }

  if (state.status === 'BLOCKED_NEEDS_HUMAN') {
    return {
      action: 'ASK_HUMAN',
      summary: state.blocked_reason || 'Run is blocked and requires human intervention.',
    };
  }

  if (state.status === 'WAITING_FOR_HUMAN') {
    return {
      action: 'WAIT_HUMAN_APPROVAL',
      summary:
        state.waiting_reason ||
        `Waiting for human input/approval at stage ${state.stage}. Run '${CLI} approve ${state.run_id}' once confirmed.`,
    };
  }

  const multi = (cfg.components || []).length > 1;

  switch (state.stage) {
    case 'TRIAGE': {
      if (!fs.existsSync(path.join(runDir, '01-brief.md'))) {
        const quickNote =
          state.lane === 'quick'
            ? ` Quick lane: SPEC is skipped, so the brief must also carry a '**Seam:**' line${multi ? " and a '**Component:**' line" : ''}; the gate turns its acceptance criteria into the single ticket.`
            : '';
        return spawn(
          'triage-investigator',
          `Spawn triage-investigator to reproduce/verify the problem and write 01-brief.md.${quickNote}`
        );
      }
      return advance('01-brief.md exists. Run advance to validate the Triage gate.');
    }

    case 'SPEC': {
      if (!fs.existsSync(path.join(runDir, '02-spec.md')) || frontier.total === 0) {
        return spawn(
          'spec-architect',
          `Spawn spec-architect to define test seams in 02-spec.md and break work into vertical tickets in tickets/*.md.${multi ? ' Every ticket must name its **Component:**.' : ''}`,
          { packs: packsFor(null), components: (cfg.components || []).map((c) => c.name) }
        );
      }
      return advance('02-spec.md and tickets exist. Run advance to validate the Spec gate.');
    }

    case 'IMPLEMENT': {
      if (frontier.all_done) {
        return advance('All tickets are done. Run advance to enter review.');
      }
      const sliceAction = (t, action, summary, extra = {}) => {
        const components = ticketComponents(t, cfg);
        return {
          action,
          subagent: 'slice-builder',
          prompt_file: promptFile('slice-builder'),
          pi_runner: PI_RUNNER,
          ticket: t.id,
          ticket_file: `tickets/${t.file}`,
          components,
          packs: packsFor(components),
          complete_command: `${CLI} complete-ticket ${state.run_id} ${t.id} --commit <sha> --verification "<RED->GREEN summary>"`,
          summary,
          ...extra,
        };
      };
      if (frontier.claimed.length > 0) {
        const t = frontier.claimed[0];
        return sliceAction(
          t,
          'RESUME_OR_COMPLETE_TICKET',
          `Ticket ${t.id} (${t.title}) is currently claimed. Complete it with slice-builder.`
        );
      }
      if (frontier.unblocked.length > 0) {
        const t = frontier.unblocked[0];
        return sliceAction(
          t,
          'CLAIM_AND_SPAWN_SLICE_BUILDER',
          `Claim unblocked frontier ticket ${t.id} (${t.title}) and spawn a fresh slice-builder sub-agent.`,
          {
            claim_command: `${CLI} claim-ticket ${state.run_id} ${t.id}`,
            parallel_candidates: frontier.unblocked.map((u) => u.id),
          }
        );
      }
      return {
        action: 'ASK_HUMAN',
        summary: 'No unblocked tickets available, but not all tickets are done. Check ticket dependencies.',
      };
    }

    case 'REVIEW': {
      const round = state.review_round || 1;
      const axes = reviewAxes(state);
      const missing = axes.filter((axis) => !fs.existsSync(path.join(runDir, 'reviews', `round-${round}-${axis}.md`)));
      if (missing.length > 0) {
        const subagents = missing.map((axis) => `${axis}-reviewer`);
        return {
          action: 'SPAWN_PARALLEL_REVIEWERS',
          subagents,
          prompt_files: Object.fromEntries(subagents.map((s) => [s, promptFile(s)])),
          pi_runner: PI_RUNNER,
          round,
          packs: packsFor(null),
          verify_command: `${CLI} verify ${state.run_id}`,
          summary: `Spawn ${subagents.join(', ')} in parallel for Review Round ${round}.`,
        };
      }
      return advance(`Review Round ${round} reports exist. Run advance to evaluate verdicts.`);
    }

    case 'PR': {
      if (!fs.existsSync(path.join(runDir, '05-pr.md'))) {
        return spawn('pr-publisher', 'Spawn pr-publisher to verify, push branch, open PR, and write 05-pr.md.', {
          verify_command: `${CLI} verify ${state.run_id}`,
        });
      }
      return advance('05-pr.md exists. Run advance to mark the Factory Run DONE.');
    }

    default:
      return { action: 'UNKNOWN', summary: `Unknown stage: ${state.stage}` };
  }
}

/** Review axes for a run: standards + spec, plus security when enabled at init. */
function reviewAxes(state) {
  return state.security_review ? ['standards', 'spec', 'security'] : ['standards', 'spec'];
}

/**
 * Render PROGRESS.md (the low-resolution dashboard read on every session start).
 */
function baselineLine(state) {
  const baseline = state.baseline;
  if (!baseline) return '';
  if (baseline.failing.length === 0) return `- **Baseline**: all checks green at \`${baseline.sha}\`\n`;
  return `- **Baseline**: already failing before this run (gated on changed files only, or not gated): ${baseline.failing.map((f) => `\`${f}\``).join(', ')}\n`;
}

export function renderProgressMarkdown(state, frontier, nextAction) {
  const stageOrder = ['TRIAGE', 'SPEC', 'IMPLEMENT', 'REVIEW', 'PR', 'DONE'];
  const currentIdx = stageOrder.indexOf(state.stage);

  const stageBar = stageOrder
    .map((s, idx) => {
      if (state.stage === 'DONE' || idx < currentIdx) return `[x] ${s}`;
      if (idx === currentIdx) return `[>] **${s}** (${state.status})`;
      return `[ ] ${s}`;
    })
    .join('  →  ');

  const ticketLines =
    frontier.total === 0
      ? '_No tickets generated yet._'
      : [
          ...frontier.done.map(
            (t) => `- [x] **${t.id}: ${t.title}** (\`done\`${t.commit ? `, commit \`${t.commit}\`` : ''}) — [tickets/${t.file}](./tickets/${t.file})`
          ),
          ...frontier.claimed.map(
            (t) => `- [/] **${t.id}: ${t.title}** (\`claimed\`) — [tickets/${t.file}](./tickets/${t.file})`
          ),
          ...frontier.unblocked.map(
            (t) => `- [ ] **${t.id}: ${t.title}** (\`ready — on frontier\`) — [tickets/${t.file}](./tickets/${t.file})`
          ),
          ...frontier.blocked.map(
            (t) =>
              `- [ ] **${t.id}: ${t.title}** (\`blocked by: ${t.open_blockers.join(', ')}\`) — [tickets/${t.file}](./tickets/${t.file})`
          ),
        ].join('\n');

  const decisionsLines =
    (state.decisions || []).length === 0
      ? '_No decisions recorded yet._'
      : state.decisions.map((d) => `- **${d.at.slice(0, 10)}**: ${d.text}`).join('\n');

  return `# Factory Run: ${state.title}

- **Run ID**: \`${state.run_id}\`
- **Mode**: \`${state.mode}\`
- **Issue / Ref**: ${state.issue || 'Local prompt'}
- **Branch**: \`${state.branch || 'n/a'}\` (base: \`${state.base_branch || 'unknown'}\` @ \`${state.base_sha || 'unknown'}\`)
- **Lane**: \`${state.lane || 'standard'}\`${state.lane === 'quick' ? ' (SPEC skipped)' : ''}
- **Review Round**: ${state.review_round} / ${state.max_review_rounds} (axes: ${reviewAxes(state).join(', ')})
${baselineLine(state)}
## Pipeline Progress

${stageBar}

## Next Orchestrator Action

> **${nextAction.action}**: ${nextAction.summary}

## Artifacts

- Problem Input: [00-problem.md](./00-problem.md)
- Stage 1 Agent Brief: ${fs.existsSync(path.join(state._runDir || '', '01-brief.md')) ? '[01-brief.md](./01-brief.md)' : '_Pending_'}
- Stage 2 Spec & Seams: ${fs.existsSync(path.join(state._runDir || '', '02-spec.md')) ? '[02-spec.md](./02-spec.md)' : '_Pending_'}
- Stage 5 Pull Request: ${fs.existsSync(path.join(state._runDir || '', '05-pr.md')) ? '[05-pr.md](./05-pr.md)' : '_Pending_'}

## Tracer-Bullet Tickets (${frontier.done.length}/${frontier.total} Done)

${ticketLines}

## Decisions So Far

${decisionsLines}
`;
}

/**
 * Build the full run view (state + live ticket frontier + next action) without touching disk.
 */
function buildRunView(runDir, state) {
  const tickets = loadTickets(runDir);
  const frontier = computeFrontier(tickets);
  const cleanState = { ...state };
  delete cleanState._runDir;

  cleanState.ticket_summary = {
    total: frontier.total,
    done: frontier.done.length,
    unblocked: frontier.unblocked.map((t) => t.id),
    claimed: frontier.claimed.map((t) => t.id),
    blocked: frontier.blocked.map((t) => t.id),
  };

  const configPath = path.resolve(runDir, '..', '..', 'config.json');
  const config = fs.existsSync(configPath)
    ? normalizeConfig(JSON.parse(fs.readFileSync(configPath, 'utf8')))
    : null;
  const nextAction = computeNextAction(cleanState, frontier, runDir, config);
  return { ...cleanState, frontier, next_action: nextAction };
}

/**
 * Regenerate PROGRESS.md (only when its content changed). Never touches state.json,
 * so it is safe to call from read-only commands.
 */
function refreshProgress(runDir, view) {
  const { frontier, next_action: nextAction, ...state } = view;
  const progressMd = renderProgressMarkdown({ ...state, _runDir: runDir }, frontier, nextAction);
  const progressPath = path.join(runDir, 'PROGRESS.md');
  const current = fs.existsSync(progressPath) ? fs.readFileSync(progressPath, 'utf8') : null;
  if (current !== progressMd) writeFileAtomic(progressPath, progressMd);
}

/**
 * Persist a state transition: bump updated_at, write state.json atomically, regenerate PROGRESS.md.
 * Only mutating commands may call this — updated_at decides which run is "latest".
 */
function saveRunState(runDir, state) {
  state.updated_at = new Date().toISOString();
  const view = buildRunView(runDir, state);
  const { frontier, next_action: _nextAction, ...persisted } = view;
  writeFileAtomic(path.join(runDir, 'state.json'), JSON.stringify(persisted, null, 2) + '\n');
  refreshProgress(runDir, view);
  return view;
}

function readState(runDir) {
  return JSON.parse(fs.readFileSync(path.join(runDir, 'state.json'), 'utf8'));
}

/**
 * Refuse to start a run on top of uncommitted work: slice commits would sweep it into the PR.
 * Changes under .factory/ are ignored because the factory itself writes there.
 */
function assertCleanWorkingTree(rootDir) {
  const dirty = git(rootDir, ['status', '--porcelain', '--', '.', ':(exclude).factory']);
  if (dirty) {
    const files = dirty.split(/\r?\n/).slice(0, 10).join('\n  ');
    throw new Error(
      `Working tree has uncommitted changes; commit or stash them before starting a factory run ` +
        `(or pass --allow-dirty to accept them into the run):\n  ${files}`
    );
  }
}

/**
 * Put the repo on the branch this run will commit to, and return its name.
 * Default: create `factory/<slug>` from the current HEAD. Never commits onto the base branch.
 */
function prepareRunBranch(rootDir, { slug, baseBranch, branch, useCurrentBranch }) {
  const current = git(rootDir, ['branch', '--show-current']) || null;

  if (branch) {
    if (branch === baseBranch) {
      throw new Error(`Refusing to run on the base branch '${baseBranch}'. Pick a feature branch name.`);
    }
    if (current === branch) return branch;
    const args = localBranchExists(rootDir, branch) ? ['switch', branch] : ['switch', '-c', branch];
    gitOrThrow(rootDir, args, `Could not switch to branch '${branch}'`);
    return branch;
  }

  if (useCurrentBranch) {
    if (!current) throw new Error('HEAD is detached; --use-current-branch needs a named branch.');
    if (current === baseBranch) {
      throw new Error(
        `Refusing --use-current-branch on the base branch '${baseBranch}'; slice commits would land directly on it.`
      );
    }
    return current;
  }

  const runBranch = `factory/${slug}`;
  if (localBranchExists(rootDir, runBranch)) {
    throw new Error(
      `Branch '${runBranch}' already exists. Pass --branch <name> to use a different branch, ` +
        `or delete/rename the old one.`
    );
  }
  gitOrThrow(rootDir, ['switch', '-c', runBranch], `Could not create branch '${runBranch}'`);
  return runBranch;
}

/**
 * Initialize a new Factory Run in .factory/runs/<runId>/.
 */
/**
 * A repo whose checks already fail would block every ticket. Refuse until the user knowingly
 * accepts that baseline; accepted failing checks are then gated on changed files only.
 */
function assertBaselineAccepted(baseline, accepted) {
  const failing = baseline.results.filter((r) => !r.ok);
  if (failing.length === 0) return;
  const brokenPrepare = failing.find((r) => r.step === 'prepare');
  if (brokenPrepare) {
    throw new Error(
      `Baseline check failed: the '${brokenPrepare.component}' prepare command fails before any change (${brokenPrepare.reason}). ` +
        `Fix it or change it with 'factory-state setup --prepare-cmd ... --component ${brokenPrepare.component}'.\n${brokenPrepare.tail || ''}`
    );
  }
  if (accepted) return;
  const details = failing
    .map((r) => `- ${r.component}:${r.step} (${r.command}) → ${r.reason}\n${(r.tail || '').replace(/^/gm, '    ')}`)
    .join('\n');
  throw new Error(
    `Baseline check: these checks already fail on ${baseline.sha} before the run changes anything:\n${details}\n\n` +
      `Options:\n` +
      `  1. Fix them first (or fix the command in .factory/config.json), then re-run init.\n` +
      `  2. Re-run init with --accept-baseline: those checks are then gated only on files the run changes ` +
      `(using scoped_commands) or skipped where no scoped command exists.\n` +
      `  3. Re-run init with --skip-baseline to not check at all (every ticket will fail on these checks).`
  );
}

export function initRun(rootDir, options = {}) {
  const config = loadConfig(rootDir);
  const title = options.title || options.slug || options.prompt || 'factory-run';
  const slug = slugify(options.slug || title);
  const datePrefix = new Date().toISOString().slice(0, 10);
  const runId = options.runId || `${datePrefix}-${slug}`;

  const runDir = path.join(rootDir, '.factory', 'runs', runId);
  if (fs.existsSync(path.join(runDir, 'state.json'))) {
    throw new Error(
      `Factory run '${runId}' already exists. Resume it with 'resume ${runId}', or pick a different slug.`
    );
  }

  // Git preparation happens before anything is written, so a refusal leaves no half-created run.
  const baseBranch = config.base_branch || 'main';
  let baseSha = options.baseSha || null;
  let branch = options.branch || null;
  let baseline = null;
  if (isGitRepo(rootDir)) {
    baseSha = baseSha || git(rootDir, ['rev-parse', '--short', 'HEAD']);
    if (!options.allowDirty) assertCleanWorkingTree(rootDir);
    if (!options.skipBaseline && config.baseline !== 'skip') {
      baseline = captureBaseline(rootDir, config);
      assertBaselineAccepted(baseline, Boolean(options.acceptBaseline));
    }
    branch = prepareRunBranch(rootDir, {
      slug,
      baseBranch,
      branch: options.branch,
      useCurrentBranch: Boolean(options.useCurrentBranch),
    });
  }

  fs.mkdirSync(path.join(runDir, 'tickets'), { recursive: true });
  fs.mkdirSync(path.join(runDir, 'research'), { recursive: true });
  fs.mkdirSync(path.join(runDir, 'reviews'), { recursive: true });

  const mode = options.mode || config.default_mode || 'interactive';
  const lane = options.lane === 'quick' ? 'quick' : 'standard';

  const problemContent = `# Problem Statement

- **Run ID:** ${runId}
- **Issue Reference:** ${options.issue || 'None'}
- **Created At:** ${new Date().toISOString()}

## Raw Request

${options.prompt || title}
`;
  fs.writeFileSync(path.join(runDir, '00-problem.md'), problemContent, 'utf8');

  const initialState = {
    run_id: runId,
    title: String(title).slice(0, 100),
    slug,
    issue: options.issue || null,
    mode,
    stage: 'TRIAGE',
    status: 'ACTIVE',
    branch,
    base_branch: baseBranch,
    base_sha: baseSha,
    lane,
    strictness: config.strictness || 'greenfield',
    security_review: resolveSecurityReview(config),
    baseline,
    review_round: 0,
    max_review_rounds: config.max_review_rounds || 3,
    spec_approved: mode === 'afk' || lane === 'quick',
    decisions: [],
    history: [
      {
        at: new Date().toISOString(),
        event: 'INIT',
        stage: 'TRIAGE',
        note:
          `Initialized ${lane} run in '${mode}' mode${branch ? ` on branch '${branch}'` : ''}` +
          `${baseline && baseline.failing.length ? ` (accepted failing baseline: ${baseline.failing.join(', ')})` : ''}.`,
      },
    ],
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  };

  return saveRunState(runDir, initialState);
}

/**
 * Inspect the current status, ticket frontier, and next orchestrator action for a run.
 */
export function getRunStatus(rootDir, runId = 'latest') {
  const runDir = resolveRunDir(rootDir, runId);
  const view = buildRunView(runDir, readState(runDir));
  refreshProgress(runDir, view);
  return view;
}

/**
 * List all runs in .factory/runs/.
 */
export function listRuns(rootDir) {
  const runsRoot = path.join(rootDir, '.factory', 'runs');
  if (!fs.existsSync(runsRoot)) return [];

  return fs
    .readdirSync(runsRoot, { withFileTypes: true })
    .filter((d) => d.isDirectory() && fs.existsSync(path.join(runsRoot, d.name, 'state.json')))
    .map((d) => {
      const runDir = path.join(runsRoot, d.name);
      const st = buildRunView(runDir, readState(runDir));
      return {
        run_id: st.run_id,
        title: st.title,
        stage: st.stage,
        status: st.status,
        mode: st.mode,
        tickets_done: st.frontier.done.length,
        tickets_total: st.frontier.total,
        next_action: st.next_action.action,
        updated_at: st.updated_at,
      };
    })
    .sort((a, b) => b.updated_at.localeCompare(a.updated_at));
}

/**
 * Claim an unblocked frontier ticket before spawning slice-builder.
 */
export function claimTicket(rootDir, runId, ticketId) {
  const runDir = resolveRunDir(rootDir, runId);
  const state = JSON.parse(fs.readFileSync(path.join(runDir, 'state.json'), 'utf8'));
  const tickets = loadTickets(runDir);
  const frontier = computeFrontier(tickets);

  const normalizedTarget = String(ticketId).replace(/^#/, '');
  const ticket = tickets.find((t) => t.id === normalizedTarget || t.file === normalizedTarget);
  if (!ticket) {
    throw new Error(`Ticket '${ticketId}' not found in run '${state.run_id}'.`);
  }

  if (ticket.status === 'done') {
    throw new Error(`Ticket '${ticket.id}' is already done.`);
  }

  const isUnblocked = frontier.unblocked.some((u) => u.id === ticket.id);
  const isAlreadyClaimed = frontier.claimed.some((c) => c.id === ticket.id);
  if (!isUnblocked && !isAlreadyClaimed) {
    const blockedEntry = frontier.blocked.find((b) => b.id === ticket.id);
    const blockers = blockedEntry ? blockedEntry.open_blockers.join(', ') : 'unknown';
    throw new Error(`Ticket '${ticket.id}' is blocked by unfinished ticket(s): ${blockers}.`);
  }

  let raw = fs.readFileSync(ticket.path, 'utf8');
  if (/^\*\*Status:\*\*\s*.+$/im.test(raw)) {
    raw = raw.replace(/^\*\*Status:\*\*\s*.+$/im, '**Status:** claimed');
  } else if (/^Status:\s*.+$/im.test(raw)) {
    raw = raw.replace(/^Status:\s*.+$/im, 'Status: claimed');
  } else {
    raw += `\n\n**Status:** claimed\n`;
  }
  fs.writeFileSync(ticket.path, raw, 'utf8');

  state.history.push({
    at: new Date().toISOString(),
    event: 'CLAIM_TICKET',
    stage: state.stage,
    ticket: ticket.id,
  });

  return saveRunState(runDir, state);
}

function isAncestor(rootDir, ancestor, descendant) {
  return git(rootDir, ['merge-base', '--is-ancestor', ancestor, descendant]) !== null;
}

/**
 * Check what git can prove about a slice: everything is committed, the commit exists, is new on this
 * run's branch, and (unless the ticket opts out) the slice's commits touched at least one test file.
 */
function assertSliceCommitted(rootDir, state, ticket, commitRef) {
  const dirty = git(rootDir, ['status', '--porcelain', '--', '.', ':(exclude).factory']);
  if (dirty) {
    throw new Error(
      `Cannot complete ticket '${ticket.id}': the working tree has uncommitted changes. ` +
        `slice-builder must commit the slice before completion:\n  ${dirty.split(/\r?\n/).slice(0, 10).join('\n  ')}`
    );
  }

  const commitFull = git(rootDir, ['rev-parse', '--verify', '--quiet', `${commitRef}^{commit}`]);
  if (!commitFull) {
    throw new Error(`Cannot complete ticket '${ticket.id}': '${commitRef}' is not a commit in this repository.`);
  }
  const baseFull = git(rootDir, ['rev-parse', '--verify', '--quiet', `${state.base_sha}^{commit}`]);
  if (baseFull && (commitFull === baseFull || isAncestor(rootDir, commitFull, baseFull))) {
    throw new Error(
      `Cannot complete ticket '${ticket.id}': commit ${commitRef} is already part of the base (${state.base_sha}); ` +
        `the slice has not been committed on the run branch yet.`
    );
  }
  if (!isAncestor(rootDir, commitFull, 'HEAD')) {
    throw new Error(`Cannot complete ticket '${ticket.id}': commit ${commitRef} is not on the current branch.`);
  }

  if (ticket.test_change_required && baseFull) {
    const previous = [...(state.history || [])]
      .reverse()
      .filter((h) => h.event === 'COMPLETE_TICKET' && h.commit)
      .map((h) => git(rootDir, ['rev-parse', '--verify', '--quiet', `${h.commit}^{commit}`]))
      .find((sha) => sha && sha !== commitFull && isAncestor(rootDir, sha, commitFull));
    const from = previous || baseFull;
    const files = (git(rootDir, ['diff', '--name-only', '--relative', `${from}..${commitFull}`]) || '')
      .split(/\r?\n/)
      .filter(Boolean);
    if (!files.some(isTestFile)) {
      throw new Error(
        `Cannot complete ticket '${ticket.id}': none of the files changed in ${from.slice(0, 7)}..${commitFull.slice(0, 7)} ` +
          `is a test file. Add a test at the ticket's seam, or (prefactoring/config-only tickets) set ` +
          `'**Test change:** not-required — <reason>' in the ticket.`
      );
    }
  }
  return commitFull.slice(0, 7);
}

/**
 * Complete a claimed ticket. This is a real gate: the ticket must be claimed, every acceptance
 * criterion must already be ticked by slice-builder, the slice must be committed on the run branch
 * (with a test change), and the targeted components' test/typecheck/lint must pass.
 */
export function completeTicket(rootDir, runId, ticketId, options = {}) {
  if (options.checkAll) {
    throw new Error(
      `The --check-all option was removed: it ticked every acceptance criterion and defeated the gate. ` +
        `slice-builder must tick each satisfied criterion in the ticket itself; complete-ticket only verifies.`
    );
  }

  const runDir = resolveRunDir(rootDir, runId);
  const state = readState(runDir);
  const tickets = loadTickets(runDir);

  const normalizedTarget = String(ticketId).replace(/^#/, '');
  const ticket = tickets.find((t) => t.id === normalizedTarget || t.file === normalizedTarget);
  if (!ticket) {
    throw new Error(`Ticket '${ticketId}' not found in run '${state.run_id}'.`);
  }
  if (ticket.status === 'done') {
    throw new Error(`Ticket '${ticket.id}' is already done.`);
  }
  if (ticket.status !== 'claimed') {
    throw new Error(
      `Ticket '${ticket.id}' must be claimed (claim-ticket) before it can be completed; current status: ${ticket.status}.`
    );
  }

  let raw = fs.readFileSync(ticket.path, 'utf8');

  const remainingUnchecked = [...raw.matchAll(/^\s*-\s*\[\s\]\s+(.+)$/gm)].map((m) => m[1].trim());
  if (remainingUnchecked.length > 0) {
    throw new Error(
      `Cannot complete ticket '${ticket.id}': ${remainingUnchecked.length} acceptance criteria are still unchecked:\n` +
        remainingUnchecked.map((c) => `  - ${c}`).join('\n')
    );
  }

  const config = loadConfig(rootDir);
  const components = ticketComponents(ticket, config);

  let commitSha = options.commit || null;
  if (isGitRepo(rootDir) && state.base_sha) {
    commitSha = assertSliceCommitted(rootDir, state, ticket, options.commit || 'HEAD');
  } else if (!commitSha) {
    commitSha = git(rootDir, ['rev-parse', '--short', 'HEAD']) || null;
  }

  const { results: verified, skipped } = runVerification(rootDir, config, {
    context: `Cannot complete ticket '${ticket.id}'`,
    components,
    scope: 'ticket',
    baseSha: state.base_sha,
    baseline: state.baseline,
  });

  if (/^\*\*Status:\*\*\s*.+$/im.test(raw)) {
    raw = raw.replace(/^\*\*Status:\*\*\s*.+$/im, '**Status:** done');
  } else if (/^Status:\s*.+$/im.test(raw)) {
    raw = raw.replace(/^Status:\s*.+$/im, 'Status: done');
  } else {
    raw += `\n**Status:** done\n`;
  }

  if (commitSha) {
    if (/^\*\*Commit:\*\*\s*.+$/im.test(raw)) {
      raw = raw.replace(/^\*\*Commit:\*\*\s*.+$/im, `**Commit:** ${commitSha}`);
    } else {
      raw += `\n**Commit:** ${commitSha}\n`;
    }
  }

  if (options.verification) {
    raw += `\n## Verification\n\n${options.verification}\n`;
  }

  const verifiedLines = [
    ...verified.map(
      (v) => `- \`${v.component}\` \`${v.name}\` (\`${v.command}\`${v.mode === 'scoped' ? ', scoped' : ''}): PASS (${v.duration_ms} ms)`
    ),
    ...skipped.map((s) => `- \`${s.component}\` \`${s.step}_cmd\`: SKIPPED (${s.reason})`),
  ].join('\n');
  raw += `\n## Gate verification\n\nRun by \`factory-state complete-ticket\` at ${new Date().toISOString()}:\n\n${verifiedLines}\n`;

  fs.writeFileSync(ticket.path, raw, 'utf8');

  state.history.push({
    at: new Date().toISOString(),
    event: 'COMPLETE_TICKET',
    stage: state.stage,
    ticket: ticket.id,
    commit: commitSha,
    components,
    verified,
    skipped,
  });

  return saveRunState(runDir, state);
}

/**
 * Run full verification for a run (all components, full commands, baseline honoured).
 * Read-only: used by reviewers and pr-publisher for the final green check.
 */
export function verifyRun(rootDir, runId = 'latest', options = {}) {
  const runDir = resolveRunDir(rootDir, runId);
  const state = readState(runDir);
  const config = loadConfig(rootDir);
  const result = runVerification(rootDir, config, {
    context: `Verification of run '${state.run_id}'`,
    components: options.components || null,
    scope: 'full',
    baseSha: state.base_sha,
    baseline: state.baseline,
  });
  return { run_id: state.run_id, passed: true, ...result };
}

/**
 * Parse a review file for Verdict: PASS | FAIL and any bulleted findings.
 */
function parseReviewReport(filePath) {
  if (!fs.existsSync(filePath)) {
    return { exists: false, verdict: null, findings: [] };
  }
  const raw = fs.readFileSync(filePath, 'utf8');
  const verdictMatch =
    raw.match(/^\*\*Verdict:\*\*\s*(PASS|FAIL)/im) ||
    raw.match(/^Verdict:\s*(PASS|FAIL)/im) ||
    raw.match(/^##\s*Verdict\s*\n+\s*(PASS|FAIL)/im);

  const verdict = verdictMatch ? verdictMatch[1].toUpperCase() : null;
  const findings = [...raw.matchAll(/^\s*-\s+(.+)$/gm)].map((m) => m[1].trim());
  return { exists: true, verdict, findings, raw };
}

/**
 * Validate current stage gate and advance the Factory Run to the next stage.
 */
/** Text of a `**Label:** value` line in a markdown artifact, or null. */
function fieldValue(markdown, label) {
  const match =
    markdown.match(new RegExp(`^\\*\\*${label}:\\*\\*\\s*(.+)$`, 'im')) ||
    markdown.match(new RegExp(`^${label}:\\s*(.+)$`, 'im'));
  return match ? match[1].trim() : null;
}

/**
 * Quick lane: skip SPEC by deriving a minimal 02-spec.md and a single ticket from the brief.
 * The brief's acceptance criteria become the ticket's criteria (unticked).
 */
function writeQuickLaneSpec(runDir, state, brief, config) {
  const criteria = [...brief.matchAll(/^\s*-\s*\[[ xX]\]\s+(.+)$/gm)].map((m) => m[1].trim());
  if (criteria.length === 0) {
    throw new Error(
      `Stage Gate Failed (TRIAGE -> IMPLEMENT, quick lane): '01-brief.md' needs at least one '- [ ] <criterion>' acceptance criterion.`
    );
  }
  const components = config.components || [];
  const component = fieldValue(brief, 'Components?') || (components.length > 1 ? 'all' : null);
  if (component && component !== 'all') {
    const unknown = component
      .split(/[,;]+/)
      .map((c) => c.trim().replace(/`/g, ''))
      .filter((c) => c && !components.some((k) => k.name === c));
    if (unknown.length > 0) {
      throw new Error(
        `Stage Gate Failed (TRIAGE -> IMPLEMENT, quick lane): brief names unknown component(s) ${unknown.join(', ')} (known: ${components.map((c) => c.name).join(', ')}).`
      );
    }
  }
  const seam = fieldValue(brief, 'Seam') || 'The public interface named under Key interfaces in 01-brief.md';
  const summary = fieldValue(brief, 'Summary') || state.title;

  const spec = `# Spec (quick lane)

Generated from [01-brief.md](./01-brief.md) by the quick lane, which skips the SPEC stage.

## Problem Statement
${summary}

## Testing Decisions
- **Seam:** ${seam}
- Tests assert the acceptance criteria in 01-brief.md through that seam.

## Out of Scope
See 01-brief.md.
`;
  fs.writeFileSync(path.join(runDir, '02-spec.md'), spec, 'utf8');

  const ticketFile = `01-${slugify(summary).slice(0, 40)}.md`;
  const ticket = `# 01: ${summary}

**What to build:** ${summary}
**Seam:** ${seam}
${component ? `**Component:** ${component}\n` : ''}**Blocked by:** None
**Status:** ready-for-agent

## Acceptance criteria
${criteria.map((c) => `- [ ] ${c}`).join('\n')}
`;
  fs.writeFileSync(path.join(runDir, 'tickets', ticketFile), ticket, 'utf8');
  return ticketFile;
}

export function advanceStage(rootDir, runId = 'latest') {
  const runDir = resolveRunDir(rootDir, runId);
  const state = JSON.parse(fs.readFileSync(path.join(runDir, 'state.json'), 'utf8'));

  if (state.stage === 'TRIAGE') {
    const briefPath = path.join(runDir, '01-brief.md');
    if (!fs.existsSync(briefPath)) {
      throw new Error(`Stage Gate Failed (TRIAGE -> SPEC): '01-brief.md' does not exist.`);
    }
    const brief = fs.readFileSync(briefPath, 'utf8');

    // Check if triage investigator marked the brief as needing human clarification
    if (/^\*\*Status:\*\*\s*needs-info/im.test(brief) || /^Status:\s*needs-info/im.test(brief)) {
      state.status = 'WAITING_FOR_HUMAN';
      state.waiting_reason =
        'Triage identified open questions (Status: needs-info in 01-brief.md). Grill the user and update 01-brief.md to Status: ready-for-agent.';
      state.history.push({
        at: new Date().toISOString(),
        event: 'GATE_PAUSED_NEEDS_INFO',
        stage: 'TRIAGE',
      });
      return saveRunState(runDir, state);
    }

    if (/^\*\*Status:\*\*\s*wontfix/im.test(brief) || /^Status:\s*wontfix/im.test(brief)) {
      state.stage = 'DONE';
      state.status = 'COMPLETED';
      state.history.push({
        at: new Date().toISOString(),
        event: 'CLOSED_WONTFIX',
        stage: 'DONE',
      });
      return saveRunState(runDir, state);
    }

    const hasDesired = /desired behavior/i.test(brief);
    const hasCriteria = /acceptance criteria/i.test(brief);
    if (!hasDesired || !hasCriteria) {
      throw new Error(
        `Stage Gate Failed (TRIAGE -> SPEC): '01-brief.md' must include 'Desired behavior' and 'Acceptance criteria'.`
      );
    }

    if (state.lane === 'quick') {
      const ticketFile = writeQuickLaneSpec(runDir, state, brief, loadConfig(rootDir));
      state.stage = 'IMPLEMENT';
      state.status = 'ACTIVE';
      state.waiting_reason = null;
      state.history.push({
        at: new Date().toISOString(),
        event: 'ADVANCE_QUICK_LANE',
        from: 'TRIAGE',
        to: 'IMPLEMENT',
        ticket: ticketFile,
      });
      return saveRunState(runDir, state);
    }

    state.stage = 'SPEC';
    state.status = 'ACTIVE';
    state.waiting_reason = null;
    state.history.push({
      at: new Date().toISOString(),
      event: 'ADVANCE',
      from: 'TRIAGE',
      to: 'SPEC',
    });
    return saveRunState(runDir, state);
  }

  if (state.stage === 'SPEC') {
    const specPath = path.join(runDir, '02-spec.md');
    if (!fs.existsSync(specPath)) {
      throw new Error(`Stage Gate Failed (SPEC -> IMPLEMENT): '02-spec.md' does not exist.`);
    }
    const spec = fs.readFileSync(specPath, 'utf8');
    if (!/testing decisions|seam/i.test(spec)) {
      throw new Error(
        `Stage Gate Failed (SPEC -> IMPLEMENT): '02-spec.md' must document the agreed test seams ('Testing Decisions').`
      );
    }

    const tickets = loadTickets(runDir);
    if (tickets.length === 0) {
      throw new Error(
        `Stage Gate Failed (SPEC -> IMPLEMENT): At least one vertical slice ticket must exist in 'tickets/*.md'.`
      );
    }
    validateTicketGraph(tickets, loadConfig(rootDir));

    // In interactive mode, pause once for human approval of Spec & Seams before starting TDD loop
    if (state.mode === 'interactive' && !state.spec_approved) {
      state.status = 'WAITING_FOR_HUMAN';
      state.waiting_reason =
        'Confirm the Spec, Test Seams (02-spec.md), and Tracer-Bullet Tickets (tickets/*.md) with the user, then run approve.';
      state.history.push({
        at: new Date().toISOString(),
        event: 'WAIT_SPEC_APPROVAL',
        stage: 'SPEC',
      });
      return saveRunState(runDir, state);
    }

    state.stage = 'IMPLEMENT';
    state.status = 'ACTIVE';
    state.waiting_reason = null;
    state.history.push({
      at: new Date().toISOString(),
      event: 'ADVANCE',
      from: 'SPEC',
      to: 'IMPLEMENT',
    });
    return saveRunState(runDir, state);
  }

  if (state.stage === 'IMPLEMENT') {
    const tickets = loadTickets(runDir);
    const frontier = computeFrontier(tickets);
    if (!frontier.all_done) {
      throw new Error(
        `Stage Gate Failed (IMPLEMENT -> REVIEW): ${frontier.total - frontier.done.length} of ${frontier.total} ticket(s) are not yet 'done'.`
      );
    }

    state.stage = 'REVIEW';
    state.status = 'ACTIVE';
    state.review_round = (state.review_round || 0) + 1;
    state.history.push({
      at: new Date().toISOString(),
      event: 'ADVANCE',
      from: 'IMPLEMENT',
      to: 'REVIEW',
      review_round: state.review_round,
    });
    return saveRunState(runDir, state);
  }

  if (state.stage === 'REVIEW') {
    const round = state.review_round || 1;
    const axes = reviewAxes(state);
    const reports = axes.map((axis) => ({
      axis,
      file: `reviews/round-${round}-${axis}.md`,
      ...parseReviewReport(path.join(runDir, 'reviews', `round-${round}-${axis}.md`)),
    }));

    const missing = reports.filter((r) => !r.exists);
    if (missing.length > 0) {
      throw new Error(
        `Stage Gate Failed (REVIEW): ${reports.map((r) => `'${r.file}'`).join(', ')} must all exist (missing: ${missing.map((r) => r.file).join(', ')}).`
      );
    }
    if (reports.some((r) => !r.verdict)) {
      throw new Error(
        `Stage Gate Failed (REVIEW): every review file must declare '**Verdict:** PASS' or '**Verdict:** FAIL'.`
      );
    }

    if (reports.every((r) => r.verdict === 'PASS')) {
      state.stage = 'PR';
      state.status = 'ACTIVE';
      state.history.push({
        at: new Date().toISOString(),
        event: 'REVIEW_PASSED',
        round,
        axes,
        from: 'REVIEW',
        to: 'PR',
      });
      return saveRunState(runDir, state);
    }

    // At least one axis failed
    if (round >= (state.max_review_rounds || 3)) {
      state.status = 'BLOCKED_NEEDS_HUMAN';
      state.blocked_reason = `Review failed at Round ${round} (max_review_rounds=${state.max_review_rounds} reached). Human review required.`;
      state.history.push({
        at: new Date().toISOString(),
        event: 'REVIEW_MAX_ROUNDS_EXCEEDED',
        round,
      });
      return saveRunState(runDir, state);
    }

    // Create a remediation ticket in tickets/ so a fresh slice-builder fixes the findings
    const existingTickets = loadTickets(runDir);
    const nextNum = String(existingTickets.length + 1).padStart(2, '0');
    const fixSlug = `${nextNum}-review-round-${round}-fixes.md`;
    const fixPath = path.join(runDir, 'tickets', fixSlug);

    const failed = reports.filter((r) => r.verdict === 'FAIL');
    const label = (axis) => axis.charAt(0).toUpperCase() + axis.slice(1);
    const combinedFindings = failed.flatMap((r) =>
      r.findings.filter((f) => !/^(none|n\/a)\b/i.test(f)).map((f) => `[${label(r.axis)}] ${f}`)
    );
    const criteriaList =
      combinedFindings.length > 0
        ? combinedFindings.map((f) => `- [ ] ${f}`).join('\n')
        : `- [ ] Resolve all findings in ${failed.map((r) => r.file).join(' and ')}`;
    const links = reports.map((r) => `[round-${round}-${r.axis}.md](../${r.file})`).join(', ');
    const fixConfig = loadConfig(rootDir);
    const componentLine = (fixConfig.components || []).length > 1 ? '**Component:** all\n' : '';

    const fixTicketMd = `# ${nextNum}: Remediation for Review Round ${round}

**What to build:** Resolve all findings flagged in Review Round ${round} (${links}).
**Seam:** Existing seams from 02-spec.md
${componentLine}**Blocked by:** None
**Test change:** not-required — review fixes may be refactors; add a test for any behavioural fix
**Status:** ready-for-agent

## Acceptance criteria
${criteriaList}
`;
    fs.writeFileSync(fixPath, fixTicketMd, 'utf8');

    state.stage = 'IMPLEMENT';
    state.status = 'ACTIVE';
    state.history.push({
      at: new Date().toISOString(),
      event: 'REVIEW_FAILED_REMEDIATION_CREATED',
      round,
      failed_axes: failed.map((r) => r.axis),
      remediation_ticket: nextNum,
      from: 'REVIEW',
      to: 'IMPLEMENT',
    });
    return saveRunState(runDir, state);
  }

  if (state.stage === 'PR') {
    const prPath = path.join(runDir, '05-pr.md');
    if (!fs.existsSync(prPath)) {
      throw new Error(`Stage Gate Failed (PR -> DONE): '05-pr.md' does not exist.`);
    }
    state.stage = 'DONE';
    state.status = 'COMPLETED';
    state.history.push({
      at: new Date().toISOString(),
      event: 'ADVANCE',
      from: 'PR',
      to: 'DONE',
    });
    return saveRunState(runDir, state);
  }

  return saveRunState(runDir, state);
}

/**
 * Record human approval to clear WAITING_FOR_HUMAN and advance if waiting at SPEC gate.
 */
export function approveRun(rootDir, runId = 'latest', note = 'Approved by user') {
  const runDir = resolveRunDir(rootDir, runId);
  const state = JSON.parse(fs.readFileSync(path.join(runDir, 'state.json'), 'utf8'));

  state.status = 'ACTIVE';
  state.waiting_reason = null;
  state.blocked_reason = null;
  if (state.stage === 'SPEC') {
    state.spec_approved = true;
  }

  state.decisions = state.decisions || [];
  state.decisions.push({
    at: new Date().toISOString(),
    stage: state.stage,
    text: note,
  });

  state.history.push({
    at: new Date().toISOString(),
    event: 'HUMAN_APPROVED',
    stage: state.stage,
    note,
  });

  saveRunState(runDir, state);

  // If we were at the SPEC gate waiting for human approval, automatically advance to IMPLEMENT
  if (state.stage === 'SPEC' && state.spec_approved) {
    return advanceStage(rootDir, state.run_id);
  }
  return getRunStatus(rootDir, state.run_id);
}

/**
 * Record a key domain or architectural decision in state.json and PROGRESS.md.
 */
export function recordDecision(rootDir, runId, decisionText) {
  const runDir = resolveRunDir(rootDir, runId);
  const state = JSON.parse(fs.readFileSync(path.join(runDir, 'state.json'), 'utf8'));
  state.decisions = state.decisions || [];
  state.decisions.push({
    at: new Date().toISOString(),
    stage: state.stage,
    text: decisionText,
  });
  return saveRunState(runDir, state);
}

/**
 * Simple CLI argument parser.
 */
// Flags that never take a value, so `init --afk my-slug` keeps `my-slug` as a positional.
const BOOLEAN_FLAGS = new Set([
  'afk',
  'quick',
  'check-all',
  'allow-dirty',
  'use-current-branch',
  'accept-baseline',
  'skip-baseline',
  'help',
]);

function parseCliArgs(argv) {
  const positional = [];
  const flags = {};
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    if (arg.startsWith('--')) {
      const key = arg.slice(2);
      const next = argv[i + 1];
      if (BOOLEAN_FLAGS.has(key) || !next || next.startsWith('--')) {
        flags[key] = true;
      } else {
        flags[key] = next;
        i++;
      }
    } else {
      positional.push(arg);
    }
  }
  return { positional, flags };
}

/**
 * CLI Entrypoint
 */
function runCli() {
  const { positional, flags } = parseCliArgs(process.argv.slice(2));
  const cmd = positional[0] || 'status';
  const rootDir = path.resolve(flags.root || process.cwd());

  try {
    switch (cmd) {
      case 'setup': {
        const cfg = setupFactory(rootDir, {
          component: flags.component,
          language: flags.language,
          prepareCmd: flags['prepare-cmd'],
          testCmd: flags['test-cmd'],
          typecheckCmd: flags['typecheck-cmd'],
          lintCmd: flags['lint-cmd'],
          scopedTestCmd: flags['scoped-test-cmd'],
          scopedTypecheckCmd: flags['scoped-typecheck-cmd'],
          scopedLintCmd: flags['scoped-lint-cmd'],
          tracker: flags.tracker,
          baseBranch: flags['base-branch'],
          mode: flags.mode,
          maxReviewRounds: flags['max-review-rounds'],
          strictness: flags.strictness,
          securityReview: flags['security-review'],
          ticketVerification: flags['ticket-verification'],
          baseline: flags.baseline,
        });
        console.log(JSON.stringify(cfg, null, 2));
        break;
      }
      case 'init': {
        const slugOrTitle = positional[1] || flags.slug || flags.title || 'run';
        const res = initRun(rootDir, {
          slug: flags.slug || slugOrTitle,
          title: flags.title || slugOrTitle,
          prompt: flags.prompt || positional.slice(1).join(' ') || slugOrTitle,
          mode: flags.afk ? 'afk' : flags.mode,
          issue: flags.issue,
          branch: flags.branch,
          baseSha: flags.base,
          allowDirty: Boolean(flags['allow-dirty']),
          useCurrentBranch: Boolean(flags['use-current-branch']),
          lane: flags.quick ? 'quick' : 'standard',
          acceptBaseline: Boolean(flags['accept-baseline']),
          skipBaseline: Boolean(flags['skip-baseline']),
        });
        console.log(JSON.stringify(res, null, 2));
        break;
      }
      case 'detect': {
        console.log(JSON.stringify(detectComponents(rootDir), null, 2));
        break;
      }
      case 'verify': {
        const components = flags.component ? String(flags.component).split(',') : null;
        console.log(JSON.stringify(verifyRun(rootDir, positional[1] || 'latest', { components }), null, 2));
        break;
      }
      case 'list': {
        console.log(JSON.stringify(listRuns(rootDir), null, 2));
        break;
      }
      case 'status': {
        const target = positional[1];
        if (!target) {
          const runs = listRuns(rootDir);
          if (runs.length === 0) {
            console.log(JSON.stringify({ runs: [], message: 'No factory runs found.' }, null, 2));
          } else {
            console.log(JSON.stringify(getRunStatus(rootDir, 'latest'), null, 2));
          }
        } else {
          console.log(JSON.stringify(getRunStatus(rootDir, target), null, 2));
        }
        break;
      }
      case 'frontier': {
        const st = getRunStatus(rootDir, positional[1] || 'latest');
        console.log(JSON.stringify(st.frontier, null, 2));
        break;
      }
      case 'claim-ticket': {
        const runId = positional.length >= 3 ? positional[1] : 'latest';
        const ticketId = positional.length >= 3 ? positional[2] : positional[1];
        const st = claimTicket(rootDir, runId, ticketId);
        console.log(JSON.stringify(st, null, 2));
        break;
      }
      case 'complete-ticket': {
        const runId = positional.length >= 3 ? positional[1] : 'latest';
        const ticketId = positional.length >= 3 ? positional[2] : positional[1];
        const st = completeTicket(rootDir, runId, ticketId, {
          commit: flags.commit,
          checkAll: Boolean(flags['check-all']),
          verification: flags.verification,
        });
        console.log(JSON.stringify(st, null, 2));
        break;
      }
      case 'advance': {
        const st = advanceStage(rootDir, positional[1] || 'latest');
        console.log(JSON.stringify(st, null, 2));
        break;
      }
      case 'approve': {
        const runId = positional[1] || 'latest';
        const note = flags.note || positional.slice(2).join(' ') || 'Approved by user';
        const st = approveRun(rootDir, runId, note);
        console.log(JSON.stringify(st, null, 2));
        break;
      }
      case 'record-decision': {
        const runId = positional.length >= 3 ? positional[1] : 'latest';
        const text = positional.length >= 3 ? positional.slice(2).join(' ') : positional[1];
        const st = recordDecision(rootDir, runId, text);
        console.log(JSON.stringify(st, null, 2));
        break;
      }
      default:
        console.error(
          `Unknown command '${cmd}'. Available commands: setup, detect, init, list, status, frontier, claim-ticket, complete-ticket, verify, advance, approve, record-decision`
        );
        process.exit(1);
    }
  } catch (err) {
    console.error(JSON.stringify({ error: err.message }, null, 2));
    process.exit(1);
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === path.resolve(fileURLToPath(import.meta.url))) {
  runCli();
}
