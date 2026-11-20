# Seam Catalogue by Project Type

A **seam** is the public boundary where a slice's behaviour is observed and tested without coupling to internals. Pick the highest practical seam for the component's `project_type` (in `.factory/config.json`). Prefer seams the repo already tests at, and mirror its test prior art.

## backend (HTTP/RPC services: Laravel, Symfony, Django, FastAPI, Express/Nest, Go net/http/gin/echo)

| Seam | Use for | Test with |
| :--- | :--- | :--- |
| **HTTP endpoint / handler** (default) | Request validation, authz, status codes, response shape, persisted effects | Framework test client: Laravel feature tests, Symfony `WebTestCase`, Django/DRF `Client`, FastAPI `TestClient`, `supertest`, Go `httptest` |
| **Application service / use case** | Business rules with several outcomes, behind a thin controller | Direct call with in-memory fakes for repositories, clock, gateways |
| **Repository / adapter** | Query correctness, mapping | Real database in the test environment (transactions/refresh), never mocked |
| **Message/queue handler** | Async jobs, events | Invoke the handler with a message, and assert effects and dispatched messages using the framework's fakes or in-memory transport |

Assert observable outcomes: response status and body, database rows, dispatched jobs/mail/events. Mock only true externals (payment APIs, third-party HTTP).

## frontend (React, Next.js, Vue/Nuxt, Svelte, Angular)

| Seam | Use for | Test with |
| :--- | :--- | :--- |
| **Rendered component** (default) | UI behaviour: what the user sees and can do | Testing Library (`@testing-library/react`/`vue`), `userEvent`, role/label queries |
| **Custom hook / composable / store** | Stateful logic reused across components | Hook/composable test utilities, store unit tests |
| **Pure module** | Formatting, validation, reducers | Plain unit tests |
| **User flow (E2E)** | Multi-page journeys, auth, routing | Playwright/Cypress, only if the repo has it configured |

Mock the network at the boundary (MSW or an injected API client). Never assert on component internals, CSS class names (unless that is the contract), or snapshot whole trees as the only assertion.

## cli (Cobra, Click/Typer, Symfony Console, Commander/oclif, clap)

| Seam | Use for | Test with |
| :--- | :--- | :--- |
| **Command entry point** (default) | argv → stdout/stderr/exit code/files | In-process run with captured streams: `CliRunner`, `CommandTester`, `bytes.Buffer`, `assert_cmd` |
| **Golden output** | Larger or formatted output | `testdata/*.golden` files with an explicit update flag |
| **Core function** | Logic behind the command | Direct call with injected filesystem/clock |

See `frameworks/cli.md` for design rules.

## library (packages consumed by other code)

| Seam | Use for | Test with |
| :--- | :--- | :--- |
| **Public API** (default) | Everything exported, exactly as consumers call it | Unit tests that import only the public entry point |
| **Type-level contract** (TS) | Generic signatures and inference | Type tests (`expectTypeOf`, `tsd`, `@ts-expect-error` negatives) |

Never test non-exported internals. If something is hard to test through the public API, the API probably needs a better seam; raise it in the spec.

## unknown

Pick the closest of the above from how the code is used, and say which you chose in `02-spec.md` under Testing Decisions.
