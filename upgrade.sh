#!/usr/bin/env bash
# 在 Ubuntu 24.04 服务器上升级 SimpleTask
# 须在仓库根目录执行；需要 root 权限。
#
# 用法:
#   sudo ./upgrade.sh
#   sudo ./upgrade.sh --with-nginx --domain app.example.com
#   sudo ./upgrade.sh --with-ssl --domain app.example.com --email admin@example.com
#
# Options:
#   --prefix DIR      安装目录（默认 /opt/SimpleTask）
#   --listen ADDR     服务监听地址（默认 127.0.0.1:8088）
#   --domain DOMAIN   绑定域名（如 app.example.com）
#   --email EMAIL     Let's Encrypt 通知邮箱（推荐填写）
#   --with-nginx      更新 Nginx HTTP 配置
#   --with-ssl        Nginx HTTPS + 申请/续期 Let's Encrypt 证书（含 --with-nginx）
#   -h, --help        显示本说明

set -euo pipefail

# ── 默认值 ───────────────────────────────────────────────────────────────────
PREFIX="${PREFIX:-/opt/SimpleTask}"
LISTEN_ADDR="${LISTEN_ADDR:-127.0.0.1:8088}"
DOMAIN="${DOMAIN:-}"
EMAIL="${EMAIL:-}"
WITH_NGINX="${WITH_NGINX:-}"
WITH_SSL="${WITH_SSL:-}"

BUILD_SUCCESSFUL=false
ROOT=""

# ── 输出函数 ──────────────────────────────────────────────────────────────────
log()  { printf '\e[34m[upgrade]\e[0m %s\n'        "$*"; }
ok()   { printf '\e[32m[upgrade]\e[0m %s\n'        "$*"; }
warn() { printf '\e[33m[upgrade]\e[0m WARNING: %s\n' "$*"; }
die()  { printf '\e[31m[upgrade]\e[0m ERROR: %s\n' "$*" >&2; exit 1; }
step() { printf '\n\e[1;36m── %s ──\e[0m\n' "$*"; }

usage() {
	cat <<'EOF'
Usage: upgrade.sh [options]

针对 Ubuntu 24.04 优化：
  - Go     优先使用 apt（Ubuntu 24.04 仓库自带 1.22.x），无需手动下载
  - certbot 使用 snap（Ubuntu 官方推荐方式，自带自动续期）
  - UFW    自动开放所需端口（80/443 或应用端口）

Options:
  --prefix DIR      安装目录（默认 /opt/SimpleTask）
  --listen ADDR     服务监听地址（默认 127.0.0.1:8088）
  --domain DOMAIN   绑定域名（如 app.example.com）
  --email EMAIL     Let's Encrypt 通知邮箱（证书到期前会收到提醒）
  --with-nginx      更新 Nginx HTTP 配置
  --with-ssl        Nginx HTTPS + 自动申请/续期证书（含 --with-nginx；需 --domain）
  -h, --help        显示本说明

示例:
  # 仅升级程序
  sudo ./upgrade.sh

  # 升级 + 绑定域名（HTTP）
  sudo ./upgrade.sh --with-nginx --domain app.example.com

  # 升级 + 绑定域名 + 自动申请 SSL
  sudo ./upgrade.sh --with-ssl --domain app.example.com --email admin@example.com

  # 换域名/换证书
  sudo ./upgrade.sh --with-ssl --domain new.example.com --email admin@example.com
EOF
}

# ── 系统检查 ──────────────────────────────────────────────────────────────────
check_system() {
	[[ "$(uname -s)" == Linux ]] || die "本脚本仅支持 Linux"
	[[ $EUID -eq 0 ]] || die "需要 root 权限，请使用 sudo 运行"

	if [[ -f /etc/os-release ]]; then
		# shellcheck source=/dev/null
		source /etc/os-release
		if [[ "${ID:-}" != "ubuntu" ]]; then
			warn "当前系统为 ${PRETTY_NAME:-unknown}，本脚本针对 Ubuntu 24.04 优化"
		elif [[ "${VERSION_ID:-}" != "24.04" ]]; then
			warn "当前 Ubuntu 版本为 ${VERSION_ID:-unknown}，推荐 24.04"
		else
			ok "系统: ${PRETTY_NAME}"
		fi
	fi
}

# ── 检测 CPU 架构（用于 Go 下载 URL）─────────────────────────────────────────
detect_arch() {
	case "$(uname -m)" in
		x86_64)  echo "amd64" ;;
		aarch64) echo "arm64" ;;
		armv7l)  echo "armv6l" ;;
		*)       die "不支持的 CPU 架构: $(uname -m)" ;;
	esac
}

# ── Go 环境（Ubuntu 24.04：优先 apt，回退官方 tarball）──────────────────────
GO_MIN_MAJOR=1
GO_MIN_MINOR=22
GO_FALLBACK_VERSION="go1.22.12"   # tarball 回退版本

go_version_ok() {
	local ver
	ver=$(go version 2>/dev/null | grep -oP 'go\K[0-9]+\.[0-9]+' | head -1) || return 1
	local major minor
	major=$(echo "$ver" | cut -d. -f1)
	minor=$(echo "$ver" | cut -d. -f2)
	[[ "$major" -gt "$GO_MIN_MAJOR" ]] || \
		{ [[ "$major" -eq "$GO_MIN_MAJOR" ]] && [[ "$minor" -ge "$GO_MIN_MINOR" ]]; }
}

ensure_go() {
	step "Go 环境"

	# 已安装且版本足够
	if go_version_ok; then
		ok "已安装: $(go version)"
		export PATH="$(go env GOPATH 2>/dev/null || echo "$HOME/go")/bin:$PATH"
		return 0
	fi

	# Ubuntu 24.04：通过 apt 安装（仓库自带 1.22.x，足够用）
	if [[ -f /etc/os-release ]]; then
		source /etc/os-release
		if [[ "${ID:-}" == "ubuntu" ]]; then
			log "通过 apt 安装 golang-go..."
			DEBIAN_FRONTEND=noninteractive apt-get install -y golang-go
			export PATH="/usr/lib/go/bin:$PATH"
			if go_version_ok; then
				ok "Go 安装成功: $(go version)"
				return 0
			fi
		fi
	fi

	# 回退：从 go.dev 下载官方 tarball
	local arch go_url
	arch=$(detect_arch)
	go_url="https://go.dev/dl/${GO_FALLBACK_VERSION}.linux-${arch}.tar.gz"
	log "apt 版本不足，从 go.dev 下载 ${GO_FALLBACK_VERSION} (${arch})..."
	DEBIAN_FRONTEND=noninteractive apt-get install -y -qq wget ca-certificates
	wget -q --show-progress -O /tmp/go.tgz "$go_url" || die "Go 下载失败: $go_url"
	rm -rf /usr/local/go
	tar -C /usr/local -xzf /tmp/go.tgz
	rm -f /tmp/go.tgz
	export PATH="/usr/local/go/bin:$PATH"
	go_version_ok || die "Go 安装失败，请检查网络或手动安装 Go $GO_MIN_MAJOR.$GO_MIN_MINOR+"
	ok "Go 安装成功: $(go version)"
}

# ── 编译 ──────────────────────────────────────────────────────────────────────
build_if_needed() {
	step "编译"
	[[ "${NO_BUILD:-}" == "1" ]] && { log "跳过编译（NO_BUILD=1）"; BUILD_SUCCESSFUL=true; return; }

	rm -f "$ROOT/SimpleTask.new"
	log "go build..."
	go build -o SimpleTask.new . || die "编译失败"
	ok "编译完成: SimpleTask.new"
	BUILD_SUCCESSFUL=true
}

# ── 停止服务 ──────────────────────────────────────────────────────────────────
ensure_service_stopped() {
	step "停止服务"
	if systemctl is-active --quiet SimpleTask 2>/dev/null; then
		log "停止 SimpleTask systemd 服务..."
		systemctl stop SimpleTask
	fi
	# 清理可能残留的进程
	local pids
	pids=$(pgrep -f "$PREFIX/SimpleTask" 2>/dev/null || true)
	if [[ -n "$pids" ]]; then
		log "清理残留进程: $pids"
		echo "$pids" | xargs kill -15 2>/dev/null || true
		sleep 2
		echo "$pids" | xargs kill -9 2>/dev/null || true
	fi
	ok "服务已停止"
}

# ── 替换二进制 ────────────────────────────────────────────────────────────────
upgrade_binary() {
	step "替换二进制"
	install -d -m 0755 "$PREFIX"
	install -d -m 0750 "$PREFIX/data"
	install -m 0755 "$ROOT/SimpleTask.new" "$PREFIX/SimpleTask"
	rm -f "$ROOT/SimpleTask.new"

	# 创建专用系统用户
	if ! id "simpletask" &>/dev/null; then
		log "创建系统用户 simpletask..."
		useradd --system --shell /usr/sbin/nologin \
			--home-dir "$PREFIX" --no-create-home simpletask
	fi
	chown -R simpletask:simpletask "$PREFIX"
	ok "二进制已替换: $PREFIX/SimpleTask"
}

# ── 数据库迁移 ────────────────────────────────────────────────────────────────
check_and_migrate_database() {
	local data_dir="$PREFIX/data"
	local new_db="$data_dir/SimpleTask.db"
	local old_db="$data_dir/biztracker.db"
	[[ -f "$new_db" ]] && return 0
	if [[ -f "$old_db" ]]; then
		log "迁移数据库: biztracker.db → SimpleTask.db"
		cp "$old_db" "$new_db"
		chown simpletask:simpletask "$new_db"
		ok "数据库迁移完成"
	fi
}

# ── systemd 服务 ──────────────────────────────────────────────────────────────
# $1: AUTH_SECURE_COOKIE (true/false)
# $2: BASE_URL (可为空)
systemd_service_after_upgrade() {
	local secure_cookie="${1:-false}"
	local base_url="${2:-}"
	step "systemd 服务"

	# 构建 Environment 行（多行拼接）
	local env_block
	env_block="Environment=LISTEN_ADDR=${LISTEN_ADDR}"
	[[ "$secure_cookie" == "true" ]] && env_block+=$'\n'"Environment=AUTH_SECURE_COOKIE=true"
	[[ -n "$base_url" ]]             && env_block+=$'\n'"Environment=BASE_URL=${base_url}"

	cat >"/etc/systemd/system/SimpleTask.service" <<EOF
[Unit]
Description=SimpleTask Application
After=network.target

[Service]
Type=simple
User=simpletask
Group=simpletask
WorkingDirectory=${PREFIX}
${env_block}
ExecStart=${PREFIX}/SimpleTask
Restart=on-failure
RestartSec=5
# 安全加固
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ReadWritePaths=${PREFIX}/data

[Install]
WantedBy=multi-user.target
EOF
	chmod 0644 "/etc/systemd/system/SimpleTask.service"
	systemctl daemon-reload
	ok "systemd 服务已更新"
	[[ "$secure_cookie" == "true" ]] && ok "  AUTH_SECURE_COOKIE=true"
	[[ -n "$base_url" ]]             && ok "  BASE_URL=${base_url}"
}

# ── UFW 防火墙 ────────────────────────────────────────────────────────────────
setup_ufw() {
	local port="$1"
	command -v ufw >/dev/null 2>&1 || return 0
	ufw status 2>/dev/null | grep -q "Status: active" || return 0

	step "UFW 防火墙"
	if [[ "$WITH_SSL" == "1" || "$WITH_NGINX" == "1" ]]; then
		# 放行 Nginx（80 + 443）
		ufw allow "Nginx Full" 2>/dev/null || {
			ufw allow 80/tcp
			ufw allow 443/tcp
		}
		ok "UFW: 已放行 80/tcp 和 443/tcp"
	else
		# 直接暴露应用端口
		ufw allow "${port}/tcp"
		ok "UFW: 已放行 ${port}/tcp"
	fi
}

# ── Nginx 安装 + HTTP 配置 ────────────────────────────────────────────────────
upgrade_nginx() {
	local port server_name avail enabled
	port="$(listen_port_from_addr "$LISTEN_ADDR")"
	server_name="${DOMAIN:-_}"
	avail="/etc/nginx/sites-available/SimpleTask"
	enabled="/etc/nginx/sites-enabled/SimpleTask"

	# 安装 nginx
	if ! command -v nginx >/dev/null 2>&1; then
		log "安装 nginx..."
		DEBIAN_FRONTEND=noninteractive apt-get install -y nginx
		systemctl enable --now nginx
	fi

	# 清理 sites-enabled 中所有失效的软链接（指向不存在文件），避免 nginx -t 报错
	local broken
	for broken in /etc/nginx/sites-enabled/*; do
		# 是符号链接但目标不存在
		if [[ -L "$broken" && ! -e "$broken" ]]; then
			log "移除失效 nginx 链接: $broken"
			rm -f "$broken"
		fi
	done

	# 禁用 default 站点（避免 80 端口冲突）
	[[ -L /etc/nginx/sites-enabled/default || -f /etc/nginx/sites-enabled/default ]] && {
		rm -f /etc/nginx/sites-enabled/default
		log "已禁用 nginx default 站点"
	}

	log "写入 Nginx HTTP 配置 (server_name=${server_name}, upstream=127.0.0.1:${port})..."
	cat >"$avail" <<NGINX
# SimpleTask — Nginx HTTP reverse proxy
# Generated by upgrade.sh  $(date -u +%Y-%m-%dT%H:%M:%SZ)
server {
    listen 80;
    listen [::]:80;
    server_name ${server_name};

    # client_max_body_size 20m;

    location / {
        proxy_pass         http://127.0.0.1:${port};
        proxy_http_version 1.1;
        proxy_set_header   Host              \$host;
        proxy_set_header   X-Real-IP         \$remote_addr;
        proxy_set_header   X-Forwarded-For   \$proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto \$scheme;
        proxy_redirect     off;
        proxy_read_timeout 60s;
    }
}
NGINX
	chmod 0644 "$avail"
	ln -sf "$avail" "$enabled"

	nginx -t || die "Nginx 配置检查失败，请查看: $avail"
	if systemctl is-active --quiet nginx; then
		systemctl reload nginx
	else
		systemctl enable --now nginx
	fi
	ok "Nginx HTTP 配置已更新 (域名: ${server_name})"
}

# ── certbot（Ubuntu 24.04 官方推荐：snap）────────────────────────────────────
install_certbot() {
	if command -v certbot >/dev/null 2>&1; then
		ok "certbot 已安装: $(certbot --version 2>&1)"
		return 0
	fi

	# Ubuntu 24.04 官方推荐方式：snap
	log "通过 snap 安装 certbot（Ubuntu 24.04 官方推荐）..."

	# 确保 snapd 已安装并运行
	if ! command -v snap >/dev/null 2>&1; then
		log "安装 snapd..."
		DEBIAN_FRONTEND=noninteractive apt-get install -y snapd
	fi
	systemctl enable --now snapd.socket 2>/dev/null || true
	# snapd 需要短暂初始化时间
	for i in 1 2 3; do
		snap list core &>/dev/null && break
		log "等待 snapd 就绪 (${i}/3)..."
		sleep 4
	done

	snap install core 2>/dev/null || true
	snap refresh core  2>/dev/null || true
	snap install --classic certbot || die "certbot snap 安装失败"

	# 将 snap certbot 链接到 PATH
	ln -sf /snap/bin/certbot /usr/bin/certbot
	ok "certbot 安装完成: $(certbot --version 2>&1)"
}

# ── SSL：申请/续期证书 ────────────────────────────────────────────────────────
request_ssl() {
	[[ -n "$DOMAIN" ]] || die "--with-ssl 需要 --domain（例: --domain app.example.com）"

	step "SSL 证书"

	# 1. 安装 certbot
	install_certbot

	# 2. 写 HTTP nginx 配置（certbot ACME 验证需要 80 端口可访问）
	upgrade_nginx

	# 3. 构造 certbot 参数
	local certbot_args=(
		--nginx
		--non-interactive
		--agree-tos
		-d "$DOMAIN"
	)
	if [[ -n "$EMAIL" ]]; then
		certbot_args+=(--email "$EMAIL")
	else
		certbot_args+=(--register-unsafely-without-email)
		warn "未指定 --email：证书到期前不会收到提醒邮件"
	fi

	# 4. 申请证书（certbot --nginx 自动修改 nginx 配置并添加 HTTPS）
	log "为 ${DOMAIN} 申请 Let's Encrypt 证书..."
	if certbot "${certbot_args[@]}"; then
		ok "证书申请成功: /etc/letsencrypt/live/${DOMAIN}/"
	else
		die "certbot 失败，请检查:
  1. 域名 ${DOMAIN} 的 DNS A/AAAA 记录是否已指向本服务器公网 IP
  2. 端口 80（HTTP）是否开放（用于 ACME 验证）
  3. 端口 443（HTTPS）是否开放
  4. 若启用了 CDN 代理（Cloudflare 橙色云），请暂时关闭后重试"
	fi

	# 5. 更新 systemd：启用安全 Cookie + BASE_URL
	systemd_service_after_upgrade "true" "https://${DOMAIN}"

	# 6. snap certbot 会自动设置 systemd timer 续期，确认一下
	check_certbot_renewal
}

# ── 确认续期已配置 ────────────────────────────────────────────────────────────
check_certbot_renewal() {
	step "证书自动续期"

	# snap certbot 自动安装 systemd timer
	if systemctl is-active --quiet snap.certbot.renew.timer 2>/dev/null || \
	   systemctl list-timers --all 2>/dev/null | grep -q certbot; then
		ok "自动续期 systemd timer 已激活"
		return 0
	fi

	# 手动验证 certbot renew（dry-run）
	if certbot renew --dry-run --quiet 2>/dev/null; then
		ok "certbot renew dry-run 通过"
	else
		warn "certbot renew dry-run 有警告，请手动检查: certbot renew --dry-run"
	fi

	# 回退：写 cron（不应执行到这里，snap 应已配置 timer）
	local cron_file="/etc/cron.d/certbot-simpletask"
	if [[ ! -f "$cron_file" ]]; then
		cat >"$cron_file" <<'CRON'
# Let's Encrypt auto-renewal — added by upgrade.sh
0 3 * * * root certbot renew --quiet --nginx && systemctl reload nginx
CRON
		chmod 0644 "$cron_file"
		ok "已写入续期 cron: $cron_file"
	fi
}

# ── 工具函数 ──────────────────────────────────────────────────────────────────
listen_port_from_addr() {
	local addr="$1"
	if [[ "$addr" =~ :([0-9]+)$ ]]; then
		printf '%s' "${BASH_REMATCH[1]}"
	else
		printf '%s' "8088"
	fi
}

# ── 主流程 ────────────────────────────────────────────────────────────────────
main() {
	# 解析参数
	while [[ $# -gt 0 ]]; do
		case "$1" in
		-h|--help)    usage; exit 0 ;;
		--prefix)     [[ -n "${2:-}" ]] || die "--prefix 需要参数"; PREFIX="$2";      shift 2 ;;
		--listen)     [[ -n "${2:-}" ]] || die "--listen 需要参数"; LISTEN_ADDR="$2"; shift 2 ;;
		--domain)     [[ -n "${2:-}" ]] || die "--domain 需要参数"; DOMAIN="$2";      shift 2 ;;
		--email)      [[ -n "${2:-}" ]] || die "--email 需要参数";  EMAIL="$2";       shift 2 ;;
		--with-nginx) WITH_NGINX="1";                   shift ;;
		--with-ssl)   WITH_SSL="1"; WITH_NGINX="1";     shift ;;
		*)            die "未知选项: $1（运行 --help 查看用法）" ;;
		esac
	done

	ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
	[[ -f "$ROOT/go.mod" ]] || die "请在仓库根目录执行（未找到 go.mod）"

	# 系统检查（必须 root）
	check_system

	# 打印配置摘要
	printf '\n\e[1m=== SimpleTask Upgrade ===\e[0m\n'
	log "PREFIX      : $PREFIX"
	log "LISTEN_ADDR : $LISTEN_ADDR"
	[[ -n "$DOMAIN"          ]] && log "DOMAIN      : $DOMAIN"
	[[ -n "$EMAIL"           ]] && log "EMAIL       : $EMAIL"
	[[ "$WITH_SSL"   == "1"  ]] && log "模式        : HTTPS + Let's Encrypt"
	[[ "$WITH_NGINX" == "1" && "$WITH_SSL" != "1" ]] && log "模式        : HTTP + Nginx"
	[[ "$WITH_NGINX" != "1"  ]] && log "模式        : 直接监听 ${LISTEN_ADDR}"
	printf '\n'

	# 基础依赖（ca-certificates 用于 HTTPS 下载；wget 用于可能的 Go 下载）
	step "系统依赖"
	export DEBIAN_FRONTEND=noninteractive
	apt-get update -qq
	apt-get install -y -qq ca-certificates curl wget lsof

	# Go + 编译
	ensure_go
	export PATH="/usr/local/go/bin:/usr/lib/go/bin:$PATH"
	build_if_needed

	[[ "$BUILD_SUCCESSFUL" == "true" ]] || die "编译步骤未执行"
	[[ -f "$ROOT/SimpleTask.new" ]]     || die "编译失败：未找到 SimpleTask.new"

	# 停服 → 替换二进制 → 迁移数据库
	if systemctl is-enabled --quiet SimpleTask 2>/dev/null; then
		ensure_service_stopped
	fi
	upgrade_binary
	check_and_migrate_database

	# UFW 防火墙
	setup_ufw "$(listen_port_from_addr "$LISTEN_ADDR")"

	# Nginx + SSL
	if [[ "$WITH_SSL" == "1" ]]; then
		request_ssl    # 内部调用 upgrade_nginx + systemd（带 HTTPS 配置）
	elif [[ "$WITH_NGINX" == "1" ]]; then
		upgrade_nginx
		systemd_service_after_upgrade "false" ""
	else
		systemd_service_after_upgrade "false" ""
	fi

	# 启动服务
	step "启动服务"
	systemctl enable SimpleTask >/dev/null 2>&1 || true
	systemctl start SimpleTask || die "启动失败，查看日志: journalctl -u SimpleTask -n 50 --no-pager"
	sleep 1
	if systemctl is-active --quiet SimpleTask; then
		ok "SimpleTask 正在运行"
	else
		die "服务启动异常，查看: journalctl -u SimpleTask -n 50 --no-pager"
	fi

	# 完成摘要
	printf '\n\e[1m=== 升级完成 ===\e[0m\n'
	if [[ "$WITH_SSL" == "1" && -n "$DOMAIN" ]]; then
		ok "访问地址  : https://${DOMAIN}"
		ok "证书位置  : /etc/letsencrypt/live/${DOMAIN}/"
		ok "自动续期  : snap/systemd timer 已激活（certbot renew）"
	elif [[ "$WITH_NGINX" == "1" && -n "$DOMAIN" ]]; then
		ok "访问地址  : http://${DOMAIN}"
		ok "提示      : 加 --with-ssl 可自动申请 HTTPS 证书"
	else
		ok "访问地址  : http://localhost:$(listen_port_from_addr "$LISTEN_ADDR")"
	fi
	ok "二进制    : $PREFIX/SimpleTask"
	ok "数据目录  : $PREFIX/data/"
	ok "systemd   : /etc/systemd/system/SimpleTask.service"
	[[ "$WITH_NGINX" == "1" ]] && ok "Nginx 配置: /etc/nginx/sites-available/SimpleTask"
	ok "查看日志  : journalctl -u SimpleTask -f"
	ok "服务状态  : systemctl status SimpleTask"
}

main "$@"
