# React Overlay

Loaded on top of `javascript.md` (and `typescript.md`) when the component depends on `react`. The class-oriented universal directives translate as follows: components are **functions**, composition happens through props/children/hooks, and "dependency injection" means props or context.

## Components

- Use function components and hooks only. Name components in PascalCase, one exported component per file unless the repo co-locates small helpers.
- Props are read-only. Type them with an explicit `Props` type in TS. Avoid boolean props that switch a component between two different UIs; make two components or use a `variant` union.
- Keep components presentational where possible. Move data fetching and side effects into custom hooks (`useOrder(id)`) or the app's data layer (TanStack Query, SWR, Redux Toolkit, server components).
- Derive values during render instead of syncing them into state with `useEffect`. Use effects only to synchronise with external systems, and always clean up subscriptions and timers.
- Stable `key`s from data IDs, never array indexes for dynamic lists.
- Memoise (`useMemo`, `useCallback`, `memo`) only for measured problems or referential-identity requirements, unless the repo uses the React Compiler.

## Accessibility and safety

- Use semantic elements (`button`, `label` + `htmlFor`, headings in order). Interactive elements must be keyboard reachable. Images have `alt`.
- Never use `dangerouslySetInnerHTML` with untrusted content. Sanitise URLs before putting them in `href`/`src`.

## Tests

- **React Testing Library** with the repo's runner. Query by role/label/text (`getByRole('button', { name: /pay/i })`) and simulate with `userEvent`. Assert what the user sees, never internal state or implementation details.
- Mock network at the boundary (MSW or an injected client), not individual hooks.
- Use Playwright/Cypress E2E for multi-page flows if the repo has it.
