package cloud

import (
	"strings"
	"testing"
)

func TestTicketValidation(t *testing.T) {
	if !validTicketCategory("instance") || validTicketCategory("arbitrary") {
		t.Fatal("ticket category validation is unsafe")
	}
	if !validTicketPriority("urgent") || validTicketPriority("critical") {
		t.Fatal("ticket priority validation is unsafe")
	}
	if !validTicketStatus(ticketOpen) || validTicketStatus("deleted") {
		t.Fatal("ticket status validation is unsafe")
	}
}

func TestTicketTextLimits(t *testing.T) {
	if _, err := normalizedTicketText("  ", 160, "主题"); err == nil {
		t.Fatal("blank ticket text must be rejected")
	}
	if _, err := normalizedTicketText(strings.Repeat("x", 161), 160, "主题"); err == nil {
		t.Fatal("overlong ticket subject must be rejected")
	}
	if value, err := normalizedTicketText("  已修复  ", 160, "主题"); err != nil || value != "已修复" {
		t.Fatalf("ticket text normalization failed: %q %v", value, err)
	}
}
