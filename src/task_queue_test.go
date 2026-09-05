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

func TestExpiredDangerousLifecycleTasksAreQuarantined(t *testing.T) {
	for _, action := range []string{"stop", "update", "restart", "destroy", "purge", "retry-deploy"} {
		if !dangerousRecoveredTask(action) {
			t.Fatalf("%s must require administrator review after lease expiry", action)
		}
	}
	for _, action := range []string{"create", "start", "bandwidth"} {
		if dangerousRecoveredTask(action) {
			t.Fatalf("%s is not an automatically quarantined dangerous action", action)
		}
	}
}
