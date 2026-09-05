# Integration tests

`gtstack` is a translation layer, and `gh stack` is at v0.1.1. A renamed flag,
a changed default, or a moved state file breaks `gt` without changing a line of
its own code, and the unit tests in `cmd/gt` cannot see any of it.

These tests can, because they drive the real extension.

```sh
gh extension install github/gh-stack
go test -tags=integration ./test/integration/
```

The build tag keeps them out of `go test ./...`, so the ordinary CI run does
not need `gh` installed.

## How the tests are isolated

Every test builds its own throwaway repository under `t.TempDir()` whose only
remote is a **bare repository on disk**. `gh stack` fetches, rebases and pushes
against it happily. Global and system git config are pointed at `/dev/null`, so
a developer's signing key or default-branch setting cannot change the result,
and `GH_TOKEN` is unset for the whole run.

Nothing here reaches GitHub. That is deliberate: everything under test is meant
to work locally, so a command that quietly started needing the API should fail
rather than pass.

## What is covered

`workflow_test.go` runs `gt` end to end and checks what happened to the
repository afterwards — branches, commits, rebase results, the state file:

| Area | Commands |
| --- | --- |
| Starting and extending a stack | `gt create`, including the fork guard and generated branch names |
| Amend and cascade | `gt modify`, on the top branch, in the middle, and on a branch with no commits |
| Rebasing | `gt restack`, `-d`, `-u`, `-o` |
| Sync | `gt sync`, `gt sync -d` |
| Reading the stack | `gt log`, `gt ls`, `gt ll`, `gt log --json` |
| Navigation | `gt up`, `down`, `top`, `bottom`, `trunk` |
| Checkout routing | tracked branch, untracked branch, trunk, `-t` |
| Conflicts | pause detection, `gt continue`, `gt abort` |
| Refusals | unsupported commands, unknown commands, conflicting flags |

`contract_test.go` pins the parts of the `gh stack` interface `gt` depends on:

- **Command surface.** Every native invocation `gt` can emit, taken from
  `cmd/gt`, is replayed with `--help` appended. Cobra still parses the flags
  and then stops, so this catches a renamed or dropped flag without running
  anything.
- **Help snapshots.** The full `--help` text of each subcommand is recorded in
  `testdata/help/`. Much of what `gt` relies on is prose rather than a flag —
  that `--auto` creates drafts, that submit covers the whole stack, that
  `--no-trunk` skips the fetch. A snapshot diff is the cheapest way to be told
  those changed.
- **State format.** `.git/gh-stack` exists, is schema v1, and still spells
  `stacks[].trunk.branch` and `stacks[].branches[].branch` in stack order. `gt`
  refuses to run against a schema it does not know, so a bump here is a
  hard stop rather than a warning.
- **Behaviour `gt` works around.** `gh stack add <name>` leaves the index
  alone, and `gh stack add -m` on a branch with no commits puts the commit on
  that branch instead of creating a new one. Both are why `gt` stages and
  commits for itself.
- **Generated branch names.** `gh stack add -m` still produces `MM-DD-slug`,
  which `gt` reimplements because it never lets `gh stack` name the branch.

## What is not covered

`gh stack submit`, `merge` and `pr` need the GitHub API, so they cannot run
here. They are covered by the command-surface and help-snapshot checks only: a
renamed flag is caught, a silently changed default is not.

Closing that gap means a sandbox repository, a token with write access, and
real pull requests on every run.

Also untested: `gh stack switch` and `gh stack modify` open a TUI, and the
`gh-stack-modify-state` marker `gt` looks for can only be produced through it.

## Known `gh stack` v0.1.1 behaviour

`gh stack` does not find its state from a **linked git worktree**: every one of
its commands reports the current branch as untracked. `gt` resolves the state
through `--git-common-dir` and gets the right answer, then the `gh stack`
command it runs fails anyway. `TestLinkedWorktree` pins both halves, so when
the extension learns to handle worktrees, that test fails and says so.

## Re-recording the help snapshots

When a snapshot diff turns out to be harmless:

```sh
go test -tags=integration ./test/integration/ -run TestHelpSnapshots -update
```

Read the diff before you do. The snapshots are the only signal for the commands
that cannot be run.

## The daily job

`.github/workflows/gh-stack-compat.yml` installs a `gh-stack` release and runs
this suite. It runs every day against `latest`, and takes a version as a
`workflow_dispatch` input — a tag such as `v0.1.1`, or `latest`.

On failure it opens an issue labelled `gh-stack-compat` with the failing output,
which GitHub emails to you. A later green run closes it. While an issue is
already open, further failures are added as comments rather than new issues.

When it goes red, dispatch it again with `v0.1.1` — the version gtstack
targets. If that run is green, the extension changed; if it is red too,
something in the tests or the runner did.

> [!NOTE]
> GitHub disables scheduled workflows in a repository with no activity for 60
> days. A manual dispatch re-enables them.
