# xcloud-agent

平台裸机的 Docker 执行 Agent，作为 systemd 服务运行，不是 Docker 容器。

- 本地开发：复制 [`agent/.env.example`](.env.example) 为 `agent/.env`，执行 `make agent-build` 后运行 `make agent-run`。
- 生产部署：使用 `/etc/xcloud-agent.env` 与 [`deploy/xcloud-agent.service`](../deploy/xcloud-agent.service)。
- 平台节点配置、网络边界、升级和排障：见 [平台 Agent 节点](../docs/04-Agent节点.md)。
- 所有运行单元的变量归属：见 [配置参考](../docs/00-配置参考.md)。
