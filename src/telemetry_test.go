package cloud

import "testing"

func TestConsoleTelemetryValidation(t *testing.T) {
	for _, event := range []string{"page_view", "create_service", "instance_action", "order_filter", "renew_order", "admin_action"} {
		if !allowedConsoleEvent(event) {
			t.Fatalf("expected %q to be allowed", event)
		}
	}
	for _, page := range []string{"promotions", "settings"} {
		if !allowedConsolePage(page) {
			t.Fatalf("expected page %q to be allowed", page)
		}
	}
	if allowedConsoleEvent("free_form_event") {
		t.Fatal("unexpected event accepted")
	}
	for _, value := range []string{"retry_task", "destroy-now", "cancel-destroy", "all", "success", ""} {
		if !safeTelemetryText(value) {
			t.Fatalf("expected %q to be safe", value)
		}
	}
	for _, value := range []string{"instance-123", "user@example.com", "hello world", "../secret"} {
		if safeTelemetryText(value) {
			t.Fatalf("unexpected telemetry value accepted: %q", value)
		}
	}
}
