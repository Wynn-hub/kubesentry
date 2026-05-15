# Contributing to KubeSentry

Thank you for your interest in contributing to KubeSentry! This document provides guidelines and instructions for contributing.

## Code of Conduct

Please review our [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) before contributing.

## Getting Started

### Prerequisites

- **Go** 1.26+ (for development)
- **Docker** (for building multi-platform images)
- **kubectl** (for Kubernetes interaction)
- **Helm** 3.8+ (for chart management)
- **Make** (for running commands)

### Local Development Setup

```bash
# Clone the repository
git clone https://github.com/Wynn-hub/kubesentry.git
cd kubesentry

# Install dependencies
go mod download

# Run tests
make test

# Run linter
make lint

# Build binaries
make build
```

## Development Workflow

### 1. Branching Strategy

Five branch types are used. **No merges — always rebase.**

| Branch | Pattern | Lifetime | Purpose |
|--------|---------|---------|---------|
| `main` | `main` | Permanent | Source of truth. Contains every merged feature and fix. |
| release | `release-x.y.z` | Long-lived | Cut from `main` at release time. Accepts hotfix cherry-picks only. |
| feature | `feat/<description>` | Short | New functionality. Rebase onto `main`, delete after landing. |
| fix | `fix/<description>` | Short | Bug fixes against `main`. Rebase onto `main`, delete after landing. |
| hotfix | `hotfix/<description>` | Short | Urgent fixes for old releases. Must land in `main` first. Delete after landing. |

**Naming rules:**
- Use kebab-case for `<description>`: `feat/policy-dry-run`, `fix/rbac-policygroups`, `hotfix/crd-schema-description`
- Avoid vague names: `dev`, `test`, `wip` are not allowed

### 2. Rebase Workflow

**Feature / fix branches → `main`:**

```bash
# Sync with latest main before landing
git fetch origin
git rebase origin/main          # run on your feat/fix branch

# Push rebased branch (use --force-with-lease, never bare --force)
git push --force-with-lease origin feat/<name>

# Fast-forward main
git checkout main
git rebase feat/<name>
git push origin main

# Clean up
git push origin --delete feat/<name>
git branch -d feat/<name>
```

**Rules:**
- Rebase onto `main` HEAD before every landing — no exceptions
- `--force-with-lease` only when pushing rebased branches to remote
- `main` itself is never force-pushed
- One logical change per commit; do not bundle unrelated changes

### 3. Making Changes

- **Single responsibility**: Each commit should represent a single logical change
- **Test-driven development**: Write tests before or alongside implementation
- **Keep commits small**: Aim for commits that can be reviewed in ~5 minutes
- **No debug statements**: Remove all `console.log` and debug code before committing

### 4. Running Tests

```bash
# Run all tests with coverage
GOROOT=/opt/homebrew/Cellar/go/1.26.2/libexec go test ./... -cover

# Run a specific package
GOROOT=/opt/homebrew/Cellar/go/1.26.2/libexec go test ./internal/webhook/... -v

# Run a specific test
GOROOT=/opt/homebrew/Cellar/go/1.26.2/libexec go test ./internal/webhook/... -run TestHandlerDeniesPrivilegedPod -v
```

### 5. Code Quality

Before committing, ensure:

```bash
# Lint checks
make lint

# Helm chart validation
helm lint charts/kubesentry

# All tests passing with 80%+ coverage
go test ./... -cover
```

Expected coverage:
- `internal/webhook`: >85%
- `internal/builtins`: >80%
- `internal/operator`: >80%
- `internal/api/v1alpha1`: >80%

## Commit Message Format

Follow [conventional commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description under 50 chars, verb-first>

<explanation of problem, root cause, solution, impact — max 72 chars/line>

<ticket link if applicable>
```

### Types

- `feat` — new feature (appears in release notes)
- `fix` — bug fix (appears in release notes)
- `perf` — performance improvement
- `test` — test additions or fixes
- `refactor` — code structure change without feature/bug fix
- `build` — Dockerfile, Makefile, CI config changes
- `docs` — documentation or code comments
- `chore` — maintenance, dependency updates

### Examples

```
feat(webhook): add structured logging for admission decisions

Log allowed and denied admissions with resource details for observability.
Helps troubleshoot policy rejections in production clusters.

https://github.com/Wynn-hub/kubesentry/issues/42
```

```
fix(operator): skip reconcile when policy generation unchanged

Prevent unnecessary PolicyVersion creation when policy.spec and generation
have not advanced. Reduces etcd churn on large deployments.
```

## Policy Development Guide

### OPA Rego v0 Syntax

⚠️ **Important**: This project bundles OPA which only supports Rego v0 syntax (not v1).

**Do:**
- Use wildcard iteration: `set[_]` to iterate
- Use `_` for unused variables
- Use `input.request.object.spec` to access request payload

**Don't:**
- Use `import rego.v1`
- Use `some x in set` iteration
- Use `any(...)` or `all(...)` functions

### Package Requirement

All Rego files must use the `kubesentry` package and expose a `deny` set:

```rego
package kubesentry

deny[msg] {
  condition1
  msg := "reason"
}

deny[msg] {
  condition2
  msg := "another reason"
}
```

The webhook evaluates `data.kubesentry.deny` to collect violations.

### Container Path Differences

| Resource | Container Path in Rego |
|---|---|
| Pod | `input.request.object.spec.containers[_]` |
| Deployment | `input.request.object.spec.template.spec.containers[_]` |
| DaemonSet | `input.request.object.spec.template.spec.containers[_]` |
| StatefulSet | `input.request.object.spec.template.spec.containers[_]` |

**Init containers** are under `.initContainers[_]` for all resource types.

### Adding a Built-in Policy

1. Create a new Rego file in `internal/builtins/rego/`:
   ```bash
   touch internal/builtins/rego/myPolicyKey.rego
   ```

2. Write the policy using v0 syntax:
   ```rego
   package kubesentry

   deny[msg] {
       c := input.request.object.spec.containers[_]
       # your condition
       msg := "reason"
   }
   ```

3. Register in `internal/builtins/library.go`:
   ```go
   "myPolicyKey": {
       Rego: regoFiles["myPolicyKey"],
       Description: "Policy description for users",
       DefaultMode: "enforce",  // or "audit"
       Match: v1alpha1.PolicyMatch{
           Operations: []string{"CREATE", "UPDATE"},
           Resources: []v1alpha1.MatchResource{{
               APIGroups:   []string{""},
               APIVersions: []string{"v1"},
               Resources:   []string{"pods"},
           }},
       },
   },
   ```

4. Add a test entry in `internal/builtins/library_test.go` if adding Rego that won't compile.

5. Update [README.md](README.md) built-in policies table.

## Submitting Changes

### Pull Request Process

1. **Push your branch:**
   ```bash
   git push -u origin feat/my-feature
   ```

2. **Create a pull request** on GitHub with:
   - Clear title and description
   - Reference any related issues (e.g., "Closes #123")
   - Test plan listing manual steps to verify the change

3. **Land requirements:**
   - All CI checks pass (tests, lint)
   - No conflicts with `main` (rebase first if behind)
   - Commit history is linear — no merge commits
   - At least one code review approval

### Code Review Checklist

Reviewers will verify:
- ✓ Changes align with project architecture
- ✓ Tests cover new logic (80%+ coverage requirement)
- ✓ Commit messages follow convention
- ✓ No hardcoded secrets or credentials
- ✓ No breaking API changes (or documented breaking changes)
- ✓ Documentation updated if needed

## Reporting Issues

### Security Vulnerabilities

**Do not** open a public GitHub issue. Email wying8408@gmail.com with:
- Description of the vulnerability
- Steps to reproduce
- Potential impact

### Bug Reports

Include:
- KubeSentry version
- Kubernetes version
- Reproduction steps
- Expected vs. actual behavior
- Relevant logs or YAML

### Feature Requests

Describe:
- Use case and motivation
- Expected behavior
- Acceptance criteria

## Testing Kubernetes Integration

### Local Testing with Kind

```bash
# Create a local cluster
kind create cluster --name kubesentry-test

# Build and load images
make build
kind load docker-image wynnhub/kubesentry-webhook:latest --name kubesentry-test
kind load docker-image wynnhub/kubesentry-operator:latest --name kubesentry-test

# Install chart
helm install kubesentry charts/kubesentry \
  --namespace kubesentry-system --create-namespace \
  --set webhook.image.pullPolicy=Never \
  --set operator.image.pullPolicy=Never

# Apply a test policy from examples/
kubectl apply -f examples/policy-no-privileged.yaml

# Test with example pods
kubectl apply -f examples/test-normal-pod.yaml  # Should succeed
kubectl apply -f examples/test-privileged-pod.yaml  # Should fail

# Check webhook logs
kubectl logs -n kubesentry-system -l app=webhook -f
```

## Release Process

Releases are handled by maintainers using the release Makefile:

```bash
git tag v0.2.0
make release  # Cross-compile, push images, publish chart
```

See [DEVELOPMENT.md](DEVELOPMENT.md#release-process) for detailed release steps.

## Questions?

- Open a [GitHub Discussion](https://github.com/Wynn-hub/kubesentry/discussions)
- File an [Issue](https://github.com/Wynn-hub/kubesentry/issues)
- Email: wying8408@gmail.com

Thank you for contributing! 🎉
