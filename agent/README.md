# xcloud-agent

`xcloud-agent` 是安装在裸机节点上的 systemd 服务，不是 Docker 容器。控制面通过 Bearer Token 调用它；13092 端口必须由防火墙限制为控制面网段和本机，不能暴露公网。

## 快速安装

```bash
docker network create xcloud_network
sudo install -d -m 0750 /var/lib/xcloud/instances
```

创建 `/etc/xcloud-agent.env`：

```dotenv
AGENT_ADDR=0.0.0.0:13092
XCLOUD_AGENT_TOKEN=节点独立的高强度随机令牌
XCLOUD_DOCKER_NETWORK=xcloud_network
XCLOUD_INSTANCE_DATA_ROOT=/var/lib/xcloud/instances
```

构建并安装：

```bash
make agent-build
sudo ./agent/xcloud-agent
systemctl status xcloud-agent
```

该命令会安装到 `/opt/xcloud-agent/`，写入 systemd 服务并启动。仅本地运行可使用 `./agent/xcloud-agent --serve`。

## 接口

- `GET /healthz`：存活检查。
- `GET /container/status`：节点状态。
- `POST /container/create`：创建实例。
- `POST /container/:name/start`、`stop`：启停实例。
- `GET /container/:name/status`、`logs`：查看状态和日志。
- `DELETE /container/:name`：删除实例。

控制接口均要求 `Authorization: Bearer $XCLOUD_AGENT_TOKEN`。实例使用 `xcloud_network` 内网、数据卷和资源限制，不映射宿主机端口。生产步骤见 [部署指南](../docs/03-部署指南.md)。
