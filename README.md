# AlemonX Cloud

AlemonX Cloud 是面向 AlemonX 容器实例的自建云平台。用户通过 Auth 登录，选择已上架的镜像、版本和资源规格；平台通过裸机上的 `xcloud-agent` 管理 Docker 容器，并使用专属子域名访问实例。

> 当前阶段：基础设施、认证、实例原型、MySQL 数据模型和 RabbitMQ 队列基础已实现；商品、订单确认、资源预占和队列执行器仍在开发，不能将其视为付费生产版本。

## 文档导航

- [架构与状态](docs/架构与状态.md)：组件职责、访问链路与已实现边界。
- [部署手册](docs/部署手册.md)：本地、Docker Compose、裸机 agent、Nginx、DNS/TLS。
- [接口参考](docs/API.md)：当前可调用的 HTTP 接口与权限。
- [MVP 路线图](docs/MVP路线图.md)：首发范围、数据模型和未完成项。

## 开发启动

```bash
cp .env.example .env
cd frontend && yarn && yarn dev
```

另开一个终端启动服务端：

```bash
make dev
```

前端开发服务器为 `http://localhost:5173`，`/api` 自动代理到 `http://localhost:8082`。开发模式可使用内置超级管理员登录；发布模式不注册该接口。

## 构建与检查

```bash
make frontend-build
make build
make test
make agent-test
```

## Docker 构建

公开依赖无需 GitHub token，直接执行：

```bash
make docker-build
# 或 docker compose build
```

如果之后加入了私有 GitHub Go 模块，请使用具有该仓库读取权限的 fine-grained token：

```bash
export GITHUB_TOKEN=github_pat_...
make docker-build
# 或 docker compose build
```

Token 通过 BuildKit secret 仅在构建步骤中挂载，不会写入最终镜像或镜像层。

服务端源码位于 `src/`，前端源码位于 `frontend/src/`，裸机 agent 位于 `agent/`。
