#!/usr/bin/env bash
# 在服务器上为 SimpleTask 启用 HTTPS（Nginx 终止 TLS + Let's Encrypt）。
# 须在仓库根目录执行；需 root；域名 DNS 已指向本机；建议已用 install.sh --with-nginx 装好 HTTP 反代。
#
# 用法:
#   sudo ./enable-ssl.sh your.domain.com
#   sudo CERTBOT_EMAIL=you@example.com ./enable-ssl.sh your.domain.com
#   sudo ./enable-ssl.sh --email you@example.com your.domain.com

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NGINX_APP_PORT="${NGINX_APP_PORT:-}"

usage() {
	cat <<'EOF'
Usage: enable-ssl.sh [options] DOMAIN

  为 SimpleTask 配置 HTTPS：安装 certbot（若缺失）、申请/检测 Let’s Encrypt 证书、
  写入 deploy/tasktracker.nginx-https.example.conf、重载 Nginx，
  并为 tasktracker systemd 增加 AUTH_SECURE_COOKIE=true 与 BASE_URL=https://DOMAIN。

  前置条件:
    - root 执行
    - Ubuntu 等已安装 Nginx；建议已执行: sudo ./install.sh --with-nginx
    - 域名 DOMAIN 的 DNS 已指向本服务器公网 IP
    - 防火墙放行 80/443

Options:
  --email ADDR   Let’s Encrypt 注册邮箱（非交互申请证书时必需；也可环境变量 CERTBOT_EMAIL）
  -h, --help     显示本说明

环境变量:
  NGINX_APP_PORT  反代到本机端口（默认从 systemd tasktracker 的 LISTEN_ADDR 解析，否则 8088）
  CERTBOT_EMAIL     同 --email

示例:
  sudo ./enable-ssl.sh app.example.com
  sudo CERTBOT_EMAIL=admin@example.com ./enable-ssl.sh app.example.com
EOF
}

log() { printf '%s\n' "$*"; }
die() { printf 'enable-ssl.sh: %s\n' "$*" >&2; exit 1; }

# 与 upgrade.sh 一致：从 systemd 或环境变量解析应用端口
nginx_app_port() {
	local p="${NGINX_APP_PORT:-}"
	if [[ -n "$p" ]]; then
		if [[ "$p" =~ :([0-9]+)$ ]]; then
			printf '%s' "${BASH_REMATCH[1]}"
		else
			printf '%s' "$p"
		fi
		return 0
	fi
	if systemctl show tasktracker -p Environment &>/dev/null; then
		local line
		line="$(systemctl show tasktracker -p Environment --value 2>/dev/null | tr ' ' '\n' | grep '^LISTEN_ADDR=' | head -1 || true)"
		if [[ -n "$line" ]]; then
			line="${line#LISTEN_ADDR=}"
			line="${line//\"/}"
			if [[ "$line" =~ :([0-9]+)$ ]]; then
				printf '%s' "${BASH_REMATCH[1]}"
				return 0
			fi
		fi
	fi
	printf '%s' "8088"
}

apply_nginx_https_template() {
	local domain port tmpl avail enabled
	domain="$1"
	port="$(nginx_app_port)"
	tmpl="$ROOT/deploy/tasktracker.nginx-https.example.conf"
	[[ -f "$tmpl" ]] || die "缺少模板: $tmpl"
	cert="/etc/letsencrypt/live/${domain}/fullchain.pem"
	key="/etc/letsencrypt/live/${domain}/privkey.pem"
	[[ -f "$cert" && -f "$key" ]] || die "未找到证书: $cert 与 $key"
	avail="/etc/nginx/sites-available/tasktracker"
	enabled="/etc/nginx/sites-enabled/tasktracker"
	sed -e "s/example.com/${domain}/g" -e "s/@PORT@/${port}/g" "$tmpl" >"$avail"
	chmod 0644 "$avail"
	ln -sf "$avail" "$enabled"
	log "nginx: 已写入 HTTPS 配置 $avail（→ 127.0.0.1:${port}）"
}

# 将 HTTP 站点中的 server_name 改为具体域名，便于 certbot 校验
ensure_server_name_for_certbot() {
	local domain avail f
	domain="$1"
	avail="/etc/nginx/sites-available/tasktracker"
	[[ -f "$avail" ]] || return 0
	if grep -q 'server_name _;' "$avail" 2>/dev/null; then
		cp -a "$avail" "${avail}.bak.enable-ssl"
		sed -i "s/server_name _;/server_name ${domain};/" "$avail"
		log "nginx: 已将 server_name 设为 ${domain}（备份: ${avail}.bak.enable-ssl）"
	fi
}

ensure_systemd_ssl_env() {
	local domain unit drop
	domain="$1"
	unit="/etc/systemd/system/tasktracker.service"
	drop="/etc/systemd/system/tasktracker.service.d/ssl.conf"
	[[ -f "$unit" ]] || die "未找到 $unit（请先 install.sh 安装 systemd 服务）"
	install -d -m 0755 /etc/systemd/system/tasktracker.service.d
	cat >"$drop" <<EOF
# 由 enable-ssl.sh 生成：HTTPS 与 Cookie Secure
[Service]
Environment=AUTH_SECURE_COOKIE=true
Environment=BASE_URL=https://${domain}
EOF
	chmod 0644 "$drop"
	log "systemd: 已写入 $drop"
	systemctl daemon-reload
	systemctl restart tasktracker
	log "systemd: 已重启 tasktracker（AUTH_SECURE_COOKIE=true, BASE_URL=https://${domain}）"
}

main() {
	local DOMAIN="" EMAIL="${CERTBOT_EMAIL:-}"
	while [[ $# -gt 0 ]]; do
		case "$1" in
		-h | --help)
			usage
			exit 0
			;;
		--email)
			[[ -n "${2:-}" ]] || die "--email 需要参数"
			EMAIL="$2"
			shift 2
			;;
		*)
			[[ -z "$DOMAIN" ]] || die "多余参数: $1"
			DOMAIN="$1"
			shift
			;;
		esac
	done

	[[ -n "$DOMAIN" ]] || die "请指定域名，例如: sudo $0 app.example.com"
	[[ "${EUID:-0}" -eq 0 ]] || die "请使用 root 或 sudo 执行"

	[[ -d "$ROOT/deploy" ]] || die "请在仓库根目录执行（缺少 deploy/）"

	command -v nginx >/dev/null 2>&1 || die "请先安装 Nginx: apt install -y nginx"
	[[ -f /etc/nginx/sites-available/tasktracker ]] || die "未找到 /etc/nginx/sites-available/tasktracker。请先执行: sudo ./install.sh --with-nginx"

	export DEBIAN_FRONTEND=noninteractive
	apt-get update -qq
	apt-get install -y certbot python3-certbot-nginx

	if [[ -e /etc/nginx/sites-enabled/default ]]; then
		rm -f /etc/nginx/sites-enabled/default
		log "nginx: 已禁用 /etc/nginx/sites-enabled/default（避免与 tasktracker 抢占 80）"
	fi

	local cert_path="/etc/letsencrypt/live/${DOMAIN}/fullchain.pem"
	if [[ ! -f "$cert_path" ]]; then
		ensure_server_name_for_certbot "$DOMAIN"
		if nginx -t 2>/dev/null; then
			systemctl reload nginx 2>/dev/null || true
		fi
		log "正在申请 Let’s Encrypt 证书（域名须已解析到本机）..."
		if [[ -n "$EMAIL" ]]; then
			certbot certonly --nginx -d "$DOMAIN" --non-interactive --agree-tos \
				-m "$EMAIL"
		else
			log "提示: 非交互申请可设置: export CERTBOT_EMAIL=你的邮箱"
			certbot certonly --nginx -d "$DOMAIN"
		fi
	fi

	[[ -f "$cert_path" ]] || die "证书仍未就绪: $cert_path"

	apply_nginx_https_template "$DOMAIN"
	nginx -t
	systemctl reload nginx
	log "nginx: 已检测配置并重载。"

	if systemctl is-enabled tasktracker &>/dev/null; then
		ensure_systemd_ssl_env "$DOMAIN"
	else
		log "提示: 未检测到 tasktracker 服务，请手动设置 AUTH_SECURE_COOKIE=true 与 BASE_URL=https://${DOMAIN}"
	fi

	log ""
	log "HTTPS 已启用。请在浏览器访问: https://${DOMAIN}/"
	log "若使用 ufw: sudo ufw allow 'Nginx Full' && sudo ufw status"
}

main "$@"
