package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Record the subprocess boundary so these tests cover flag parsing as well
// as translation, without creating PRs or changing a real stack.
func TestNativeCommandFlags(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args")
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$GTSTACK_TEST_ARGS\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GTSTACK_TEST_ARGS", argsPath)
	// Unspecified flags must leave the native environment default alone.
	t.Setenv("GH_STACK_NO_ATOMIC", "1")

	tests := []struct {
		name string
		run  func([]string) error
		args []string
		want []string
	}{
		{"submit defaults", cmdSubmit, nil, []string{"stack", "submit", "--auto"}},
		{"submit no atomic", cmdSubmit, []string{"--no-atomic"}, []string{"stack", "submit", "--auto", "--no-atomic=true"}},
		{"submit atomic override", cmdSubmit, []string{"--no-atomic=false"}, []string{"stack", "submit", "--auto", "--no-atomic=false"}},
		{"submit editor and publish", cmdSubmit, []string{"-e", "-p", "--no-atomic"}, []string{"stack", "submit", "--open", "--no-atomic=true"}},
		{"sync defaults", cmdSync, nil, []string{"stack", "sync"}},
		{"sync accept base", cmdSync, []string{"--accept-base-change"}, []string{"stack", "sync", "--accept-base-change"}},
		{"sync no consent", cmdSync, []string{"--accept-base-change=false"}, []string{"stack", "sync"}},
		{"sync no atomic", cmdSync, []string{"--no-atomic"}, []string{"stack", "sync", "--no-atomic=true"}},
		{"sync atomic override", cmdSync, []string{"--no-atomic=false"}, []string{"stack", "sync", "--no-atomic=false"}},
		{"sync combined", cmdSync, []string{"-d", "--accept-base-change", "--no-atomic"}, []string{"stack", "sync", "--prune", "--accept-base-change", "--no-atomic=true"}},
		{"restack defaults", cmdRestack, nil, []string{"stack", "rebase"}},
		{"restack accept base", cmdRestack, []string{"--accept-base-change"}, []string{"stack", "rebase", "--accept-base-change"}},
		{"restack no consent", cmdRestack, []string{"--accept-base-change=false"}, []string{"stack", "rebase"}},
		{"restack downstack", cmdRestack, []string{"-d", "--accept-base-change"}, []string{"stack", "rebase", "--downstack", "--accept-base-change"}},
		{"restack upstack", cmdRestack, []string{"-u", "--accept-base-change"}, []string{"stack", "rebase", "--upstack", "--accept-base-change"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(tt.args); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(argsPath)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n"); !slices.Equal(got, tt.want) {
				t.Errorf("gh received %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSubmitArgs(t *testing.T) {
	tests := []struct {
		name          string
		edit, publish bool
		want          []string
	}{
		// The bare `gt submit` must not open the editor: Graphite's does not,
		// and matching that is the point of the command.
		{"default", false, false, []string{"stack", "submit", "--auto"}},
		{"publish", false, true, []string{"stack", "submit", "--auto", "--open"}},
		{"edit", true, false, []string{"stack", "submit"}},
		{"edit and publish", true, true, []string{"stack", "submit", "--open"}},
	}
	for _, tt := range tests {
		if got := submitArgs(tt.edit, tt.publish); !slices.Equal(got, tt.want) {
			t.Errorf("%s: submitArgs(%v, %v) = %q, want %q", tt.name, tt.edit, tt.publish, got, tt.want)
		}
	}
}
