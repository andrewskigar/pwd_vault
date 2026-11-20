# TypeScript Additions

Loaded after `javascript.md` for TypeScript components (`tsconfig.json` or a `typescript` dependency). `language_version` is the TypeScript version. Respect the project's `tsconfig.json` and never weaken compiler options (`strict`, `noUncheckedIndexedAccess`, …) to get green.

## Types

- No `any`. Use `unknown` at untrusted boundaries and narrow with type predicates (`value is Order`) or schema parsers (`z.infer<typeof Order>`).
- Give exported functions, public methods and module interfaces explicit parameter and return types.
- Model states and fallible results as **discriminated unions** instead of optional fields that allow invalid combinations:

```typescript
type CheckoutResult =
  | { readonly status: 'paid'; readonly receiptId: string }
  | { readonly status: 'declined'; readonly reason: DeclineReason };
```

- Use string-literal unions or `as const` maps instead of `enum` (unless the repo uses enums), and branch exhaustively:

```typescript
function assertNever(value: never): never {
  throw new Error(`Unhandled case: ${JSON.stringify(value)}`);
}
```

- Use `readonly` on DTO/event properties and `ReadonlyArray<T>`/`readonly T[]` for collections.
- Use branded types for IDs that must not be mixed up (`type OrderId = string & { readonly __brand: 'OrderId' }`) when the domain has several look-alike IDs.
- Never use `@ts-ignore`, `as any`, or non-null `!` to silence the compiler. `@ts-expect-error` is acceptable only in negative type tests.

## Version-gated features

- **TS 4.9+:** `satisfies` for config objects that must match a type without widening.
- **TS 5.0+:** `const` type parameters, and standard decorators where the framework expects them.
- **TS 5.2+:** `using` declarations for disposable resources, if the runtime target supports them.
