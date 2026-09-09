package agentcore

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComposeKeepsManagedBoundaryAndWorkspace(t *testing.T) {
	compose, err := Compose(ComposeInput{Name: "xcloud-r0123456789abcdef", Image: "registry.example/app@sha256:abc", Route: "r0123456789abcdef", DataDir: "/data", WorkspaceDir: "/workspace", CPU: 2, MemoryMB: 2048})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"xcloud.managed: \"true\"", "xcloud.route: \"r0123456789abcdef\"", "/workspace:/app/workspace", "mem_limit: \"2048m\""} {
		if !strings.Contains(compose, want) {
			t.Fatalf("compose missing %s", want)
		}
	}
	if strings.Contains(compose, "bandwidth") || strings.Contains(compose, "Mbps") {
		t.Fatal("managed Compose must not contain traffic-control metadata")
	}
}
func TestWorkspacePathRejectsTraversal(t *testing.T) {
	if _, _, err := WorkspacePath("/safe", "../../etc/passwd"); err == nil {
		t.Fatal("expected traversal rejection")
	}
}

func TestWorkspaceOperationsRejectSymlinksAndKeepRelativePaths(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "nested"), 0750); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteWorkspaceText(root, "nested/readme.txt", "你好 xCloud"); err != nil {
		t.Fatal(err)
	}
	content, size, _, err := ReadWorkspaceText(root, "nested/readme.txt")
	if err != nil || content != "你好 xCloud" || size == 0 {
		t.Fatalf("read workspace = %q, %d, %v", content, size, err)
	}
	encoded := base64.StdEncoding.EncodeToString([]byte("binary"))
	if path, size, err := UploadWorkspaceBase64(root, "nested/file.bin", encoded); err != nil || path != "nested/file.bin" || size != 6 {
		t.Fatalf("upload workspace = %q, %d, %v", path, size, err)
	}
	if err := os.Symlink("/etc", filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := ReadWorkspaceText(root, "escape/passwd"); err == nil {
		t.Fatal("expected symlink rejection")
	}
	_, entries, err := ListWorkspace(root, "")
	if err != nil || len(entries) != 1 || entries[0].Name != "nested" {
		t.Fatalf("list workspace = %#v, %v", entries, err)
	}
}

func TestTerminalDockerArgsAreFixed(t *testing.T) {
	args, err := TerminalDockerArgs("xcloud-r0123456789abcdef")
	if err != nil || !strings.Contains(strings.Join(args, " "), "exec /bin/sh -i") {
		t.Fatalf("terminal args = %v, %v", args, err)
	}
	if _, err := TerminalDockerArgs("other-container"); err == nil {
		t.Fatal("expected unmanaged container rejection")
	}
}

func TestManagedNameIsTheSingleBoundaryRule(t *testing.T) {
	for _, name := range []string{"xcloud-01234567", "xcloud-a1b2c3d4e5f6a7b8"} {
		if !ValidName(name) {
			t.Fatalf("expected managed name %q to be valid", name)
		}
	}
	for _, name := range []string{"xcloud-short", "xcloud-abc-def12", "xcloud-ABCDEF12", "other-01234567"} {
		if ValidName(name) {
			t.Fatalf("expected managed name %q to be invalid", name)
		}
	}
}
