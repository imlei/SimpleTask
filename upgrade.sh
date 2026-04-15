#!/usr/bin/env bash
# 在服务器上升级 SimpleTask：编译新版本、替换二进制、更新服务、配置 Nginx（若有）。
# 须在仓库根目录执行；root 最佳。
#
# 用法:
#   sudo ./upgrade.sh
#   sudo ./upgrade.sh --with-nginx --domain app.example.com
#   sudo ./upgrade.sh --with-ssl --domain app.example.com --email admin@example.com
#   sudo ./upgrade.sh --prefix /opt/SimpleTask --listen :9090
#
# Options:
#   --prefix DIR      安装目录（默认 /opt/SimpleTask）
#   --listen ADDR     服务监听地址（默认 :8088，写入 systemd）
#   --domain DOMAIN   绑定域名（如 app.example.com）；--with-nginx/--with-ssl 时生效
#   --email EMAIL     Let's Encrypt 通知邮箱（--with-ssl 时可选，强烈推荐填写）
#   --with-nginx      同时更新 Nginx 配置（HTTP）
#   --with-ssl        同时配置 Nginx HTTPS + 申请/续期 Let's Encrypt 证书（含 --with-nginx）
#   -h, --help        显示本说明

set -euo pipefail

PREFIX="${PREFIX:-/opt/SimpleTask}"
LISTEN_ADDR="${LISTEN_ADDR:-:8088}"
DOMAIN="${DOMAIN:-}"
EMAIL="${EMAIL:-}"
WITH_NGINX="${WITH_NGINX:-}"
WITH_SSL="${WITH_SSL:-}"

usage() {
	cat <<'EOF'
Usage: upgrade.sh [options]

  升级 SimpleTask：编译新版本、替换二进制、更新 systemd、数据保留、Nginx + SSL（若指定）。

Options:
  --prefix DIR      安装目录（默认 /opt/SimpleTask）
  --listen ADDR     服务监听地址（默认 :8088，写入 systemd）
  --domain DOMAIN   绑定域名（如 app.example.com）
  --email EMAIL     Let's Encrypt 通知邮箱（推荐填写，证书到期会收到提醒）
  --with-nginx      更新 Nginx HTTP 配置（需 --domain 绑定域名，否则为通配）
  --with-ssl        Nginx HTTPS + 自动申请/续期 Let's Encrypt 证书（含 --with-nginx；需 --domain）
  -h, --help        显示本说明

环境变量（可替代同名选项）:
  PREFIX            安装目录
  LISTEN_ADDR       监听地址
  DOMAIN            绑定域名
  EMAIL             Let's Encrypt 邮箱

示例:
  # 仅升级程序
  sudo ./upgrade.sh

  # 升级 + 绑定域名（HTTP）
  sudo ./upgrade.sh --with-nginx --domain app.example.com

  # 升级 + 绑定域名 + 自动申请 SSL（HTTPS）
  sudo ./upgrade.sh --with-ssl --domain app.example.com --email admin@example.com

  # 换域名（先申请新证书，再更新 Nginx + systemd）
  sudo ./upgrade.sh --with-ssl --domain new.example.com --email admin@example.com
EOF
}

log() { printf '\033[0;34mupgrade.sh:\033[0m %s\n' "$*"; }
ok()  { printf '\033[0;32mupgrade.sh:\033[0m %s\n' "$*"; }
die() { printf '\033[0;31mupgrade.sh: ERROR:\033[0m %s\n' "$*" >&2; exit 1; }

current_version=""
BUILD_SUCCESSFUL=false

# ── Go 环境 ──────────────────────────────────────────────────────────────────
ensure_go() {
	local version
	version="${GOTOOLCHAIN:-go1.22.2}"
	current_version=$(go version 2>/dev/null || echo "")
	if [[ "$current_version" =~ go([0-9]+\.[0-9]+(\.[0-9]+)?) ]]; then
		current_version="${BASH_REMATCH[1]}"
	fi
	case "$current_version" in
	1.2[2-9]*|1.[3-9][0-9]*|[2-9].*) ;;
	*)
		log "Installing Go ${version}..."
		local go_tgz="https://go.dev/dl/${version}.linux-amd64.tar.gz"
		wget -q -O /tmp/go.tgz "${go_tgz}" || die "下载失败: ${go_tgz}"
		rm -rf /usr/local/go
		tar -C /usr/local -xzf /tmp/go.tgz
		rm -f /tmp/go.tgz
		export PATH="/usr/local/go/bin:$PATH"
		;;
	esac
	return 0
}

# ── 编译 ──────────────────────────────────────────────────────────────────────
build_if_needed() {
	if [[ "${NO_BUILD:-}" != "1" ]]; then
		rm -f "$ROOT/SimpleTask.new"
		log "Building SimpleTask..."
		go build -o SimpleTask.new . || die "编译失败"
		ok "SimpleTask binary generated successfully"
		BUILD_SUCCESSFUL=true
	fi
}

# ── 停止服务 ──────────────────────────────────────────────────────────────────
ensure_service_stopped() {
	log "停止 SimpleTask 服务..."
	if systemctl is-active SimpleTask >/dev/null 2>&1; then
		systemctl stop SimpleTask || die "停止 systemd 服务失败"
	fi
	local pids
	pids=$(pgrep -f "SimpleTask" 2>/dev/null || true)
	if [[ -n "$pids" ]]; then
		echo "$pids" | xargs kill -9 2>/dev/null || true
		sleep 2
	fi
}

# ── 替换二进制 ────────────────────────────────────────────────────────────────
upgrade_binary() {
	log "Replacing SimpleTask binary..."
	install -d -m 0755 "$PREFIX"
	install -m 0755 "$ROOT/SimpleTask.new" "$PREFIX/SimpleTask"
	rm -f "$ROOT/SimpleTask.new"
	if ! id "SimpleTask" &>/dev/null; then
		log "Creating SimpleTask user..."
		useradd -r -s /bin/false -d "$PREFIX" SimpleTask || die "创建用户失败"
	fi
	chown -R SimpleTask:SimpleTask "$PREFIX"
}

# ── 数据库迁移 ────────────────────────────────────────────────────────────────
check_and_migrate_database() {
	local data_dir="$PREFIX/data"
	local new_db="$data_dir/SimpleTask.db"
	local old_db="$data_dir/biztracker.db"
	if [[ -f "$new_db" ]]; then
		return 0
	fi
	if [[ -f "$old_db" ]]; then
		log "Migrating database: biztracker.db → SimpleTask.db"
		cp "$old_db" "$new_db"
	fi
}

# ── systemd 服务 ──────────────────────────────────────────────────────────────
# $1: 是否启用 secure cookie（true/false）
# $2: BASE_URL（可为空）
systemd_service_after_upgrade() {
	local secure_cookie="${1:-false}"
	local base_url="${2:-}"

	log "Updating SimpleTask systemd service..."

	local env_lines="Environment=LISTEN_ADDR=$LISTEN_ADDR"
	if [[ "$secure_cookie" == "true" ]]; then
		env_lines+=$'\n'"Environment=AUTH_SECURE_COOKIE=true"
	fi
	if [[ -n "$base_url" ]]; then
		env_lines+=$'\n'"Environment=BASE_URL=$base_url"
	fi

	cat >"/etc/systemd/system/SimpleTask.service" <<EOF
[Unit]
Description=SimpleTask
After=network.target

[Service]
Type=simple
User=SimpleTask
Group=SimpleTask
WorkingDirectory=$PREFIX
${env_lines}
ExecStart=$PREFIX/SimpleTask
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF
	chmod 0644 "/etc/systemd/system/SimpleTask.service"
	systemctl daemon-reload
	ok "systemd service updated"
}

# ── Nginx HTTP 配置 ───────────────────────────────────────────────────────────
upgrade_nginx() {
	local port server_name avail enabled
	port="$(listen_port_from_addr "$LISTEN_ADDR")"
	server_name="${DOMAIN:-_}"

	command -v nginx >/dev/null 2>&1 || {
		log "Nginx not installed, installing..."
		apt-get install -y nginx
	}

	avail="/etc/nginx/sites-available/SimpleTask"
	enabled="/etc/nginx/sites-enabled/SimpleTask"

	log "Writing Nginx HTTP config (server_name: $server_name, port: $port)..."
	cat >"$avail" <<EOF
# SimpleTask — Nginx reverse proxy (HTTP)
# Generated by upgrade.sh on $(date -u +%Y-%m-%dT%H:%M:%SZ)
server {
    listen 80;
    listen [::]:80;
    server_name ${server_name};

    # client_max_body_size 20m;

    location / {
        proxy_pass http://127.0.0.1:${port};
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_redirect off;
    }
}
EOF
	chmod 0644 "$avail"
	ln -sf "$avail" "$enabled"

	if nginx -t 2>/dev/null; then
		systemctl reload nginx
		ok "Nginx HTTP config updated (domain: $server_name)"
	else
		die "Nginx config invalid — check /etc/nginx/sites-available/SimpleTask"
	fi
}

# ── SSL：申请/续期 Let's Encrypt 证书 ─────────────────────────────────────────
request_ssl() {
	[[ -n "$DOMAIN" ]] || die "--with-ssl 需要 --domain 参数（如: --domain app.example.com）"

	local port
	port="$(listen_port_from_addr "$LISTEN_ADDR")"

	# 1. 安装 certbot（若未安装）
	if ! command -v certbot >/dev/null 2>&1; then
		log "Installing certbot..."
		apt-get install -y certbot python3-certbot-nginx || die "certbot 安装失败"
	fi
	ok "certbot ready: $(certbot --version 2>&1)"

	# 2. 先写 HTTP 配置，让 certbot ACME 验证通过
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
		log "警告: 未指定 --email，证书到期不会收到提醒邮件"
	fi

	# 4. 申请证书（certbot --nginx 会自动修改 Nginx 配置添加 SSL）
	log "Requesting SSL certificate for $DOMAIN..."
	if certbot "${certbot_args[@]}"; then
		ok "SSL certificate obtained for $DOMAIN"
	else
		# 若域名 DNS 尚未指向此服务器，certbot 会失败
		die "certbot 失败，请确认:
  1. 域名 $DOMAIN 的 DNS A/AAAA 记录已指向本服务器 IP
  2. 端口 80 和 443 防火墙已放行
  3. 若使用 CDN，请先暂停（certbot 需要直连验证）"
	fi

	# 5. 更新 systemd：启用 secure cookie + BASE_URL
	systemd_service_after_upgrade "true" "https://${DOMAIN}"

	# 6. 设置 certbot 自动续期（systemd timer 或 cron）
	setup_certbot_renewal
}

# ── Certbot 自动续期 ──────────────────────────────────────────────────────────
setup_certbot_renewal() {
	# certbot deb 包通常已经安装了 systemd timer；若没有则写 cron
	if systemctl list-timers --all 2>/dev/null | grep -q certbot; then
		ok "certbot systemd timer already active (auto-renewal enabled)"
		return 0
	fi

	local cron_file="/etc/cron.d/certbot-simpletask"
	if [[ ! -f "$cron_file" ]]; then
		log "Setting up certbot cron renewal..."
		cat >"$cron_file" <<'CRON'
# Let's Encrypt auto-renewal for SimpleTask (added by upgrade.sh)
0 3 * * * root certbot renew --quiet --nginx && systemctl reload nginx
CRON
		chmod 0644 "$cron_file"
		ok "Certbot renewal cron installed: $cron_file"
	else
		ok "Certbot renewal cron already exists: $cron_file"
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
	while [[ $# -gt 0 ]]; do
		case "$1" in
		-h | --help)
			usage; exit 0 ;;
		--prefix)
			[[ -n "${2:-}" ]] || die "--prefix 需要参数"
			PREFIX="$2"; shift 2 ;;
		--listen)
			[[ -n "${2:-}" ]] || die "--listen 需要参数"
			LISTEN_ADDR="$2"; shift 2 ;;
		--domain)
			[[ -n "${2:-}" ]] || die "--domain 需要参数"
			DOMAIN="$2"; shift 2 ;;
		--email)
			[[ -n "${2:-}" ]] || die "--email 需要参数"
			EMAIL="$2"; shift 2 ;;
		--with-nginx)
			WITH_NGINX="1"; shift ;;
		--with-ssl)
			WITH_SSL="1"; WITH_NGINX="1"; shift ;;
		*)
			die "未知选项: $1 （运行 --help 查看用法）" ;;
		esac
	done

	ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
	[[ -f "$ROOT/go.mod" ]] || die "请在仓库根目录执行（缺少 go.mod）"
	[[ "$(uname -s)" == Linux ]] || die "升级仅支持 Linux"

	export PATH="/usr/local/go/bin:$PATH"

	log "=== SimpleTask Upgrade ==="
	log "PREFIX      : $PREFIX"
	log "LISTEN_ADDR : $LISTEN_ADDR"
	[[ -n "$DOMAIN"     ]] && log "DOMAIN      : $DOMAIN"
	[[ -n "$EMAIL"      ]] && log "EMAIL       : $EMAIL"
	[[ "$WITH_SSL"  == "1" ]] && log "SSL         : enabled (Let's Encrypt)"
	[[ "$WITH_NGINX" == "1" && "$WITH_SSL" != "1" ]] && log "NGINX       : HTTP only"

	# 依赖
	apt-get update -qq
	apt-get install -y -qq systemd

	ensure_go
	build_if_needed

	[[ "$BUILD_SUCCESSFUL" == "true" ]] || die "编译未执行，无法升级"
	[[ -f "$ROOT/SimpleTask.new" ]]     || die "编译失败：未生成 SimpleTask 二进制"

	# 停服 → 替换 → 迁库
	if systemctl is-enabled SimpleTask >/dev/null 2>&1; then
		ensure_service_stopped
	fi
	upgrade_binary
	check_and_migrate_database

	# Nginx + SSL
	if [[ "$WITH_SSL" == "1" ]]; then
		request_ssl   # 内部调用 upgrade_nginx + systemd（带 HTTPS 配置）
	elif [[ "$WITH_NGINX" == "1" ]]; then
		upgrade_nginx
		systemd_service_after_upgrade "false" ""
	else
		systemd_service_after_upgrade "false" ""
	fi

	# 启动
	log "启动 SimpleTask..."
	systemctl enable SimpleTask >/dev/null 2>&1 || true
	systemctl start SimpleTask || die "启动失败，查看日志: journalctl -u SimpleTask -n 50"

	ok "=== 升级完成 ==="
	if [[ "$WITH_SSL" == "1" && -n "$DOMAIN" ]]; then
		ok "访问地址  : https://${DOMAIN}"
		ok "证书续期  : 已配置自动续期（certbot renew）"
	elif [[ "$WITH_NGINX" == "1" && -n "$DOMAIN" ]]; then
		ok "访问地址  : http://${DOMAIN}"
		ok "提示      : 加 --with-ssl 可自动申请 SSL 证书"
	else
		ok "访问地址  : http://localhost${LISTEN_ADDR}"
	fi
	ok "二进制    : $PREFIX/SimpleTask"
	ok "systemd   : /etc/systemd/system/SimpleTask.service"
	ok "数据目录  : $PREFIX/data/"
	[[ "${WITH_NGINX}" == "1" ]] && ok "Nginx     : /etc/nginx/sites-available/SimpleTask"
	ok "日志      : journalctl -u SimpleTask -f"
}

main "$@"
