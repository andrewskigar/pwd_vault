# Go Craftsmanship & Idioms

Language pack for a Go component (`go.mod`). The minimum version is `language_version` (the `go` directive). **Use only features your version supports.** Idiomatic Go and the repo's conventions override generic OO advice in the universal directives: Go has no classes, so "final by default" and getters/setters translate to the rules below.

## Baseline (Go 1.21+)

**Types and static analysis**
- No `any`/`interface{}` in domain models, DTOs or service contracts. Use concrete structs, small interfaces, or generics with tight constraints.
- Define distinct domain types (`type OrderID string`, `type Cents int64`) instead of passing raw primitives across package boundaries.
- Code must pass `go vet ./...` and the configured `golangci-lint` without `//nolint`.

**Encapsulation and immutability**
- Pass small DTOs, events and value objects by value, and copy slices/maps at boundaries so callers can't mutate shared state.
- For types with invariants, keep fields unexported, validate in `NewX(...) (X, error)`, and expose read access as `x.Email()` (no `Get` prefix).
- Don't chain through nested structs (`order.Customer().Billing().Card()`). Add a method on the owner.

**Composition and dependencies**
- Accept interfaces, return structs. Define small (1–3 method) interfaces in the **consuming** package.
- Use composition through struct fields. Don't embed structs just to borrow methods if that leaks them into the public API.
- No package-level mutable state, global singletons, or side-effecting `init()` for dependencies. Wire everything explicitly in `main` or constructors (`NewCheckoutService(repo, clock)`).

**Control flow and errors**
- Use typed constants for discrete states, validate them at boundaries, and avoid `fallthrough`.
- Handle errors immediately with guard clauses, and wrap them with context: `fmt.Errorf("charge order %s: %w", id, err)`. Callers use `errors.Is`/`errors.As`. Keep the happy path unindented.
- Never `panic` in domain or application code.
- Pass `context.Context` as the first parameter to anything that does I/O.

## Go 1.22+

- Loop variables are per-iteration, so drop the `v := v` copies.
- Range over integers (`for i := range n`).
- `net/http` patterns with methods and wildcards (`mux.HandleFunc("GET /orders/{id}", ...)`).

## Go 1.23+

- Range-over-func iterators (`iter.Seq[T]`) for custom collections where they simplify callers.

## Tests

- Table-driven tests with `t.Run`, plus `httptest` for handlers.
- Use fakes that satisfy consumer-defined interfaces instead of mocking frameworks, unless the repo already uses one.
