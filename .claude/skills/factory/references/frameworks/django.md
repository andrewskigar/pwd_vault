# Django Overlay

Loaded on top of `python.md` when the component depends on `django`. Follow the project's app layout and existing patterns (services or selectors modules, class-based or function views).

## Where the generic rules bend

- **Models** subclass `models.Model` and are mutable. Put invariants in model methods or a service layer, not scattered across views. Use `TextChoices`/`IntegerChoices` for state fields.
- **Views, forms, serializers, admin** subclass framework bases. That is expected, not an inheritance smell.
- The ORM's chained querysets are a framework API. Keep complex queries in model managers/querysets or selector functions so views stay thin.
- Settings are global by design. Read them at the edges and pass values into domain functions instead of importing `settings` deep inside.

## Boundaries

- Validate input with forms or DRF serializers. Never use `fields = '__all__'` on input forms or serializers that accept user data.
- Check permissions explicitly (`permission_classes`, `@login_required`, object-level checks), and scope querysets to the user to avoid insecure direct object references.
- Wrap multi-step writes in `transaction.atomic()`. Migrations come from `makemigrations`, are committed, and must not drop data without a data migration.

## Tests

- `pytest-django` (`@pytest.mark.django_db`) or `django.test.TestCase`, whichever the repo uses.
- Test at the HTTP seam with the test `Client`/`APIClient`: assert status codes, response bodies and resulting database state. Use model factories (factory_boy) if present.
- Mock only external services (HTTP, payment, email via `mail.outbox`), never the ORM.
