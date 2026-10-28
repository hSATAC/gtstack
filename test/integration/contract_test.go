//go:build integration

package integration

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var updateGolden = flag.Bool("update", false, "rewrite the help snapshots in testdata")

// ghStackCalls is every native command gt can run, transcribed from cmd/gt.
// Placeholder branch names stand in for the ones gt computes.
//
// Appending --help makes each call parse its flags and stop, which is the only
// way to check `submit` and `merge`: they need the GitHub API, so running them
// for real would mean opening and merging pull requests.
var ghStackCalls = [][]string{
	{"stack", "init", "--base", "main", "a-branch"},
	{"stack", "add", "a-branch"},
	{"stack", "rebase"},
	{"stack", "rebase", "--upstack"},
	{"stack", "rebase", "--downstack"},
	{"stack", "rebase", "--upstack", "--no-trunk"},
	{"stack", "rebase", "--accept-base-change"},
	{"stack", "rebase", "--downstack", "--accept-base-change"},
	{"stack", "rebase", "--upstack", "--accept-base-change"},
	{"stack", "rebase", "--continue"},
	{"stack", "rebase", "--abort"},
	{"stack", "modify", "--continue"},
	{"stack", "modify", "--abort"},
	{"stack", "submit"},
	{"stack", "submit", "--auto"},
	{"stack", "submit", "--open"},
	{"stack", "submit", "--auto", "--open"},
	{"stack", "submit", "--auto", "--no-atomic=true"},
	{"stack", "submit", "--auto", "--no-atomic=false"},
	{"stack", "submit", "--open", "--no-atomic=true"},
	{"stack", "sync"},
	{"stack", "sync", "--prune"},
	{"stack", "sync", "--accept-base-change"},
	{"stack", "sync", "--no-atomic=true"},
	{"stack", "sync", "--no-atomic=false"},
	{"stack", "sync", "--prune", "--accept-base-change", "--no-atomic=true"},
	{"stack", "view"},
	{"stack", "view", "--short"},
	{"stack", "view", "--json"},
	{"stack", "checkout", "a-branch"},
	{"stack", "checkout", "a-branch", "--print-path"},
	{"stack", "switch"},
	{"stack", "up"},
	{"stack", "down"},
	{"stack", "top"},
	{"stack", "bottom"},
	{"stack", "trunk"},
	{"stack", "trunk", "--print-path"},
	{"stack", "merge"},
	// gt offers to install the extension when `gh stack` does not resolve.
	{"extension", "install", "github/gh-stack"},
	// gt pr is the one command that does not go through the extension.
	{"pr", "view", "--web"},
}

// TestGhStackSurface fails when a subcommand or a flag gt passes disappears or
// is renamed. It says nothing about what those flags now mean; the help
// snapshots cover that.
func TestGhStackSurface(t *testing.T) {
	f := newFixture(t)
	for _, call := range ghStackCalls {
		args := append(append([]string{}, call...), "--help")
		if r := f.run("gh", args...); r.code != 0 {
			t.Errorf("`gh %s` exited %d; gt still runs this command\n%s",
				strings.Join(call, " "), r.code, r.output())
		}
	}
}

// helpSubcommands are the parts of gh stack gt drives. The empty entry is the
// top-level help, which lists the subcommands.
var helpSubcommands = []string{
	"", "init", "add", "rebase", "submit", "sync", "view",
	"checkout", "switch", "up", "down", "top", "bottom", "trunk", "modify", "merge",
}

// TestHelpSnapshots pins the documented behaviour, not just the flag names.
// Much of what gt depends on is prose: that `--auto` creates drafts, that
// submit covers the whole stack, that `--no-trunk` skips the fetch. A snapshot
// diff is the only cheap way to notice those changing, and it is the only
// signal at all for the commands that need the GitHub API.
//
// When a diff turns out to be harmless, re-record it:
//
//	go test -tags=integration ./test/integration/ -run TestHelpSnapshots -update
func TestHelpSnapshots(t *testing.T) {
	f := newFixture(t)
	for _, sub := range helpSubcommands {
		name, args := "stack", []string{"stack", "--help"}
		if sub != "" {
			name, args = "stack-"+sub, []string{"stack", sub, "--help"}
		}
		t.Run(name, func(t *testing.T) {
			r := f.run("gh", args...)
			if r.code != 0 {
				t.Fatalf("`gh %s` exited %d\n%s", strings.Join(args, " "), r.code, r.output())
			}
			got := normalizeHelp(r.output())
			path := filepath.Join("testdata", "help", name+".txt")

			if *updateGolden {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v\nrecord it with: go test -tags=integration ./test/integration/ -run TestHelpSnapshots -update", err)
			}
			if got != string(want) {
				t.Errorf("`gh %s` no longer matches %s\n%s\n"+
					"Check whether gt still translates correctly, then re-record with -update.",
					strings.Join(args, " "), path, firstDifference(string(want), got))
			}
		})
	}
}

// normalizeHelp removes the differences that are not worth a failure: trailing
// spaces on a line, and a missing or doubled final newline.
func normalizeHelp(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

// firstDifference describes where two texts diverge, with enough context to
// read in a CI log or an issue.
func firstDifference(want, got string) string {
	wantLines, gotLines := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(wantLines) || i < len(gotLines); i++ {
		w, g := at(wantLines, i), at(gotLines, i)
		if w == g {
			continue
		}
		var b strings.Builder
		fmt.Fprintf(&b, "first difference at line %d:\n", i+1)
		for j := max(0, i-3); j < i; j++ {
			fmt.Fprintf(&b, "   %s\n", at(wantLines, j))
		}
		fmt.Fprintf(&b, "  -%s\n  +%s\n", w, g)
		fmt.Fprintf(&b, "(%d lines recorded, %d now)", len(wantLines), len(gotLines))
		return b.String()
	}
	return "the texts differ only in whitespace"
}

func at(lines []string, i int) string {
	if i < 0 || i >= len(lines) {
		return "<end of output>"
	}
	return lines[i]
}

// TestStateFileContract covers what gt reads out of `.git/gh-stack` to decide
// whether `gt create` starts a stack or extends one, and whether a branch sits
// at a fork. A schema bump makes gt refuse to run at all, by design.
func TestStateFileContract(t *testing.T) {
	f := newFixture(t)
	f.layer("layer-one", "Add layer one")
	f.layer("layer-two", "Add layer two")

	dir := f.git("rev-parse", "--path-format=absolute", "--git-common-dir")
	if _, err := os.Stat(filepath.Join(dir, "gh-stack")); err != nil {
		t.Fatalf("gh stack no longer keeps its state at .git/gh-stack: %v", err)
	}
	st := f.state()
	if st.SchemaVersion != 1 {
		t.Fatalf("state schema is v%d, but gt only understands v1 and refuses to run against anything else", st.SchemaVersion)
	}
	if len(st.Stacks) != 1 {
		t.Fatalf("state records %d stacks, want 1: %+v", len(st.Stacks), st)
	}
	if got := st.Stacks[0].Trunk.Branch; got != "main" {
		t.Errorf("stacks[0].trunk.branch is %q, want main", got)
	}
	// Order matters: gt reads position in this list as position in the stack.
	if got := f.tracked(); len(got) != 2 || got[0] != "layer-one" || got[1] != "layer-two" {
		t.Errorf("stacks[0].branches is %q, want [layer-one layer-two] bottom to top", got)
	}
}

// TestViewJSONContract: gt sends people to `gh stack view --json` for anything
// it cannot translate, and promises the output is pipeable, so gt's own
// announcements must stay off stdout.
func TestViewJSONContract(t *testing.T) {
	f := newFixture(t)
	f.layer("layer-one", "Add layer one")
	f.layer("layer-two", "Add layer two")

	r := f.gt("log", "--json")
	var view struct {
		Trunk         string `json:"trunk"`
		CurrentBranch string `json:"currentBranch"`
		Branches      []struct {
			Name string `json:"name"`
		} `json:"branches"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &view); err != nil {
		t.Fatalf("`gt log --json` did not produce JSON on stdout: %v\nstdout: %s\nstderr: %s", err, r.stdout, r.stderr)
	}
	if view.Trunk != "main" {
		t.Errorf("trunk is %q, want main", view.Trunk)
	}
	if view.CurrentBranch != "layer-two" {
		t.Errorf("currentBranch is %q, want layer-two", view.CurrentBranch)
	}
	if len(view.Branches) != 2 || view.Branches[0].Name != "layer-one" || view.Branches[1].Name != "layer-two" {
		t.Errorf("branches are %+v, want layer-one then layer-two", view.Branches)
	}
}

// TestGeneratedBranchNameMatchesGhStack: gt makes the commit itself, so it
// never lets `gh stack add -m` name the branch. It reimplements the naming
// rule instead, and these are the cases cmd/gt's unit tests pin on gt's side.
func TestGeneratedBranchNameMatchesGhStack(t *testing.T) {
	f := newFixture(t)
	f.layer("first-layer", "Add the first layer")

	cases := []struct {
		message, slug string
	}{
		{"Add the login form", "add_the_login_form"},
		{"Fix   multiple   spaces", "fix_multiple_spaces"},
		{"v2 API", "v2_api"},
		{"trailing!!!", "trailing"},
	}
	for i, tc := range cases {
		f.write(fmt.Sprintf("naming-%d.txt", i), tc.message+"\n")
		f.git("add", "-A")
		before := time.Now()
		if r := f.run("gh", "stack", "add", "-m", tc.message); r.code != 0 {
			t.Fatalf("`gh stack add -m %q` exited %d\n%s", tc.message, r.code, r.output())
		}

		got, matched := f.branch(), false
		// A run that straddles midnight would otherwise be flaky.
		for _, at := range []time.Time{before, time.Now()} {
			matched = matched || got == at.Format("01-02")+"-"+tc.slug
		}
		if !matched {
			t.Errorf("`gh stack add -m %q` named the branch %q, want %s-%s; gt would have picked the second",
				tc.message, got, before.Format("01-02"), tc.slug)
		}
	}
}

// TestAddLeavesTheIndexAlone: gt stages the work, asks gh stack for the branch
// alone, then commits with git. That sequence only works while a bare
// `gh stack add` keeps its hands off the index.
func TestAddLeavesTheIndexAlone(t *testing.T) {
	f := newFixture(t)
	f.layer("first-layer", "Add the first layer")

	f.write("staged.txt", "staged\n")
	f.git("add", "-A")
	if r := f.run("gh", "stack", "add", "second-layer"); r.code != 0 {
		t.Fatalf("`gh stack add second-layer` exited %d\n%s", r.code, r.output())
	}

	if got := f.branch(); got != "second-layer" {
		t.Fatalf("on branch %q, want second-layer", got)
	}
	if got := f.git("diff", "--cached", "--name-only"); got != "staged.txt" {
		t.Errorf("staged files are %q after the add, want staged.txt still waiting", got)
	}
	if got := f.commitsIn("first-layer..second-layer"); got != 0 {
		t.Errorf("gh stack add made %d commits of its own, want 0", got)
	}
}

// TestAddWithMessageOnEmptyBranch records the quirk that made gt stage and
// commit for itself: on a branch carrying no commits, `gh stack add -m` puts
// the commit on that branch instead of creating a new one. If this ever
// changes, gt's workaround stays correct -- but it is worth being told.
func TestAddWithMessageOnEmptyBranch(t *testing.T) {
	f := newFixture(t)
	f.gt("create", "empty-layer")

	f.write("work.txt", "work\n")
	f.git("add", "-A")
	if r := f.run("gh", "stack", "add", "-m", "Work on the empty layer"); r.code != 0 {
		t.Fatalf("`gh stack add -m` exited %d\n%s", r.code, r.output())
	}

	if got := f.branch(); got != "empty-layer" {
		t.Errorf("`gh stack add -m` on an empty branch moved to %q; it used to commit in place on empty-layer", got)
	}
	if got := f.commitsIn("main..empty-layer"); got != 1 {
		t.Errorf("empty-layer has %d commits over main, want 1", got)
	}
}
