# HTTP 接口参考

所有 `/api/instances*` 接口要求已登录会话；`/api/admin/*` 还要求 Auth 返回 `cloud:admin` 或 `*` 权限。错误响应格式为 `{ "message": "中文错误说明" }`。

| 方法 | 路径 | 权限 | 当前行为 |
|---|---|---|---|
| GET | `/healthz` | 无 | 服务与版本健康检查 |
| GET | `/api/ping` | 无 | 连通性检查 |
| POST | `/api/oidc/authorize` | 无 | 创建 PKCE 授权地址 |
| POST | `/api/oidc/callback` | 无 | 交换授权码并创建会话 |
| GET | `/api/oidc/session` | 无 | 获取当前会话；未登录返回 401 |
| POST | `/api/logout` | 已登录 | 删除服务端会话 |
| GET | `/api/instances` | 已登录 | 获取当前用户实例 |
| POST | `/api/instances` | 已登录 | 原型直接创建实例，后续将由订单接口替代 |
| POST | `/api/instances/:id/start` | 已登录 | 请求启动容器 |
| POST | `/api/instances/:id/stop` | 已登录 | 请求停止容器 |
| DELETE | `/api/instances/:id` | 已登录 | 请求删除容器与实例记录 |
| GET | `/api/admin/nodes` | 管理员 | 当前为节点接口占位，待接入实际节点表 |
| POST | `/api/dev/login` | 仅 debug | 创建内置超级管理员会话 |

## Agent 控制接口

Agent 只接受来自回环网络的请求，并要求 `Authorization: Bearer $XCLOUD_AGENT_TOKEN`。这些接口不应通过公网、浏览器或 Nginx 主站暴露。

| 方法 | 路径 | 用途 |
|---|---|---|
| GET | `/healthz` | agent 健康检查 |
| POST | `/container/create` | 创建受限 AlemonX 容器 |
| POST | `/container/:name/start` | 启动容器 |
| POST | `/container/:name/stop` | 停止容器 |
| DELETE | `/container/:name` | 删除容器 |
