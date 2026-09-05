//go:build integration

package integration

import (
	"strings"
	"testing"
	"time"
)

// TestCreateStartsAndExtendsStack covers the two halves of gt create: off the
// trunk it starts a stack, on top of one it appends a layer. gt stages and
// commits itself in both cases rather than letting `gh stack add -m` do it.
func TestCreateStartsAndExtendsStack(t *testing.T) {
	f := newFixture(t)

	f.write("layer-one.txt", "one\n")
	first := f.gt("create", "layer-one", "-a", "-m", "Add layer one")
	if !first.announced("gh stack init --base main layer-one") {
		t.Errorf("create off the trunk did not run `gh stack init --base main layer-one`\n%s", first.output())
	}
	if !first.announced("git commit -m 'Add layer one'") {
		t.Errorf("create did not make the commit itself\n%s", first.output())
	}

	f.write("layer-two.txt", "two\n")
	second := f.gt("create", "layer-two", "-a", "-m", "Add layer two")
	if !second.announced("gh stack add layer-two") {
		t.Errorf("create on top of a stack did not run `gh stack add layer-two`\n%s", second.output())
	}

	if got := f.branch(); got != "layer-two" {
		t.Errorf("on branch %q, want layer-two", got)
	}
	if got := f.tracked(); len(got) != 2 || got[0] != "layer-one" || got[1] != "layer-two" {
		t.Errorf("tracked stack is %q, want [layer-one layer-two]", got)
	}
	if got := f.state().Stacks[0].Trunk.Branch; got != "main" {
		t.Errorf("stack trunk is %q, want main", got)
	}
	// One commit per layer, each carrying its own message: proof that neither
	// commit landed on the branch below it.
	if got := f.commitsIn("main..layer-one"); got != 1 {
		t.Errorf("layer-one has %d commits over main, want 1", got)
	}
	if got := f.commitsIn("layer-one..layer-two"); got != 1 {
		t.Errorf("layer-two has %d commits over layer-one, want 1", got)
	}
	if got := f.subject("layer-one"); got != "Add layer one" {
		t.Errorf("layer-one subject is %q, want %q", got, "Add layer one")
	}
	if got := f.subject("layer-two"); got != "Add layer two" {
		t.Errorf("layer-two subject is %q, want %q", got, "Add layer two")
	}
	if got := f.git("status", "--porcelain"); got != "" {
		t.Errorf("work tree is not clean after create:\n%s", got)
	}
}

// TestCreateGeneratesBranchNameFromMessage checks gt's own naming. That it
// still matches what `gh stack add -m` produces is checked separately, in
// TestGeneratedBranchNameMatchesGhStack.
func TestCreateGeneratesBranchNameFromMessage(t *testing.T) {
	f := newFixture(t)

	f.write("work.txt", "work\n")
	before := time.Now()
	f.gt("create", "-a", "-m", "Add the login form")

	got := f.branch()
	// A run that straddles midnight would otherwise be flaky.
	for _, at := range []time.Time{before, time.Now()} {
		if got == at.Format("01-02")+"-add_the_login_form" {
			return
		}
	}
	t.Errorf("created branch %q, want %s-add_the_login_form", got, before.Format("01-02"))
}

// TestCreateRefusesInMiddleOfStack is the fork guard: gh stack keeps a stack
// flat, so branching from the middle has nowhere to go.
func TestCreateRefusesInMiddleOfStack(t *testing.T) {
	f := newFixture(t)
	f.layer("layer-one", "Add layer one")
	f.layer("layer-two", "Add layer two")
	f.gt("down")

	f.write("fork.txt", "fork\n")
	r := f.gtFails("create", "forked", "-a", "-m", "Fork the stack")
	if !strings.Contains(r.stderr, "not the top of its stack") {
		t.Errorf("error does not explain the fork:\n%s", r.output())
	}
	if got := f.tracked(); len(got) != 2 {
		t.Errorf("refused create still changed the stack: %q", got)
	}
	if got := f.branch(); got != "layer-one" {
		t.Errorf("refused create moved to %q, want to stay on layer-one", got)
	}
}

func TestCreateWithoutNameOrMessage(t *testing.T) {
	f := newFixture(t)
	r := f.gtFails("create")
	if !strings.Contains(r.stderr, "needs a branch name") {
		t.Errorf("unexpected error:\n%s", r.output())
	}
}

// TestModifyAmendsAndRestacksUpstack is the command with no gh stack
// equivalent: gt amends with git, then asks gh stack to cascade the rebase.
func TestModifyAmendsAndRestacksUpstack(t *testing.T) {
	f := newFixture(t)
	f.layer("layer-one", "Add layer one")
	f.layer("layer-two", "Add layer two")
	f.gt("down")

	f.write("layer-one.txt", "one, revised\n")
	r := f.gt("modify", "-a", "-m", "Add layer one, revised")
	if !r.announced("gh stack rebase --upstack --no-trunk") {
		t.Errorf("modify did not cascade the rebase\n%s", r.output())
	}

	if got := f.commitsIn("main..layer-one"); got != 1 {
		t.Errorf("layer-one has %d commits over main, want 1: modify added instead of amending", got)
	}
	if got := f.subject("layer-one"); got != "Add layer one, revised" {
		t.Errorf("layer-one subject is %q, want the amended message", got)
	}
	if f.git("rev-parse", "layer-one") != f.git("rev-parse", "layer-two^") {
		t.Errorf("layer-two was not rebased onto the amended layer-one\n%s", f.git("log", "--graph", "--oneline", "--all"))
	}
	if got := f.subject("layer-two"); got != "Add layer two" {
		t.Errorf("layer-two subject is %q, want it unchanged", got)
	}
}

// TestModifyCommitsWhenBranchHasNoCommits guards the case gt goes out of its
// way to handle: amending here would rewrite the parent's commit instead.
func TestModifyCommitsWhenBranchHasNoCommits(t *testing.T) {
	f := newFixture(t)
	f.gt("create", "empty-layer")
	if got := f.commitsIn("main..HEAD"); got != 0 {
		t.Fatalf("new branch already has %d commits, want 0", got)
	}

	f.write("work.txt", "work\n")
	r := f.gt("modify", "-a", "-m", "First commit on this layer")
	if strings.Contains(r.stderr, "--amend") {
		t.Errorf("modify amended a branch with no commits of its own\n%s", r.output())
	}
	if got := f.commitsIn("main..HEAD"); got != 1 {
		t.Errorf("branch has %d commits over main, want 1", got)
	}
	if got := f.subject("main"); got != "Initial commit" {
		t.Errorf("trunk commit became %q; modify rewrote the parent", got)
	}
}

// TestModifyOnTopSkipsRebase: there is nothing above the top branch, so gt
// must not spend a rebase on it.
func TestModifyOnTopSkipsRebase(t *testing.T) {
	f := newFixture(t)
	f.layer("layer-one", "Add layer one")

	f.write("layer-one.txt", "one, revised\n")
	r := f.gt("modify", "-a", "-m", "Add layer one, revised")
	if strings.Contains(r.stderr, "gh stack rebase") {
		t.Errorf("modify on the top branch still rebased\n%s", r.output())
	}
}

func TestModifyOutsideAStack(t *testing.T) {
	f := newFixture(t)
	f.write("loose.txt", "loose\n")
	r := f.gtFails("modify", "-a", "-m", "Not in a stack")
	if !strings.Contains(r.stderr, "not part of a stack") {
		t.Errorf("unexpected error:\n%s", r.output())
	}
}

func TestNavigation(t *testing.T) {
	f := newFixture(t)
	f.layer("layer-one", "Add layer one")
	f.layer("layer-two", "Add layer two")
	f.layer("layer-three", "Add layer three")

	steps := []struct {
		command, native, want string
	}{
		{"down", "gh stack down", "layer-two"},
		{"down", "gh stack down", "layer-one"},
		{"up", "gh stack up", "layer-two"},
		{"top", "gh stack top", "layer-three"},
		{"bottom", "gh stack bottom", "layer-one"},
		{"trunk", "gh stack trunk", "main"},
	}
	for _, step := range steps {
		r := f.gt(step.command)
		if !r.announced(step.native) {
			t.Errorf("gt %s did not run `%s`\n%s", step.command, step.native, r.output())
		}
		if got := f.branch(); got != step.want {
			t.Fatalf("gt %s landed on %q, want %q", step.command, got, step.want)
		}
	}
}

func TestLogViews(t *testing.T) {
	f := newFixture(t)
	f.layer("layer-one", "Add layer one")
	f.layer("layer-two", "Add layer two")

	full := f.gt("log")
	if !full.announced("gh stack view") {
		t.Errorf("gt log did not run `gh stack view`\n%s", full.output())
	}
	short := f.gt("ls")
	if !short.announced("gh stack view --short") {
		t.Errorf("gt ls did not run `gh stack view --short`\n%s", short.output())
	}
	long := f.gt("ll")
	if !long.announced("git log --graph --oneline --decorate --all") {
		t.Errorf("gt ll did not run the git graph\n%s", long.output())
	}

	// The stack view is the one piece of output people read, and gt promises
	// to keep stdout clean so it can be piped.
	for _, view := range []struct {
		name string
		r    result
	}{{"gt log", full}, {"gt ls", short}} {
		for _, branch := range []string{"layer-one", "layer-two"} {
			if !strings.Contains(view.r.stdout, branch) {
				t.Errorf("%s stdout does not mention %s\n%s", view.name, branch, view.r.output())
			}
		}
	}
}

func TestRestackOntoMovedTrunk(t *testing.T) {
	f := newFixture(t)
	f.layer("layer-one", "Add layer one")
	f.layer("layer-two", "Add layer two")

	f.git("checkout", "--quiet", "main")
	f.write("trunk-moves.txt", "moved\n")
	f.git("add", "-A")
	f.git("commit", "--quiet", "-m", "Trunk moves on")
	f.git("push", "--quiet", "origin", "main")
	f.git("checkout", "--quiet", "layer-two")

	r := f.gt("restack")
	if !r.announced("gh stack rebase") {
		t.Errorf("gt restack did not run `gh stack rebase`\n%s", r.output())
	}
	if f.git("rev-parse", "main") != f.git("rev-parse", "layer-one^") {
		t.Errorf("layer-one was not rebased onto the moved trunk\n%s", f.git("log", "--graph", "--oneline", "--all"))
	}
	if f.git("rev-parse", "layer-one") != f.git("rev-parse", "layer-two^") {
		t.Errorf("layer-two was not carried along\n%s", f.git("log", "--graph", "--oneline", "--all"))
	}
}

func TestRestackDirections(t *testing.T) {
	f := newFixture(t)
	f.layer("layer-one", "Add layer one")
	f.layer("layer-two", "Add layer two")
	f.layer("layer-three", "Add layer three")
	f.gt("down")

	if r := f.gt("restack", "-d"); !r.announced("gh stack rebase --downstack") {
		t.Errorf("gt restack -d did not pass --downstack\n%s", r.output())
	}
	if r := f.gt("restack", "-u"); !r.announced("gh stack rebase --upstack") {
		t.Errorf("gt restack -u did not pass --upstack\n%s", r.output())
	}
	if r := f.gtFails("restack", "-d", "-u"); !strings.Contains(r.stderr, "conflict") {
		t.Errorf("unexpected error for conflicting directions:\n%s", r.output())
	}
	if r := f.gtFails("restack", "-o"); !strings.Contains(r.stderr, "no gh stack equivalent") {
		t.Errorf("unexpected error for --only:\n%s", r.output())
	}
}

// TestSyncPushesTheStack exercises the parts of `gh stack sync` that do not
// need the API: fetch, cascade rebase, and push every branch.
func TestSyncPushesTheStack(t *testing.T) {
	f := newFixture(t)
	f.layer("layer-one", "Add layer one")
	f.layer("layer-two", "Add layer two")

	if r := f.gt("sync"); !r.announced("gh stack sync") {
		t.Errorf("gt sync did not run `gh stack sync`\n%s", r.output())
	}
	if r := f.gt("sync", "-d"); !r.announced("gh stack sync --prune") {
		t.Errorf("gt sync -d did not pass --prune\n%s", r.output())
	}

	remote := f.git("ls-remote", "--heads", "origin")
	for _, branch := range []string{"layer-one", "layer-two"} {
		if !strings.Contains(remote, "refs/heads/"+branch) {
			t.Errorf("sync did not push %s\n%s", branch, remote)
		}
	}
}

// TestCheckoutRouting: `gh stack checkout` reads a bare name as a stack
// number, then a PR, then a tracked branch, so gt sends anything it knows to
// be untracked to git instead.
func TestCheckoutRouting(t *testing.T) {
	f := newFixture(t)
	f.layer("layer-one", "Add layer one")
	f.layer("layer-two", "Add layer two")

	if r := f.gt("checkout", "layer-one"); !r.announced("gh stack checkout layer-one") {
		t.Errorf("a tracked branch did not go through gh stack\n%s", r.output())
	} else if got := f.branch(); got != "layer-one" {
		t.Errorf("landed on %q, want layer-one", got)
	}

	f.git("branch", "scratch")
	if r := f.gt("checkout", "scratch"); !r.announced("git checkout scratch") {
		t.Errorf("an untracked branch did not go through git\n%s", r.output())
	} else if got := f.branch(); got != "scratch" {
		t.Errorf("landed on %q, want scratch", got)
	}

	if r := f.gt("checkout", "main"); !r.announced("git checkout main") {
		t.Errorf("the trunk did not go through git\n%s", r.output())
	}

	f.gt("checkout", "layer-two")
	if r := f.gt("co", "-t"); !r.announced("gh stack trunk") {
		t.Errorf("gt co -t did not run `gh stack trunk`\n%s", r.output())
	} else if got := f.branch(); got != "main" {
		t.Errorf("landed on %q, want main", got)
	}
}

// pausedRebase builds a stack whose two layers touch the same file, then
// rewrites the bottom one so the cascading rebase stops on a conflict. gt
// finds a paused operation by the marker file gh stack leaves behind, so the
// name of that file is part of the contract.
func pausedRebase(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	f.write("shared.txt", "one\n")
	f.gt("create", "layer-one", "-a", "-m", "Add layer one")
	f.write("shared.txt", "two\n")
	f.gt("create", "layer-two", "-a", "-m", "Add layer two")
	f.gt("down")

	f.write("shared.txt", "conflicting\n")
	// The rebase is expected to stop, so the exit status is not the point here.
	f.run(gtBin, "modify", "-a", "-m", "Rewrite layer one")

	if !f.gitFileExists("gh-stack-rebase-state") {
		t.Fatalf("no .git/gh-stack-rebase-state after a conflicting rebase; gt cannot detect a paused operation\n%s",
			f.git("status"))
	}
	return f
}

func TestContinueAfterResolvingConflict(t *testing.T) {
	f := pausedRebase(t)

	if r := f.run(gtBin, "continue"); !r.announced("gh stack rebase --continue") {
		t.Fatalf("gt continue did not resume the rebase\n%s", r.output())
	}

	f.write("shared.txt", "resolved\n")
	f.git("add", "shared.txt")
	r := f.gt("continue")
	if !r.announced("gh stack rebase --continue") {
		t.Errorf("gt continue did not resume the rebase\n%s", r.output())
	}
	if f.gitFileExists("gh-stack-rebase-state") {
		t.Errorf("the paused-rebase marker survived a successful continue")
	}
	if f.git("rev-parse", "layer-one") != f.git("rev-parse", "layer-two^") {
		t.Errorf("layer-two was not rebased onto layer-one\n%s", f.git("log", "--graph", "--oneline", "--all"))
	}
}

func TestAbortRestoresTheStack(t *testing.T) {
	f := pausedRebase(t)
	before := f.git("rev-parse", "layer-two")

	r := f.gt("abort")
	if !r.announced("gh stack rebase --abort") {
		t.Errorf("gt abort did not abort the rebase\n%s", r.output())
	}
	if f.gitFileExists("gh-stack-rebase-state") {
		t.Errorf("the paused-rebase marker survived the abort")
	}
	if got := f.git("rev-parse", "layer-two"); got != before {
		t.Errorf("layer-two is at %s after the abort, want %s", got, before)
	}
}

func TestContinueWithNothingPaused(t *testing.T) {
	f := newFixture(t)
	f.layer("layer-one", "Add layer one")
	r := f.gtFails("continue")
	if !strings.Contains(r.stderr, "no gh stack rebase or modify is in progress") {
		t.Errorf("unexpected error:\n%s", r.output())
	}
}

// TestUnsupportedCommands: gt refuses these on purpose, with the reason and
// the nearest alternative. Silently forwarding one would be worse than
// stopping.
func TestUnsupportedCommands(t *testing.T) {
	f := newFixture(t)
	for _, name := range []string{"fold", "reorder", "split", "absorb", "track", "undo", "config"} {
		r := f.run(gtBin, name)
		if r.code != 2 {
			t.Errorf("gt %s exited %d, want 2\n%s", name, r.code, r.output())
		}
		if !strings.Contains(r.stderr, "cannot be translated") {
			t.Errorf("gt %s did not explain itself\n%s", name, r.output())
		}
	}
	if r := f.run(gtBin, "no-such-command"); r.code != 2 || !strings.Contains(r.stderr, "unknown command") {
		t.Errorf("an unknown command exited %d\n%s", r.code, r.output())
	}
}

// TestLinkedWorktree covers both sides of a case gt cannot fix on its own.
//
// gt reads the stack state from the shared git directory, so it recognises a
// tracked branch from a linked worktree. gh stack v0.1.1 does not: run there,
// every one of its commands reports the branch as untracked. So gt reaches the
// right decision and then the command it runs fails anyway.
//
// The second half of this test is deliberately pinned to the broken
// behaviour. When gh stack learns to find its state from a linked worktree,
// this test fails and gt's handling finally pays off.
func TestLinkedWorktree(t *testing.T) {
	f := newFixture(t)
	f.layer("layer-one", "Add layer one")
	// A branch checked out in the main repository cannot also be checked out
	// in a worktree.
	f.gt("trunk")

	linked := f.dir + "-worktree"
	f.git("worktree", "add", "--quiet", linked, "layer-one")
	worktree := &fixture{t: t, dir: linked, origin: f.origin}

	// gt's side: the state lives in the shared git directory and gt finds it.
	// Reading it as absent would turn `gt create` into `gh stack init` and
	// start a second stack.
	if got := worktree.tracked(); len(got) != 1 || got[0] != "layer-one" {
		t.Errorf("gt sees %q as the stack from a linked worktree, want [layer-one]", got)
	}

	// gh stack's side.
	r := worktree.run("gh", "stack", "view")
	if r.code == 0 {
		t.Errorf("`gh stack view` now works in a linked worktree; gt already resolves state through "+
			"--git-common-dir, so drop this expectation and test the worktree path for real\n%s", r.output())
	} else if !strings.Contains(r.output(), "not part of a stack") {
		t.Errorf("`gh stack view` failed differently in a linked worktree than the known v0.1.1 limitation\n%s", r.output())
	}
}
