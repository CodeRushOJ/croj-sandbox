package release

import (
	"os"
	"strings"
	"testing"
)

func TestGoDependenciesKeepPublishedSecurityFloors(t *testing.T) {
	moduleBytes, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	module := string(moduleBytes)
	for _, required := range []string{
		"golang.org/x/net v0.56.0",
		"golang.org/x/sys v0.46.0",
		"golang.org/x/text v0.39.0",
	} {
		if !strings.Contains(module, required) {
			t.Errorf("go.mod is missing security floor %q", required)
		}
	}
}
