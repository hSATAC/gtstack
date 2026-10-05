# gtstack

> Graphite muscle memory, backed by GitHub's native stacks.

`gtstack` installs a `gt` command that translates the Graphite CLI commands
you already know into [GitHub's official `gh stack`][gh-stack] commands and
plain Git operations.

It is intentionally a thin compatibility shim. `gh stack` remains the source
of truth for stack state; `gtstack` has no backend, metadata format, or config
of its own.

> [!IMPORTANT]
> `gtstack` is early software. It currently supports **linear stacks only** and
> targets `gh-stack` **v0.2.0**, state schema **v1**. When an operation cannot
> be translated safely, it stops with an actionable error instead of guessing.

## Quick start

### Requirements

- Git 2.36 or newer for cross-worktree operations
- [GitHub CLI (`gh`)][gh-cli], authenticated with GitHub
- The [`github/gh-stack`][gh-stack] extension
- Go 1.23 or newer to build from source

Install the extension:

```sh
gh extension install github/gh-stack
```

Install `gt` with Homebrew (macOS). The fully qualified name taps
`hSATAC/toybox` for you:

```sh
brew install hSATAC/toybox/gtstack
```

Or install it with Go:

```sh
go install github.com/hSATAC/gtstack/cmd/gt@latest
```

Or build from source and put `gt` on your `PATH`:

```sh
git clone https://github.com/hSATAC/gtstack.git
cd gtstack
mkdir -p bin
go build -o bin/gt ./cmd/gt
export PATH="$PWD/bin:$PATH"
```

Linux builds are attached to each [release][releases] as `tar.gz` archives.

`gtstack` deliberately uses the same binary name as Graphite. If Graphite is
still installed, use `type -a gt` to see which binary your shell will run.
You can distinguish them with:

```console
$ gt --version
gt 0.1.0 (gh-stack shim)
```

### Use it like Graphite

```sh
# Start a stack and commit the first layer
gt create -am "Add the API client"

# Add another layer
gt create -am "Use the API client"

# Inspect the stack
gt log

# Create or update the native GitHub stack
gt submit --stack
```

Each `gt` invocation operates on the same stack state as `gh stack`, so you can
mix the two CLIs whenever you need a native command:

```sh
gt log
gh stack modify
gt submit --stack
```

## Why not just alias `gh stack` to `gt`?

`gh stack alias gt` only forwards arguments unchanged, but the two CLIs do not
have the same command surface. Some commands have different names:

```text
gt create   → gh stack add
gt log      → gh stack view
gt restack  → gh stack rebase
```

Others have actively conflicting meanings. Graphite's `gt modify` amends the
current branch and restacks its descendants, while `gh stack modify` opens a
TUI for restructuring the stack. `gtstack` translates the intent instead of
blindly forwarding the command.

## Command mapping

| Graphite command | What `gtstack` runs |
| --- | --- |
| `gt create` / `gt c` | `gh stack init` for a new stack, or `gh stack add` at the top of an existing stack |
| `gt modify` / `gt m` | `git commit [--amend]`, then `gh stack rebase --upstack --no-trunk` |
| `gt submit` / `gt s` / `gt ss` | `gh stack submit --auto` (`-e` opens the editor) |
| `gt sync` | `gh stack sync` (`-d` maps to `--prune`) |
| `gt restack` | `gh stack rebase` (`-d`/`-u` map to `--downstack`/`--upstack`) |
| `gt continue` / `gt abort` | Continue or abort the paused `gh stack rebase` or `gh stack modify` |
| `gt checkout` / `gt co` | `gh stack checkout`, `gh stack switch`, or `git checkout`, depending on the target |
| `gt get <pr>` / `gt get <branch>` | `gh stack checkout <pr>` / `gh stack checkout <branch>` (a branch not tracked locally is fetched from GitHub together with its stack) |
| `gt log` / `gt ls` / `gt ll` | `gh stack view`, `gh stack view --short`, or `git log --graph` |
| `gt up`, `down`, `top`, `bottom`, `trunk` | The corresponding `gh stack` navigation command |
| `gt merge` | `gh stack merge` |
| `gt switch` | `gh stack switch` |
| `gt pr` | `gh pr view --web` |
| `gt init` | Nothing. Explains that `gh stack` has no repository-level init |

Run `gt <command> --help` for the supported flags. Unknown flags are rejected
rather than silently dropped.

## Important differences from Graphite

### Linear stacks only

`gh stack` represents a stack as a flat ordered list. `gtstack` therefore does
not support forks:

- `gt create` must run from the top of the current stack.
- `gt create` requires a new branch name. Use `gt checkout` for an existing
  branch, or native `gh stack init` / `add` to adopt it into a stack.
- If a branch belongs to more than one stack, `gtstack` stops and asks you to
  use `gh stack` directly.

### `gt submit` covers the whole stack

Graphite's `gt submit` submits the current branch and its downstack branches.
`gh stack submit` covers the entire stack, so `gtstack` does too. Pass `-e` to
open the submit editor and deselect branches.

`gh stack submit` opens that editor whenever it has a terminal. Graphite's
submit does not, so `gt submit` passes `--auto` and skips it. New PRs are
created as drafts; pass `-p` to mark them ready for review.

> [!NOTE]
> `-p` maps to `--open`, which marks **new and existing** PRs ready for review.
> It will publish a PR you had deliberately left as a draft, so `gt submit`
> never passes it for you.

### `gt modify` is implemented with Git

`gh stack` has no equivalent of Graphite's amend operation. `gtstack` commits
or amends with Git, then asks `gh stack` to rebase the upstack branches. If the
current branch has no commits of its own, it creates a commit rather than
rewriting its parent's commit.

### Checkout is stack-scoped

`gt co` without an argument opens `gh stack switch`, which lists branches in
the current stack rather than every tracked branch. Trunk and untracked local
branches are checked out directly with Git.

### Linked worktrees

With `gh-stack` v0.2.0, linked worktrees share the stack catalog. `gt log` and
`gt create` work from a linked worktree, and `gt modify`, `gt restack`, and
`gt sync` can rebase a stack whose branches are checked out in different
worktrees. Resolve conflicts in the worktree reported by `gh stack`; then
`gt continue` or `gt abort` can run from another worktree in the repository.

Affected worktrees must be clean before a rebase; `gh stack` does not stash
changes or create or remove worktrees. `gt modify` commits in the current
worktree before asking `gh stack` to rebase its descendants, so a rejected
rebase can leave that commit in place.

These operations follow gh-stack's worktree semantics. [Graphite 1.8.4 and
newer][graphite-worktrees] generally avoid modifying branches checked out in
other worktrees; gh-stack can rebase those branches in their owning worktrees.

The new native `gh stack checkout --print-path` flag can report a target's
worktree path. The passthrough navigation commands (`gt up`, `down`, `top`,
`bottom`, and `trunk`) also accept `--print-path`, but `gt checkout` does not
yet expose it. `gtstack` has no shell integration to change your shell's
working directory.

Graphite's `gt create --onto <branch>` remains unsupported. The new ability
to adopt branches across worktrees makes a translation worth exploring for
linear stacks, but it still needs explicit branch creation, parent selection,
and failure handling; upgrading the extension alone does not implement it.

## Transparent by default

Before running each native operation, `gtstack` prints the exact command to
stderr:

```console
$ git add -A
$ gh stack add 07-31-add_the_login_form
$ git commit -m 'Add the login form'
```

This keeps stdout clean for pipelines such as:

```sh
gt log --json | jq
```

Color is disabled when stderr is not a terminal or when `NO_COLOR` is set.

## Unsupported Graphite commands

`gtstack` is a workflow bridge, not a complete reimplementation of Graphite.
Each command below stops with the reason and the nearest `git` or `gh stack`
alternative; run it to see the advice for that command.

None of these are merely unfinished. They are grouped by what blocks the
translation.

### Available only in the `gh stack modify` TUI

```text
fold  move  reorder  rename  delete
```

`gh stack` can perform these, but only inside an interactive editor. No flag
drives them, so there is nothing for a one-line `gt` command to call.
Version 0.2.0 lets that editor operate across worktrees; it does not add
non-interactive flags for these operations or `gt create --insert`.

### `gh stack` has no equivalent

```text
absorb  split  squash  pop  revert  undo  freeze  unfreeze
```

The operation does not exist in the `gh stack` model. Most of them name a Git
recipe instead — `gt squash` points at `git reset --soft <parent>`, for example.
`freeze` and `unfreeze` have no counterpart at all.

### The scope does not match

```text
track  untrack  unlink
```

Graphite tracks and untracks one branch. `gh stack` works a whole stack at a
time: `gh stack init` adopts a list of branches into a new stack, and
`gh stack unstack` drops an entire stack. Translating between them would
silently widen what you asked for, so `gt` refuses instead.

### Not a stack operation

```text
aliases   auth      changelog  children  config  dash
demo      docs      feedback   guide     info    parent
```

`gh` itself, GitHub, or nothing at all handles these. `gt` has no configuration
of its own, so `gt config` points at `gh`.

## Extension and state compatibility

If `github/gh-stack` is missing, `gtstack` offers to install it interactively:

```text
gt: the gh stack extension is not installed. Install it now? [Y/n]
```

In a non-interactive environment it never installs software automatically; it
prints the installation command and exits instead.

To decide whether `gt create` should initialize or extend a stack, `gtstack`
reads the state written by `gh stack` at `<git-common-dir>/gh-stack`
(`.git/gh-stack` in a regular checkout). It refuses to run
against an unknown schema version so that a future `gh-stack` update cannot
silently corrupt a stack.

[gh-cli]: https://cli.github.com/
[gh-stack]: https://github.com/github/gh-stack
[graphite-worktrees]: https://graphite.com/docs/multiple-worktrees
[releases]: https://github.com/hSATAC/gtstack/releases
