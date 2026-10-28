## Stage 1: Adopt gh-stack v0.3.0

**Goal**: Update the compatibility baseline, reviewed help snapshots, and version documentation.
**Success Criteria**: CI pins v0.3.0; the existing integration suite passes against that release; schema v1 remains supported.
**Tests**: gofmt, go vet, go build, unit tests, and the complete integration suite against v0.3.0.
**Status**: Complete

## Stage 2: Forward base-change and atomic-push options

**Goal**: Support --accept-base-change on sync/restack and --no-atomic on submit/sync without changing their defaults.
**Success Criteria**: Flags reach gh-stack, explicit false overrides the atomic-push environment setting, and documentation explains consent and push scope.
**Tests**: Command forwarding tests, native command-surface checks, and local integration coverage for sync/restack including a remote without atomic-push support.
**Status**: Complete

## Stage 3: Expose checkout worktree paths

**Goal**: Support checkout/co --print-path with explicit targets and --trunk, preserving safe local-branch routing.
**Success Criteria**: stdout contains only the resolved path; branches held in other worktrees stay there; unoccupied targets follow checkout semantics; missing or conflicting targets are rejected.
**Tests**: Integration tests for tracked/untracked branches, trunk, paths containing spaces, current/unoccupied branches, and invalid arguments; final formatting, vet, build, and full test suites.
**Status**: In Progress
