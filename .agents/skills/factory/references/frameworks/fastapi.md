# FastAPI Overlay

Loaded on top of `python.md` when the component depends on `fastapi`.

## Idioms

- **Pydantic models** are the boundary DTOs. Use separate request/response models (never return ORM objects directly), `model_config = ConfigDict(frozen=True)` for value-like models, and `Field` constraints for validation.
- **Dependency injection** goes through `Depends(...)`: DB sessions, repositories, clocks and current-user resolution are dependencies. That is the framework's DI, so don't add a global container. Override dependencies in tests with `app.dependency_overrides`.
- Keep route functions thin: parse, call a service function with typed arguments, map the result to a response model. Business logic must not import `fastapi`.
- Use `async def` routes only when the whole call chain is async. Don't call blocking I/O inside them; use `def` routes or run the blocking work in a threadpool.
- Declare status codes and error responses explicitly (`status_code=201`, `HTTPException` or custom exception handlers mapping domain errors).

## Security

- Use `Security`/`Depends` for authentication on every new router or endpoint, with scopes where the app uses them. Validate path and query parameters with types and constraints.

## Tests

- Test at the HTTP seam with `TestClient` (or `httpx.AsyncClient` with `ASGITransport` for async apps). Assert status, JSON body and side effects.
- Swap real dependencies via `dependency_overrides` with in-memory fakes, and reset them after each test.
