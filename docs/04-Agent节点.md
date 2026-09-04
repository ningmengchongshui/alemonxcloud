# Agent 节点

Agent 运行在裸机上，负责 Docker 容器的创建、启停、状态、日志和删除。控制面不直接操作 Docker Socket。

## 安装

```bash
docker network create xcloud_network
sudo install -d -m 0750 /var/lib/xcloud/instances
make agent-build
sudo ./agent/xcloud-agent
systemctl status xcloud-agent
```

生产环境将配置放入 `/etc/xcloud-agent.env`：

```dotenv
AGENT_ADDR=0.0.0.0:13092
XCLOUD_AGENT_TOKEN=<本节点独立令牌>
XCLOUD_DOCKER_NETWORK=xcloud_network
XCLOUD_INSTANCE_DATA_ROOT=/var/lib/xcloud/instances
```

仅本地运行可使用 `./agent/xcloud-agent --serve`。

## 实例 Compose 文件

每个实例的配置固定生成在：

```text
/var/lib/xcloud/instances/xcloud-<实例哈希>/docker-compose.yml
```

Agent 使用该文件执行 `docker compose up -d`。配置包含套餐 CPU、内存、PID
限制，私有 cgroup namespace，Go/Node/Python 的运行时并发参数，健康检查、
路由标签和数据卷。用户容器不声明 `ports`，也不挂载 Docker socket。

可以人工查看该文件排障；平台重新创建同一实例时会依据控制面套餐重新生成。

## 安全要求

- 每个节点使用不同的 Agent 令牌。
- 13092 只允许控制面网段和本机访问。
- 不在页面、日志或接口响应中返回令牌或密文。
- Agent 以 root 运行，因为它需要管理宿主机 Docker；只开放受保护接口。

## 节点登记

在超级管理台填写 Agent 内网地址、独立令牌、CPU 和内存。保存前控制面会调用 `/container/status` 验证令牌。默认 90 秒无心跳的节点不参与新订单分配。
