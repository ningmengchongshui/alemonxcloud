package cloud

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	agentcore "xcloud/agent-core"
)

func instanceStats(c *gin.Context) {
	item, n, ok := instanceWorkspaceNode(c)
	if !ok {
		return
	}
	if item.Status == "destroyed" || item.Status == "purged" || item.RuntimeStatus != "running" && item.Status != "running" {
		c.JSON(http.StatusConflict, gin.H{"message": "实例未运行，暂无实时监控"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	var stats agentcore.ContainerStats
	if err := nodeRequest(ctx, n, http.MethodGet, "/container/"+url.PathEscape(item.ContainerName)+"/stats", nil, &stats); err != nil || stats.CPUPercent == "" || stats.MemoryUsage == "" || stats.SampledAt == "" {
		c.JSON(http.StatusBadGateway, gin.H{"message": "监控采集暂不可用，请确认节点在线且已升级监控功能"})
		return
	}
	c.JSON(http.StatusOK, stats)
}

func instanceSummary(c *gin.Context) {
	item, ok := ownedInstance(c)
	if !ok {
		return
	}
	var placement, nodeID string
	if err := instanceDB.QueryRowContext(c.Request.Context(), `SELECT COALESCE(placement_type,'platform'),COALESCE(node_id,'') FROM xcloud_instances WHERE id=?`, item.ID).Scan(&placement, &nodeID); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"instance": item, "selfHosted": placement == "selfhosted", "nodeId": nodeID})
}
