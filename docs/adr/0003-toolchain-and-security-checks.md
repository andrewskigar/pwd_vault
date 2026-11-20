# ADR 0003: Toolchain, lint v2 and security checks

Status: accepted

## Context
- govulncheck flagged `golang.org/x/sys` below v0.44 (GO-2026-5024).
- golangci-lint v1 cannot target go 1.26 or later, and cannot read go1.27.4 export data at all, so it broke as soon as the system Go became go1.27.4.
- By human decision this cutover was built first, before the other vault-hardening tickets, so they are all verified against go 1.27 and lint v2.

## Decision
- `go.mod` declares `go 1.27` and `toolchain go1.27.4`.
- `golang.org/x/crypto` v0.59.0, `golang.org/x/term` v0.48.0 and `golang.org/x/sys` v0.50.0.
- golangci-lint v2 (`version: "2"`) with errcheck, govet, ineffassign, staticcheck and unused, gofmt as a formatter, and the `std-error-handling` exclusion preset. Without that preset v2 reports 8 errcheck findings on `fmt.Fprint*` and `defer Close` that v1 suppressed by default.

## Security checks
- Primary (source mode): `govulncheck ./...`
- Fallback (binary mode), used when source mode cannot load packages:
  `go build -o save_pwd.exe ./cmd/save_pwd` then `govulncheck -mode=binary save_pwd.exe`. It must report no vulnerabilities. Do not commit the binary.
- Why a fallback: govulncheck v1.10.0's source mode was expected to be unable to load the go1.27.4 standard library. Keep the fallback for whenever source mode fails to load packages after a toolchain bump.
- Mode used at the cutover: source mode (`govulncheck ./...` with govulncheck v1.10.0 built with go1.27.4) loaded the packages and reported "No vulnerabilities found." Binary mode on a fresh build also reported "No vulnerabilities found."

## Consequences
Contributors need golangci-lint v2 and govulncheck built with go1.27.4 or later on PATH. `golangci-lint run --new-from-rev=<sha>` keeps working for scoped lint.
