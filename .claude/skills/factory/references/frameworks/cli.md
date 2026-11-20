# CLI Overlay

Loaded for components whose `project_type` is `cli` (Cobra/urfave-cli, Click/Typer, Symfony Console/Laravel Zero, Commander/yargs/oclif, clap). A CLI's contract is **argv + stdin + env → stdout + stderr + exit code + files written**. Design and test against that contract.

## Design

- Keep `main`/the command entry point thin: parse arguments, build dependencies, call a function that takes typed options plus injected I/O (writers for stdout/stderr, a filesystem or root dir, a clock, env lookup), and map its result to an exit code.
  - Go: `func run(args []string, stdout, stderr io.Writer) int`.
  - Python: `def main(argv: list[str] | None = None) -> int`.
  - Node: `export async function main(argv, { stdout, stderr })`.
- **stdout carries results** (pipeable, stable format; add `--json` if machines consume it). **stderr carries diagnostics and progress.** Never mix them.
- Use deliberate exit codes: `0` success, `1` failure, `2` usage error. Document any others. Never `exit()` deep inside library code; return errors upward.
- Validate arguments early with helpful messages, and show usage on errors. Support `--help`, and `--version` if the tool is distributed.
- Be safe by default: confirm or require `--force` for destructive actions, write files atomically (temp + rename), don't follow symlinks blindly, and pass arguments to subprocesses as arrays rather than shell strings.
- Respect `NO_COLOR` and non-TTY output (no colours or spinners when piped).

## Tests (the seam)

- Call the entry function in-process with argv and captured streams, then assert stdout, stderr and the exit code:
  - Go: `bytes.Buffer` writers.
  - Python: Click's `CliRunner`, Typer's `CliRunner`, or `capsys`.
  - PHP: Symfony `CommandTester` / Laravel `$this->artisan(...)->expectsOutput()->assertExitCode(0)`.
  - Node: inject writable streams or use `execa` against the built binary.
  - Rust: `assert_cmd`.
- Use **golden files** for larger outputs (`testdata/*.golden`, updated with an explicit flag such as `-update`), and normalise paths, timestamps and colours first.
- Use a temp directory per test for file effects, and assert the files produced.
