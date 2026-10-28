//go:build integration

package integration

import "testing"

func TestSyncAtomicPushOptions(t *testing.T) {
	for _, tc := range []struct {
		name   string
		env    string
		args   []string
		pushed bool
	}{
		{"atomic by default", "", nil, false},
		{"disabled by flag", "", []string{"--no-atomic"}, true},
		{"disabled by environment", "1", nil, true},
		{"flag overrides environment", "1", []string{"--no-atomic=false"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GH_STACK_NO_ATOMIC", tc.env)
			f := newFixture(t)
			f.layer("layer-one", "Add layer one")
			f.layer("layer-two", "Add layer two")
			f.git("--git-dir="+f.origin, "config", "receive.advertiseAtomic", "false")

			// Native sync reports push failures as warnings. Check the remote
			// refs rather than treating its exit code as proof of a push.
			r := f.gt(append([]string{"sync"}, tc.args...)...)
			for _, branch := range []string{"layer-one", "layer-two"} {
				want := ""
				if tc.pushed {
					want = f.git("rev-parse", branch) + "\trefs/heads/" + branch
				}
				if got := f.git("ls-remote", "--heads", "origin", branch); got != want {
					t.Errorf("remote %s is %q, want %q\n%s", branch, got, want, r.output())
				}
			}
		})
	}
}

// A local stack does not need base-change consent, but the option must reach
// the extension and preserve the ordinary rebase behavior.
func TestAcceptBaseChangeOnLocalStack(t *testing.T) {
	for _, command := range []string{"sync", "restack"} {
		t.Run(command, func(t *testing.T) {
			f := newFixture(t)
			f.layer("layer-one", "Add layer one")
			f.gt("trunk")
			f.write("trunk-moves.txt", "moved\n")
			f.git("add", "-A")
			f.git("commit", "--quiet", "-m", "Move trunk")
			f.git("push", "--quiet", "origin", "main")
			f.gt("checkout", "layer-one")

			f.gt(command, "--accept-base-change")

			if f.git("rev-parse", "main") != f.git("rev-parse", "layer-one^") {
				t.Errorf("%s did not rebase layer-one onto the moved trunk", command)
			}
		})
	}
}
