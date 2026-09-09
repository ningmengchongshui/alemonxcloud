package cloud

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type taskDiagnostic struct {
	TaskID      string    `json:"taskId"`
	ErrorCode   string    `json:"errorCode"`
	SafeMessage string    `json:"safeMessage"`
	Diagnostic  string    `json:"diagnostic"`
	CreatedAt   time.Time `json:"createdAt"`
}

type agentOperationSummary struct {
	ID            string     `json:"operationId"`
	TaskID        string     `json:"taskId"`
	Action        string     `json:"action"`
	DesiredState  string     `json:"desiredState"`
	Status        string     `json:"status"`
	ObservedState string     `json:"observedState,omitempty"`
	SafeError     string     `json:"safeError,omitempty"`
	StartedAt     time.Time  `json:"startedAt"`
	FinishedAt    *time.Time `json:"finishedAt,omitempty"`
}

func saveSelfHostedTaskDiagnostic(ctx context.Context, task controlTask, cause error) {
	var failure *selfHostedCommandError
	if !errors.As(cause, &failure) || strings.TrimSpace(failure.Diagnostic) == "" || instanceDB == nil {
		return
	}
	var nodeID, ownerID string
	if err := instanceDB.QueryRowContext(ctx, `SELECT node_id,owner_id FROM xcloud_instances WHERE id=? AND placement_type='selfhosted'`, task.InstanceID).Scan(&nodeID, &ownerID); err != nil || nodeID == "" || ownerID == "" {
		return
	}
	code := failure.Code
	if code == "" {
		code = "agent_command_failed"
	}
	_, _ = instanceDB.ExecContext(ctx, `INSERT INTO xcloud_task_diagnostics (task_id,node_id,owner_id,error_code,safe_message,diagnostic,expires_at,created_at,updated_at) VALUES (?,?,?,?,?,?,DATE_ADD(NOW(), INTERVAL 30 DAY),NOW(),NOW()) ON DUPLICATE KEY UPDATE error_code=VALUES(error_code),safe_message=VALUES(safe_message),diagnostic=VALUES(diagnostic),expires_at=VALUES(expires_at),updated_at=NOW()`, task.ID, nodeID, ownerID, code, truncateError(failure.Message), failure.Diagnostic)
}

func cleanupExpiredTaskDiagnostics(ctx context.Context) {
	if instanceDB != nil {
		_, _ = instanceDB.ExecContext(ctx, `DELETE FROM xcloud_task_diagnostics WHERE expires_at<=NOW() LIMIT 500`)
		_, _ = instanceDB.ExecContext(ctx, `DELETE FROM xcloud_selfhosted_readiness_events WHERE created_at<=DATE_SUB(NOW(),INTERVAL 30 DAY) LIMIT 500`)
		_, _ = instanceDB.ExecContext(ctx, `DELETE FROM xcloud_selfhosted_agent_operations WHERE created_at<=DATE_SUB(NOW(),INTERVAL 30 DAY) LIMIT 500`)
	}
}

func instanceTaskDiagnosticHandler(c *gin.Context) {
	instance, ok := ownedInstance(c)
	if !ok {
		return
	}
	var item taskDiagnostic
	err := instanceDB.QueryRowContext(c.Request.Context(), `SELECT d.task_id,d.error_code,d.safe_message,d.diagnostic,d.created_at FROM xcloud_task_diagnostics d JOIN xcloud_tasks t ON t.id=d.task_id WHERE d.task_id=? AND t.instance_id=? AND d.owner_id=?`, c.Param("taskID"), instance.ID, instance.OwnerID).Scan(&item.TaskID, &item.ErrorCode, &item.SafeMessage, &item.Diagnostic, &item.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(404, gin.H{"message": "该任务没有可用节点诊断"})
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(200, item)
}

func adminTaskDiagnosticHandler(c *gin.Context) {
	var item taskDiagnostic
	err := instanceDB.QueryRowContext(c.Request.Context(), `SELECT task_id,error_code,safe_message,diagnostic,created_at FROM xcloud_task_diagnostics WHERE task_id=?`, c.Param("id")).Scan(&item.TaskID, &item.ErrorCode, &item.SafeMessage, &item.Diagnostic, &item.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(404, gin.H{"message": "该任务没有可用节点诊断"})
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(200, item)
}

func instanceAgentOperationHandler(c *gin.Context) {
	instance, ok := ownedInstance(c)
	if !ok {
		return
	}
	var item agentOperationSummary
	err := instanceDB.QueryRowContext(c.Request.Context(), `SELECT id,task_id,action,desired_state,status,COALESCE(observed_state,''),COALESCE(safe_error,''),started_at,finished_at FROM xcloud_selfhosted_agent_operations WHERE task_id=? AND instance_id=? ORDER BY created_at DESC LIMIT 1`, c.Param("taskID"), instance.ID).Scan(&item.ID, &item.TaskID, &item.Action, &item.DesiredState, &item.Status, &item.ObservedState, &item.SafeError, &item.StartedAt, &item.FinishedAt)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(404, gin.H{"message": "该任务没有 Agent 操作记录"})
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}
	if item.Status == "running" || item.Status == "unknown" {
		var nodeID, containerName string
		if lookupErr := instanceDB.QueryRowContext(c.Request.Context(), `SELECT node_id,container_name FROM xcloud_instances WHERE id=?`, instance.ID).Scan(&nodeID, &containerName); lookupErr == nil {
			if n, nodeErr := nodeByID(c.Request.Context(), nodeID); nodeErr == nil && n.NodeKind == selfHostedNodeKind {
				var live struct {
					Status        string `json:"status"`
					ObservedState string `json:"observedState"`
					SafeError     string `json:"safeError"`
				}
				path := "/container/" + url.PathEscape(containerName) + "/operation-status?operationId=" + url.QueryEscape(item.ID)
				if nodeRequest(c.Request.Context(), n, http.MethodGet, path, nil, &live) == nil {
					item.Status, item.ObservedState, item.SafeError = live.Status, live.ObservedState, live.SafeError
					_, _ = instanceDB.ExecContext(c.Request.Context(), `UPDATE xcloud_selfhosted_agent_operations SET status=?,observed_state=?,safe_error=?,updated_at=NOW() WHERE id=?`, item.Status, nullableString(item.ObservedState), nullableString(item.SafeError), item.ID)
				}
			}
		}
	}
	c.JSON(200, item)
}

func adminRevokedSelfHostedReadinessEvents(c *gin.Context) {
	rows, err := instanceDB.QueryContext(c.Request.Context(), `SELECT e.ready,e.issue_code,e.message,e.created_at FROM xcloud_selfhosted_readiness_events e JOIN xcloud_nodes n ON n.id=e.node_id WHERE n.id=? AND n.node_kind='selfhosted' AND n.enabled=FALSE AND e.created_at>DATE_SUB(NOW(),INTERVAL 30 DAY) ORDER BY e.id DESC LIMIT 100`, c.Param("nodeID"))
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()
	items := []gin.H{}
	for rows.Next() {
		var ready bool
		var code, message string
		var createdAt time.Time
		if err := rows.Scan(&ready, &code, &message, &createdAt); err != nil {
			internalError(c, err)
			return
		}
		items = append(items, gin.H{"ready": ready, "code": code, "message": message, "createdAt": createdAt})
	}
	if err := rows.Err(); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(200, items)
}
