package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type selfHostedCommandError struct {
	Code       string
	Message    string
	Diagnostic string
}

func (e *selfHostedCommandError) Error() string {
	if e.Message == "" {
		return "节点自建 Agent 操作失败"
	}
	return "节点 " + e.Message
}

// selfHostedNodeRequest is the narrow command bridge to a customer-owned
// node. It only translates structured xCloud lifecycle calls; it never sends
// shell text, host paths, or arbitrary Docker IDs through the tunnel.
func selfHostedNodeRequest(ctx context.Context, n node, method, path string, payload any, result any) error {
	if method != httpMethodPost && method != httpMethodDelete {
		return errors.New("自建节点不支持该操作")
	}
	action := ""
	switch {
	case path == "/container/create":
		action = "create"
	case path == "/container/pull":
		action = "pull-image"
	case strings.Contains(path, "/logs"):
		action = "logs"
	case strings.Contains(path, "/files/content") && method == http.MethodGet:
		action = "file-read"
	case strings.Contains(path, "/files/content"):
		action = "file-write"
	case strings.Contains(path, "/files/upload"):
		action = "file-upload"
	case strings.Contains(path, "/files") && method == http.MethodGet:
		action = "files"
	case strings.HasSuffix(path, "/start"):
		action = "start"
	case strings.HasSuffix(path, "/stop"):
		action = "stop"
	case strings.HasSuffix(path, "/restart"):
		action = "restart"
	case strings.HasSuffix(path, "/resize"):
		action = "resize"
	case strings.HasSuffix(path, "/reinstall"):
		action = "reinstall"
	case strings.HasSuffix(path, "/bandwidth"):
		action = "bandwidth"
	case strings.Contains(path, "?purge=true"):
		action = "purge"
	case strings.HasSuffix(path, "/destroy"):
		action = "destroy"
	default:
		return errors.New("自建 Agent 尚不支持该受管接口")
	}
	endpoint, token := tunnelInternalURL(), env("XCLOUD_TUNNEL_COMMAND_TOKEN", "")
	if endpoint == "" || token == "" {
		return errors.New("未配置自建节点 Gateway 命令通道")
	}
	commandPayload := map[string]any{}
	if payload != nil {
		rawPayload, _ := json.Marshal(payload)
		_ = json.Unmarshal(rawPayload, &commandPayload)
	}
	pathOnly := strings.SplitN(path, "?", 2)[0]
	trimmed := strings.TrimPrefix(pathOnly, "/container/")
	if name := strings.Split(trimmed, "/")[0]; name != "" {
		commandPayload["name"], _ = url.PathUnescape(name)
	}
	if parsed, parseErr := url.Parse(path); parseErr == nil {
		commandPayload["path"] = parsed.Query().Get("path")
		commandPayload["tail"] = parsed.Query().Get("tail")
		commandPayload["since"] = parsed.Query().Get("since")
	}
	raw, err := json.Marshal(map[string]any{"deviceId": n.ControlDeviceID, "action": action, "payload": commandPayload})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/command", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求自建 Agent: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("自建 Agent 返回 %s", resp.Status)
	}
	var reply struct {
		OK         bool            `json:"ok"`
		Error      string          `json:"error"`
		ErrorCode  string          `json:"errorCode"`
		Diagnostic string          `json:"diagnostic"`
		Data       json.RawMessage `json:"data"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&reply); err != nil {
		return err
	}
	if !reply.OK {
		return &selfHostedCommandError{Code: reply.ErrorCode, Message: reply.Error, Diagnostic: reply.Diagnostic}
	}
	if result != nil && len(reply.Data) > 0 {
		return json.Unmarshal(reply.Data, result)
	}
	return nil
}
