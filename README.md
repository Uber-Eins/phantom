# Phantom

Phantom 是面向个人单机部署的 Xray WebUI。它保留登录与 2FA、仪表盘、本机入站、客户端、流量与在线统计、单客户端分享链接和二维码、Xray 出站与路由，以及数据库备份恢复。

运行模型固定为单管理员、单机 Xray、SQLite、rootless Podman 6+ 和用户级 Quadlet。镜像只发布到 `ghcr.io/uber-eins/phantom`，`stable` 是唯一维护的部署通道。

## 前置条件

- Linux `amd64`
- Podman 6 或更高版本
- systemd 用户实例和 cgroup v2
- rootless 网络允许所需入站端口

本项目使用 Host 网络，不配置端口映射。若要让 rootless 进程监听 `80–1023`，宿主机必须由管理员设置：

```sh
sudo sysctl -w net.ipv4.ip_unprivileged_port_start=80
```

要持久化此值，请按发行版方式写入 sysctl 配置。面板默认监听 `2053`，本机 Xray 入站可使用 `80–65535`。

## 安装

```sh
install -d -m 700 "$HOME/.local/share/phantom"
podman quadlet install ./deploy/quadlet/phantom.container
systemctl --user daemon-reload
systemctl --user enable --now phantom.service
```

首次启动后访问 `http://<服务器地址>:2053/`。初始账号和密码均为 `admin`；登录后应立即修改密码并按需启用 2FA。

让服务在用户退出登录后继续运行：

```sh
loginctl enable-linger "$USER"
```

查看状态、健康检查和日志：

```sh
systemctl --user status phantom.service
podman healthcheck run phantom
journalctl --user -u phantom.service
```

## 自动更新

Quadlet 跟踪 `ghcr.io/uber-eins/phantom:stable`，并声明 `AutoUpdate=registry`。启用用户级更新计时器：

```sh
systemctl --user enable --now podman-auto-update.timer
podman auto-update --dry-run
```

镜像替换后，SQLite 数据仍保存在
`$HOME/.local/share/phantom`。面板仅通过 `stable` 镜像更新；Xray Core
也可在仪表盘中单独切换版本。Xray 更新写入容器层，因此替换镜像后会恢复为镜像内置版本。

## 备份与恢复

仪表盘的“备份与恢复”入口可以下载 SQLite 备份并恢复备份。恢复会替换当前数据库并重启本机 Xray；操作前请另存一份最新备份。

也可以在服务停止时备份整个数据目录：

```sh
systemctl --user stop phantom.service
cp -a "$HOME/.local/share/phantom" "$HOME/.local/share/phantom.backup"
systemctl --user start phantom.service
```

恢复目录备份时同样先停止服务，再将备份内容复制回
`$HOME/.local/share/phantom`，然后启动服务。

更多 Quadlet 运维说明见 [deploy/README.md](deploy/README.md)。
