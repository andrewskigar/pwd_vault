# ADR 0001: Hexagonal layout enforced by a test

Status: accepted

## Context
The tool handles secrets, so the business rules should be testable without a terminal or filesystem, and storage should be swappable.

## Decision
A pure core (`internal/domain`, `internal/app`, `internal/passgen`) owns the ports (`VaultStore`, `Clock`). The cobra CLI and the encrypted-file store are adapters that depend inward. `main` is the only package that knows both adapters. `internal/architecture_test.go` fails if a core package imports an adapter, a third-party module, `os`, `net`, `math/rand` or `unsafe`.

## Consequences
Use cases are tested against an in-memory store; the CLI is tested in-process with golden files. New I/O must go through a port.
