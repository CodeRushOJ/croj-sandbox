package release

import (
	"os"
	"strings"
	"testing"
)

func TestSignedTagPublishesAuditableMultiArchitectureImage(t *testing.T) {
	workflowBytes, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(workflowBytes)
	for _, required := range []string{
		`tags: ["v*"]`,
		`if: ${{ github.ref_type == 'tag' }}`,
		`packages: write`,
		`test "$(git rev-parse "$GITHUB_REF_NAME^{commit}")" = "$GITHUB_SHA"`,
		`platforms: linux/amd64,linux/arm64`,
		`ghcr.io/coderushoj/croj-sandbox:${{ github.ref_name }}`,
		`ghcr.io/coderushoj/croj-sandbox:sha-${{ github.sha }}`,
		`provenance: mode=max`,
		`sbom: true`,
		`id: push`,
		`IMAGE_DIGEST: ${{ steps.push.outputs.digest }}`,
		`sandbox-image.json`,
		`name: sandbox-image-${{ github.sha }}`,
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("release workflow is missing %q", required)
		}
	}
}
