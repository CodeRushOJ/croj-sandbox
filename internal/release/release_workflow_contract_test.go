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

func TestReleaseCheckoutPreservesAnnotatedTagObject(t *testing.T) {
	workflowBytes, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(workflowBytes)
	publishJobStart := strings.Index(workflow, "\n  publish-image:")
	if publishJobStart == -1 {
		t.Fatal("release workflow is missing the publish-image job")
	}
	testJob := workflow[:publishJobStart]
	publishJob := workflow[publishJobStart:]
	const pinnedCheckoutV6 = `uses: actions/checkout@df4cb1c069e1874edd31b4311f1884172cec0e10 # v6`

	for _, required := range []string{
		pinnedCheckoutV6,
		`git rev-parse "$GITHUB_REF_NAME^{tag}" >/dev/null`,
		`test "$(git rev-parse "$GITHUB_REF_NAME^{commit}")" = "$GITHUB_SHA"`,
		`git fetch --no-tags origin main`,
		`test "$GITHUB_SHA" = "$(git rev-parse origin/main)"`,
	} {
		if !strings.Contains(publishJob, required) {
			t.Errorf("publish-image job is missing %q", required)
		}
	}
	if !strings.Contains(testJob, pinnedCheckoutV6) {
		t.Errorf("test and publish-image jobs must use the same pinned checkout v6 action")
	}
}

func TestPublishJobGrantsPermissionsRequiredForAttestedImages(t *testing.T) {
	workflowBytes, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(workflowBytes)
	publishJobStart := strings.Index(workflow, "\n  publish-image:")
	if publishJobStart == -1 {
		t.Fatal("release workflow is missing the publish-image job")
	}
	publishJob := workflow[publishJobStart:]
	permissionsStart := strings.Index(publishJob, "\n    permissions:")
	stepsStart := strings.Index(publishJob, "\n    steps:")
	if permissionsStart == -1 || stepsStart == -1 || permissionsStart >= stepsStart {
		t.Fatal("publish-image job must define scoped permissions before its steps")
	}
	permissions := publishJob[permissionsStart:stepsStart]

	for _, required := range []string{
		"contents: read",
		"packages: write",
		"id-token: write",
		"attestations: write",
	} {
		if !strings.Contains(permissions, required) {
			t.Errorf("publish-image permissions are missing %q", required)
		}
	}
}
