package cloud

import (
	"net/http"
	"testing"
)

func TestSelfHostedCommandActionAllowsOnlyMappedReads(t *testing.T) {
	for _, item := range []struct{ path, action string }{
		{"/container/xcloud-r0123456789abcdef/logs?tail=300", "logs"},
		{"/container/xcloud-r0123456789abcdef/files?path=workspace", "files"},
		{"/container/xcloud-r0123456789abcdef/files/content?path=workspace/a.txt", "file-read"},
		{"/container/xcloud-r0123456789abcdef/operation-status?operationId=aop_12345678", "operation-status"},
	} {
		got, err := selfHostedCommandAction(http.MethodGet, item.path)
		if err != nil || got != item.action {
			t.Fatalf("GET %s = %q, %v; want %q", item.path, got, err, item.action)
		}
	}
	if _, err := selfHostedCommandAction(http.MethodGet, "/container/xcloud-r0123456789abcdef/unsafe"); err == nil {
		t.Fatal("unmapped GET must be rejected")
	}
}
