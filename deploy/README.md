# Rootless Quadlet 部署

维护的部署文件只有 [`quadlet/phantom.container`](quadlet/phantom.container)。它使用 Host 网络、SQLite 持久卷、registry 自动更新和本机 HTTP/HTTPS 健康检查。

## 安装或更新单元

```sh
install -d -m 700 "$HOME/.local/share/phantom"
podman quadlet install ./deploy/quadlet/phantom.container
systemctl --user daemon-reload
systemctl --user enable --now phantom.service
```

修改 Quadlet 后重新执行 `podman quadlet install`，再运行：

```sh
systemctl --user daemon-reload
systemctl --user restart phantom.service
```

单元固定使用：

- `ghcr.io/uber-eins/phantom:stable`
- `Network=host`
- `AutoUpdate=registry`
- `%h/.local/share/phantom:/etc/x-ui`
- `TZ=Asia/Singapore`
- `XUI_PORT=2053`

Host 网络意味着不需要也不应配置 `PublishPort`。低位端口由宿主机的
`net.ipv4.ip_unprivileged_port_start` 控制。

## 登录后继续运行

```sh
loginctl enable-linger "$USER"
systemctl --user is-enabled phantom.service
```

## 健康与日志

```sh
systemctl --user status phantom.service
podman inspect --format '{{.State.Health.Status}}' phantom
podman healthcheck run phantom
journalctl --user -u phantom.service
```

健康命令同时尝试本机 HTTP 和 HTTPS，因此启用面板证书后不需要修改 Quadlet。

## Registry 自动更新

```sh
systemctl --user enable --now podman-auto-update.timer
podman auto-update --dry-run
systemctl --user list-timers podman-auto-update.timer
```

只有已经保存的 `stable` 镜像版本会被部署。更新失败时，systemd 保留服务失败状态和日志；SQLite 数据不在镜像层中。
仪表盘中的 Xray Core 版本切换写入当前容器层；镜像更新替换容器后会恢复为镜像内置版本。

## 备份与恢复

在线数据库备份和恢复通过仪表盘完成。完整目录备份需要先停止服务：

```sh
systemctl --user stop phantom.service
cp -a "$HOME/.local/share/phantom" "$HOME/.local/share/phantom.backup"
systemctl --user start phantom.service
```

恢复目录时停止服务，将备份内容复制回
`$HOME/.local/share/phantom`，确认目录只对当前用户可读写，然后重新启动。首次启动和恢复后都可用以下命令验证：

```sh
podman healthcheck run phantom
podman auto-update --dry-run
```
