// Project detection: finds every component (language + folder) in a repo, its language version,
// frameworks, project type, and the verification commands whose tools are actually installed.
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

/** Root of the factory skill bundle (the folder holding SKILL.md). */
export const SKILL_DIR = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', '..');

const IGNORED_DIRS = new Set([
  'node_modules', 'vendor', 'dist', 'build', 'target', 'out', 'coverage', 'venv', 'env',
  '__pycache__', 'fixtures', 'testdata', '__fixtures__', 'examples', 'docs', 'tmp', 'storage',
  'bin', 'obj', 'public',
]);

export const STEPS = ['test', 'typecheck', 'lint'];

const FRAMEWORK_PACKS = {
  laravel: 'references/frameworks/laravel.md',
  symfony: 'references/frameworks/symfony.md',
  django: 'references/frameworks/django.md',
  fastapi: 'references/frameworks/fastapi.md',
  react: 'references/frameworks/react.md',
  next: 'references/frameworks/next.md',
  vue: 'references/frameworks/vue.md',
};

const LANGUAGE_PACKS = {
  php: ['references/languages/php.md'],
  javascript: ['references/languages/javascript.md'],
  typescript: ['references/languages/javascript.md', 'references/languages/typescript.md'],
  python: ['references/languages/python.md'],
  go: ['references/languages/go.md'],
  rust: ['references/languages/rust.md'],
};

const LANGUAGE_ALIASES = { ts: 'typescript', js: 'javascript', py: 'python', golang: 'go', rs: 'rust' };

export function canonicalLanguage(language) {
  if (!language) return null;
  const lower = String(language).toLowerCase();
  return LANGUAGE_ALIASES[lower] || lower;
}

function readText(filePath) {
  try {
    return fs.readFileSync(filePath, 'utf8');
  } catch {
    return null;
  }
}

function readJson(filePath) {
  const text = readText(filePath);
  if (text === null) return null;
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
}

function exists(...segments) {
  return fs.existsSync(path.join(...segments));
}

/** "^8.2 || ^8.3" → "8.2", ">=3.11,<4" → "3.11", "1.22.1" → "1.22". */
export function parseMinVersion(constraint) {
  if (!constraint) return null;
  const match = String(constraint).match(/(\d+)(?:\.(\d+))?/);
  if (!match) return null;
  return match[2] !== undefined ? `${match[1]}.${match[2]}` : match[1];
}

/** Compare dotted versions numerically: returns true when version >= minimum. */
export function versionAtLeast(version, minimum) {
  if (!version) return false;
  const a = String(version).split('.').map(Number);
  const b = String(minimum).split('.').map(Number);
  for (let i = 0; i < Math.max(a.length, b.length); i++) {
    const left = a[i] || 0;
    const right = b[i] || 0;
    if (left !== right) return left > right;
  }
  return true;
}

// ---------------------------------------------------------------------------------------------
// Language detectors. Each returns a partial component or null when the folder is not that stack.
// ---------------------------------------------------------------------------------------------

function detectPhp(dir) {
  if (!exists(dir, 'composer.json')) return null;
  const composer = readJson(path.join(dir, 'composer.json')) || {};
  const require = composer.require || {};
  const deps = { ...require, ...(composer['require-dev'] || {}) };
  const scripts = composer.scripts || {};
  const has = (pkg) => Object.prototype.hasOwnProperty.call(deps, pkg);
  const hasBin = (name) => exists(dir, 'vendor', 'bin', name);
  const firstScript = (names) => names.find((n) => Object.prototype.hasOwnProperty.call(scripts, n));

  const frameworks = [];
  if (has('laravel/framework')) frameworks.push('laravel');
  if (has('symfony/framework-bundle')) frameworks.push('symfony');
  if (has('laravel-zero/framework')) frameworks.push('laravel-zero');
  if (has('symfony/console') && frameworks.length === 0) frameworks.push('symfony-console');

  let projectType = 'backend';
  if (frameworks.includes('laravel') || frameworks.includes('symfony')) projectType = 'backend';
  else if (composer.bin || frameworks.includes('laravel-zero') || frameworks.includes('symfony-console')) projectType = 'cli';
  else if (composer.type === 'library') projectType = 'library';

  const commands = { prepare: null, test: null, typecheck: null, lint: null };
  const scoped = { test: null, typecheck: null, lint: null };

  if (firstScript(['test'])) commands.test = 'composer test';
  else if (has('pestphp/pest') || hasBin('pest')) commands.test = 'vendor/bin/pest';
  else if (has('phpunit/phpunit') || hasBin('phpunit')) commands.test = 'vendor/bin/phpunit';

  const hasPhpstan =
    has('phpstan/phpstan') || has('larastan/larastan') || has('nunomaduro/larastan') || hasBin('phpstan');
  const hasNeon = ['phpstan.neon', 'phpstan.neon.dist', 'phpstan.dist.neon'].some((f) => exists(dir, f));
  const typecheckScript = firstScript(['phpstan', 'stan', 'analyse', 'analyze', 'psalm', 'static-analysis']);
  if (hasPhpstan) {
    const levelArg = hasNeon ? '' : ' --level=5';
    const srcDirs = Object.values(composer.autoload?.['psr-4'] || {})
      .flat()
      .map((d) => String(d).replace(/\/+$/, ''))
      .filter((d) => d && exists(dir, d));
    const pathArg = hasNeon ? '' : ` ${(srcDirs.length ? srcDirs : ['src']).join(' ')}`;
    commands.typecheck = `vendor/bin/phpstan analyse --no-progress${levelArg}${pathArg}`;
    scoped.typecheck = `vendor/bin/phpstan analyse --no-progress${levelArg} {files}`;
  } else if (has('vimeo/psalm') || hasBin('psalm')) {
    commands.typecheck = 'vendor/bin/psalm --no-progress';
    scoped.typecheck = 'vendor/bin/psalm --no-progress {files}';
  }
  if (typecheckScript) commands.typecheck = `composer ${typecheckScript}`;

  if (has('laravel/pint') || hasBin('pint')) {
    commands.lint = 'vendor/bin/pint --test';
    scoped.lint = 'vendor/bin/pint --test {files}';
  } else if (has('friendsofphp/php-cs-fixer') || hasBin('php-cs-fixer')) {
    commands.lint = 'vendor/bin/php-cs-fixer fix --dry-run --diff';
    scoped.lint = 'vendor/bin/php-cs-fixer fix --dry-run --diff --path-mode=intersection {files}';
  } else if (has('squizlabs/php_codesniffer') || hasBin('phpcs')) {
    commands.lint = 'vendor/bin/phpcs';
    scoped.lint = 'vendor/bin/phpcs {files}';
  }
  const lintScript = firstScript(['lint', 'cs', 'cs-check', 'pint', 'style']);
  if (lintScript) commands.lint = `composer ${lintScript}`;

  return {
    language: 'php',
    language_version: parseMinVersion(composer.config?.platform?.php || require.php),
    frameworks,
    project_type: projectType,
    commands,
    scoped_commands: scoped,
    extensions: ['.php'],
  };
}

function detectJs(dir, rootDir) {
  if (!exists(dir, 'package.json')) return null;
  const pkg = readJson(path.join(dir, 'package.json')) || {};
  const deps = { ...(pkg.dependencies || {}), ...(pkg.devDependencies || {}) };
  const scripts = pkg.scripts || {};
  const has = (name) => Object.prototype.hasOwnProperty.call(deps, name);

  const lockIn = (file) => exists(dir, file) || exists(rootDir, file);
  let pm = 'npm';
  const declared = String(pkg.packageManager || '').split('@')[0];
  if (['pnpm', 'yarn', 'bun', 'npm'].includes(declared)) pm = declared;
  else if (lockIn('pnpm-lock.yaml')) pm = 'pnpm';
  else if (lockIn('yarn.lock')) pm = 'yarn';
  else if (lockIn('bun.lockb') || lockIn('bun.lock')) pm = 'bun';

  const runScript = (name) => {
    if (name === 'test') return pm === 'bun' ? 'bun run test' : `${pm} test`;
    if (pm === 'yarn') return `yarn ${name}`;
    return `${pm} run ${name}`;
  };
  const execBin = (bin) => ({ npm: `npx ${bin}`, yarn: `yarn ${bin}`, pnpm: `pnpm exec ${bin}`, bun: `bunx ${bin}` })[pm];

  const isTs = exists(dir, 'tsconfig.json') || has('typescript');

  const frameworks = [];
  if (has('next')) frameworks.push('next', 'react');
  else if (has('react')) frameworks.push('react');
  if (has('nuxt')) frameworks.push('nuxt', 'vue');
  else if (has('vue')) frameworks.push('vue');
  if (has('svelte') || has('@sveltejs/kit')) frameworks.push('svelte');
  if (has('@angular/core')) frameworks.push('angular');
  const backendLibs = ['express', 'fastify', 'koa', 'hono', '@nestjs/core'].filter(has);
  frameworks.push(...backendLibs.map((b) => b.replace('@nestjs/core', 'nestjs')));
  const cliLibs = ['commander', 'yargs', '@oclif/core', 'oclif', 'cac', 'citty', 'clipanion', 'ink'].filter(has);
  if (cliLibs.length) frameworks.push('cli');

  const frontend = ['next', 'react', 'vue', 'nuxt', 'svelte', 'angular'].some((f) => frameworks.includes(f));
  let projectType = pkg.main || pkg.exports || pkg.module ? 'library' : 'unknown';
  if (frontend) projectType = 'frontend';
  else if (backendLibs.length) projectType = 'backend';
  else if (pkg.bin || cliLibs.length) projectType = 'cli';

  const commands = { prepare: null, test: null, typecheck: null, lint: null };
  const scoped = { test: null, typecheck: null, lint: null };
  const placeholderTest = /no test specified/i.test(scripts.test || '');

  if (scripts.test && !placeholderTest) commands.test = runScript('test');
  else if (has('vitest')) commands.test = execBin('vitest run');
  else if (has('jest')) commands.test = execBin('jest');
  if (has('vitest')) scoped.test = execBin('vitest related --run {files}');
  else if (has('jest')) scoped.test = execBin('jest --findRelatedTests --passWithNoTests {files}');

  const typecheckScript = ['typecheck', 'type-check', 'check-types', 'tsc', 'types'].find((s) => scripts[s]);
  if (typecheckScript) commands.typecheck = runScript(typecheckScript);
  else if (has('vue-tsc')) commands.typecheck = execBin('vue-tsc --noEmit');
  else if (isTs && exists(dir, 'tsconfig.json')) commands.typecheck = execBin('tsc --noEmit');

  if (scripts.lint) commands.lint = runScript('lint');
  else if (has('@biomejs/biome')) commands.lint = execBin('biome check .');
  else if (has('eslint')) commands.lint = execBin('eslint .');
  if (has('@biomejs/biome')) scoped.lint = execBin('biome check {files}');
  else if (has('eslint')) scoped.lint = execBin('eslint {files}');

  return {
    language: isTs ? 'typescript' : 'javascript',
    language_version: isTs ? parseMinVersion(deps.typescript) : parseMinVersion(pkg.engines?.node),
    frameworks,
    project_type: projectType,
    package_manager: pm,
    commands,
    scoped_commands: scoped,
    extensions: ['.js', '.jsx', '.ts', '.tsx', '.mjs', '.cjs', '.mts', '.cts', '.vue', '.svelte'],
  };
}

function detectPython(dir) {
  const manifests = ['pyproject.toml', 'requirements.txt', 'setup.py', 'setup.cfg', 'Pipfile'];
  if (!manifests.some((m) => exists(dir, m))) return null;

  let requirementFiles = [];
  try {
    requirementFiles = fs.readdirSync(dir).filter((f) => /^requirements.*\.(txt|in)$/i.test(f));
  } catch {
    requirementFiles = [];
  }
  const pyproject = readText(path.join(dir, 'pyproject.toml')) || '';
  const corpus = [pyproject, ...[...requirementFiles, 'setup.cfg', 'setup.py', 'Pipfile'].map((f) => readText(path.join(dir, f)) || '')]
    .join('\n')
    .toLowerCase();
  const hasDep = (name) =>
    new RegExp(`(^|[\\s"'\\[,=])${name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}([\\s"'\\]<>=~!;,\\[]|$)`, 'm').test(corpus);
  const hasSection = (name) => new RegExp(`^\\[${name.replace(/\./g, '\\.')}`, 'm').test(pyproject);

  let prefix = '';
  if (exists(dir, 'uv.lock') || hasSection('tool.uv')) prefix = 'uv run ';
  else if (exists(dir, 'poetry.lock') || hasSection('tool.poetry')) prefix = 'poetry run ';
  else if (exists(dir, 'pdm.lock') || hasSection('tool.pdm')) prefix = 'pdm run ';
  else if (exists(dir, 'Pipfile')) prefix = 'pipenv run ';
  const tool = (name, args = '') => `${prefix ? `${prefix}${name}` : `python -m ${name}`}${args ? ` ${args}` : ''}`;

  const frameworks = ['django', 'fastapi', 'flask', 'click', 'typer'].filter(hasDep);
  let projectType = 'library';
  if (['django', 'fastapi', 'flask'].some((f) => frameworks.includes(f))) projectType = 'backend';
  else if (frameworks.includes('click') || frameworks.includes('typer') || hasSection('project.scripts')) projectType = 'cli';
  if (frameworks.includes('click') || frameworks.includes('typer')) frameworks.push('cli');

  const commands = { prepare: null, test: null, typecheck: null, lint: null };
  const scoped = { test: null, typecheck: null, lint: null };

  const hasPytest =
    hasDep('pytest') || hasSection('tool.pytest') || exists(dir, 'pytest.ini') || exists(dir, 'conftest.py');
  if (hasPytest) commands.test = tool('pytest');
  else if (frameworks.includes('django') && exists(dir, 'manage.py')) commands.test = `${prefix}python manage.py test`;
  else if (exists(dir, 'tests')) commands.test = `${prefix}python -m unittest discover`;

  if (hasDep('mypy') || hasSection('tool.mypy') || exists(dir, 'mypy.ini')) {
    commands.typecheck = tool('mypy', '.');
    scoped.typecheck = tool('mypy', '{files}');
  } else if (hasDep('pyright') || hasSection('tool.pyright') || exists(dir, 'pyrightconfig.json')) {
    commands.typecheck = tool('pyright');
    scoped.typecheck = tool('pyright', '{files}');
  }

  if (hasDep('ruff') || hasSection('tool.ruff') || exists(dir, 'ruff.toml') || exists(dir, '.ruff.toml')) {
    commands.lint = tool('ruff', 'check .');
    scoped.lint = tool('ruff', 'check {files}');
  } else if (hasDep('flake8') || exists(dir, '.flake8')) {
    commands.lint = tool('flake8', '.');
    scoped.lint = tool('flake8', '{files}');
  }

  const requiresPython = pyproject.match(/requires-python\s*=\s*["']([^"']+)["']/i);
  const pinned = readText(path.join(dir, '.python-version'));

  return {
    language: 'python',
    language_version: parseMinVersion(requiresPython?.[1] || pinned),
    frameworks,
    project_type: projectType,
    commands,
    scoped_commands: scoped,
    extensions: ['.py', '.pyi'],
  };
}

function detectGo(dir) {
  if (!exists(dir, 'go.mod')) return null;
  const gomod = readText(path.join(dir, 'go.mod')) || '';
  const has = (mod) => gomod.includes(mod);

  const frameworks = [];
  if (has('github.com/spf13/cobra')) frameworks.push('cobra', 'cli');
  if (has('github.com/urfave/cli')) frameworks.push('urfave-cli', 'cli');
  const web = [
    ['github.com/gin-gonic/gin', 'gin'],
    ['github.com/labstack/echo', 'echo'],
    ['github.com/go-chi/chi', 'chi'],
    ['github.com/gofiber/fiber', 'fiber'],
    ['google.golang.org/grpc', 'grpc'],
  ].filter(([mod]) => has(mod));
  frameworks.push(...web.map(([, name]) => name));

  const hasMain = exists(dir, 'main.go') || exists(dir, 'cmd');
  let projectType = 'library';
  if (web.length) projectType = 'backend';
  else if (frameworks.includes('cli') || hasMain) projectType = 'cli';

  const hasGolangci = ['.golangci.yml', '.golangci.yaml', '.golangci.toml', '.golangci.json'].some((f) => exists(dir, f));

  return {
    language: 'go',
    language_version: parseMinVersion(gomod.match(/^go\s+([\d.]+)/m)?.[1]),
    frameworks: [...new Set(frameworks)],
    project_type: projectType,
    commands: {
      prepare: null,
      test: 'go test ./...',
      typecheck: 'go vet ./...',
      lint: hasGolangci ? 'golangci-lint run' : null,
    },
    scoped_commands: {
      test: null,
      typecheck: null,
      lint: hasGolangci ? 'golangci-lint run --new-from-rev={base_sha}' : null,
    },
    extensions: ['.go'],
  };
}

function detectRust(dir) {
  if (!exists(dir, 'Cargo.toml')) return null;
  const cargo = readText(path.join(dir, 'Cargo.toml')) || '';
  const hasCrate = (name) => new RegExp(`^${name}\\s*=`, 'm').test(cargo);
  const frameworks = [];
  if (hasCrate('clap')) frameworks.push('clap', 'cli');
  const web = ['axum', 'actix-web', 'rocket', 'warp'].filter(hasCrate);
  frameworks.push(...web);

  let projectType = 'library';
  if (web.length) projectType = 'backend';
  else if (frameworks.includes('cli') || exists(dir, 'src', 'main.rs') || /^\[\[bin\]\]/m.test(cargo)) projectType = 'cli';

  return {
    language: 'rust',
    language_version: parseMinVersion(cargo.match(/^rust-version\s*=\s*"([^"]+)"/m)?.[1]),
    edition: cargo.match(/^edition\s*=\s*"(\d+)"/m)?.[1] || null,
    frameworks,
    project_type: projectType,
    commands: {
      prepare: null,
      test: 'cargo test',
      typecheck: 'cargo check --all-targets',
      lint: 'cargo clippy --all-targets -- -D warnings',
    },
    scoped_commands: { test: null, typecheck: null, lint: null },
    extensions: ['.rs'],
  };
}

const DETECTORS = [detectPhp, detectPython, detectGo, detectRust, detectJs];

// ---------------------------------------------------------------------------------------------
// Component discovery
// ---------------------------------------------------------------------------------------------

/** Root first, then sub-folders breadth-first up to maxDepth, skipping dependency/build/hidden dirs. */
function candidateDirs(rootDir, maxDepth) {
  const result = ['.'];
  let frontier = ['.'];
  for (let depth = 1; depth <= maxDepth; depth++) {
    const next = [];
    for (const rel of frontier) {
      let entries = [];
      try {
        entries = fs.readdirSync(path.join(rootDir, rel), { withFileTypes: true });
      } catch {
        continue;
      }
      for (const entry of entries) {
        if (!entry.isDirectory() || entry.name.startsWith('.') || IGNORED_DIRS.has(entry.name)) continue;
        const childRel = rel === '.' ? entry.name : `${rel}/${entry.name}`;
        next.push(childRel);
      }
    }
    next.sort();
    result.push(...next);
    frontier = next;
  }
  return result;
}

function isAncestorPath(ancestor, child) {
  if (ancestor === '.') return child !== '.';
  return child.startsWith(`${ancestor}/`);
}

/** Skill-relative pack files for a component, most general first. */
export function packsFor(component) {
  const packs = [...(LANGUAGE_PACKS[canonicalLanguage(component.language)] || [])];
  for (const framework of component.frameworks || []) {
    if (FRAMEWORK_PACKS[framework] && !packs.includes(FRAMEWORK_PACKS[framework])) packs.push(FRAMEWORK_PACKS[framework]);
  }
  if (component.project_type === 'cli') packs.push('references/frameworks/cli.md');
  return packs;
}

/**
 * Find every component in the repo. A nested manifest of a language already claimed by an ancestor
 * folder is treated as a workspace member of that ancestor (npm/pnpm workspaces, Go multi-module).
 * A JS manifest sharing a folder with another language (e.g. Laravel + Vite) only counts when it is
 * a real frontend: it uses a framework, has a test command, or has a tsconfig.
 */
export function detectComponents(rootDir, { maxDepth = 2 } = {}) {
  const found = [];
  for (const rel of candidateDirs(rootDir, maxDepth)) {
    const abs = path.join(rootDir, rel);
    const here = [];
    for (const detector of DETECTORS) {
      const component = detector(abs, rootDir);
      if (!component) continue;
      const family = component.language === 'typescript' ? 'javascript' : component.language;
      const claimed = found.some(
        (c) => (c.language === 'typescript' ? 'javascript' : c.language) === family && isAncestorPath(c.path, rel)
      );
      if (claimed) continue;
      here.push({ ...component, path: rel });
    }
    const nonJs = here.filter((c) => c.language !== 'javascript' && c.language !== 'typescript');
    for (const component of here) {
      const isJs = component.language === 'javascript' || component.language === 'typescript';
      if (isJs && nonJs.length > 0) {
        const realFrontend =
          component.frameworks.length > 0 || component.commands.test || exists(abs, 'tsconfig.json');
        if (!realFrontend) continue;
      }
      found.push(component);
    }
  }
  return assignNames(found).map((c) => ({ ...c, packs: packsFor(c) }));
}

function assignNames(components) {
  const familyName = (c) => (c.language === 'typescript' || c.language === 'javascript' ? 'js' : c.language);
  const used = new Set();
  return components.map((c) => {
    let base;
    if (c.path === '.') {
      const sharesRoot = components.filter((o) => o.path === '.').length > 1;
      base = sharesRoot && familyName(c) === 'js' ? 'frontend' : familyName(c);
    } else {
      base = path.posix.basename(c.path);
    }
    let name = base;
    if (used.has(name)) name = `${base}-${familyName(c)}`;
    let n = 2;
    while (used.has(name)) name = `${base}-${n++}`;
    used.add(name);
    return { name, ...c };
  });
}

const SOURCE_EXT = /\.(php|py|go|rs|js|jsx|ts|tsx|mjs|cjs|vue|svelte)$/i;

/**
 * Greenfield repos get the full craftsmanship rules; brownfield repos (an established codebase)
 * follow the existing local style first and apply the packs to new code only.
 */
export function detectStrictness(rootDir) {
  try {
    const listed = execFileSync('git', ['ls-files'], { cwd: rootDir, encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] });
    const sourceFiles = listed.split(/\r?\n/).filter((f) => SOURCE_EXT.test(f) && !f.startsWith('.agents/') && !f.startsWith('.claude/'));
    return sourceFiles.length > 30 ? 'brownfield' : 'greenfield';
  } catch {
    return 'greenfield';
  }
}
