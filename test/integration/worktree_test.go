//go:build integration

package integration

import (
	"strings"
	"testing"
)

// gh stack can adopt a branch in another worktree without checking it out.
// gt create commits separately, so it must reject existing names before any
// staging or adoption can put a commit on the wrong branch.
func TestCreateRefusesExistingBranch(t *testing.T) {
	for _, startStack := range []bool{true, false} {
		name := "extend-stack"
		if startStack {
			name = "start-stack"
		}
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			if !startStack {
				f.layer("layer-one", "Add layer one")
			}
			f.git("branch", "existing")
			linked := f.worktree("existing")
			before := f.git("rev-parse", "HEAD")
			f.write("new.txt", "must stay uncommitted\n")

			r := f.run(gtBin, "create", "existing", "-a", "-m", "New layer")

			if r.code == 0 || !strings.Contains(r.stderr, "already exists") {
				t.Errorf("create did not reject the existing branch\n%s", r.output())
			}
			if got := f.git("rev-parse", "HEAD"); got != before {
				t.Errorf("create changed the original branch from %s to %s", before, got)
			}
			if got := linked.git("rev-parse", "HEAD"); got != before {
				t.Errorf("create changed the existing branch from %s to %s", before, got)
			}
			if got := f.git("status", "--porcelain"); got != "?? new.txt" {
				t.Errorf("create staged or committed changes: status is %q, want an untracked new.txt", got)
			}
			if startStack {
				if f.gitFileExists("gh-stack") {
					t.Errorf("create initialized stack tracking for an existing branch")
				}
			} else if got := f.tracked(); len(got) != 1 || got[0] != "layer-one" {
				t.Errorf("create changed stack tracking: %q", got)
			}
		})
	}
}

func TestModifyAcrossWorktrees(t *testing.T) {
	f := newFixture(t)
	f.layer("layer-one", "Add layer one")
	f.layer("layer-two", "Add layer two")
	f.gt("down")
	linked := f.worktree("layer-two")

	f.write("layer-one.txt", "one, revised\n")
	f.gt("modify", "-a", "-m", "Revise layer one")

	if got := f.subject("layer-one"); got != "Revise layer one" {
		t.Errorf("layer-one subject is %q, want the amended message", got)
	}
	if f.git("rev-parse", "layer-one") != linked.git("rev-parse", "HEAD^") {
		t.Errorf("the linked worktree was not rebased onto the amended parent")
	}
	if got := linked.git("show", "HEAD:layer-one.txt"); got != "one, revised" {
		t.Errorf("the linked worktree contains %q, want the amended content", got)
	}
	if got := linked.branch(); got != "layer-two" {
		t.Errorf("the linked worktree switched to %q, want layer-two", got)
	}
	if got := linked.git("status", "--porcelain"); got != "" {
		t.Errorf("the linked worktree is dirty after modify:\n%s", got)
	}
}

func TestRestackAndSyncAcrossWorktrees(t *testing.T) {
	for _, command := range []string{"restack", "sync"} {
		t.Run(command, func(t *testing.T) {
			f := newFixture(t)
			f.layer("layer-one", "Add layer one")
			f.layer("layer-two", "Add layer two")
			f.gt("trunk")
			linked := f.worktree("layer-two")

			f.write("trunk-moves.txt", "moved\n")
			f.git("add", "-A")
			f.git("commit", "--quiet", "-m", "Move trunk")
			f.git("push", "--quiet", "origin", "main")
			f.gt("checkout", "layer-one")

			linked.gt(command)

			if f.git("rev-parse", "main") != f.git("rev-parse", "HEAD^") {
				t.Errorf("layer-one in the other worktree was not rebased onto trunk")
			}
			if f.git("rev-parse", "HEAD") != linked.git("rev-parse", "HEAD^") {
				t.Errorf("layer-two was not rebased onto layer-one")
			}
			for _, tree := range []*fixture{f, linked} {
				if got := tree.git("status", "--porcelain"); got != "" {
					t.Errorf("worktree %s is dirty after %s:\n%s", tree.dir, command, got)
				}
			}
			if command == "sync" {
				for _, branch := range []string{"layer-one", "layer-two"} {
					remote := linked.git("ls-remote", "--heads", "origin", branch)
					want := linked.git("rev-parse", branch) + "\trefs/heads/" + branch
					if remote != want {
						t.Errorf("remote %s is %q, want %q", branch, remote, want)
					}
				}
			}
		})
	}
}

func TestRecoverRebaseAcrossWorktrees(t *testing.T) {
	for _, command := range []string{"continue", "abort"} {
		t.Run(command, func(t *testing.T) {
			f := newFixture(t)
			f.write("shared.txt", "one\n")
			f.gt("create", "layer-one", "-a", "-m", "Add layer one")
			f.write("shared.txt", "two\n")
			f.gt("create", "layer-two", "-a", "-m", "Add layer two")
			f.gt("down")
			linked := f.worktree("layer-two")
			before := linked.git("rev-parse", "HEAD")

			f.write("shared.txt", "conflicting\n")
			r := f.gtFails("modify", "-a", "-m", "Rewrite layer one")
			if !f.gitFileExists("gh-stack-rebase-state") {
				t.Fatalf("no shared recovery record after a conflicting rebase\n%s", r.output())
			}
			if got := linked.git("diff", "--name-only", "--diff-filter=U"); got != "shared.txt" {
				t.Fatalf("conflict in the linked worktree is %q, want shared.txt", got)
			}
			if command == "continue" {
				linked.write("shared.txt", "resolved\n")
				linked.git("add", "shared.txt")
			}

			// Recovery starts outside the worktree that holds the conflict.
			f.gt(command)

			if f.gitFileExists("gh-stack-rebase-state") {
				t.Errorf("the shared recovery record survived %s", command)
			}
			if got := linked.branch(); got != "layer-two" {
				t.Errorf("the linked worktree is on %q after %s, want layer-two", got, command)
			}
			if got := linked.git("status", "--porcelain"); got != "" {
				t.Errorf("the linked worktree is dirty after %s:\n%s", command, got)
			}
			if command == "abort" {
				if got := linked.git("rev-parse", "HEAD"); got != before {
					t.Errorf("abort restored layer-two to %s, want %s", got, before)
				}
			} else {
				if f.git("rev-parse", "layer-one") != linked.git("rev-parse", "HEAD^") {
					t.Errorf("continue did not rebase layer-two onto layer-one")
				}
				if got := linked.git("show", "HEAD:shared.txt"); got != "resolved" {
					t.Errorf("continue committed %q, want the conflict resolution", got)
				}
			}
		})
	}
}
