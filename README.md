# Go + React 开发模板

最小的全栈项目骨架：Go/Gin 提供 API 和静态文件服务，React/Vite 提供前端界面。

## 快速开始

```bash
cp .env.example .env
cd frontend && yarn && yarn dev
```

另开一个终端启动 API：

```bash
go run .
```

前端开发服务器运行在 `http://localhost:5173`，`/api` 请求会代理到 `http://localhost:8082`。

## 约定

- `GET /healthz`：存活检查
- `GET /api/ping`：示例 API
- `src/`：xcloud-server 的 Go 源码、持久化与测试
- `frontend/src/App.tsx`：前端应用起点
- 生产构建：`make frontend-build && make build`

## xcloud MVP 部署要点

- 生产环境必须设置 `MYSQL_DSN`，实例记录会自动迁移到 `xcloud_instances` 表；未设置时仅使用开发内存存储。
- 设置 `XCLOUD_AGENT_URL` 和 `XCLOUD_AGENT_TOKEN` 后，创建实例会调用裸机 agent；缺少任一配置时，实例会显示为“等待节点接入”。
- agent 默认绑定 `127.0.0.1:9092`。用户实例的域名转发配置见 `deploy/nginx-instance-proxy.conf`，systemd 单元见 `deploy/xcloud-agent.service`。
