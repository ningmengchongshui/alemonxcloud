package cloud

import (
	"context"
	"database/sql"
	"errors"
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
