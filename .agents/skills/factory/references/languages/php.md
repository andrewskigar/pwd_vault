# PHP Craftsmanship & Idioms

Language pack for `slice-builder`, `standards-reviewer` and `spec-architect` on a PHP component (`composer.json`). The component's minimum PHP version is `language_version` in `.factory/config.json` (from `require.php` or `config.platform.php`). **Use only the sections your version supports.** Writing 8.4 syntax for an 8.2 runtime is a bug. Framework overlays (Laravel, Symfony) and the repo's own conventions override this pack.

## Baseline (PHP 8.1+)

**Types and static analysis**
- Put `declare(strict_types=1);` at the top of every new file, unless the repo deliberately omits it everywhere (precedence level 2).
- Declare native parameter, property and return types everywhere, including union types (`int|string`) and `never`/`void`/`static`.
- Don't use `mixed` in domain code. Model dynamic payloads as typed DTOs, and type collections with PHPDoc generics (`@param list<OrderLine> $lines`, `@return array<non-empty-string, int>`, `@template T`).
- New code must pass PHPStan/Psalm **at the level configured in `phpstan.neon` / `psalm.xml`**. Never lower the level, never add baseline entries for new code, and never use `@phpstan-ignore*` or `@psalm-suppress`. In greenfield, aim for the maximum level.

**Immutability and encapsulation**
- Use `readonly` promoted properties for DTOs, value objects, commands, queries and events (8.1). Validate in the constructor.
- No generic `getX()`/`setX()` pairs. Expose `public readonly` state, or behaviour methods named for the domain (`$order->markPaid($at)`).

**Composition and dependencies**
- Make concrete classes `final` by default (the framework may say otherwise, see the overlay). Compose through narrow interfaces instead of `extends` hierarchies.
- Inject dependencies through `__construct` (PSR-11 containers wire them). No singletons, static service locators or `new` of collaborators inside domain logic.
- No `return $this` on domain entities or services.

**Control flow**
- Use backed enums (`enum PaymentStatus: string`) instead of class-constant lists and magic strings, and put transition logic on the enum where it is cohesive.
- Use `match` instead of `switch` for value/state branching (strict comparison, no fall-through, `UnhandledMatchError` on gaps).
- Use guard clauses and early returns, and throw domain-specific exceptions.

```php
<?php

declare(strict_types=1);

namespace App\Domain\Order;

final class OrderPlaced
{
    /** @param list<OrderLine> $lines */
    public function __construct(
        public readonly OrderId $orderId,
        public readonly array $lines,
        public readonly Money $total,
    ) {}
}
```

## PHP 8.2+

- Declare DTOs and events as `final readonly class` instead of marking each property.
- DNF types (`(Countable&Iterator)|null`) and standalone `true`/`false`/`null` types are allowed.

## PHP 8.3+

- Type class constants (`const string PREFIX = 'ord_';`).
- Mark overridden interface/parent methods with `#[\Override]`.

## PHP 8.4+

- **Asymmetric visibility** for state that is read publicly but only mutated internally: `public private(set) string $email`.
- **Property hooks** for validation or normalisation on assignment, and for computed properties. Prefer them over setter methods:

```php
final class CustomerAccount
{
    public private(set) string $email {
        set(string $value) {
            $normalized = strtolower(trim($value));
            if (!filter_var($normalized, FILTER_VALIDATE_EMAIL)) {
                throw new InvalidEmail($value);
            }
            $this->email = $normalized;
        }
    }

    public function __construct(public readonly string $name, string $email)
    {
        $this->email = $email;
    }
}
```

- `new Foo()->bar()` without wrapping parentheses is allowed.

## PHP 8.5+

- The pipe operator `|>` with first-class callables is fine for linear, side-effect-free transformations.

## Formatting

- Follow the repo's formatter config (Pint preset, `.php-cs-fixer.php`, `phpcs.xml`). Without one, use PER-CS:
  - 4-space indentation.
  - Braces for classes and methods on the next line; an empty promoted-constructor body is `{}` on the same line.
  - Trailing commas in multi-line lists.
  - One attribute per line.
  - Modifier order `final`/`abstract` → visibility → `private(set)` → `static` → `readonly` → type.
