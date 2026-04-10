#!/usr/bin/env bash
# 在服务器上安装 SimpleTask：编译 Go 二进制、创建用户、systemd 服务、Nginx 反向代理（可选）。
# 默认 Ubuntu 24.x；root 执行最佳；Linux 仅支持 systemd。
#
# 用法:
#   sudo ./install.sh
#   sudo ./install.sh --with-nginx
#   sudo ./install.sh --prefix /srv/SimpleTask --listen :9090
#   sudo ./install.sh --no-systemd
#
# Options:
#   --prefix DIR     安装目录（默认 /opt/SimpleTask）
#   --listen ADDR    服务监听地址（默认 :8088，写入 systemd）
#   --with-nginx    配置 Nginx 反向代理（HTTP）
#   --no-systemd    仅编译、安装目录/用户，不创建 systemd 服务
#   --build-only     仅编译当前目录 ./SimpleTask，不进行系统安装。

set -euo pipefail

PREFIX="${PREFIX:-/opt/SimpleTask}"
LISTEN_ADDR="${LISTEN_ADDR:-:8088}"

usage() {
	cat <<'EOF'
Usage: install.sh [options]

  安装 SimpleTask：下载 Go（若未安装）、编译、创建专用用户、systemd 服务、
  可选 Nginx 反向代理。默认监听 :8088，安装到 /opt/SimpleTask。

Options:
  --build-only     仅编译当前目录 ./SimpleTask，不进行系统安装。
  --prefix DIR     安装目录（默认 /opt/SimpleTask）。
  --listen ADDR    服务监听地址（默认 :8088，写入 systemd）。
  --with-nginx    配置 Nginx 反向代理（HTTP）。
  --no-systemd    仅编译、安装目录/用户，不创建 systemd 服务。
  -h, --help      显示本说明。

环境变量:
  PREFIX          安装目录（默认 /opt/SimpleTask）
  LISTEN_ADDR     监听地址（默认 :8088）

示例:
  sudo ./install.sh
  sudo ./install.sh --with-nginx
  sudo PREFIX=/srv/SimpleTask ./install.sh --no-systemd
EOF
}

log() { printf 'install.sh: %s\n' "$*"; }
die() { printf 'install.sh: %s\n' "$*" >&2; exit 1; }

ensure_go() {
	local version go_version current_version
	version="${GOTOOLCHAIN:-go1.22.2}"
	current_version=$(go version 2>/dev/null || echo "")
	if [[ "$current_version" =~ go([0-9]+\.[0-9]+(\.[0-9]+)?) ]]; then
		current_version="${BASH_REMATCH[1]}"
	fi
	case "$current_version" in
	1.2[2-9]*|.*) ;;
	*)  # 版本不足 1.22 或未安装
		log "Installing Go..."
		if [[ "${FORCE_GO:-}" == "1" || ! -x "$(command -v go)" ]]; then
			local go_tgz="https://go.dev/dl/${version}.linux-amd64.tar.gz"
			log "Downloading ${go_tgz}..."
			wget -O /tmp/go.tgz "${go_tgz}" || die "下载失败: ${go_tgz}"
			rm -rf /usr/local/go
			tar -C /usr/local -xzf /tmp/go.tgz
			rm -f /tmp/go.tgz
			export PATH="/usr/local/go/bin:$PATH"
		else
			log "Found Go $current_version, but recommended ${version}+."
			log "For compatibility, using the installed Go with --build-only:"
			log "  sudo install -m 0755 \"$ROOT/SimpleTask\" \"$PREFIX/SimpleTask\""
			return 1  # 不满足版本要求，采用仅编译模式
		fi
		;;
	esac
	return 0
}

build_if_needed() {
	if [[ "${NO_BUILD:-}" != "1" && ! -f "$ROOT/SimpleTask" ]]; then
		if [[ "$current_version" =~ 1.2[2-9]*|. ]]; then
			log "Building SimpleTask..."
			go build -o SimpleTask . || die "编译失败"
		else
			die "Go 版本不足 1.22，请先安装 Go 1.22.2+"
		fi
	fi
}

install_systemd() {
	local svc
	svc="/etc/systemd/system/SimpleTask.service"
	install -d -m 0755 "$PREFIX"
	install -d -m 0755 "$PREFIX/data"
	install -m 0755 "$ROOT/SimpleTask" "$PREFIX/SimpleTask"
	if ! id -u SimpleTask >/dev/null 2>&1; then
		log "Creating user SimpleTask..."
		useradd --system --home-dir "$PREFIX" --shell /usr/sbin/nologin SimpleTask
	fi
	chown -R SimpleTask:SimpleTask "$PREFIX"

	cat >"$svc" <<EOF
[Unit]
Description=SimpleTask
After=network.target

[Service]
Type=simple
User=SimpleTask
Group=SimpleTask
WorkingDirectory=$PREFIX
Environment=LISTEN_ADDR=$LISTEN_ADDR
ExecStart=$PREFIX/SimpleTask
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF
	chmod 0644 "$svc"
	systemctl daemon-reload
	systemctl enable --now SimpleTask
	log "systemd: enabled and started SimpleTask (LISTEN_ADDR=$LISTEN_ADDR)"
	log "Check: systemctl status SimpleTask"
}

install_nginx() {
	local port tmpl avail enabled
	port="$(listen_port_from_addr "$LISTEN_ADDR")"
	tmpl="$ROOT/deploy/SimpleTask.nginx.conf"
	[[ -f "$tmpl" ]] || die "missing nginx template: $tmpl"
	command -v nginx >/dev/null 2>&1 || die "nginx not installed (apt install nginx)"
	avail="/etc/nginx/sites-available/SimpleTask"
	enabled="/etc/nginx/sites-enabled/SimpleTask"
	sed "s/@PORT@/${port}/g" "$tmpl" >"$avail"
	chmod 0644 "$avail"
	ln -sf "$avail" "$enabled"
	# 默认站点与 SimpleTask 同时 listen 80 会冲突，禁用 default
	if [[ -e /etc/nginx/sites-enabled/default ]]; then
		rm -f /etc/nginx/sites-enabled/default
		log "nginx: disabled /etc/nginx/sites-enabled/default (conflicted with SimpleTask on :80)"
	fi
	nginx -t && systemctl reload nginx
	log "nginx: configured. Access SimpleTask via http://yourdomain/"
	log "HTTPS: sudo \"$ROOT/enable-ssl.sh\" <your.domain>   # DNS 指向本机后执行；或见 deploy/SimpleTask.nginx-https.example.conf"
}

listen_port_from_addr() {
	local addr="$1"
	if [[ "$addr" =~ :([0-9]+)$ ]]; then
		printf '%s' "${BASH_REMATCH[1]}"
	else
		printf '%s' "8088"  # default
	fi
}

main() {
	local DOMAIN="" EMAIL="${CERTBOT_EMAIL:-}"
	while [[ $# -gt 0 ]]; do
		case "$1" in
		-h | --help)
			usage
			exit 0
			;;
		--prefix)
			[[ -n "${2:-}" ]] || die "--prefix 需要参数"
			PREFIX="$2"
			shift 2
			;;
		--listen)
			[[ -n "${2:-}" ]] || die "--listen 需要参数"
			LISTEN_ADDR="$2"
			shift 2
			;;
		--with-nginx)
			WITH_NGINX="1"
			shift
			;;
		--no-systemd)
			NO_SYSTEMD="1"
			shift
			;;
		--build-only)
			BUILD_ONLY="1"
			shift
			;;
		*)
			die "未知选项: $1"
			;;
		esac
	done

	ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
	[[ -f "$ROOT/go.mod" ]] || die "请在仓库根目录执行（缺少 go.mod）"
	[[ "$(uname -s)" == Linux ]] || die "root 完整安装仅支持 Linux；当前: $(uname -s)。请手动安装 Go 后执行: go build -o SimpleTask ."

	export PATH="/usr/local/go/bin:$PATH"  # 确保优先使用系统安装的 Go

	if [[ "${BUILD_ONLY:-}" == "1" ]]; then
		ensure_go
		build_if_needed
		log "Built: $ROOT/SimpleTask"
		exit 0
	fi

	if [[ "${EUID:-0}" -ne 0 ]]; then
		log "非 root 执行将仅编译、不创建系统服务/用户/配置"
		BUILD_ONLY=1
	fi

	# 安装依赖（非 root 仅跳过）
	apt-get update -qq
	apt-get install -y git wget ca-certificates systemd

	# Go 编译
	ensure_go
	build_if_needed

	if [[ ! -f "$ROOT/SimpleTask" ]]; then
		die "编译失败：未生成 SimpleTask"
	fi

	# 仅编译模式
	if [[ "${BUILD_ONLY:-}" == "1" ]]; then
		install -d -m 0755 "$PREFIX"
		install -m 0755 "$ROOT/SimpleTask" "$PREFIX/SimpleTask"
		if ! id -u SimpleTask >/dev/null 2>&1; then
			useradd --system --home-dir "$PREFIX" --shell /usr/sbin/nologin SimpleTask
		fi
		chown -R SimpleTask:SimpleTask "$PREFIX"
		log "已安装到 $PREFIX/SimpleTask（no systemd）。运行: sudo -u SimpleTask DATA_DIR=$PREFIX/data $PREFIX/SimpleTask"
		return 0
	fi

	# 系统服务模式
	install_systemd
	if [[ "${WITH_NGINX:-}" == "1" ]]; then
		install_nginx
	fi

	log "SimpleTask 安装完成！"
	log "配置文件位置："
	log "  二进制: $PREFIX/SimpleTask"
	log "  systemd: /etc/systemd/system/SimpleTask.service"
	log "  数据: $PREFIX/data/"
	log "访问：http://localhost${LISTEN_ADDR}"
}

main "$@"