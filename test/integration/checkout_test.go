//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckoutPrintPath(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target string
		args   []string
	}{
		{"tracked branch", "layer-one", []string{"checkout", "layer-one", "--print-path"}},
		{"untracked branch", "scratch", []string{"co", "scratch", "--print-path"}},
		{"named trunk", "main", []string{"checkout", "main", "--print-path"}},
		{"trunk flag", "main", []string{"co", "-t", "--print-path"}},
	} {
		for _, heldElsewhere := range []bool{false, true} {
			location := "current worktree"
			if heldElsewhere {
				location = "other worktree"
			}
			t.Run(tc.name+"/"+location, func(t *testing.T) {
				f := newFixture(t)
				f.layer("layer-one", "Add layer one")
				f.layer("layer-two", "Add layer two")
				f.git("branch", "scratch")
				owner := f
				wantBranch := tc.target
				if heldElsewhere {
					owner = f.worktree(tc.target)
					wantBranch = "layer-two"
				}
				wantPath := owner.git("rev-parse", "--show-toplevel")
				before := owner.git("rev-parse", tc.target)

				r := f.gt(tc.args...)

				if r.stdout != wantPath+"\n" {
					t.Errorf("stdout is %q, want only %q\n%s", r.stdout, wantPath+"\n", r.stderr)
				}
				if got := f.branch(); got != wantBranch {
					t.Errorf("current worktree is on %q, want %q", got, wantBranch)
				}
				if got := owner.branch(); got != tc.target {
					t.Errorf("target worktree is on %q, want %q", got, tc.target)
				}
				if got := owner.git("rev-parse", "HEAD"); got != before {
					t.Errorf("checkout changed the target commit from %s to %s", before, got)
				}
			})
		}
	}
}

func TestCheckoutPrintPathFromSubdirectory(t *testing.T) {
	// An untracked branch works even before the repository has a stack.
	f := newFixture(t)
	dir := filepath.Join(f.dir, "nested directory")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	nested := &fixture{t: t, dir: dir, origin: f.origin}
	r := nested.gt("checkout", "main", "--print-path")
	if want := f.git("rev-parse", "--show-toplevel") + "\n"; r.stdout != want {
		t.Errorf("stdout is %q, want the worktree root %q", r.stdout, want)
	}
}

func TestCheckoutPrintPathPreservesPathWhitespace(t *testing.T) {
	f := newFixture(t)
	f.git("branch", "scratch")
	dir := filepath.Join(t.TempDir(), "linked worktree ")
	f.git("worktree", "add", "--quiet", dir, "scratch")
	path, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	linked := &fixture{t: t, dir: dir, origin: f.origin}
	for _, caller := range []*fixture{f, linked} {
		r := caller.gt("co", "scratch", "--print-path")
		if r.stdout != path+"\n" {
			t.Errorf("stdout is %q, want the exact path %q", r.stdout, path+"\n")
		}
	}
}

func TestCheckoutPrintPathRejectsInvalidTargets(t *testing.T) {
	f := newFixture(t)
	f.layer("layer-one", "Add layer one")
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"missing target", []string{"checkout", "--print-path"}, "requires a target or --trunk"},
		{"extra target", []string{"checkout", "main", "layer-one", "--print-path"}, "at most one target"},
		{"conflicting target", []string{"co", "-t", "layer-one", "--print-path"}, "--trunk cannot be combined with a target"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := f.gtFails(tc.args...)
			if !strings.Contains(r.stderr, tc.want) {
				t.Errorf("expected %q in error\n%s", tc.want, r.output())
			}
			if r.stdout != "" {
				t.Errorf("failed checkout printed a path: %q", r.stdout)
			}
			if got := f.branch(); got != "layer-one" {
				t.Errorf("invalid target switched to %q", got)
			}
		})
	}
}

func TestCheckoutPrintPathPreservesDirtyWorktree(t *testing.T) {
	for _, target := range []string{"layer-one", "scratch"} {
		t.Run(target, func(t *testing.T) {
			f := newFixture(t)
			f.layer("layer-one", "Add layer one")
			f.git("branch", "scratch")
			f.layer("layer-two", "Add layer two")
			f.write("layer-two.txt", "uncommitted\n")
			before := f.git("diff")

			r := f.gtFails("checkout", target, "--print-path")

			if r.stdout != "" {
				t.Errorf("failed checkout printed a path: %q", r.stdout)
			}
			if got := f.branch(); got != "layer-two" {
				t.Errorf("failed checkout switched to %q", got)
			}
			if got := f.git("diff"); got != before {
				t.Errorf("checkout changed uncommitted work:\n%s", got)
			}
		})
	}
}

func TestCheckoutPrintPathRejectsUnavailableWorktree(t *testing.T) {
	f := newFixture(t)
	f.git("branch", "scratch")
	linked := f.worktree("scratch")
	// Move it without git worktree move to leave a stale registration.
	if err := os.Rename(linked.dir, linked.dir+" moved"); err != nil {
		t.Fatal(err)
	}
	r := f.gtFails("checkout", "scratch", "--print-path")
	if r.stdout != "" {
		t.Errorf("unavailable worktree produced a path: %q", r.stdout)
	}
}
