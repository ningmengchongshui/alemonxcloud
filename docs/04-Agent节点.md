# Agent 节点

Agent 运行在裸机上，负责 Docker 容器、镜像、资源探测与实例路由。控制面不直接操作 Docker Socket。
它是一个有版本的稳定执行面：控制面通过心跳获取 API 版本和能力清单，只有节点声明支持的扩展功能才会下发。

## 安装

```bash
docker network create xcloud_network
sudo dnf install -y iproute util-linux # Debian/Ubuntu 使用 apt install iproute2 util-linux
sudo chmod -R 777 /var/lib/xcloud/instances
make agent-build VERSION=v1.0.8
sudo ./agent/xcloud-agent
systemctl status xcloud-agent
```

生产环境将配置放入 `/etc/xcloud-agent.env`：

```dotenv
AGENT_ADDR=0.0.0.0:13092
XCLOUD_AGENT_TOKEN=<本节点独立令牌>
XCLOUD_DOCKER_NETWORK=xcloud_network
XCLOUD_INSTANCE_DATA_ROOT=/var/lib/xcloud/instances
# 默认完全关闭带宽整形；启动 Agent 后会一次性移除旧规则，随后不再执行 tc。
# 旧的 XCLOUD_ENABLE_BANDWIDTH_SHAPING 已失效，不能意外重新开启限速。
# 未来只有完成隔离压测并明确评审后，才可设置：
# XCLOUD_TRAFFIC_CONTROL_ENABLED=true
# XCLOUD_BANDWIDTH_BURST_MULTIPLIER=4
```

仅本地运行可使用 `./agent/xcloud-agent --serve`。

检查已安装版本：

```bash
/opt/xcloud-agent/xcloud-agent --version
curl -H "Authorization: Bearer <本节点令牌>" http://127.0.0.1:13092/container/status
```

## 稳定接口与能力协商

所有控制接口均要求 `Authorization: Bearer <本节点令牌>`。`GET /container/status` 是心跳和协议协商入口，返回 `agentVersion`、`apiVersion` 与 `capabilities`。当前 API v1 一次性覆盖以下执行面能力：

| 能力 | 接口 | 用途 |
| --- | --- | --- |
| 节点资源 | `GET /container/status` | Docker 版本、CPU、内存、磁盘、托管容器数量、协议能力 |
| 容器生命周期 | `POST /container/create`、`/:name/start`、`/:name/stop`、`/:name/restart`、`/:name/destroy`、`DELETE /:name?purge=true` | 创建、启停、按当前控制面配置重建重启、销毁运行资源，或在保留期后清理数据 |
| 容器查询 | `GET /container`、`/:name/status`、`/:name/inspect`、`/:name/logs` | 托管容器清单、运行/健康、有限元数据、最近 200 行日志 |
| 镜像管理 | `POST /container/pull`、`GET /container/images`、`GET /container/images/inspect?image=` | 预拉取、查看节点本地镜像和验证摘要 |
| 实例访问 | 非控制路径 + `X-Route-Key` | 仅按受控路由键反代到受管容器，不公开宿主机端口 |
| 带宽上限 | `network.bandwidth.v1`、`network.bandwidth.status.v1`、`network.bandwidth.queue.v1`、`POST /container/:name/bandwidth` | 默认关闭，套餐带宽仅作展示；仅在完成隔离压测并显式启用后，才以服务出口整形方式执行 |

控制面不会用“接口是否返回 404”来猜 Agent 是否需要更新。未声明某项能力的节点保留既有生命周期服务，但新功能会显示为不支持并跳过。例如镜像版本的“预拉取”只会调用声明 `image.pull.v1` 的节点。

带宽规则未能应用时，实例仍保持运行，控制面记录为待补偿或失败并只对运行中的实例重试；不会为了限速失败而销毁、关机或阻断依赖下载。

## 升级策略

Agent 不自动从控制面下载或替换二进制文件；这避免控制面被入侵后接管裸机。按以下方式滚动升级，每次只处理一台节点：

1. 在可信构建机执行 `make agent-build VERSION=v1.x.y`，并校验发布包的校验和。
2. 将二进制传到目标裸机，以 root 执行 `sudo ./xcloud-agent`。安装器会原子替换 `/opt/xcloud-agent/xcloud-agent` 并重启 systemd 服务。
3. 在超级管理台刷新节点，确认显示“已兼容”、版本和能力清单已更新；然后再升级下一台。

旧版 Agent 不会因控制面升级而停止基础实例；它会显示为“协议未知”。只有 API 主版本不兼容或需要其未声明的新能力时才需要升级，因此不必为每次控制面发布频繁更新 Agent。

## 实例 Compose 文件

每个实例的配置固定生成在：

```text
/var/lib/xcloud/instances/xcloud-<实例哈希>/docker-compose.yml
```

Agent 使用该文件执行 `docker compose up -d`。配置包含套餐 CPU、内存、PID
限制，私有 cgroup namespace，Go/Node/Python 的运行时并发参数，健康检查、
路由标签和数据卷。用户容器不声明 `ports`，也不挂载 Docker socket。

可以人工查看该文件排障；平台重新创建或从用户实例页执行“重启”时，都会依据
控制面保存的当前实例配置重新生成。Compose 仅在配置实际变化时重建运行容器，并保留
数据目录；配置未变化时不会销毁正常容器。重启不会拉取新镜像，也不会改变实例选择的版本标记或订单中的镜像快照。需要获得同一
版本标记的最新镜像时，必须由用户明确执行“更新实例”。

## 生命周期任务恢复

控制面以 MySQL 时钟发放 5 分钟任务租约，并每 30 秒续租。每次领取都会产生新的
执行代次；只有持有该代次和实例独占锁的 Worker 才能调用 Agent 或写回任务结果。

Worker 崩溃后，`create` 与 `start` 仅在实例仍处于预期状态时重新投递。`update`、
`restart`、`destroy`、`purge` 和部署重试会进入管理后台的“待复核”，不会自动重放。
管理员应先确认实例当前状态、容器和数据目录，再选择“确认恢复”或“作废任务”；不要
通过手工调用 Agent 的 destroy/create 接口来绕过该流程。

## 安全要求

- 每个节点使用不同的 Agent 令牌。
- 13092 只允许控制面网段和本机访问。
- 不在页面、日志或接口响应中返回令牌或密文。
- Agent 以 root 运行，因为它需要管理宿主机 Docker；只开放受保护接口。

## 节点登记

在超级管理台填写 Agent 内网地址、独立令牌、CPU 和内存。保存前控制面会调用 `/container/status` 验证令牌、硬件与协议能力。默认 90 秒无心跳的节点不参与新订单分配。
