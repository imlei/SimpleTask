#!/usr/bin/env bash
# 在服务器上的 **Git 克隆目录** 内执行：拉取远端 → 按 go.mod 编译 → 安装二进制 → 重启 systemd。
# 可选：刷新 Nginx 反代配置（HTTP 或 HTTPS 模板），与 install.sh 使用同一 deploy 模板。
# 可与 cron/systemd timer 配合做定期自检更新（仍建议先在测试环境验证 main 分支）。

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PREFIX="${PREFIX:-/opt/tasktracker}"
GOTOOLCHAIN="${GOTOOLCHAIN:-go1.22.2}"
REMOTE="${GIT_REMOTE:-origin}"
BRANCH="${GIT_BRANCH:-main}"
UPGRADE_NGINX="${UPGRADE_NGINX:-0}"
UPGRADE_NGINX_SSL="${UPGRADE_NGINX_SSL:-0}"

usage() {
	cat <<'EOF'
Usage: upgrade.sh [options]

  必须在包含本仓库的 Git 工作副本根目录中运行（与 install.sh 同级）。

  步骤：git fetch +（若有新提交）git pull → GOTOOLCHAIN go build → 复制到 PREFIX/tasktracker → systemctl restart
  若已是最新但仍需刷新 Nginx，可配合 --nginx / --nginx-ssl（需 root）。

Options:
  -h, --help           显示本说明。
  --nginx              刷新 HTTP 反代（deploy/tasktracker.nginx.conf → /etc/nginx/sites-available/tasktracker）
  --nginx-ssl DOMAIN   用 HTTPS 模板覆盖站点（deploy/tasktracker.nginx-https.example.conf），
                       DOMAIN 为 server_name 与 Let's Encrypt 目录名；需已存在证书。

环境变量（可选）:
  PREFIX              安装目录（默认 /opt/tasktracker）
  GOTOOLCHAIN         与 README / go.mod 一致（默认 go1.22.2）
  GIT_REMOTE          默认 origin
  GIT_BRANCH          默认 main
  SKIP_SYSTEMD        设为 1 时不执行 systemctl restart
  UPGRADE_NGINX       设为 1 等同 --nginx
  UPGRADE_NGINX_SSL   设为 1 等同 --nginx-ssl（须同时设 NGINX_SSL_DOMAIN）
  NGINX_SSL_DOMAIN    HTTPS 域名（与 --nginx-ssl 二选一即可）
  NGINX_APP_PORT      反代到本机端口；不设则从 systemd tasktracker 的 LISTEN_ADDR 解析，默认 8088

示例:
  cd /path/to/TaskTracker && ./upgrade.sh
  sudo ./upgrade.sh                              # 推荐：便于写入 PREFIX 并重启服务
  sudo UPGRADE_NGINX=1 ./upgrade.sh              # 升级并刷新 HTTP Nginx 配置
  sudo ./upgrade.sh --nginx-ssl app.example.com # 升级并用仓库 HTTPS 模板重写站点（需证书）

配合 cron（每周日 4:00，请把路径改成你的克隆目录）:
  0 4 * * 0 cd /opt/tasktracker/TaskTracker && sudo ./upgrade.sh >>/var/log/tasktracker-upgrade.log 2>&1
EOF
}

log() { printf '%s\n' "$*"; }
die() { printf 'upgrade.sh: %s\n' "$*" >&2; exit 1; }

# 从 systemd tasktracker 的 LISTEN_ADDR 解析端口，如 127.0.0.1:8088 → 8088
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

upgrade_nginx_http() {
	local port tmpl avail enabled
	port="$(nginx_app_port)"
	tmpl="$ROOT/deploy/tasktracker.nginx.conf"
	[[ -f "$tmpl" ]] || die "缺少模板: $tmpl"
	avail="/etc/nginx/sites-available/tasktracker"
	enabled="/etc/nginx/sites-enabled/tasktracker"
	sed "s/@PORT@/${port}/g" "$tmpl" >"$avail"
	chmod 0644 "$avail"
	ln -sf "$avail" "$enabled"
	log "nginx: 已更新 HTTP 反代 $avail（→ 127.0.0.1:${port}）"
}

upgrade_nginx_https() {
	local domain port tmpl avail enabled cert key
	domain="${NGINX_SSL_DOMAIN:-}"
	[[ -n "$domain" ]] || die "HTTPS 站点需要域名：NGINX_SSL_DOMAIN=你的域名 或 --nginx-ssl 域名"
	cert="/etc/letsencrypt/live/${domain}/fullchain.pem"
	key="/etc/letsencrypt/live/${domain}/privkey.pem"
	[[ -f "$cert" && -f "$key" ]] || die "未找到证书（请先申请）：$cert 与 $key"
	port="$(nginx_app_port)"
	tmpl="$ROOT/deploy/tasktracker.nginx-https.example.conf"
	[[ -f "$tmpl" ]] || die "缺少模板: $tmpl"
	avail="/etc/nginx/sites-available/tasktracker"
	enabled="/etc/nginx/sites-enabled/tasktracker"
	sed -e "s/example.com/${domain}/g" -e "s/@PORT@/${port}/g" "$tmpl" >"$avail"
	chmod 0644 "$avail"
	ln -sf "$avail" "$enabled"
	log "nginx: 已更新 HTTPS 站点 $avail（域名 ${domain}，反代到 127.0.0.1:${port}）"
}

maybe_refresh_nginx() {
	[[ "${EUID:-0}" -eq 0 ]] || return 0
	command -v nginx >/dev/null 2>&1 || { log "nginx 未安装，跳过站点刷新"; return 0; }
	if [[ "${UPGRADE_NGINX_SSL}" == "1" ]]; then
		upgrade_nginx_https
	elif [[ "${UPGRADE_NGINX}" == "1" ]]; then
		upgrade_nginx_http
	else
		return 0
	fi
	nginx -t
	systemctl reload nginx
	log "nginx: 配置已检测并重载。"
}

main() {
	local saved_args=("$@")
	while [[ $# -gt 0 ]]; do
		case "$1" in
		-h | --help)
			usage
			exit 0
			;;
		--nginx)
			UPGRADE_NGINX=1
			shift
			;;
		--nginx-ssl)
			[[ -n "${2:-}" ]] || die "--nginx-ssl 需要域名参数"
			UPGRADE_NGINX_SSL=1
			NGINX_SSL_DOMAIN="$2"
			shift 2
			;;
		*)
			die "unknown option: $1 (try --help)"
			;;
		esac
	done

	cd "$ROOT"
	[[ -d .git ]] || die "当前目录不是 Git 克隆（无 .git）。请先在服务器上 git clone 仓库再执行本脚本。"

	command -v git >/dev/null 2>&1 || die "需要 git"
	command -v go >/dev/null 2>&1 || die "需要 go（PATH 含 /usr/local/go/bin）"

	export PATH="/usr/local/go/bin:${PATH}"
	export GOTOOLCHAIN

	log "git fetch $REMOTE $BRANCH ..."
	git fetch "$REMOTE" "$BRANCH"

	if ! git rev-parse "$REMOTE/$BRANCH" >/dev/null 2>&1; then
		die "找不到远端引用 $REMOTE/$BRANCH（请先 git fetch，并检查 GIT_REMOTE / GIT_BRANCH）"
	fi

	local behind
	behind="$(git rev-list --count HEAD.."$REMOTE/$BRANCH" 2>/dev/null)" || behind=1

	if [[ "${behind:-0}" -eq 0 ]]; then
		log "已是最新（$REMOTE/$BRANCH），跳过 git pull / 编译。"
	else
		log "git pull $REMOTE $BRANCH ..."
		git pull "$REMOTE" "$BRANCH"

		log "go build ..."
		go build -o tasktracker .

		if [[ ! -f "$ROOT/tasktracker" ]]; then
			die "编译失败：未生成 tasktracker"
		fi

		if [[ "${EUID:-0}" -ne 0 ]]; then
			log "编译完成。请用 root 安装并重启服务，例如："
			log "  sudo install -m 0755 \"$ROOT/tasktracker\" \"$PREFIX/tasktracker\""
			log "  sudo systemctl restart tasktracker"
			exit 0
		fi

		install -d -m 0755 "$PREFIX"
		install -m 0755 "$ROOT/tasktracker" "$PREFIX/tasktracker"
		if [[ -n "${SKIP_SYSTEMD:-}" && "$SKIP_SYSTEMD" != "0" ]]; then
			log "已安装到 $PREFIX/tasktracker（SKIP_SYSTEMD，未重启服务）"
		elif systemctl is-enabled tasktracker >/dev/null 2>&1; then
			systemctl restart tasktracker
			log "已重启 tasktracker 服务。"
		else
			log "未检测到 tasktracker systemd 单元，已仅更新二进制：$PREFIX/tasktracker"
		fi
	fi

	if [[ "${EUID:-0}" -eq 0 ]] && { [[ "${UPGRADE_NGINX}" == "1" ]] || [[ "${UPGRADE_NGINX_SSL}" == "1" ]]; }; then
		maybe_refresh_nginx
	elif [[ "${UPGRADE_NGINX}" == "1" ]] || [[ "${UPGRADE_NGINX_SSL}" == "1" ]]; then
		log "提示：刷新 Nginx 需要 root。请执行: sudo \"$ROOT/upgrade.sh\" $(printf '%q ' "${saved_args[@]}")"
	fi
}

main "$@"
