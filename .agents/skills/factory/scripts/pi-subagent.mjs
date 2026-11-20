#!/usr/bin/env node
// Run factory sub-agents with the Pi coding agent, which has no built-in sub-agents.
// Each sub-agent is a separate, session-less `pi -p` process (its own context window) with the
// agent's instructions + task appended to the system prompt and its tools restricted by access level.
//
//   node pi-subagent.mjs run --agent slice-builder --task "Implement ticket ..." \
//                            [--agent spec-reviewer --task "..."]   # several pairs run in parallel
//                            [--model <pattern>] [--cwd <dir>] [--concurrency 4]
//
// FACTORY_AGENT=<name> is set in each child so the factory guard extension can confine reviewers.
// FACTORY_PI_BIN overrides the `pi` executable (a command string, e.g. for tests or custom installs).
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { parseFrontmatter, toolsFor } from './lib/harness.mjs';

const SKILL_DIR = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const OUTPUT_TAIL = 6000;
const KICKOFF =
  'Carry out the task in your instructions now. When finished, reply with a short report: what you did, files written, and any blocker.';

export function parseArgs(argv) {
  const jobs = [];
  const options = { concurrency: 4 };
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    const next = () => {
      const value = argv[++i];
      if (value === undefined) throw new Error(`${arg} needs a value`);
      return value;
    };
    if (arg === 'run') continue;
    if (arg === '--agent') jobs.push({ agent: next(), task: null });
    else if (arg === '--task' || arg === '--task-file') {
      const job = jobs.at(-1);
      if (!job) throw new Error(`${arg} must follow an --agent`);
      job.task = arg === '--task' ? next() : fs.readFileSync(next(), 'utf8');
    } else if (arg === '--model') options.model = next();
    else if (arg === '--cwd') options.cwd = next();
    else if (arg === '--concurrency') options.concurrency = Math.max(1, Number(next()) || 1);
    else throw new Error(`Unknown argument '${arg}'`);
  }
  if (jobs.length === 0) throw new Error('Pass at least one --agent <name> --task <text>.');
  for (const job of jobs) if (!job.task) throw new Error(`Agent '${job.agent}' has no --task.`);
  return { jobs, options };
}

/** Build the pi argv for one job and write its system-prompt file. */
export function preparePiInvocation(job, options = {}) {
  const promptPath = path.join(SKILL_DIR, 'agents', `${job.agent}.md`);
  if (!fs.existsSync(promptPath)) {
    const known = fs.readdirSync(path.join(SKILL_DIR, 'agents')).map((f) => f.replace(/\.md$/, ''));
    throw new Error(`Unknown factory agent '${job.agent}'. Known: ${known.join(', ')}.`);
  }
  const { meta, body } = parseFrontmatter(fs.readFileSync(promptPath, 'utf8'));
  const tools = toolsFor('pi', meta.access || 'write');
  const systemFile = path.join(fs.mkdtempSync(path.join(os.tmpdir(), 'factory-pi-')), `${job.agent}.md`);
  fs.writeFileSync(
    systemFile,
    `${body.trimEnd()}\n\n---\n\n## Your task (from the factory orchestrator)\n\n${job.task.trim()}\n\n` +
      `The factory skill folder is: ${SKILL_DIR.replace(/\\/g, '/')}\n`,
    'utf8'
  );
  const args = ['-p', '--no-session', '--tools', tools.join(','), '--append-system-prompt', systemFile];
  const model = options.model || meta['pi-model'];
  if (model) args.push('--model', model);
  args.push(KICKOFF);
  return { args, systemFile, tools };
}

function quoteForShell(arg) {
  return /^[\w@%+=:,./\\-]+$/.test(arg) ? arg : `"${String(arg).replace(/"/g, '\\"')}"`;
}

function runOne(job, options) {
  return new Promise((resolve) => {
    let invocation;
    try {
      invocation = preparePiInvocation(job, options);
    } catch (err) {
      resolve({ agent: job.agent, exit_code: 1, output: '', error: err.message });
      return;
    }
    const custom = process.env.FACTORY_PI_BIN;
    // pi is a .cmd shim on Windows and FACTORY_PI_BIN may be a command string, so both need a shell.
    const useShell = Boolean(custom) || process.platform === 'win32';
    const bin = custom || 'pi';
    const child = useShell
      ? spawn([bin, ...invocation.args.map(quoteForShell)].join(' '), {
          cwd: options.cwd || process.cwd(),
          shell: true,
          env: { ...process.env, FACTORY_AGENT: job.agent },
        })
      : spawn(bin, invocation.args, {
          cwd: options.cwd || process.cwd(),
          env: { ...process.env, FACTORY_AGENT: job.agent },
        });
    let stdout = '';
    let stderr = '';
    child.stdout.on('data', (d) => (stdout += d));
    child.stderr.on('data', (d) => (stderr += d));
    child.on('error', (err) => {
      resolve({
        agent: job.agent,
        exit_code: 1,
        output: stdout.slice(-OUTPUT_TAIL),
        error: `${err.message}. Is pi installed and on PATH (or FACTORY_PI_BIN set)?`,
      });
    });
    child.on('close', (code) => {
      fs.rmSync(path.dirname(invocation.systemFile), { recursive: true, force: true });
      resolve({
        agent: job.agent,
        exit_code: code ?? 1,
        tools: invocation.tools,
        output: stdout.trim().slice(-OUTPUT_TAIL),
        ...(code ? { stderr: stderr.trim().slice(-OUTPUT_TAIL) } : {}),
      });
    });
  });
}

export async function runJobs(jobs, options = {}) {
  const results = new Array(jobs.length);
  let next = 0;
  const worker = async () => {
    while (next < jobs.length) {
      const index = next++;
      results[index] = await runOne(jobs[index], options);
    }
  };
  await Promise.all(Array.from({ length: Math.min(options.concurrency || 4, jobs.length) }, worker));
  return results;
}

if (process.argv[1] && path.resolve(process.argv[1]) === path.resolve(fileURLToPath(import.meta.url))) {
  try {
    const { jobs, options } = parseArgs(process.argv.slice(2));
    const results = await runJobs(jobs, options);
    console.log(JSON.stringify({ results }, null, 2));
    process.exit(results.every((r) => r.exit_code === 0) ? 0 : 1);
  } catch (err) {
    console.error(JSON.stringify({ error: err.message }, null, 2));
    process.exit(1);
  }
}
