# SimpleTask

当前版本：**0.0.7**（Git 标签 `v0.0.7`；程序启动日志中也会打印版本号）。

Go 实现的业务任务与价目表管理，单二进制 + 内嵌 Web，便于部署在服务器上。（GitHub 仓库名可能仍为 `TaskTracker`，克隆时可指定本地目录为 `SimpleTask`。）

**编译要求：Go 1.22.2 或更高**（`go version` 查看；`go.mod` 中 `toolchain go1.22.2`）。**生产部署以 Ubuntu 24.04 LTS 为准**（见下文）。若仅用 `apt install golang-go` 且版本偏旧，请从 [Go 官方下载页](https://go.dev/dl/) 安装 **1.22.2+**，并把 `/usr/local/go/bin` 放在 `PATH` 最前（或卸载/忽略系统自带的旧 `go`）。

## 功能

- **任务**：公司、日期、业务、价格、价目表多选、完成状态与完成日期、月度报表与 CSV 导出
- **Invoice**：可由任务快速生成发票，打开独立页面打印/导出 PDF
- **价目表**：服务项目与价格（多币种）
- **登录**：默认需要登录；**首次启动若无账户**在浏览器打开 **`/setup.html`** 创建管理员；账户信息存于 SQLite 表 `app_user`（密码 bcrypt）
- **存储**：SQLite 数据库文件 **`DATA_DIR/SimpleTask.db`**（新安装）；若目录下仅有旧版 **`biztracker.db`**，启动时会**自动沿用**。若仍有旧版 **`data.json`** / **`users.json`**，首次在空库启动时会**自动导入一次**

## 运行

```bash
go build -o SimpleTask .
./SimpleTask
```

默认监听 `:8088`，数据目录 `./data`（可用 `DATA_DIR` 指定）。

### 环境变量

| 变量 | 说明 |
|------|------|
| `LISTEN_ADDR` | 监听地址，默认 `:8088` |
| `DATA_DIR` | 数据目录，默认 `./data` |
| `AUTH_DISABLE` | 设为 `1` 或 `true` 时**关闭**登录（仅建议本机调试） |
| `AUTH_USER` / `AUTH_PASSWORD` | 可选：在**数据库中尚无用户**时自动创建首个用户（密码至少 6 位） |
| `AUTH_SECURE_COOKIE` | 使用 **HTTPS**（Nginx TLS 或下方 `TLS_*`）时设为 `true`，登录 Cookie 带 `Secure` |
| `BASE_URL` | 对外访问根 URL（如 `https://example.com`），用于邮件内链接等 |
| `TLS_CERT_FILE` | PEM 证书路径；与 `TLS_KEY_FILE` **同时设置**时，进程用 **HTTPS** 监听 `LISTEN_ADDR`（不设则仅 HTTP） |
| `TLS_KEY_FILE` | TLS 私钥 PEM 路径 |

### HTTPS（SSL）

**方式 A：Nginx 终止 TLS（推荐，与 `install.sh --with-nginx` 搭配）**

**一键脚本（推荐）**：DNS 已指向服务器且已执行 `sudo ./install.sh --with-nginx` 后，在**仓库根目录**执行：

```bash
sudo ./enable-ssl.sh 你的域名
# 或非交互申请证书（需邮箱）：
sudo CERTBOT_EMAIL=you@example.com ./enable-ssl.sh 你的域名
```

脚本会安装 certbot、申请 Let's Encrypt 证书、用 **`deploy/SimpleTask.nginx-https.example.conf`** 写入 Nginx 站点、重载 Nginx，并为 systemd 增加 **`AUTH_SECURE_COOKIE=true`** 与 **`BASE_URL=https://你的域名`**。详见 `./enable-ssl.sh --help`。

**手动**（与 `upgrade.sh --nginx-ssl 域名` 类似，但证书需自行先申请）：

1. 应用仅监听本机：`LISTEN_ADDR=127.0.0.1:8088`，并设置 **`AUTH_SECURE_COOKIE=true`**（以及 **`BASE_URL=https://你的域名`**）。
2. 参考仓库 **`deploy/SimpleTask.nginx-https.example.conf`**，将 `example.com`、证书路径改为你的域名与 Let's Encrypt 路径。
3. 安装证书（示例）：`sudo apt install -y certbot python3-certbot-nginx`，然后 `sudo certbot --nginx -d your.domain`（按提示操作）。
4. 防火墙放行 443（见下文「防火墙」）。

**方式 B：由 Go 进程直接提供 HTTPS**

同时设置 **`TLS_CERT_FILE`**、**`TLS_KEY_FILE`** 与 **`LISTEN_ADDR`**（例如 `:8443`；监听 **`:443`** 在 Linux 上通常需 root 或 `setcap`）。若未手动设置 **`AUTH_SECURE_COOKIE`**，程序会自动启用安全 Cookie。

## 在 Ubuntu 24 服务器部署

以下步骤针对 **Ubuntu 24.04 LTS**（`VERSION_ID` 为 `24.x`）。仓库自带的 **`install.sh`** 在 **root 一键安装**时也会校验系统为官方 Ubuntu 24.x。

### 一键脚本 `install.sh`（推荐）

在 **Ubuntu 24.x** 上克隆仓库后，在仓库根目录执行：

```bash

sudo apt update
sudo apt install -y git wget ca-certificates

git clone https://github.com/imlei/TaskTracker.git SimpleTask
cd SimpleTask

chmod +x install.sh
sudo ./install.sh --with-nginx
```

脚本会在 **Go 版本低于 1.22.2 或未安装** 时从官方下载并安装到 `/usr/local/go`，随后编译、将二进制安装到 `/opt/SimpleTask`、创建 `SimpleTask` 用户并启用 **systemd** 服务。非 root 执行时**仅**在当前目录执行 `go build`（需本机已有 **Go 1.22.2+**，不校验发行版）。完整参数见 `./install.sh --help`。

**一键附带 Nginx 反向代理（推荐生产）**：在同一 Ubuntu 24.x 上执行：

```bash
sudo ./install.sh --with-nginx
```

效果：`SimpleTask` 仅监听 **`127.0.0.1:8088`**（不对外网暴露应用端口），**Nginx** 监听 **80** 并反代到本机；会安装 `nginx`、写入站点配置、禁用默认 `default` 站点（避免与 80 端口冲突）、`nginx -t` 后 reload。若需自定义应用端口，可再加 `--listen :9090`（脚本会将应用绑定为 `127.0.0.1:9090` 并同步写入 Nginx 的 `proxy_pass`）。

### 后续更新 /「自己升级」（推荐流程）

程序本身**不会**在运行时从互联网拉取新版本二进制；要在有新提交时自动或半自动更新，请在服务器上**保留一份 Git 克隆**（例如 `/opt/SimpleTask/SimpleTask`），使用仓库里的 **`upgrade.sh`**：

```bash
cd /path/to/SimpleTask   # 与首次 git clone 的目录一致
chmod +x upgrade.sh
sudo ./upgrade.sh
```

脚本会：`git fetch` / `git pull`（默认 `origin main`）→ 按 `go.mod` 编译 → 将 `SimpleTask` 安装到 **`PREFIX`**（默认 `/opt/SimpleTask`）→ **`systemctl restart SimpleTask`**（若已启用该服务）。环境变量 **`GIT_BRANCH`**、**`GIT_REMOTE`**、**`GOTOOLCHAIN`**、**`PREFIX`** 可覆盖默认行为；仅编译不装服务时可用普通用户执行，脚本会提示 `sudo` 命令。

**升级同时刷新 Nginx（可选）**：仓库内 **`deploy/`** 模板更新后，可在升级时一并写回 Nginx 配置（需 **root**）：

- 仅 HTTP 反代：`sudo ./upgrade.sh --nginx` 或 `sudo UPGRADE_NGINX=1 ./upgrade.sh`
- HTTPS（需已存在 Let's Encrypt 证书，域名与 `server_name`、证书目录一致）：`sudo ./upgrade.sh --nginx-ssl your.domain.com`

若当前 **已是最新提交**，仍会执行 Nginx 刷新（便于只同步配置）。`./upgrade.sh --help` 可查看全部选项。

**定时检查更新（示例）**：用 root 的 crontab 每周拉一次 main（生产环境请谨慎：**直接跟踪 main 可能引入未充分测试的提交**，更稳妥的是打 **Git tag**，在 CI 里发布再部署，或只拉指定 tag/分支）：

```cron
0 4 * * 0 cd /opt/SimpleTask/SimpleTask && sudo ./upgrade.sh >>/var/log/SimpleTask-upgrade.log 2>&1
```

也可用 **systemd timer** 调用同一脚本，思路与 cron 相同。

### 1. 安装 Go（用于在服务器上编译）

**不要**只依赖 `apt install golang-go`（版本可能低于 1.22.2）。在 [go.dev/dl](https://go.dev/dl/) 下载 **Linux** 对应架构的 **1.22.2 或更高** 的 `.tar.gz`，然后安装到 `/usr/local/go`，例如：

```bash
sudo apt update
sudo apt install -y git wget ca-certificates
cd /tmp
wget -O go.tgz "在此处粘贴官网提供的 .tar.gz 完整链接"
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go.tgz
echo 'export PATH=/usr/local/go/bin:$PATH' >> ~/.profile
source ~/.profile
go version   # 应显示 go1.22.2 或更高
```

若已用 `apt` 装过旧版 `go`，请确保 `which go` 指向 `/usr/local/go/bin/go`。

**检查 Go 版本（建议在编译前执行）：**

| 命令 | 说明 |
|------|------|
| `go version` | 应出现 **go1.22.2** 或更高（如 `go version go1.22.2 linux/amd64`）；若低于 1.22.2，说明需升级或 `PATH` 未指向新安装。 |
| `which go` | 建议为 `/usr/local/go/bin/go`；若为 `/usr/bin/go`，多半是系统旧包，需把 `/usr/local/go/bin` 放在 `PATH` 前面或新开终端。 |
| `go env GOROOT` | 确认标准库来自新安装目录（一般为 `/usr/local/go`），避免混用旧 GOROOT。 |

### 2. 获取代码并编译

```bash
git clone https://github.com/imlei/TaskTracker.git SimpleTask
cd SimpleTask
go build -o SimpleTask .
```

也可在本地 Windows/macOS 交叉编译后只上传二进制（见下节「仅上传二进制」）。

### 3. 目录与权限

建议使用专用用户与固定数据目录（便于备份 **`SimpleTask.db`** 或旧版 **`biztracker.db`**）：

```bash
sudo useradd --system --home /opt/SimpleTask --shell /usr/sbin/nologin SimpleTask || true
sudo mkdir -p /opt/SimpleTask
sudo cp SimpleTask /opt/SimpleTask/
sudo chown -R SimpleTask:SimpleTask /opt/SimpleTask
```

数据将写入 `/opt/SimpleTask/data`（通过 `DATA_DIR` 指定）。

### 4. systemd 服务（开机自启）

创建 `/etc/systemd/system/SimpleTask.service`：

```ini
[Unit]
Description=SimpleTask
After=network.target

[Service]
Type=simple
User=SimpleTask
Group=SimpleTask
WorkingDirectory=/opt/SimpleTask
Environment=DATA_DIR=/opt/SimpleTask/data
Environment=LISTEN_ADDR=:8088
# 若前面有 Nginx 终止 TLS，可设为 true
# Environment=AUTH_SECURE_COOKIE=true
ExecStart=/opt/SimpleTask/SimpleTask
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

启用并启动：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now SimpleTask
sudo systemctl status SimpleTask
```

查看日志：`journalctl -u SimpleTask -f`。

### 5. Nginx 反向代理（手动配置）

若未使用 `install.sh --with-nginx`，可自行安装 Nginx，并将模板 **`deploy/SimpleTask.nginx.conf`** 中的 `@PORT@` 替换为应用端口（与 `LISTEN_ADDR` 一致，默认 `8088`），放到 `/etc/nginx/sites-available/SimpleTask` 并启用：

```bash
sudo apt install -y nginx
sudo sed "s/@PORT@/8088/g" deploy/SimpleTask.nginx.conf | sudo tee /etc/nginx/sites-available/SimpleTask >/dev/null
sudo ln -sf /etc/nginx/sites-available/SimpleTask /etc/nginx/sites-enabled/SimpleTask
sudo rm -f /etc/nginx/sites-enabled/default   # 若与默认站点冲突
sudo nginx -t && sudo systemctl reload nginx
```

同时把 **systemd** 里的 `LISTEN_ADDR` 改为仅本机，例如 **`127.0.0.1:8088`**（或与你替换的端口一致），避免外网直连应用：

```bash
sudo systemctl edit SimpleTask
# 在 [Service] 下写入：
# Environment=LISTEN_ADDR=127.0.0.1:8088
sudo systemctl daemon-reload
sudo systemctl restart SimpleTask
```

HTTPS 详见上文「**HTTPS（SSL）**」；Nginx 示例见 **`deploy/SimpleTask.nginx-https.example.conf`**。

### 6. 防火墙（Ubuntu）

**未使用 Nginx** 时，若使用 `ufw`，放行应用端口（默认 8088）：

```bash
sudo apt install -y ufw
sudo ufw allow 8088/tcp
sudo ufw enable
```

**已使用 Nginx 反代**（`install.sh --with-nginx` 或按上一节配置）时，对外只需 **80/443**，不必再对公网开放 8088：

```bash
sudo ufw allow 'Nginx HTTP'
sudo ufw allow 'Nginx HTTPS'   # 配置 TLS 后
# 若曾放行过 8088，可删除：sudo ufw delete allow 8088/tcp
sudo ufw enable
```

### 7. 首次访问

- **直连应用端口**：浏览器打开 `http://服务器IP:8088`（若配置了 TLS，则为 `https://…` 与对应端口）。
- **经 Nginx（HTTP）**：打开 `http://服务器IP/` 或你的域名。
- **经 Nginx（HTTPS）**：打开 `https://你的域名/`。

按提示在 **`/setup.html`** 创建管理员；若已用环境变量预置 `AUTH_USER` / `AUTH_PASSWORD` 且数据库中已写入首个用户，则直接登录即可。

### 8. 仅上传二进制（交叉编译示例）

在开发机上：

```bash
GOOS=linux GOARCH=amd64 go build -o SimpleTask .
```

将 `SimpleTask` 上传到 **Ubuntu 24.x** 服务器的 `/opt/SimpleTask/`，再按上文配置 systemd 与权限即可。

## 许可证

按仓库所有者约定使用。