# xcloud-agent

`xcloud-agent` 是安装在裸机节点上的 **systemd 服务**，本身不是 Docker 容器。它默认仅监听
`127.0.0.1:9092`，控制接口要求 `Authorization: Bearer $XCLOUD_AGENT_TOKEN`，不得向公网开放。

服务需要能执行宿主机 Docker 命令。先创建供用户容器使用的内部网络：

```bash
docker network create xcloud_network
```

构建二进制后，将 `deploy/xcloud-agent.service` 安装为 systemd 单元。Agent 创建的 AlemonX
容器沿用 `docker-compose/docker-compose.alemonx.yml` 的环境变量、`/root` 与工作区数据卷、
1 GB 共享内存，以及 CPU/内存资源限制；容器不映射宿主机端口。
