#!/usr/bin/env bash
# 在服务器上的 **Git 克隆目录** 内执行：拉取远端 → 按 go.mod 编译 → 安装二进制 → 重启 systemd。
# 可与 cron/systemd timer 配合做定期自检更新（仍建议先在测试环境验证 main 分支）。

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PREFIX="${PREFIX:-/opt/tasktracker}"
GOTOOLCHAIN="${GOTOOLCHAIN:-go1.22.2}"
REMOTE="${GIT_REMOTE:-origin}"
BRANCH="${GIT_BRANCH:-main}"

usage() {
	cat <<'EOF'
Usage: upgrade.sh [options]

  必须在包含本仓库的 Git 工作副本根目录中运行（与 install.sh 同级）。

  步骤：git fetch + git pull → GOTOOLCHAIN go build → 复制到 PREFIX/tasktracker → systemctl restart

Options:
  -h, --help     显示本说明。

环境变量（可选）:
  PREFIX         安装目录（默认 /opt/tasktracker）
  GOTOOLCHAIN    与 README / go.mod 一致（默认 go1.22.2）
  GIT_REMOTE     默认 origin
  GIT_BRANCH     默认 main
  SKIP_SYSTEMD   设为 1 时不执行 systemctl restart

示例:
  cd /path/to/TaskTracker && ./upgrade.sh
  sudo ./upgrade.sh                    # 推荐：便于写入 PREFIX 并重启服务

配合 cron（每周日 4:00，请把路径改成你的克隆目录）:
  0 4 * * 0 cd /opt/tasktracker/TaskTracker && sudo ./upgrade.sh >>/var/log/tasktracker-upgrade.log 2>&1
EOF
}

log() { printf '%s\n' "$*"; }
die() { printf 'upgrade.sh: %s\n' "$*" >&2; exit 1; }

main() {
	while [[ $# -gt 0 ]]; do
		case "$1" in
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
		log "已是最新（$REMOTE/$BRANCH），无需更新。"
		exit 0
	fi

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
		exit 0
	fi
	if systemctl is-enabled tasktracker >/dev/null 2>&1; then
		systemctl restart tasktracker
		log "已重启 tasktracker 服务。"
	else
		log "未检测到 tasktracker systemd 单元，已仅更新二进制：$PREFIX/tasktracker"
	fi
}

main "$@"
