# TaskTracker

Go 实现的业务任务与价目表管理，单二进制 + 内嵌 Web，便于部署在服务器上。

**编译要求：Go 1.21 或更高**（`go version` 查看）。**生产部署以 Ubuntu 24.04 LTS 为准**（见下文）。若仅用 `apt install golang-go` 且版本偏旧，可能出现 `package slices is not in GOROOT` 等错误，请从 [Go 官方下载页](https://go.dev/dl/) 安装新版，并把 `/usr/local/go/bin` 放在 `PATH` 最前（或卸载/忽略系统自带的旧 `go`）。

## 功能

- **任务**：公司、日期、业务、价格、价目表多选、完成状态与完成日期、月度报表与 CSV 导出
- **Invoice**：可由任务快速生成发票，打开独立页面打印/导出 PDF
- **价目表**：服务项目与价格（多币种）
- **登录**：默认需要登录；**首次启动若无账户**在浏览器打开 **`/setup.html`** 创建管理员；账户信息存于 SQLite 表 `app_user`（密码 bcrypt）
- **存储**：SQLite 数据库文件 **`DATA_DIR/tasktracker.db`**（新安装）；若目录下仅有旧版 **`biztracker.db`**，启动时会**自动沿用**。若仍有旧版 **`data.json`** / **`users.json`**，首次在空库启动时会**自动导入一次**

## 运行

```bash
go build -o tasktracker .
./tasktracker
```

默认监听 `:8088`，数据目录 `./data`（可用 `DATA_DIR` 指定）。

### 环境变量

| 变量 | 说明 |
|------|------|
| `LISTEN_ADDR` | 监听地址，默认 `:8088` |
| `DATA_DIR` | 数据目录，默认 `./data` |
| `AUTH_DISABLE` | 设为 `1` 或 `true` 时**关闭**登录（仅建议本机调试） |
| `AUTH_USER` / `AUTH_PASSWORD` | 可选：在**数据库中尚无用户**时自动创建首个用户（密码至少 6 位） |
| `AUTH_SECURE_COOKIE` | 使用 HTTPS 时设为 `true` |

## 在 Ubuntu 24 服务器部署

以下步骤针对 **Ubuntu 24.04 LTS**（`VERSION_ID` 为 `24.x`）。仓库自带的 **`install.sh`** 在 **root 一键安装**时也会校验系统为官方 Ubuntu 24.x。

### 一键脚本 `install.sh`（推荐）

在 **Ubuntu 24.x** 上克隆仓库后，在仓库根目录执行：

```bash
chmod +x install.sh
sudo ./install.sh
```

脚本会在 **Go 版本低于 1.21 或未安装** 时从官方下载并安装到 `/usr/local/go`，随后编译、将二进制安装到 `/opt/tasktracker`、创建 `tasktracker` 用户并启用 **systemd** 服务。非 root 执行时**仅**在当前目录执行 `go build`（需本机已有 Go 1.21+，不校验发行版）。完整参数见 `./install.sh --help`。

### 1. 安装 Go（用于在服务器上编译）

**不要**只依赖 `apt install golang-go`（版本可能仍低于 1.21）。在 [go.dev/dl](https://go.dev/dl/) 下载 **Linux** 对应架构的 `.tar.gz`，然后安装到 `/usr/local/go`，例如：

```bash
sudo apt update
sudo apt install -y git wget ca-certificates
cd /tmp
wget -O go.tgz "在此处粘贴官网提供的 .tar.gz 完整链接"
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go.tgz
echo 'export PATH=/usr/local/go/bin:$PATH' >> ~/.profile
source ~/.profile
go version   # 应显示 go1.21 或更高
```

若已用 `apt` 装过旧版 `go`，请确保 `which go` 指向 `/usr/local/go/bin/go`。

**检查 Go 版本（建议在编译前执行）：**

| 命令 | 说明 |
|------|------|
| `go version` | 应出现 `go1.21` 或更高（如 `go version go1.23.5 linux/amd64`）；若仍是 `go1.19` 等，说明 `PATH` 未指向新安装。 |
| `which go` | 建议为 `/usr/local/go/bin/go`；若为 `/usr/bin/go`，多半是系统旧包，需把 `/usr/local/go/bin` 放在 `PATH` 前面或新开终端。 |
| `go env GOROOT` | 确认标准库来自新安装目录（一般为 `/usr/local/go`），避免混用旧 GOROOT。 |

### 2. 获取代码并编译

```bash
git clone https://github.com/imlei/TaskTracker.git
cd TaskTracker
go build -o tasktracker .
```

也可在本地 Windows/macOS 交叉编译后只上传二进制（见下节「仅上传二进制」）。

### 3. 目录与权限

建议使用专用用户与固定数据目录（便于备份 **`tasktracker.db`** 或旧版 **`biztracker.db`**）：

```bash
sudo useradd --system --home /opt/tasktracker --shell /usr/sbin/nologin tasktracker || true
sudo mkdir -p /opt/tasktracker
sudo cp tasktracker /opt/tasktracker/
sudo chown -R tasktracker:tasktracker /opt/tasktracker
```

数据将写入 `/opt/tasktracker/data`（通过 `DATA_DIR` 指定）。

### 4. systemd 服务（开机自启）

创建 `/etc/systemd/system/tasktracker.service`：

```ini
[Unit]
Description=TaskTracker
After=network.target

[Service]
Type=simple
User=tasktracker
Group=tasktracker
WorkingDirectory=/opt/tasktracker
Environment=DATA_DIR=/opt/tasktracker/data
Environment=LISTEN_ADDR=:8088
# 若前面有 Nginx 终止 TLS，可设为 true
# Environment=AUTH_SECURE_COOKIE=true
ExecStart=/opt/tasktracker/tasktracker
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

启用并启动：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now tasktracker
sudo systemctl status tasktracker
```

查看日志：`journalctl -u tasktracker -f`。

### 5. 防火墙（Ubuntu）

若使用 `ufw`，放行监听端口（默认 8088）：

```bash
sudo apt install -y ufw
sudo ufw allow 8088/tcp
sudo ufw enable
```

生产环境建议在前面加 **Nginx** 或 **Caddy** 做反向代理与 HTTPS，对外只开放 80/443。

### 6. 首次访问

浏览器打开 `http://服务器IP:8088`，按提示在 **`/setup.html`** 创建管理员；若已用环境变量预置 `AUTH_USER` / `AUTH_PASSWORD` 且数据库中已写入首个用户，则直接登录即可。

### 仅上传二进制（交叉编译示例）

在开发机上：

```bash
GOOS=linux GOARCH=amd64 go build -o tasktracker .
```

将 `tasktracker` 上传到 **Ubuntu 24.x** 服务器的 `/opt/tasktracker/`，再按上文配置 systemd 与权限即可。

## 许可证

按仓库所有者约定使用。
