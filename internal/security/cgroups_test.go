package security

import "testing"

func TestCgroupIDForProcessIncludesSanitizedInstance(t *testing.T) {
	t.Setenv("CROJ_SANDBOX_INSTANCE_ID", "pod/123 unsafe")

	got := CgroupIDForProcess("execute", 42)
	want := "execute_pod_123_unsafe_42"
	if got != want {
		t.Fatalf("CgroupIDForProcess() = %q, want %q", got, want)
	}
}
