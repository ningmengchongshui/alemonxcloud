# 平台 Agent 节点

本页只说明平台自有裸机上的 `xcloud-agent`。用户自建节点使用 `xcloud-control`，见 [部署指南](03-部署指南.md) 与 [配置参考](00-配置参考.md)。

`xcloud-agent` 是 systemd 服务，不是 Docker 容器。它管理宿主机 Docker Compose、受管镜像、实例状态、日志、文件、终端、资源限制和实例路由；控制面通过节点独立 Bearer Token 调用它。

## 安装

程序、systemd unit、环境文件和实例数据由既有发布流程管理。`make agent-deploy` 不会创建、复制或覆盖其中任意文件。

```bash
cd /path/to/alemonxcloud
git pull --ff-only
make agent-deploy
```

它先编译当前源码，再只读比较编译产物与 `/opt/xcloud-agent/xcloud-agent`、比较 systemd unit、检查环境文件是否存在；仅当全部一致时才重启。若提示缺少或不匹配文件，命令会在重启前退出，需由既有发布流程修复差异。

`/etc/xcloud-agent.env`：

```dotenv
AGENT_ADDR=0.0.0.0:13092
XCLOUD_AGENT_TOKEN=本节点独立的长随机令牌
XCLOUD_DOCKER_NETWORK=xcloud_network
XCLOUD_INSTANCE_DATA_ROOT=/var/lib/xcloud/instances
```

本地开发可复制 [`agent/.env.example`](../agent/.env.example) 为 `agent/.env`，并运行 `make agent-run`。

## 网络与安全

- `13092` 只允许控制面 Docker 网段和本机访问，不能暴露公网。
- 每个节点必须使用不同 Token；Token 仅在超级管理台新增/更新节点时输入，读取接口不返回明文。
- Agent 以 root 运行以管理 Docker，但只允许平台定义的受管操作；不接受任意 Shell、Docker ID、宿主路径或用户镜像仓库。
- 每个实例的 Compose 文件和数据目录位于 `/var/lib/xcloud/instances/xcloud-<哈希>/`；工作区为 `workspace/`，容器内路径为 `/app/workspace`。

## 心跳与能力

`GET /container/status` 是节点心跳和能力协商入口，返回 Agent 版本、Docker 版本、CPU、内存、磁盘、托管实例数和能力清单。超级管理台保存节点后会验证该接口；90 秒未收到心跳的节点不参与新调度。

基础执行面包括：实例创建、启动、停止、重启、调整规格、重装、销毁、物理清理、状态、日志、文件、终端、镜像拉取和实例路由。控制面只会调用节点已声明的能力，避免通过 404 猜测 Agent 是否需要升级。

## 升级

Agent 不自动下载或替换自身。每次只升级一个节点：构建可信二进制，部署并重启 systemd，确认管理台中的版本、能力与心跳正常后，再处理下一台。旧版节点可继续提供兼容能力；当新功能依赖它未声明的能力时，控制面会明确提示不支持。

## 排障

```bash
systemctl status xcloud-agent
journalctl -u xcloud-agent -f
curl -fsS -H "Authorization: Bearer <节点令牌>" http://127.0.0.1:13092/container/status
docker network inspect xcloud_network
```

不要手工调用 Agent 的销毁或创建接口绕过控制面任务。实例、订单、任务和审计的状态应始终由控制面维护。
