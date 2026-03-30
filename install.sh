#!/usr/bin/env bash
# TaskTracker 部署脚本：按需安装 Go（低于 1.21 时）、编译、可选安装 systemd 服务。
# root 完整安装仅验证为 Ubuntu 24.x（如 24.04 LTS）；用法见 ./install.sh --help

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PREFIX="${PREFIX:-/opt/tasktracker}"
LISTEN_ADDR="${LISTEN_ADDR:-:8088}"
GO_MIN="1.21"

BUILD_ONLY=false
WITH_SYSTEMD=true
GO_VERSION="${GO_VERSION:-}"

usage() {
	cat <<'EOF'
Usage: install.sh [options]

  非 root：在仓库根目录执行 go build（需已安装 Go 1.21+）。
  root：仅在 Ubuntu 24.x 上执行；按需下载安装 Go 到 /usr/local/go，编译后将二进制部署到 PREFIX 并可选写入 systemd。

Options:
  --build-only     仅编译当前目录 ./tasktracker，不进行系统安装。
  --prefix DIR     安装目录（默认 /opt/tasktracker）。
  --listen ADDR    服务监听地址（默认 :8088，写入 systemd）。
  --no-systemd     root 安装时不写入 systemd、不启用服务。
  --go-version VER 指定要下载的 Go 版本号，如 1.23.5（默认从 go.dev 读取稳定版）。
  -h, --help       显示本说明。

环境变量（可选）:
  PREFIX, LISTEN_ADDR, GO_VERSION

示例:
  ./install.sh
  sudo ./install.sh
  sudo PREFIX=/srv/tasktracker ./install.sh --no-systemd
EOF
}

log() { printf '%s\n' "$*"; }

die() { printf 'install.sh: %s\n' "$*" >&2; exit 1; }

# 仅 root 全流程部署：要求官方 Ubuntu 24.04 / 24.10 等（VERSION_ID 为 24.x）
assert_ubuntu_24() {
	[[ -f /etc/os-release ]] || die "未找到 /etc/os-release，无法确认系统版本"
	# shellcheck source=/dev/null
	. /etc/os-release
	[[ "${ID:-}" == "ubuntu" ]] || die "root 安装仅支持 Ubuntu 24.x；当前 ID=${ID:-?}（${PRETTY_NAME:-}）"
	case "${VERSION_ID:-}" in
	24.*) ;;
	*) die "root 安装仅支持 Ubuntu 24.x（如 24.04 LTS）；当前 VERSION_ID=${VERSION_ID:-?}" ;;
	esac
}

ver_ge() {
	local a="$1" b="$2"
	[[ "$a" == "$b" ]] && return 0
	[[ "$(printf '%s\n' "$a" "$b" | sort -V | tail -n 1)" == "$a" ]]
}

go_installed_version() {
	command -v go >/dev/null 2>&1 || return 1
	go version 2>/dev/null | sed -n 's/.*go version go\([0-9.]*\).*/\1/p' | head -n 1
}

go_version_sufficient() {
	local v
	v="$(go_installed_version)" || return 1
	[[ -n "$v" ]] && ver_ge "$v" "$GO_MIN"
}

detect_go_arch() {
	case "$(uname -m)" in
	x86_64) echo amd64 ;;
	aarch64 | arm64) echo arm64 ;;
	*) die "unsupported machine: $(uname -m) (need amd64 or arm64)" ;;
	esac
}

fetch_default_go_version() {
	local v
	v="$(curl -sfL 'https://go.dev/VERSION?m=text' 2>/dev/null | tr -d '\r\n' | sed 's/^go//')" || true
	[[ -n "$v" ]] || v="1.23.5"
	printf '%s' "$v"
}

install_go_linux() {
	command -v curl >/dev/null 2>&1 || die "需要 curl：sudo apt install -y curl ca-certificates"
	local arch ver tmp
	arch="$(detect_go_arch)"
	ver="${GO_VERSION:-$(fetch_default_go_version)}"
	tmp="$(mktemp)"
	trap 'rm -f "$tmp"' EXIT
	log "Installing Go ${ver} for linux-${arch} to /usr/local/go ..."
	curl -sfL "https://go.dev/dl/go${ver}.linux-${arch}.tar.gz" -o "$tmp"
	rm -rf /usr/local/go
	tar -C /usr/local -xzf "$tmp"
	rm -f "$tmp"
	trap - EXIT
	export PATH="/usr/local/go/bin:${PATH}"
	hash -r 2>/dev/null || true
	go_version_sufficient || die "Go installed but version check failed; try --go-version"
}

ensure_go() {
	if go_version_sufficient; then
		log "Go $(go_installed_version) OK."
		return 0
	fi
	if [[ "${EUID:-0}" -ne 0 ]]; then
		die "需要 Go ${GO_MIN}+。当前: $(go_installed_version 2>/dev/null || echo 未安装)。请安装 Go 或使用: sudo $0"
	fi
	install_go_linux
}

build_binary() {
	cd "$ROOT"
	export PATH="/usr/local/go/bin:${PATH}"
	ensure_go
	log "Building tasktracker ..."
	go build -o tasktracker .
	log "Built: $ROOT/tasktracker"
}

install_system_files() {
	local svc
	svc="/etc/systemd/system/tasktracker.service"
	install -d -m 0755 "$PREFIX"
	install -d -m 0755 "$PREFIX/data"
	install -m 0755 "$ROOT/tasktracker" "$PREFIX/tasktracker"
	if ! id -u tasktracker >/dev/null 2>&1; then
		useradd --system --home-dir "$PREFIX" --shell /usr/sbin/nologin tasktracker
	fi
	chown -R tasktracker:tasktracker "$PREFIX"

	cat >"$svc" <<EOF
[Unit]
Description=TaskTracker
After=network.target

[Service]
Type=simple
User=tasktracker
Group=tasktracker
WorkingDirectory=$PREFIX
Environment=DATA_DIR=$PREFIX/data
Environment=LISTEN_ADDR=$LISTEN_ADDR
ExecStart=$PREFIX/tasktracker
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF

	systemctl daemon-reload
	systemctl enable --now tasktracker
	log "systemd: enabled and started tasktracker (LISTEN_ADDR=$LISTEN_ADDR)"
	log "Check: systemctl status tasktracker"
}

deploy_as_root() {
	build_binary
	if [[ "$WITH_SYSTEMD" != true ]]; then
		install -d -m 0755 "$PREFIX"
		install -d -m 0755 "$PREFIX/data"
		install -m 0755 "$ROOT/tasktracker" "$PREFIX/tasktracker"
		if ! id -u tasktracker >/dev/null 2>&1; then
			useradd --system --home-dir "$PREFIX" --shell /usr/sbin/nologin tasktracker
		fi
		chown -R tasktracker:tasktracker "$PREFIX"
		log "Installed to $PREFIX (no systemd). Run: sudo -u tasktracker DATA_DIR=$PREFIX/data $PREFIX/tasktracker"
		return 0
	fi
	install_system_files
}

main() {
	while [[ $# -gt 0 ]]; do
		case "$1" in
		--build-only) BUILD_ONLY=true ;;
		--prefix)
			PREFIX="${2:-}"
			[[ -n "$PREFIX" ]] || die "--prefix needs an argument"
			shift
			;;
		--listen)
			LISTEN_ADDR="${2:-}"
			[[ -n "$LISTEN_ADDR" ]] || die "--listen needs an argument"
			shift
			;;
		--no-systemd) WITH_SYSTEMD=false ;;
		--go-version)
			GO_VERSION="${2:-}"
			[[ -n "$GO_VERSION" ]] || die "--go-version needs an argument"
			shift
			;;
		-h | --help)
			usage
			exit 0
			;;
		*)
			die "unknown option: $1 (try --help)"
			;;
		esac
		shift
	done

	if [[ "$BUILD_ONLY" == true ]]; then
		build_binary
		exit 0
	fi

	if [[ "${EUID:-0}" -ne 0 ]]; then
		build_binary
		exit 0
	fi

	[[ "$(uname -s)" == Linux ]] || die "root 完整安装仅支持 Linux；当前: $(uname -s)。请手动安装 Go 后执行: go build -o tasktracker ."

	assert_ubuntu_24
	deploy_as_root
}

main "$@"
