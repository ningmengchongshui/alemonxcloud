# xcloud-agent

`xcloud-agent` 是安装在裸机节点上的 **systemd 服务**，本身不是 Docker 容器。它默认仅监听
`0.0.0.0:13092`，以便主应用容器通过 Docker 网桥访问。控制接口要求
`Authorization: Bearer $XCLOUD_AGENT_TOKEN`；必须由宿主机防火墙限制为仅允许
`xcloud_control` Docker 子网和本机访问，绝不向公网开放。

服务需要能执行宿主机 Docker 命令。先创建供用户容器使用的内部网络：

```bash
docker network create xcloud_network
```

构建二进制并以 root 运行一次即可自动安装或更新服务：二进制会复制到
`/opt/xcloud-agent/xcloud-agent`，写入 `/etc/systemd/system/xcloud-agent.service`，随后执行
`systemctl daemon-reload` 和 `systemctl enable --now xcloud-agent`。systemd 以 `--serve` 参数
运行实际 HTTP 服务，避免安装进程递归启动。

首次安装前请创建 `/etc/xcloud-agent.env`，至少设置不可为空的 `XCLOUD_AGENT_TOKEN`；其他可选
配置见根目录的部署手册。配置文件暂缺不会阻止服务进程启动，但控制接口会拒绝所有请求。

```bash
cd agent && go build -o xcloud-agent .
sudo ./xcloud-agent
systemctl status xcloud-agent
```

仅用于本机开发、且不希望安装 systemd 服务时，显式运行：

```bash
./xcloud-agent --serve
```

Agent 创建的 AlemonX 容器沿用 `docker-compose/docker-compose.alemonx.yml` 的环境变量、`/root`
与工作区数据卷、1 GB 共享内存，以及 CPU/内存资源限制；容器不映射宿主机端口。
