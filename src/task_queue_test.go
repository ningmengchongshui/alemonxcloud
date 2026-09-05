package cloud

import "testing"

func TestTaskWorkerConcurrencyUsesBoundedConfiguration(t *testing.T) {
	t.Setenv("XCLOUD_TASK_WORKERS", "6")
	if got := taskWorkerConcurrency(); got != 6 {
		t.Fatalf("workers = %d, want 6", got)
	}
	t.Setenv("XCLOUD_TASK_WORKERS", "99")
	if got := taskWorkerConcurrency(); got != 4 {
		t.Fatalf("invalid worker count = %d, want fallback 4", got)
	}
}
