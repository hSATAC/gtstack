//go:build integration

package integration

import "testing"

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
