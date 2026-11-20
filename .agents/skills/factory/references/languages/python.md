# Python Craftsmanship & Idioms

Language pack for a Python component (`pyproject.toml`, `requirements*.txt`, `setup.py`, `Pipfile`). The minimum version is `language_version` in `.factory/config.json` (from `requires-python` or `.python-version`). **Use only the syntax your version supports.** Framework overlays (Django, FastAPI) and the repo's conventions override this pack.

## Baseline (Python 3.9+)

**Types and static analysis**
- Annotate every new function, method and class attribute. Use built-in generics (`list[Order]`, `dict[str, int]`) and `from __future__ import annotations` where needed.
- No `typing.Any`, bare `dict`/`list`, or untyped `**kwargs` in domain code. Model structured data as dataclasses, `TypedDict`, or the project's model library (pydantic, attrs).
- Use `typing.Protocol` for narrow behavioural contracts at seams.
- New code must pass the configured checker (`mypy`, `pyright`) and linter (`ruff`, `flake8`) at their configured strictness. Never add `# type: ignore` or `# noqa` to force green.

**Immutability and encapsulation**
- Use `@dataclass(frozen=True)` for DTOs, value objects, commands and events (`slots=True, kw_only=True` on 3.10+).
- No `get_x()`/`set_x()` pairs. Use plain attributes, `@property` for computed or validated state, and domain-named methods for behaviour.
- No mutable default arguments. Use `None` or `field(default_factory=list)`.

**Composition and dependencies**
- Compose through injected `Protocol` collaborators rather than inheritance hierarchies.
- No module-level mutable singletons, no `__new__` singleton tricks, and no global clients imported into domain code. Pass dependencies through `__init__` or function parameters.
- Make parameters keyword-only (`*`) when there are more than two. No boolean flag parameters that switch behaviour.

**Control flow**
- Use `enum.Enum` (or `StrEnum` on 3.11+) instead of magic strings.
- Use guard clauses with early `return`/`raise`, and raise domain-specific exceptions.

```python
from __future__ import annotations

from dataclasses import dataclass
from decimal import Decimal


@dataclass(frozen=True)
class OrderPlaced:
    order_id: str
    customer_id: str
    total: Decimal
```

## Python 3.10+

- `X | None` union syntax, `@dataclass(slots=True, kw_only=True)`.
- `match`/`case` for branching over enums and unions, ending in `case _: assert_never(value)` (`typing_extensions.assert_never` before 3.11) so a missing case fails type checking.

## Python 3.11+

- `StrEnum`, `typing.Self`, `typing.assert_never`, and exception groups where they make sense.
- `@typing.final` on concrete service classes:

```python
from typing import Final, final


@final
class CustomerAccount:
    def __init__(self, *, name: str, email: str) -> None:
        self.name: Final[str] = name
        self.email = email

    @property
    def email(self) -> str:
        return self._email

    @email.setter
    def email(self, value: str) -> None:
        normalized = value.strip().lower()
        if "@" not in normalized:
            raise InvalidEmail(value)
        self._email = normalized
```

## Python 3.12+

- PEP 695 generics and aliases: `class Repo[T]: ...`, `type OrderId = str`.
- `@typing.override` on overriding methods.

## Formatting

- Follow the repo's formatter (`ruff format`, `black`, isort settings in `pyproject.toml`). Without one, use PEP 8 with an 88-character line length.
