# 0008 — No GitHub Advanced Security checks

- Status: accepted
- Date: 2026-11-24

## Context

`security.yml` ran CodeQL and dependency review. On a private repository both need GitHub Advanced Security, and dependency review failed on the first pull request ("Dependency review is not supported on this repository").

## Decision

Remove the `codeql` and `dependency-review` jobs. Keep `govulncheck` (known vulnerabilities in Go dependencies and the code paths that call them), `gitleaks` (secrets) and Dependabot (weekly updates and alerts for Go, npm, Gradle and Actions).

## Consequences

- There is no static analysis for Go, TypeScript or Kotlin beyond the linters (`golangci-lint`, eslint, detekt). Security-sensitive changes still go through the `tenancy-security-reviewer` agent.
- Nothing blocks a pull request that adds a vulnerable npm or Gradle dependency; Dependabot alerts after the fact.
- If the repository becomes public, or GHAS is bought, restore both jobs from the git history of this file.
