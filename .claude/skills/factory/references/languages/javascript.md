# JavaScript Craftsmanship & Idioms

Language pack for every JS/TS component (`package.json`). TypeScript components also load `typescript.md`. Frontend overlays (React, Next.js, Vue) and the repo's ESLint/Biome/Prettier config override this pack. These rules apply to plain JavaScript too: don't add TypeScript, a type checker or build steps a JS project doesn't have.

## Modules and dependencies

- Use ES modules (`import`/`export`) unless the package is CommonJS (`"type"` in `package.json`, `.cjs` files). Follow whichever the package uses.
- Never create stateful collaborators (DB clients, HTTP clients, clocks, random sources) as module-level singletons imported into domain logic. Inject them through function parameters or constructor options (ports & adapters), so seam tests pass in-memory fakes without `vi.mock`/`jest.mock` module patching.
- Prefer small pure functions and closures, or flat classes composed through narrow interfaces, over `extends` hierarchies.
- No `return this` chaining on business services; return results.

## Immutability and data

- Treat DTOs, events and config as immutable: don't mutate inputs, and return new objects (`{ ...order, status: 'paid' }`, `toSorted`, `with`).
- Use `Object.freeze` for exported constant tables, and `#private` fields for class state with invariants.
- No Java-style `getFoo()`/`setFoo()`. Use plain properties, or native `get`/`set` accessors when validation or computation is needed.

## Control flow

- Replace magic strings and numbers with frozen constant maps (`const OrderStatus = Object.freeze({ Paid: 'paid', ... })`) or string-literal unions in TS.
- Use guard clauses for invalid input, `null`/`undefined` and early exits, so the happy path stays flat.
- Use `async`/`await` with `try`/`catch` at boundaries. Never leave a promise floating: `await` it, return it, or deliberately `void` it with a reason.
- Throw `Error` subclasses with domain meaning, never strings.

## Boundaries

- Treat everything from the network, `JSON.parse`, `process.argv`, env vars or storage as untrusted. Validate at the boundary with the project's schema library (zod, valibot, yup, ajv) or explicit checks, then pass validated values inward.

## Tests

- Use the project's runner (Vitest, Jest, `node:test`, Mocha). Test through the seam in `seams.md`: exported functions, HTTP handlers, rendered components, or the CLI's argv → stdout/exit code.
- Never use `eslint-disable` or `@ts-ignore` to get green.
