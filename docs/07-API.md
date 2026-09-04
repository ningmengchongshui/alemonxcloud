# API 参考

除健康检查和登录入口外，接口均使用服务端会话；管理接口还要求 `cloud:admin`。

## 公开接口

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/healthz` | 进程存活 |
| GET | `/readyz` | 依赖和 Agent 就绪 |
| GET | `/metrics` | Prometheus 指标，需 Bearer 令牌 |
| GET | `/api/ping` | API 连通性 |
| POST | `/api/oidc/authorize` | 获取 OIDC 授权地址 |
| POST | `/api/oidc/callback` | 完成登录回调 |
| GET | `/api/oidc/session` | 当前会话 |
| POST | `/api/logout` | 退出登录 |

## 用户接口

`/api/instances`、`/api/catalog`、`/api/orders`、`/api/wallet`、`/api/notifications` 和 `/api/instances/:id/tasks` 提供实例、目录、订单、钱包、通知和任务能力；创建、启停、删除、付款、续费和取消使用对应的 `POST`/`DELETE` 路由。

## 管理接口

`/api/admin/catalog`、`orders`、`nodes`、`users`、`tasks`、`audit-logs`、`metrics` 提供管理查询；镜像、套餐、节点使用 `POST`/`PUT` 保存，订单使用 `confirm`/`reject`，任务使用 `retry`。

完整参数以 `src/server.go`、`src/control_handlers.go` 和 `frontend/src/services/cloudApi.ts` 为准。业务失败返回非 2xx 状态和 JSON 错误信息。
