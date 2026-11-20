# Symfony Overlay

Loaded on top of `php.md` when the component requires `symfony/framework-bundle`. Follow the app's existing bundle and directory layout.

## Where the generic rules bend

- **Doctrine entities** are mutable by design and can't be `final` when lazy-loading proxies are used (check `doctrine.orm` config). Keep invariants in entity methods instead of setters where the codebase allows. Use `readonly` DTOs for input and output.
- **Services** are autowired through constructor injection. Make them `final`, and type collaborators against interfaces you can alias in `services.yaml`. Never fetch from the container (`$container->get`) in application code.
- **Controllers** may extend `AbstractController` if the app does. Thin controllers delegate to services or handlers.
- Configure with attributes (`#[Route]`, `#[AsCommand]`, `#[AsMessageHandler]`, `#[Autowire]`), one attribute per line.

## Boundaries

- Map and validate input with `#[MapRequestPayload]`/`#[MapQueryString]` DTOs and Validator constraints. Authorise with voters (`#[IsGranted]`, `denyAccessUnlessGranted`).
- Use Messenger for async work. Handlers are `final` and invokable.
- Doctrine migrations go through `doctrine:migrations:diff` and must not destroy data silently.

## Tests

- `WebTestCase`/`KernelBrowser` for the HTTP seam, `KernelTestCase` for services wired by the container, and plain PHPUnit for pure domain code.
- Use the test environment's in-memory transports (`messenger.transport.async: 'in-memory://'`) and assert dispatched messages.
- Console commands are tested with `CommandTester` (see `cli.md`).
