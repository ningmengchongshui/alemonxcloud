# AlemonX Cloud

AlemonX Cloud 是 AlemonX 容器实例的自建控制面。用户登录后选择镜像、套餐并创建订单；控制面确认订单、分配节点，再由裸机上的 `xcloud-agent` 创建和管理 Docker 容器。

## 文档导航

- [快速开始](docs/01-快速开始.md)：本地开发和首次运行。
- [部署指南](docs/03-部署指南.md)：生产依赖、Compose、Agent、Nginx、DNS 和 TLS。
- [架构说明](docs/02-架构说明.md)：组件关系、请求链路和数据流。
- [Agent 节点](docs/04-Agent节点.md)：裸机 Agent 安装、网络和安全边界。
- [管理与运营](docs/06-管理与运营.md)：商品、节点、订单、钱包和任务的操作流程。
- [API 参考](docs/07-API.md)：HTTP 接口、权限和健康检查。
- [监控与排障](docs/08-监控与排障.md)：指标、告警和常见故障。
- [发布检查清单](docs/09-发布检查.md)：上线前必须逐项确认的内容。

## 项目结构

| 目录 | 内容 |
|---|---|
| `src/` | Go 控制面、业务 API、任务队列和数据库逻辑 |
| `frontend/` | React 用户台和超级管理台 |
| `agent/` | 裸机节点上的 Docker 管理 Agent |
| `deploy/` | Nginx、systemd 和 Prometheus 示例配置 |
| `docker-compose/` | 用户实例容器模板 |

## 最短启动方式

```bash
cp .env.example .env
cd frontend && yarn && yarn dev
```

另开终端启动 Go 服务：

```bash
make dev
```

前端默认访问 `http://localhost:5173`，API 代理到 `http://localhost:8082`。开发模式可使用 `/api/dev/login`；生产模式不会注册该接口。

常用检查：`make test`、`make lint`、`make build-fe`、`make agent-test`。

> 生产环境还需要外部 MySQL、Redis、RabbitMQ 和至少一个裸机 Agent。不要把本地开发配置直接用于收费服务。
