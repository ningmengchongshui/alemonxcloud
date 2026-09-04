package cloud

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func readiness(c *gin.Context) {
	problems := []string{}
	if instanceDB == nil {
		problems = append(problems, "mysql")
	}
	if sessionRedis == nil {
		problems = append(problems, "redis")
	}
	if !queueAvailable() {
		problems = append(problems, "rabbitmq")
	}
	if !agentOnline(c.Request.Context()) {
		problems = append(problems, "agent")
	}
	if len(problems) > 0 {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ready": false, "dependencies": problems})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ready": true})
}

// metrics is intentionally network-restricted by Nginx/firewall. It exposes
// no credentials or user data and is compatible with Prometheus text parsing.
func metrics(c *gin.Context) {
	if token := env("XCLOUD_METRICS_TOKEN", ""); token != "" && c.GetHeader("Authorization") != "Bearer "+token {
		c.Status(http.StatusUnauthorized)
		return
	}
	if instanceDB == nil {
		c.String(http.StatusServiceUnavailable, "xcloud_ready 0\n")
		return
	}
	var pending, failed, running int
	_ = instanceDB.QueryRowContext(c.Request.Context(), `SELECT COUNT(*) FROM xcloud_tasks WHERE status IN ('pending','running')`).Scan(&pending)
	_ = instanceDB.QueryRowContext(c.Request.Context(), `SELECT COUNT(*) FROM xcloud_tasks WHERE status='failed'`).Scan(&failed)
	_ = instanceDB.QueryRowContext(c.Request.Context(), `SELECT COUNT(*) FROM xcloud_instances WHERE status='running'`).Scan(&running)
	lines := []string{"# HELP xcloud_ready Control-plane readiness", "# TYPE xcloud_ready gauge", "xcloud_ready 1", "# TYPE xcloud_tasks_backlog gauge", "xcloud_tasks_backlog " + strconv.Itoa(pending), "# TYPE xcloud_tasks_failed gauge", "xcloud_tasks_failed " + strconv.Itoa(failed), "# TYPE xcloud_instances_running gauge", "xcloud_instances_running " + strconv.Itoa(running)}
	nodes, err := listNodesWithUsage(c.Request.Context())
	if err == nil {
		for _, node := range nodes {
			label := promLabel(node.ID)
			lines = append(lines, fmt.Sprintf("xcloud_node_cpu_reserved{node=%q} %g", label, node.CPUReserved), fmt.Sprintf("xcloud_node_memory_reserved_mb{node=%q} %d", label, node.MemoryReservedMB))
			if node.LastHeartbeatAt != nil {
				lines = append(lines, fmt.Sprintf("xcloud_node_heartbeat_timestamp{node=%q} %d", label, node.LastHeartbeatAt.Unix()))
			}
		}
	}
	c.Data(http.StatusOK, "text/plain; version=0.0.4", []byte(strings.Join(lines, "\n")+"\n"))
}
func promLabel(value string) string { return strings.ReplaceAll(value, `"`, "'") }
