package release

import (
	"os"
	"strings"
	"testing"
)

func TestAnnotatedTagPublishesOidcAttestedMultiArchitectureImage(t *testing.T) {
	workflowBytes, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(workflowBytes)
	for _, required := range []string{
		`tags: ["v*"]`,
		`if: ${{ github.ref_type == 'tag' }}`,
		`packages: write`,
		`id-token: write`,
		`test "$(git rev-parse "$GITHUB_REF_NAME^{commit}")" = "$GITHUB_SHA"`,
		`git fetch --no-tags origin main`,
		`test "$GITHUB_SHA" = "$(git rev-parse origin/main)"`,
		`docker/setup-qemu-action@`,
		`actions/attest-build-provenance@`,
		`subject-name: ghcr.io/coderushoj/croj-sandbox`,
		`subject-digest: ${{ steps.push.outputs.digest }}`,
		`push-to-registry: true`,
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
	if strings.Contains(workflow, ".verification.verified") {
		t.Error("release workflow must use keyless OIDC provenance instead of requiring a local tag signing key")
	}
}
