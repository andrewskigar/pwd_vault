# Next.js Overlay

Loaded with `react.md` when the component depends on `next`. Check which router the app uses (`app/` vs `pages/`) and stay consistent with it.

## App Router

- Components are **server components by default**. Add `'use client'` only to the smallest interactive leaf that needs state, effects or browser APIs, and don't pass non-serialisable props across the boundary.
- Fetch data in server components or route handlers, not in client effects. Declare caching and revalidation explicitly (`fetch` cache options, `revalidatePath`/`revalidateTag`) and follow the conventions of the app's Next version.
- **Server Actions** and **route handlers** are backend code. Validate input (zod or similar), check authentication/authorisation inside the action, and return typed results. Never trust client-provided IDs without checking ownership.
- Server-only modules (DB clients, secrets) import `server-only`. Only `NEXT_PUBLIC_` variables reach the browser, and must never hold secrets.
- Use `loading.tsx`, `error.tsx` and `not-found.tsx` for states, and `next/image`/`next/link` for media and navigation.

## Pages Router

- Data loading in `getServerSideProps`/`getStaticProps`, API routes in `pages/api`. Apply the same validation and auth rules.

## Tests

- Unit/component tests with Testing Library (see `react.md`). Test server actions and route handlers as functions with injected dependencies. Use Playwright for full flows if configured.
