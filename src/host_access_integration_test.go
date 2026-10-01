//go:build integration

package cloud

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestIntegrationHostAccessWhitelistAndApply(t *testing.T) {
	setupIntegrationDB(t)
	ctx := context.Background()
	suffix := newID("host")
	owner, stranger, id, nodeID := "owner-"+suffix, "other-"+suffix, "ins-"+suffix, "node-"+suffix
	t.Setenv("XCLOUD_NODE_TOKEN_ENCRYPTION_KEY", base64.RawStdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)))
	token, err := encryptNodeToken("test-host-access")
	if err != nil {
		t.Fatal(err)
	}
	type agentCall struct {
		HostAccess  bool `json:"hostAccess"`
		KeepStopped bool `json:"keepStopped"`
	}
	calls := make(chan agentCall, 10)
	var fail atomic.Bool
	fail.Store(true)
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var call agentCall
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/resize") || json.NewDecoder(r.Body).Decode(&call) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		calls <- call
		if fail.Load() {
			w.WriteHeader(http.StatusBadGateway)
		}
		_, _ = w.Write([]byte(`{"message":"测试 Agent 响应"}`))
	}))
	defer agent.Close()
	t.Cleanup(func() {
		_, _ = instanceDB.Exec(`DELETE FROM xcloud_task_events WHERE task_id IN (SELECT id FROM xcloud_tasks WHERE instance_id=?)`, id)
		_, _ = instanceDB.Exec(`DELETE FROM xcloud_instance_operations WHERE instance_id=?`, id)
		_, _ = instanceDB.Exec(`DELETE FROM xcloud_instance_conditions WHERE instance_id=?`, id)
		_, _ = instanceDB.Exec(`DELETE FROM xcloud_tasks WHERE instance_id=?`, id)
		_, _ = instanceDB.Exec(`DELETE FROM xcloud_instances WHERE id=?`, id)
		_, _ = instanceDB.Exec(`DELETE FROM xcloud_nodes WHERE id=?`, nodeID)
		_, _ = instanceDB.Exec(`DELETE FROM xcloud_wallets WHERE user_id IN (?,?)`, owner, stranger)
		_, _ = instanceDB.Exec(`DELETE FROM xcloud_users WHERE id IN (?,?)`, owner, stranger)
		_, _ = instanceDB.Exec(`DELETE FROM xcloud_audit_logs WHERE target_id IN (?,?)`, id, owner)
	})
	for _, userID := range []string{owner, stranger} {
		if _, err = instanceDB.Exec(`INSERT INTO xcloud_users (id,username,email,last_login_at,created_at) VALUES (?,?, '',NOW(),NOW())`, userID, userID); err != nil {
			t.Fatal(err)
		}
		if _, err = instanceDB.Exec(`INSERT INTO xcloud_wallets (user_id,balance_fen,updated_at) VALUES (?,0,NOW())`, userID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = instanceDB.Exec(`INSERT INTO xcloud_nodes (id,name,agent_url,cpu_total,memory_total_mb,enabled,agent_token_ciphertext,agent_capabilities,created_at) VALUES (?,?,?,2,2048,TRUE,?,JSON_ARRAY('container.host-access.v1'),NOW())`, nodeID, nodeID, agent.URL, token); err != nil {
		t.Fatal(err)
	}
	if _, err = instanceDB.Exec(`INSERT INTO xcloud_instances (id,owner_id,name,image,version,spec,cpu,memory_mb,status,runtime_status,access_address,container_name,created_at,node_id,route_key) VALUES (?,?,'测试实例','example/app','latest','1核',1,1024,'stopped','stopped','','xcloud-12345678',NOW(),?,'r0123456789abcdef')`, id, owner, nodeID); err != nil {
		t.Fatal(err)
	}
	request := func(userID, method, path, body string, admin bool) *httptest.ResponseRecorder {
		t.Helper()
		sid := newID("session")
		if err := storeSession(sid, session{User: oidcUser{ID: userID, IsAdmin: admin}, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
		defer deleteSession(sid)
		router := gin.New()
		router.PATCH("/instances/:id/host-access", requireSession, patchInstanceHostAccess)
		router.PUT("/admin/users/:id/host-access", requireAdmin, adminUserHostAccess)
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sid})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	patch := func(userID, body string) *httptest.ResponseRecorder {
		return request(userID, "PATCH", "/instances/"+id+"/host-access", body, false)
	}
	admin := func(allowed bool, isAdmin bool) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]bool{"allowed": allowed})
		return request(owner, "PUT", "/admin/users/"+owner+"/host-access", string(body), isAdmin)
	}
	expect := func(w *httptest.ResponseRecorder, code int) {
		t.Helper()
		if w.Code != code {
			t.Fatalf("HTTP %d, want %d: %s", w.Code, code, w.Body.String())
		}
	}
	expect(patch(owner, `{"enabled":true,"resourceVersion":1}`), http.StatusForbidden)
	expect(admin(true, false), http.StatusForbidden)
	expect(admin(true, true), http.StatusOK)
	expect(patch(owner, `{"resourceVersion":1}`), http.StatusBadRequest)
	if _, err = instanceDB.Exec(`UPDATE xcloud_users SET host_access_allowed=TRUE WHERE id=?`, stranger); err != nil {
		t.Fatal(err)
	}
	expect(patch(stranger, `{"enabled":true,"resourceVersion":1}`), http.StatusNotFound)
	if _, err = instanceDB.Exec(`UPDATE xcloud_nodes SET agent_capabilities=JSON_ARRAY() WHERE id=?`, nodeID); err != nil {
		t.Fatal(err)
	}
	expect(patch(owner, `{"enabled":true,"resourceVersion":1}`), http.StatusConflict)
	if _, err = instanceDB.Exec(`UPDATE xcloud_nodes SET agent_capabilities=JSON_ARRAY('container.host-access.v1') WHERE id=?`, nodeID); err != nil {
		t.Fatal(err)
	}
	w := patch(owner, `{"enabled":true,"resourceVersion":1}`)
	expect(w, http.StatusAccepted)
	var response struct {
		Task controlTask `json:"task"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Task.Action != "host-access" {
		t.Fatal("missing durable configuration task")
	}
	expect(patch(owner, `{"enabled":false,"resourceVersion":1}`), http.StatusConflict)
	expect(admin(false, true), http.StatusConflict)
	var allowed bool
	if err = instanceDB.QueryRow(`SELECT host_access_allowed FROM xcloud_users WHERE id=?`, owner).Scan(&allowed); err != nil || !allowed {
		t.Fatalf("conflicting revoke must roll back: %v", err)
	}
	apply := func(task controlTask, shouldFail, keepStopped bool) {
		t.Helper()
		claimed, claimErr := claimTask(ctx, task)
		if claimErr != nil || !claimed {
			t.Fatalf("claim: %t %v", claimed, claimErr)
		}
		task, err = loadTask(ctx, task.ID)
		if err != nil {
			t.Fatal(err)
		}
		fail.Store(shouldFail)
		executionErr := executeTask(ctx, task)
		var call agentCall
		select {
		case call = <-calls:
		default:
			t.Fatalf("Agent was not called: %v", executionErr)
		}
		if call.HostAccess != allowed || call.KeepStopped != keepStopped {
			t.Fatalf("unexpected Agent payload: %+v allowed=%t", call, allowed)
		}
		if (executionErr != nil) != shouldFail {
			t.Fatalf("execution: %v", executionErr)
		}
		finished, finishErr := finishTask(ctx, task, executionErr)
		if finishErr != nil || !finished {
			t.Fatalf("finish: %t %v", finished, finishErr)
		}
	}
	apply(response.Task, true, true)
	var applied bool
	if err = instanceDB.QueryRow(`SELECT host_access_applied FROM xcloud_instances WHERE id=?`, id).Scan(&applied); err != nil || applied {
		t.Fatalf("failed Agent command must not be marked applied: %v", err)
	}
	apply(response.Task, false, true)
	items, err := listStoredInstances(ctx, owner)
	if err != nil || len(items) != 1 || !items[0].HostAccessEnabled || !items[0].HostAccessApplied || !items[0].HostAccessAllowed || items[0].Status != "stopped" {
		t.Fatalf("instance projection: %+v %v", items, err)
	}
	if _, err = instanceDB.Exec(`UPDATE xcloud_instances SET status='running',runtime_status='running' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	w = admin(false, true)
	expect(w, http.StatusOK)
	var revoked struct {
		Tasks []controlTask `json:"tasks"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &revoked); err != nil || len(revoked.Tasks) != 1 {
		t.Fatalf("revoke must queue removal: %s %v", w.Body.String(), err)
	}
	allowed = false
	apply(revoked.Tasks[0], false, false)
	item, found, err := getStoredInstance(ctx, id, owner)
	if err != nil || !found || item.HostAccessAllowed || item.HostAccessEnabled || item.HostAccessApplied || item.Status != "running" {
		t.Fatalf("revoked instance: %+v %v", item, err)
	}
	expect(patch(owner, `{"enabled":true,"resourceVersion":3}`), http.StatusForbidden)
}
