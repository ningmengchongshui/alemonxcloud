package cloud

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// consoleEvent deliberately accepts only coarse, anonymous UX measurements.
// It must never receive resource IDs, account data, free-form text, logs, or credentials.
type consoleEvent struct {
	Event      string `json:"event"`
	Area       string `json:"area"`
	Page       string `json:"page"`
	Action     string `json:"action"`
	Result     string `json:"result"`
	DurationMS int    `json:"durationMs"`
	Viewport   string `json:"viewport"`
}

func consoleTelemetryHandler(c *gin.Context) {
	var event consoleEvent
	if err := c.ShouldBindJSON(&event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "体验事件格式无效"})
		return
	}
	if !allowedConsoleEvent(event.Event) || !allowedConsoleArea(event.Area) || !allowedConsolePage(event.Page) || !allowedConsoleViewport(event.Viewport) || event.DurationMS < 0 || event.DurationMS > 3_600_000 || len(event.Action) > 64 || len(event.Result) > 32 || !safeTelemetryText(event.Action) || !safeTelemetryText(event.Result) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "体验事件字段无效"})
		return
	}
	log.Printf("console telemetry event=%s area=%s page=%s action=%s result=%s duration_ms=%d viewport=%s", event.Event, event.Area, event.Page, event.Action, event.Result, event.DurationMS, event.Viewport)
	c.Status(http.StatusNoContent)
}

func allowedConsoleEvent(value string) bool {
	switch value {
	case "page_view", "create_service", "instance_action", "order_filter", "renew_order", "admin_action":
		return true
	default:
		return false
	}
}

func allowedConsoleArea(value string) bool { return value == "me" || value == "super" }

func allowedConsolePage(value string) bool {
	switch value {
	case "instances", "create", "orders", "overview", "catalog", "images", "nodes", "tasks", "users", "audit", "tickets", "promotions", "settings":
		return true
	default:
		return false
	}
}

func allowedConsoleViewport(value string) bool {
	return value == "mobile" || value == "tablet" || value == "desktop"
}

func safeTelemetryText(value string) bool {
	if value == "" {
		return true
	}
	for _, runeValue := range value {
		if !(runeValue == '_' || runeValue >= 'a' && runeValue <= 'z' || runeValue >= 'A' && runeValue <= 'Z') {
			return false
		}
	}
	return !strings.Contains(value, "__")
}
