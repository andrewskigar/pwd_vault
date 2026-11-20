### Which rules win (precedence)

When two sources of guidance disagree, the higher one wins. This order is the same for every factory agent:

1. **Repository documentation**: `AGENTS.md`, `CLAUDE.md`, `CONTRIBUTING.md`, `CODING_STANDARDS.md`, ADRs in `docs/adr/`, and linter/formatter configs.
2. **Existing code style around the change**: the conventions of the files and modules you touch. In a `brownfield` run (`strictness` in `state.json`), match the surrounding code even where a pack would do it differently, and never refactor code outside the ticket to meet a pack.
3. **Framework overlay** (`references/frameworks/*.md`): framework idioms beat generic rules. For example, Eloquent models extend `Model`, and React components are functions, not `final` classes.
4. **Language pack** (`references/languages/*.md`): apply only the sections whose minimum version the component meets (`language_version` in `.factory/config.json`).
5. **Universal craftsmanship directives** (below or in your instructions).

The orchestrator passes you `packs`: absolute paths to `precedence.md`, the language and framework packs for your component(s), and `seams.md`. Read them before writing or judging code. If a pack rule conflicts with a higher level, follow the higher level and say so in your report.
