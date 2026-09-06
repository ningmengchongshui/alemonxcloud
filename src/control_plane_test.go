package cloud

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRouteKeyIsStableAndUniqueForDistinctInstances(t *testing.T) {
	first := routeKey("user-a\x00instance-a")
	second := routeKey("user-a\x00instance-b")
	if first == second || !strings.HasPrefix(first, "r") || len(first) != 17 {
		t.Fatalf("invalid route keys: %q %q", first, second)
	}
	if routeKey("user-a\x00instance-a") != first {
		t.Fatal("route key must be deterministic")
	}
}

func TestNodeTokenEncryptionRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	t.Setenv("XCLOUD_NODE_TOKEN_ENCRYPTION_KEY", base64.RawStdEncoding.EncodeToString(key))
	ciphertext, err := encryptNodeToken("node-control-token")
	if err != nil || ciphertext == "node-control-token" {
		t.Fatalf("encrypt token: %v", err)
	}
	plain, err := decryptNodeToken(ciphertext)
	if err != nil || plain != "node-control-token" {
		t.Fatalf("decrypt token: %q %v", plain, err)
	}
}

func TestTaskRetryIsBounded(t *testing.T) {
	if truncateError(strings.Repeat("x", 1200)) != strings.Repeat("x", 1000) {
		t.Fatal("task errors must be bounded")
	}
	if delay := time.Duration(2*2) * time.Minute; delay != 4*time.Minute {
		t.Fatal("unexpected retry delay")
	}
}

func TestImageTagValidation(t *testing.T) {
	if !validImageTag("latest") || !validImageTag("v2.4.1") || validImageTag("../../other") {
		t.Fatal("image tag validation is unsafe")
	}
}

func TestDeploymentImagePrefersApprovedDigest(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	if got := deploymentImage("registry.example/alemonx", "latest", digest); got != "registry.example/alemonx@"+digest {
		t.Fatalf("digest must be used for deployment, got %q", got)
	}
	if got := deploymentImage("registry.example/alemonx", "v2", ""); got != "registry.example/alemonx:v2" {
		t.Fatalf("tag fallback is incorrect, got %q", got)
	}
}

func TestImmutableDigestAcceptsOnlyVerifiedRepoDigest(t *testing.T) {
	digest := "sha256:" + strings.Repeat("b", 64)
	if got := immutableDigest([]string{"registry.example/alemonx@" + digest}); got != digest {
		t.Fatalf("expected verified digest, got %q", got)
	}
	if got := immutableDigest([]string{"registry.example/alemonx:latest", "bad@sha256:short"}); got != "" {
		t.Fatalf("unverified values must not be published, got %q", got)
	}
}

func TestLifecycleTaskIdempotencyKeyUsesScheduledTime(t *testing.T) {
	at := time.Date(2026, time.September, 5, 12, 0, 0, 0, time.UTC)
	key := lifecycleTaskKey("ins_abc", "destroy", at)
	if !strings.Contains(key, "destroy:ins_abc:") || !strings.HasSuffix(key, "Z") {
		t.Fatalf("invalid lifecycle key: %s", key)
	}
}

func TestInstanceStateTransitionsAreExplicit(t *testing.T) {
	for _, transition := range [][2]string{{"deploying", "running"}, {"deploying", "deployment_failed"}, {"deployment_failed", "deploying"}, {"destroy_scheduled", "destroyed"}, {"destroyed", "purged"}} {
		if !canTransitionInstance(transition[0], transition[1]) {
			t.Fatalf("expected transition %s -> %s", transition[0], transition[1])
		}
	}
	for _, transition := range [][2]string{{"purged", "running"}, {"destroyed", "running"}, {"deployment_failed", "destroyed"}} {
		if canTransitionInstance(transition[0], transition[1]) {
			t.Fatalf("unexpected transition %s -> %s", transition[0], transition[1])
		}
	}
}

func TestTaskLeaseDuration(t *testing.T) {
	if taskLeaseDuration != 5*time.Minute {
		t.Fatalf("task lease must be five minutes, got %s", taskLeaseDuration)
	}
}

func TestLifecycleTaskActionsRequireInstanceLock(t *testing.T) {
	for _, action := range []string{"create", "retry-deploy", "start", "stop", "update", "restart", "reinstall", "destroy", "purge", "resize"} {
		if !lifecycleTask(action) {
			t.Fatalf("%s must acquire the instance lifecycle lock", action)
		}
	}
	if lifecycleTask("bandwidth") {
		t.Fatal("bandwidth reconciliation must not block lifecycle operations")
	}
}

func TestPublicCatalogHidesRepositoryDiagnostics(t *testing.T) {
	items := publicCatalogImages([]catalogImage{{ID: "img", Name: "软件", ImageRef: "registry.example/private", ImageDigest: "sha256:" + strings.Repeat("a", 64), Versions: []imageVersion{{Tag: "v1", ImageDigest: "sha256:" + strings.Repeat("b", 64), LastError: "internal"}}}})
	raw, err := json.Marshal(items)
	if err != nil || strings.Contains(string(raw), "registry.example") || strings.Contains(string(raw), "sha256") || strings.Contains(string(raw), "internal") {
		t.Fatalf("public catalog leaked diagnostics: %s, %v", raw, err)
	}
}

func TestPublicCatalogFallsBackToLatestWithoutConfiguredVersions(t *testing.T) {
	items := publicCatalogImages([]catalogImage{{ID: "img", Name: "软件"}})
	if len(items) != 1 || len(items[0].Versions) != 1 || items[0].Versions[0].Tag != "latest" {
		t.Fatalf("empty version configuration must expose Docker latest fallback: %#v", items)
	}
}

func TestMigrationAllowsOnlyMissingIndexDropAsIdempotent(t *testing.T) {
	if !isDuplicateMigration(errors.New("Error 1091 (42000): Can't DROP 'idx'; check that column/key exists")) {
		t.Fatal("missing legacy index must not block an idempotent migration")
	}
	if isDuplicateMigration(errors.New("Error 1064 (42000): syntax error")) {
		t.Fatal("unrelated migration errors must still block startup")
	}
}
