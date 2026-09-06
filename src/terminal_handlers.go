package cloud

import (
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var consoleTerminalUpgrader = websocket.Upgrader{
	CheckOrigin: func(request *http.Request) bool {
		origin := request.Header.Get("Origin")
		if origin == "" {
			return true
		}
		parsed, err := url.Parse(origin)
		return err == nil && parsed.Host == request.Host
	},
}

// instanceTerminalSocket authenticates the browser session before proxying the
// terminal stream. The node's control token remains server-side at all times.
func instanceTerminalSocket(c *gin.Context) {
	if instanceDB == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "终端数据服务尚未就绪"})
		return
	}
	user := c.MustGet("user").(oidcUser)
	var containerName, nodeID string
	err := instanceDB.QueryRowContext(c.Request.Context(), `SELECT i.container_name,COALESCE(i.node_id,'') FROM xcloud_instances i WHERE i.id=? AND i.owner_id=? AND i.status='running' AND COALESCE(i.runtime_status,i.status)='running'`, c.Param("id"), user.ID).Scan(&containerName, &nodeID)
	if err != nil || nodeID == "" {
		c.JSON(http.StatusNotFound, gin.H{"message": "运行中的实例不存在或暂不可进入终端"})
		return
	}
	node, err := nodeByID(c.Request.Context(), nodeID)
	if err != nil || !node.Enabled {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "终端节点暂不可用"})
		return
	}
	token, err := decryptNodeToken(node.AgentToken)
	if err != nil {
		internalError(c, err)
		return
	}
	agentURL, err := url.Parse(strings.TrimRight(node.AgentURL, "/") + "/container/" + url.PathEscape(containerName) + "/terminal")
	if err != nil {
		internalError(c, err)
		return
	}
	if agentURL.Scheme == "https" {
		agentURL.Scheme = "wss"
	} else {
		agentURL.Scheme = "ws"
	}
	agentHeader := http.Header{"Authorization": []string{"Bearer " + token}}
	agentConn, _, err := websocket.DefaultDialer.DialContext(c.Request.Context(), agentURL.String(), agentHeader)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"message": "终端服务暂不可用"})
		return
	}
	browserConn, err := consoleTerminalUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		_ = agentConn.Close()
		return
	}
	defer browserConn.Close()
	defer agentConn.Close()
	_ = writeAudit(c.Request.Context(), user.ID, "instance.terminal.open", "instance", c.Param("id"), map[string]any{"nodeID": nodeID})
	var closeOnce sync.Once
	closeBoth := func() { closeOnce.Do(func() { _ = browserConn.Close(); _ = agentConn.Close() }) }
	copyMessages := func(destination, source *websocket.Conn) {
		defer closeBoth()
		for {
			kind, message, readErr := source.ReadMessage()
			if readErr != nil {
				return
			}
			if destination.WriteMessage(kind, message) != nil {
				return
			}
		}
	}
	go copyMessages(agentConn, browserConn)
	copyMessages(browserConn, agentConn)
}
