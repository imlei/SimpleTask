#!/usr/bin/env bash
# 在服务器上升级 SimpleTask：编译新版本、替换二进制、更新服务、配置 Nginx（若有）。
# 须在仓库根目录执行；root 最佳。
#
# 用法:
#   sudo ./upgrade.sh
#   sudo ./upgrade.sh --with-nginx
#   sudo ./upgrade.sh --prefix /opt/SimpleTask --listen :9090
#
# Options:
#   --prefix DIR     安装目录（默认 /opt/SimpleTask）
#   --listen ADDR    服务监听地址（默认 :8088，写入 systemd）
#   --with-nginx    同时更新 Nginx 配置（HTTP）
#   -h, --help      显示本说明

set -euo pipefail

PREFIX="${PREFIX:-/opt/SimpleTask}"
LISTEN_ADDR="${LISTEN_ADDR:-:8088}"

usage() {
	cat <<'EOF'
Usage: upgrade.sh [options]

  升级 SimpleTask：编译新版本、替换二进制、更新 systemd、数据保留、Nginx（若有）。
  默认监听 :8088，默认安装目录 /opt/SimpleTask。

Options:
  --prefix DIR     安装目录（默认 /opt/SimpleTask）
  --listen ADDR    服务监听地址（默认 :8088，写入 systemd）
  --with-nginx    同时更新 Nginx 配置（HTTP）
  -h, --help      显示本说明

环境变量:
  PREFIX          安装目录（默认 /opt/SimpleTask）
  LISTEN_ADDR     监听地址（默认 :8088）

示例:
  sudo ./upgrade.sh
  sudo ./upgrade.sh --with-nginx
  sudo PREFIX=/srv/SimpleTask ./upgrade.sh
EOF
}

log() { printf 'upgrade.sh: %s\n' "$*"; }
die() { printf 'upgrade.sh: %s\n' "$*" >&2; exit 1; }

# 和 install.sh 保持一致
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

ensure_service_stopped() {
	log "检查并停止 SimpleTask 进程..."
	
	# 停止 systemd 服务
	if systemctl is-active SimpleTask >/dev/null 2>&1; then
		log "正在停止 SimpleTask systemd 服务..."
		systemctl stop SimpleTask || die "停止 systemd 服务失败"
	fi
	
	# 检查并杀死任何直接的 SimpleTask 进程
	local pids=$(pgrep -f "SimpleTask")
	if [[ -n "$pids" ]]; then
		log "发现正在运行的 SimpleTask 进程，正在强制关闭..."
		echo "$pids" | xargs kill -9 2>/dev/null || true
		sleep 2  # 等待进程彻底关闭
		
		# 再次检查进程是否仍在运行
		local remaining_pids=$(pgrep -f "SimpleTask")
		if [[ -n "$remaining_pids" ]]; then
			log "警告: 仍有 SimpleTask 进程在运行: $remaining_pids"
		else
			log "SimpleTask 进程已成功关闭"
		fi
	fi
	
	# 检查是否还有占用指定端口的进程
	local port=$(listen_port_from_addr "$LISTEN_ADDR")
	if [[ "$port" != "" && "$port" != "8088" ]]; then
		local port_pids=$(lsof -ti:"$port" 2>/dev/null || true)
		if [[ -n "$port_pids" ]]; then
			log "发现占用端口 $port 的进程，正在强制关闭..."
			echo "$port_pids" | xargs kill -9 2>/dev/null || true
			sleep 1
			log "端口 $port 已释放"
		fi
	fi
}

upgrade_binary() {
	log "Replacing SimpleTask binary..."
	install -d -m 0755 "$PREFIX"
	install -m 0755 "$ROOT/SimpleTask" "$PREFIX/SimpleTask"
	chown -R SimpleTask:SimpleTask "$PREFIX"
}

check_and_migrate_database() {
	local data_dir="$PREFIX/data"
	local new_db="$data_dir/SimpleTask.db"
	local old_db="$data_dir/biztracker.db"
	
	if [[ -f "$new_db" ]]; then
		log "Database file already exists and uses new name: SimpleTask.db"
		return 0
	fi
	
	if [[ -f "$old_db" ]]; then
		log "Found legacy database: biztracker.db, migrating to SimpleTask.db..."
		if [[ ! -f "$new_db" ]]; then
			cp "$old_db" "$new_db"
			log "Migrated database from biztracker.db to SimpleTask.db"
		else
			log "SimpleTask.db already exists, skipping migration"
		fi
	else
		log "No database found, will create new SimpleTask.db on first run"
	fi
}

systemd_service_after_upgrade() {
	log "Updating SimpleTask systemd service..."
	cat >"/etc/systemd/system/SimpleTask.service" <<EOF
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
	chmod 0644 "/etc/systemd/system/SimpleTask.service"
	systemctl daemon-reload
	log "systemd: enabled and started SimpleTask (LISTEN_ADDR=$LISTEN_ADDR)"
	log "Check: systemctl status SimpleTask"
}

upgrade_nginx() {
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
	if nginx -t; then
		log "nginx: reload nginx config"
		systemctl reload nginx
	else
		log "nginx: config invalid, skipping reload (restore backup if needed)"
	fi
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
		*)
			die "未知选项: $1"
			;;
		esac
	done

	ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
	[[ -f "$ROOT/go.mod" ]] || die "请在仓库根目录执行（缺少 go.mod）"
	[[ "$(uname -s)" == Linux ]] || die "升级仅支持 Linux；当前: $(uname -s)。请手动安装 Go 后执行: go build -o SimpleTask ."

	export PATH="/usr/local/go/bin:$PATH"  # 确保优先使用系统安装的 Go
	apt-get update -qq
	apt-get install -y systemd  # 确保 systemd

	ensure_go
	build_if_needed

	if [[ ! -f "$ROOT/SimpleTask" ]]; then
		die "编译失败：未生成 SimpleTask"
	fi

	# 如果系统有服务，停止服务
	if systemctl is-enabled SimpleTask >/dev/null 2>&1; then
		ensure_service_stopped
	fi

	upgrade_binary
	check_and_migrate_database
	systemd_service_after_upgrade

	if [[ "${WITH_NGINX:-}" == "1" ]]; then
		upgrade_nginx
	fi

	log "SimpleTask 升级完成！"
	log "访问：http://localhost${LISTEN_ADDR}"
	log "配置文件位置："
	log "  二进制: $PREFIX/SimpleTask"
	log "  systemd: /etc/systemd/system/SimpleTask.service"
	log "  数据: $PREFIX/data/"
	log "  nginx: /etc/nginx/sites-available/SimpleTask"
}

main "$@"