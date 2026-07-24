package security

import (
	"path/filepath"
	"testing"
)

func TestResolveUnifiedCgroupPathKeepsJobsInsidePodCgroup(t *testing.T) {
	root := t.TempDir()
	resolved, err := resolveUnifiedCgroupPath(
		root,
		[]byte("0::/kubepods.slice/pod-123/container-456.scope\n"),
	)
	if err != nil {
		t.Fatalf("resolve cgroup path: %v", err)
	}
	want := filepath.Join(root, "kubepods.slice", "pod-123", "container-456.scope")
	if resolved != want {
		t.Fatalf("path = %q, want %q", resolved, want)
	}
}

func TestResolveUnifiedCgroupPathRejectsNonUnifiedOrEscapingInput(t *testing.T) {
	root := t.TempDir()
	for _, input := range []string{
		"2:memory:/docker/example\n",
		"0::/../../host\n",
		"",
	} {
		if _, err := resolveUnifiedCgroupPath(root, []byte(input)); err == nil {
			t.Fatalf("input %q unexpectedly accepted", input)
		}
	}
}
