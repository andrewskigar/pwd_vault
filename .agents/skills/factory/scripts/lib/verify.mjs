// Verification: runs each component's test/typecheck/lint commands, optionally scoped to the files
// a run changed, honouring a baseline of checks that were already failing before the run started.
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync, spawnSync } from 'node:child_process';
import { STEPS } from './detect.mjs';

const LAST_OUTPUT_LINES = 40;

function git(cwd, args) {
  try {
    return execFileSync('git', args, { cwd, stdio: ['ignore', 'pipe', 'ignore'], encoding: 'utf8' }).trim();
  } catch {
    return null;
  }
}

/**
 * cmd.exe reads `/` in the program token as a switch, so `vendor/bin/phpunit` fails on Windows.
 * Convert a relative program path in the first token to backslashes; cmd then also finds the
 * `.bat`/`.cmd` shim via PATHEXT. Commands are stored portably and converted only at run time.
 */
export function normalizeCommandForPlatform(command, platform = process.platform) {
  if (platform !== 'win32' || !command) return command;
  return command.replace(
    /^(\s*)((?:\.{1,2}\/)?[\w.-]+(?:\/[\w.-]+)+)(?=\s|$)/,
    (_match, lead, program) => lead + program.replace(/\//g, '\\')
  );
}

/**
 * Files this run changed (committed since baseSha, staged, unstaged, or untracked), relative to
 * rootDir, excluding factory bookkeeping. Returns null outside git or without a base.
 */
export function changedFiles(rootDir, baseSha) {
  if (!baseSha || git(rootDir, ['rev-parse', '--is-inside-work-tree']) !== 'true') return null;
  const lists = [
    git(rootDir, ['diff', '--name-only', '--relative', '--diff-filter=ACMR', `${baseSha}...HEAD`]),
    git(rootDir, ['diff', '--name-only', '--relative', '--diff-filter=ACMR', 'HEAD']),
    git(rootDir, ['ls-files', '--others', '--exclude-standard']),
  ];
  const files = new Set();
  for (const list of lists) {
    for (const line of (list || '').split(/\r?\n/)) {
      const file = line.trim().replace(/\\/g, '/');
      if (file && !file.startsWith('.factory/')) files.add(file);
    }
  }
  return [...files].sort();
}

/** Narrow repo-relative files to one component: inside its folder, matching its extensions, still on disk. */
export function filesForComponent(rootDir, files, component) {
  const prefix = component.path && component.path !== '.' ? `${component.path.replace(/\/+$/, '')}/` : '';
  const extensions = component.extensions || null;
  return files
    .filter((f) => !prefix || f.startsWith(prefix))
    .filter((f) => !extensions || extensions.some((ext) => f.toLowerCase().endsWith(ext)))
    .filter((f) => fs.existsSync(path.join(rootDir, f)))
    .map((f) => f.slice(prefix.length));
}

export function substitutePlaceholders(command, { files = [], baseSha = null } = {}) {
  return command
    .replace(/\{files\}/g, files.map((f) => `"${f}"`).join(' '))
    .replace(/\{base_sha\}/g, baseSha || 'HEAD');
}

export function runShellCommand(command, cwd, timeoutMs) {
  const startedAt = Date.now();
  const proc = spawnSync(normalizeCommandForPlatform(command), {
    cwd,
    shell: true,
    encoding: 'utf8',
    maxBuffer: 64 * 1024 * 1024,
    timeout: timeoutMs,
  });
  const durationMs = Date.now() - startedAt;
  const ok = !proc.error && proc.status === 0;
  const reason = proc.error
    ? proc.error.message
    : proc.signal
      ? `killed by ${proc.signal}`
      : `exit code ${proc.status}`;
  const tail = `${proc.stdout || ''}\n${proc.stderr || ''}`.trim().split(/\r?\n/).slice(-LAST_OUTPUT_LINES).join('\n');
  return { ok, reason: ok ? null : reason, tail, duration_ms: durationMs };
}

/** Pick components by name; null/empty/'all' means every component. */
export function selectComponents(config, names) {
  const all = config.components || [];
  if (!names || names.length === 0 || names.includes('all')) return all;
  const selected = all.filter((c) => names.includes(c.name));
  const unknown = names.filter((n) => !all.some((c) => c.name === n));
  if (unknown.length > 0) {
    throw new Error(
      `Unknown component(s) ${unknown.join(', ')}. Configured components: ${all.map((c) => c.name).join(', ') || 'none'}.`
    );
  }
  return selected;
}

function timeoutFor(config) {
  return Number(config.verify_timeout_ms) || 30 * 60 * 1000;
}

/**
 * Run the verification steps for the chosen components from each component's folder.
 * - scope 'ticket' uses scoped commands when config.ticket_verification is 'scoped'.
 * - A step that failed at baseline uses its scoped command (new problems only), or is reported
 *   as skipped when no scoped command exists — it can never pass, so gating on it would stall.
 * Throws on the first failure with the tail of its output.
 */
export function runVerification(rootDir, config, options = {}) {
  const context = options.context || 'Verification';
  const targets = selectComponents(config, options.components);
  const multi = (config.components || []).length > 1;

  if (!targets.some((c) => c.commands?.test || c.scoped_commands?.test)) {
    throw new Error(
      `${context}: no test command configured for ${targets.map((c) => `'${c.name}'`).join(', ') || 'any component'} in .factory/config.json. ` +
        `Run 'factory-state setup --test-cmd "<command>"${multi ? ' --component <name>' : ''}' so completion can be verified.`
    );
  }

  const failingAtBaseline = new Set(options.baseline?.failing || []);
  const preferScoped = options.scope === 'ticket' && config.ticket_verification === 'scoped';
  const changed = changedFiles(rootDir, options.baseSha);
  const results = [];
  const skipped = [];

  for (const component of targets) {
    const cwd = path.resolve(rootDir, component.path || '.');
    const prepare = component.commands?.prepare;
    if (prepare) {
      const res = runShellCommand(prepare, cwd, timeoutFor(config));
      if (!res.ok) {
        throw new Error(`${context}: ${component.name}: prepare failed (${res.reason}): ${prepare}\n--- last output ---\n${res.tail}`);
      }
    }

    for (const step of STEPS) {
      const full = component.commands?.[step] || null;
      const scoped = component.scoped_commands?.[step] || null;
      if (!full && !scoped) continue;
      const atBaseline = failingAtBaseline.has(`${component.name}:${step}`);
      const useScoped = Boolean(scoped && changed && (atBaseline || preferScoped || !full));

      let command = full;
      let mode = 'full';
      if (useScoped) {
        mode = 'scoped';
        if (scoped.includes('{files}')) {
          const files = filesForComponent(rootDir, changed, component);
          if (files.length === 0) {
            skipped.push({ component: component.name, step, reason: 'no changed files in this component' });
            continue;
          }
          command = substitutePlaceholders(scoped, { files, baseSha: options.baseSha });
        } else {
          command = substitutePlaceholders(scoped, { baseSha: options.baseSha });
        }
      } else if (atBaseline) {
        skipped.push({
          component: component.name,
          step,
          reason: 'already failing before the run (baseline) and no scoped command is configured; not gated',
        });
        continue;
      } else if (!full) {
        continue;
      }

      const res = runShellCommand(command, cwd, timeoutFor(config));
      if (!res.ok) {
        const label = multi ? `${component.name}: ${step}_cmd` : `${step}_cmd`;
        throw new Error(`${context}: ${label} failed (${res.reason}): ${command}\n--- last output ---\n${res.tail}`);
      }
      results.push({ component: component.name, name: `${step}_cmd`, command, mode, duration_ms: res.duration_ms });
    }
  }
  return { results, skipped };
}

/**
 * Run every full command once on the current commit and record which already fail.
 * Used at init so existing lint/type debt in a brownfield repo doesn't block every ticket.
 */
export function captureBaseline(rootDir, config) {
  const sha = git(rootDir, ['rev-parse', '--short', 'HEAD']);
  const results = [];
  for (const component of config.components || []) {
    const cwd = path.resolve(rootDir, component.path || '.');
    const steps = STEPS.filter((s) => component.commands?.[s]);
    if (steps.length === 0) continue;
    if (component.commands?.prepare) {
      const res = runShellCommand(component.commands.prepare, cwd, timeoutFor(config));
      if (!res.ok) {
        results.push({ component: component.name, step: 'prepare', ok: false, reason: res.reason, tail: res.tail });
        continue;
      }
    }
    for (const step of steps) {
      const res = runShellCommand(component.commands[step], cwd, timeoutFor(config));
      results.push({
        component: component.name,
        step,
        command: component.commands[step],
        ok: res.ok,
        reason: res.reason,
        tail: res.ok ? undefined : res.tail.split(/\r?\n/).slice(-15).join('\n'),
      });
    }
  }
  return {
    captured_at: new Date().toISOString(),
    sha,
    results,
    failing: results.filter((r) => !r.ok).map((r) => `${r.component}:${r.step}`),
  };
}

const TEST_FILE_PATTERN =
  /(^|\/)(tests?|__tests__|specs?|e2e|cypress|playwright)\/|[._-](test|spec)\.[a-z]+$|_test\.(go|py)$|(^|\/)test_[^/]+\.py$|Test\.php$|(^|\/)testing\//i;

export function isTestFile(file) {
  return TEST_FILE_PATTERN.test(file.replace(/\\/g, '/'));
}
