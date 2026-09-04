package cloud

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func readiness(c *gin.Context) {
	problems := readinessProblems()
	if len(problems) > 0 {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ready": false, "dependencies": problems})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ready": true})
}

func readinessProblems() []string {
	problems := []string{}
	if instanceDB == nil {
		problems = append(problems, "mysql")
	} else {
		// database/sql only knows whether it has a pool. A real ping prevents
		// readiness from reporting success when a proxy or MySQL has dropped all
		// pooled TCP connections.
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := instanceDB.PingContext(ctx)
		cancel()
		if err != nil {
			problems = append(problems, "mysql")
		}
	}
	if sessionRedis == nil {
		problems = append(problems, "redis")
	}
	if !queueAvailable() {
		problems = append(problems, "rabbitmq")
	}
	return problems
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
	ready := 1
	if len(readinessProblems()) > 0 {
		ready = 0
	}
	lines := []string{"# HELP xcloud_ready Control-plane readiness", "# TYPE xcloud_ready gauge", "xcloud_ready " + strconv.Itoa(ready), "# TYPE xcloud_tasks_backlog gauge", "xcloud_tasks_backlog " + strconv.Itoa(pending), "# TYPE xcloud_tasks_failed gauge", "xcloud_tasks_failed " + strconv.Itoa(failed), "# TYPE xcloud_instances_running gauge", "xcloud_instances_running " + strconv.Itoa(running)}
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
