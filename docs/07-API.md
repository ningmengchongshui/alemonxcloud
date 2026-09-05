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

`/api/instances`、`/api/catalog`、`/api/orders`、`/api/wallet`、`/api/notifications` 和 `/api/instances/:id/tasks` 提供实例、目录、订单、钱包、通知和任务能力。新购使用 `POST /api/purchases`，请求包含 `planId`、`imageId`、`imageVersion` 和 `months`；续费使用 `POST /api/orders/:id/renew`，请求包含 `months`。两者均从钱包扣款，余额不足返回业务错误且不创建订单。旧的人工付款创建、付款、取消及管理确认接口已停用并返回 `410 Gone`。

退款使用 `GET /api/orders/:id/refund-quote` 取得服务端试算，再以 `POST /api/orders/:id/refund` 确认。试算返回 `eligible`、`reason`、完整天数、预扣天数、退款金额、调整后服务到期时间和预计清理时间；确认返回更新后的 `order`、退款 `entry`、`wallet` 与最终 `quote`。仅迁移后创建且服务期完整的钱包订单可自助退款。退款回到 XCoin，不会立即停止实例；后续续费服务期会前移，退款实例停止后保留数据 30 天。钱包流水的 `orderId` 可关联购买、续费和退款记录。

工单使用 `/api/tickets`：用户可创建、查看本人列表与详情、回复，或重新打开已关闭工单。创建参数为 `category`（`instance`、`billing`、`account`、`other`）、`priority`（`normal`、`high`、`urgent`）、`subject`、`body`，并可选关联本人 `instanceId` 或 `orderId`。主题最多 160 字符，内容最多 4000 字符；不支持附件。

## 管理接口

`/api/admin/catalog`、`orders`、`nodes`、`users`、`tasks`、`audit-logs`、`metrics` 提供管理查询；镜像、套餐、节点使用 `POST`/`PUT` 保存，任务使用 `retry`。镜像来源的可售版本由 `POST /api/admin/images/:id/versions` 和 `PUT /api/admin/images/:id/versions/:versionID` 管理；`POST /api/admin/images/:id/versions/:versionID/pull` 会向全部启用节点预拉取。镜像地址在数据库中唯一，已配置版本 digest 时部署必定使用 `image@sha256:...`。

管理员通过 `/api/admin/tickets` 查询并按 `status`、`priority` 筛选工单；可读取详情、回复、标记处理中、关闭及调整优先级。所有工单操作都会写入审计日志。

完整参数以 `src/server.go`、`src/control_handlers.go` 和 `frontend/src/services/cloudApi.ts` 为准。业务失败返回非 2xx 状态和 JSON 错误信息。
