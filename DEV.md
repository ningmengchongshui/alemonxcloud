# 开发指南

## 启动

```bash
cp .env.example .env
cd frontend && yarn
cd .. && make dev
```

另开终端运行前端：`make dev-fe`。后端为 `http://localhost:8082`，前端为 `http://localhost:5173`。

## 常用命令

| 命令 | 用途 |
|---|---|
| `make dev` | 启动 Go 服务 |
| `make dev-fe` | 启动 Vite |
| `make test` | Go 全量测试 |
| `make lint` | `go vet` |
| `make build` | 构建控制面二进制 |
| `make build-fe` | 构建前端 |
| `make agent-test` | Agent 测试 |
| `make agent-build` | 构建 Agent |
| `make docker-build` | 构建控制面镜像 |

## 修改后检查

```bash
make lint
make test
make build-fe
git diff --check
```

不要提交 `.env`、数据库密码、OAuth Cookie、Agent 令牌或构建产物。
