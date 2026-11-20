# Rust Craftsmanship & Idioms

Language pack for a Rust component (`Cargo.toml`). `language_version` is `rust-version` when declared. Use only features your toolchain and edition support; the repo's conventions and clippy config override this pack.

## 1. Strict Type Safety & Static Analysis (`cargo check`, `cargo clippy -- -D warnings`)

- **Newtypes over Primitive Obsession**: Wrap domain primitives in tuple structs (`#[derive(Debug, Clone, PartialEq, Eq, Hash)] pub struct CustomerId(String);`) and enforce invariants in `TryFrom` / `FromStr` so invalid states are unrepresentable at compile time.
- **Zero Lint Suppressions**: Generated code MUST pass `cargo clippy --all-targets -- -D warnings` and `rustfmt`. Never add `#[allow(...)]` or `unsafe` blocks in domain/application code.

## 2. Immutability, Ownership & Modern Encapsulation

- **Immutable by Default**: Keep bindings immutable (`let` over `let mut`) unless local mutation is essential to an algorithm. Derive `Clone`, `Debug`, `PartialEq`, `Eq` on DTOs and Domain Events.
- **Encapsulation without Boilerplate**: Keep fields private on types with invariants. Expose read access via borrowing methods named after the field (`pub fn email(&self) -> &str`, never `get_email`) per the Rust API Guidelines, and controlled mutation via domain-intent methods rather than generic `set_*` setters.
- **Data Coupling with References & Slices**: Accept borrowed slices and string slices (`&[Item]`, `&str`) rather than owned containers (`&Vec<Item>`, `&String`) when ownership is not required (avoiding Stamp Coupling).

## 3. Composition, Traits & Dependency Injection

- **Narrow Traits for Seams (DIP & ISP)**: Define focused traits for external boundaries (clocks, repositories, gateways) and inject them via generic type parameters (`fn process<R: OrderRepo>(repo: &R)`) or trait objects (`Arc<dyn OrderRepo>`).
- **Zero Global Mutable Singletons**: Never store stateful domain collaborators in global `static` / `OnceLock<Mutex<...>>` singletons. Pass dependencies explicitly into constructors (`Service::new(repo)`) or functions.

## 4. Algebraic Data Types, Exhaustive `match` & Error Discipline

- **Rich Enums (ADTs) over Boolean Flags**: Model domain states and outcomes with Rust `enum` variants carrying variant-specific data rather than structs with optional fields or boolean parameter flags.
- **Exhaustive `match`**: Use exhaustive `match` expressions over domain enums. Avoid wildcard `_ =>` arms on internal enums so adding a new variant triggers a compiler error at every call site.
- **Guard Clauses (`?` & `let ... else`)**: Use the `?` operator and `let Some(x) = opt else { return Err(...); };` guard clauses to keep happy-path logic flat.
- **No `.unwrap()` or `.expect()` in Production Code**: Never call `.unwrap()`, `.expect()`, `panic!()`, or `todo!()` in non-test production paths; propagate typed errors via `Result<T, DomainError>`.
