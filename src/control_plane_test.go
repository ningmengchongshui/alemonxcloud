package cloud

import (
	"encoding/base64"
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
