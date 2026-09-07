# xcloud-control

用户服务器上的自建节点 Agent。它以出站 WSS 连接到 Tunnel Gateway，并只管理平台审核镜像创建的受管 Docker Compose 实例。

1. 在 xCloud「自建节点」生成一次性接入 Token。
2. 写入 `/etc/xcloud-control/config.json`，仅包含 `tunnelURL` 与 `enrollToken`。
3. 执行 `sudo ./xcloud-control install`，然后启动 `xcloud-control` systemd 服务。

完整接入步骤见 [部署指南](../docs/03-部署指南.md#6-用户自建节点接入)，变量与权限边界见 [配置参考](../docs/00-配置参考.md)。旧版本地目标 URL 反代已下线，历史设备必须重新接入。
