#!/usr/bin/env bash
# Linux/systemd installer for AI Model Connectivity. Requires Bash 4+ and Python 3.8+.
if [ -z "${BASH_VERSION:-}" ]; then
    printf '%s\n' '请使用 bash 运行此脚本，不要使用 sh。 / Run this script with bash, not sh.' >&2
    exit 2
fi
set -Eeuo pipefail
umask 077

REPO="wututua/AI_Model_Connectivity"
SERVICE="model-connectivity.service"
SERVICE_USER="model-connectivity"
INSTALL_DIR="/opt/model-connectivity"
DATA_DIR="/var/lib/model-connectivity"
BACKUP_DIR="/var/backups/model-connectivity"
UNIT_FILE="/etc/systemd/system/$SERVICE"
LOCK_FILE="/run/model-connectivity-installer.lock"
OWNER_MARKER="AI_Model_Connectivity installer v1"
UPDATER_DIR="/usr/local/lib/model-connectivity-updater"
UPDATE_STATE_DIR="/var/lib/model-connectivity-updater"
UPDATE_UNIT="/etc/systemd/system/model-connectivity-update.service"
UPDATE_PATH_UNIT="/etc/systemd/system/model-connectivity-update.path"

ACTION=""
CHANNEL="stable"
VERSION=""
INSTALLED_VERSION=""
HOST="127.0.0.1"
PORT="8080"
HOST_GIVEN=0
PORT_GIVEN=0
SECURE_COOKIES="false"
CONFIG_GIVEN=0
YES=0
WORK_DIR=""
STAGE=""
SNAPSHOT=""
CHANGING=0
STOPPED=0
WAS_ACTIVE=0
WAS_ENABLED=0
UI_LANG="en"

message() {
    local format="$1"
    [[ "$UI_LANG" != zh ]] || format="$2"
    shift 2
    # Formats are literal translations; user-supplied values are separate arguments.
    # shellcheck disable=SC2059
    printf "$format" "$@"
}

info() { message "$@"; printf '\n'; }
warn() { { message 'WARNING: ' '警告：'; info "$@"; } >&2; }
die() { { message 'ERROR: ' '错误：'; info "$@"; } >&2; exit 1; }
progress() { if [[ "${CG_UPDATE_PROGRESS:-}" == 1 ]]; then printf 'CG_UPDATE_STAGE=%s\n' "$1"; fi; }

has_terminal() { [[ -t 0 ]]; }

choose_language() {
    has_terminal || return 0
    local choice
    printf '%s\n' '1. 简体中文' '2. English' >&2
    while true; do
        read -r -p '请选择语言 / Choose language [1/2; 默认/default 1]: ' choice || {
            printf '\n%s\n' '输入已结束。 / Input closed.' >&2
            exit 1
        }
        case "$choice" in
            1|"") UI_LANG=zh; return ;;
            2) UI_LANG=en; return ;;
            *) printf '%s\n' '请输入 1 或 2。 / Enter 1 or 2.' >&2 ;;
        esac
    done
}

usage() {
    if [[ "$UI_LANG" == zh ]]; then
        cat <<'EOF'
AI Model Connectivity - Linux/systemd 安装管理脚本

用法：sudo bash install-model-connectivity.sh [命令] [选项]
不指定命令时显示交互菜单。

命令：
  install       安装并启动服务
  upgrade       备份并替换完整发布包，保留设置和数据
  backup        短暂停服并创建私有离线备份
  uninstall     备份并删除服务和程序；保留数据和备份
  start         启动服务
  stop          停止服务
  restart       重启服务
  status        查看服务状态
  logs          实时查看日志（可能包含初始管理员密码）
  enable-updates  启用独立更新服务，允许管理员在后台确认更新（短暂停服）
  disable-updates 禁用后台更新入口，保留更新记录
  help          显示帮助

选项：
  --version TAG       安装指定发布版本，包括 beta/rc 标签
  --channel CHANNEL   stable（默认，稳定版）或 preview（最新发布，含预发布版本）
  --host IP           仅安装时使用；默认 127.0.0.1
  --port PORT         仅安装时使用；默认 8080
  --secure-cookies    仅安装时使用；适用于 HTTPS 反向代理
  -y, --yes           非交互确认安装、升级、备份或卸载
  -h, --help          显示帮助

示例：
  sudo bash install-model-connectivity.sh
  sudo bash install-model-connectivity.sh install --channel preview
  sudo bash install-model-connectivity.sh upgrade --channel preview --yes

在终端运行时，先选择界面语言（1. 简体中文 / 2. English，默认简体中文）。
安装时会询问未通过 --host / --port 指定的监听地址和端口，回车保留默认值。
非交互运行默认使用 English，不询问语言。
脚本不自动修改防火墙，不使用第三方下载镜像，不创建应用配置文件。
Provider 和运行设置仍保存在 SQLite 中。
EOF
        return
    fi
    cat <<'EOF'
AI Model Connectivity - Linux/systemd installer

Usage: sudo bash install-model-connectivity.sh [COMMAND] [OPTIONS]
Without a command, show an interactive menu.

Commands:
  install       Install and start the service
  upgrade       Back up, replace the complete release, and preserve settings/data
  backup        Stop the service briefly and create a private offline backup
  uninstall     Back up and remove the service/program; KEEP data and backups
  start         Start the service
  stop          Stop the service
  restart       Restart the service
  status        Show service status
  logs          Follow logs (may contain the initial administrator password)
  enable-updates  Enable administrator-confirmed web updates (brief service stop)
  disable-updates Disable web updates; keep update records
  help          Show this help

Options:
  --version TAG       Install an explicit release, including beta/rc tags
  --channel CHANNEL   stable (default) or preview (newest release, including prereleases)
  --host IP           Install only; default 127.0.0.1
  --port PORT         Install only; default 8080
  --secure-cookies    Install only; use behind an HTTPS reverse proxy
  -y, --yes           Confirm install/upgrade/backup/uninstall non-interactively
  -h, --help          Show this help

Examples:
  sudo bash install-model-connectivity.sh
  sudo bash install-model-connectivity.sh install --channel preview
  sudo bash install-model-connectivity.sh install --version v1.0.0-beta.3 --yes
  sudo bash install-model-connectivity.sh upgrade --channel preview --yes

Terminal runs first ask for a language: 1. Simplified Chinese / 2. English.
Press Enter for Simplified Chinese. Non-interactive runs use English without prompting.
Interactive installs ask for the listen address/port unless set with --host/--port.
Press Enter to keep the defaults. Upgrades preserve the existing listen settings.
No automatic firewall changes, third-party download mirrors, or application
configuration files. Provider and runtime settings remain in SQLite.
EOF
}

# Structured parsing and extraction stay in the standard library, never eval/source.
metadata() {
    PYTHONIOENCODING=utf-8 python3 - "$UI_LANG" "$@" <<'PY'
import hashlib
import ipaddress
import json
import re
import shutil
import socket
import struct
import sys
import tarfile
from pathlib import Path, PurePosixPath

language = sys.argv[1]

def text(english, chinese, *args):
    return (chinese if language == "zh" else english).format(*args)

def require(ok, english, chinese, *args):
    if not ok:
        raise ValueError(text(english, chinese, *args))

def tag(value):
    require(isinstance(value, str) and re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?", value),
            "Invalid release tag", "发布版本标签无效")
    return value

def address(host, port):
    require(re.fullmatch(r"[0-9a-fA-F:.]+", host),
            "Use a numeric IPv4/IPv6 listen address", "请使用数字形式的 IPv4/IPv6 监听地址")
    try:
        ip = ipaddress.ip_address(host)
    except ValueError:
        raise ValueError(text("Invalid IPv4/IPv6 listen address: {}", "IPv4/IPv6 监听地址无效：{}", host)) from None
    require(re.fullmatch(r"[1-9][0-9]{0,4}", port) and int(port) <= 65535,
            "Port must be 1-65535", "端口必须在 1-65535 之间")
    return ip

def version_order(value):
    core, separator, pre = tag(value)[1:].partition("-")
    identifiers = tuple((0, int(part)) if part.isdigit() else (1, part) for part in pre.split(".")) if separator else ()
    return (*map(int, core.split(".")), 0 if separator else 1, identifiers)

def digest(path):
    h = hashlib.sha256()
    with Path(path).open("rb") as f:
        for block in iter(lambda: f.read(1024 * 1024), b""):
            h.update(block)
    return h.hexdigest()

def run():
    command, *args = sys.argv[2:]
    if command == "validate":
        host, port, version = args
        address(host, port)
        if version:
            tag(version)
    elif command == "compare":
        current, target = args
        require(version_order(target) >= version_order(current),
                "Refusing an automatic downgrade. Use --version only after checking database compatibility",
                "拒绝自动降级。请确认数据库兼容性后再使用 --version 指定旧版本")
    elif command == "release":
        filename, channel, requested, asset = args
        value = json.loads(Path(filename).read_text(encoding="utf-8"))
        if isinstance(value, list):
            candidates = [r for r in value if isinstance(r, dict) and not r.get("draft")
                          and (channel == "preview" or r.get("prerelease") is False)]
            require(candidates, "No matching release; try --channel preview or --version TAG",
                    "没有匹配的发布版本；请尝试 --channel preview 或 --version TAG")
            value = max(candidates, key=lambda r: r.get("published_at") or "")
        require(isinstance(value, dict) and value.get("draft") is False,
                "No published release found", "未找到已发布版本")
        version = tag(value.get("tag_name"))
        require(not requested or version == requested, "Release tag mismatch", "发布版本标签不匹配")
        require(requested or channel == "preview" or value.get("prerelease") is False,
                "Stable channel cannot install a prerelease", "stable 通道不能安装预发布版本")
        assets = value.get("assets")
        require(isinstance(assets, list), "Release has no assets", "发布版本没有附件")
        for name in (asset, "SHA256SUMS.txt"):
            matches = [a for a in assets if a.get("name") == name and a.get("state") == "uploaded"]
            require(len(matches) == 1, "Missing or ambiguous release asset: {}", "发布附件缺失或重复：{}", name)
        print(version)
    elif command == "extract":
        archive, checksums, asset, target, arch = args
        matches = []
        for line in Path(checksums).read_text(encoding="utf-8").splitlines():
            match = re.fullmatch(r"([0-9a-fA-F]{64})[ \t]+\*?(.+)", line)
            if match and match[2] == asset:
                matches.append(match[1].lower())
        require(len(matches) == 1 and matches[0] == digest(archive),
                "SHA-256 verification failed", "SHA-256 校验失败")
        root = Path(target)
        allowed = {"model-connectivity", "web", "docs", "assets", "README.md",
                   "CONTRIBUTING.md", "SECURITY.md", "CHANGELOG.md", "LICENSE",
                   "install-model-connectivity.sh"}
        with tarfile.open(archive, "r:gz") as bundle:
            members = bundle.getmembers()
            require(len(members) <= 20000 and sum(m.size for m in members) <= 1024**3,
                    "Release archive is too large", "发布压缩包过大")
            seen = set()
            for member in members:
                path = PurePosixPath(member.name)
                require(path.parts and not path.is_absolute() and ".." not in path.parts
                        and "\\" not in member.name and path.parts[0] in allowed,
                        "Unsafe archive path", "压缩包内包含不安全的路径")
                require(member.isfile() or member.isdir(),
                        "Archive links/devices are not allowed", "压缩包不允许包含链接或设备文件")
                require(str(path) not in seen, "Duplicate archive path", "压缩包内存在重复路径")
                seen.add(str(path))
            for member in members:
                target_path = root.joinpath(*PurePosixPath(member.name).parts)
                if member.isdir():
                    target_path.mkdir(parents=True, exist_ok=True)
                    target_path.chmod(0o755)
                else:
                    target_path.parent.mkdir(parents=True, exist_ok=True)
                    with bundle.extractfile(member) as src, target_path.open("xb") as dst:
                        shutil.copyfileobj(src, dst)
                    target_path.chmod(0o755 if str(PurePosixPath(member.name)) == "model-connectivity" else 0o644)
        for directory in root.rglob("*"):
            if directory.is_dir():
                directory.chmod(0o755)
        for name in ("model-connectivity", "web/index.html", "web/assets/app.js", "LICENSE"):
            require(root.joinpath(name).is_file(), "Incomplete release: {}", "发布包不完整：{}", name)
        with root.joinpath("model-connectivity").open("rb") as binary:
            header = binary.read(20)
        require(len(header) == 20 and header[:6] == b"\x7fELF\x02\x01",
                "Release does not contain a 64-bit Linux ELF binary", "发布包内没有 64 位 Linux ELF 可执行文件")
        require(struct.unpack("<H", header[18:20])[0] == {"amd64": 62, "arm64": 183}[arch],
                "Binary architecture mismatch", "可执行文件架构不匹配")
        root.chmod(0o755)
    elif command == "write-state":
        filename, unit, version, host, port, secure = args
        tag(version)
        address(host, port)
        Path(filename).write_text(json.dumps(dict(schema=1, version=version, host=host,
            port=port, secure_cookies=secure == "true", unit_sha256=digest(unit))) + "\n", encoding="utf-8")
    elif command == "read-state":
        filename, unit = args
        value = json.loads(Path(filename).read_text(encoding="utf-8"))
        require(value.get("schema") == 1, "Unknown installer metadata", "未知的安装器元数据格式")
        tag(value["version"])
        address(value["host"], value["port"])
        require(isinstance(value["secure_cookies"], bool), "Invalid cookie setting", "Cookie 设置无效")
        require(value["unit_sha256"] == digest(unit),
                "Service file was edited; refusing to overwrite or back up an unknown layout",
                "服务文件已被修改；拒绝覆盖或备份未知的部署布局")
        print(value["version"], value["host"], value["port"], str(value["secure_cookies"]).lower(), sep="\n")
    elif command == "port-free":
        host, port = args
        ip = address(host, port)
        with socket.socket(socket.AF_INET6 if ip.version == 6 else socket.AF_INET, socket.SOCK_STREAM) as sock:
            sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
            sock.bind((host, int(port)))
    elif command == "enable-update-path":
        source, target, data, inbox = args
        value = Path(source).read_text(encoding="utf-8")
        line = "ReadWritePaths=" + data + "\n"
        extra = "ReadWritePaths=-" + inbox + "\n"
        require(value.count(line) == 1, "Unknown service layout", "未知的服务配置布局")
        if extra not in value:
            value = value.replace(line, line + extra)
        Path(target).write_text(value, encoding="utf-8")
    else:
        raise ValueError(text("Unknown metadata operation", "未知的元数据操作"))

try:
    run()
except (ValueError, KeyError, TypeError, OSError, tarfile.TarError) as error:
    print(text("ERROR: Metadata operation failed: {}", "错误：元数据操作失败：{}", error), file=sys.stderr)
    sys.exit(1)
PY
}

parse_args() {
    while (($#)); do
        case "$1" in
            install|upgrade|backup|uninstall|start|stop|restart|status|logs|help|enable-updates|disable-updates)
                [[ -z "$ACTION" ]] || die "Specify only one command." "只能指定一个命令。"
                ACTION="$1"; shift ;;
            --version|--channel|--host|--port)
                (($# >= 2)) || die "%s requires a value." "%s 需要指定参数值。" "$1"
                case "$1" in
                    --version) VERSION="$2" ;;
                    --channel) CHANNEL="$2" ;;
                    --host) HOST="$2"; HOST_GIVEN=1; CONFIG_GIVEN=1 ;;
                    --port) PORT="$2"; PORT_GIVEN=1; CONFIG_GIVEN=1 ;;
                esac
                shift 2 ;;
            --secure-cookies) SECURE_COOKIES=true; CONFIG_GIVEN=1; shift ;;
            -y|--yes) YES=1; shift ;;
            -h|--help) ACTION=help; shift ;;
            *) die "Unknown argument: %s" "未知参数：%s" "$1" ;;
        esac
    done
    [[ "$CHANNEL" == stable || "$CHANNEL" == preview ]] ||
        die "Channel must be stable or preview." "通道必须为 stable 或 preview。"
}

menu() {
    has_terminal || die "Specify a command. Download the script first instead of piping it into bash." \
        "请指定命令。请先下载脚本，不要通过管道将脚本传给 bash。"
    info "AI Model Connectivity" "AI Model Connectivity"
    info "1) Install   2) Upgrade   3) Back up   4) Status   5) Logs" \
        "1) 安装   2) 升级   3) 备份   4) 状态   5) 日志"
    info "6) Restart   7) Start     8) Stop      9) Uninstall (keep data)   0) Exit" \
        "6) 重启   7) 启动   8) 停止   9) 卸载（保留数据）   0) 退出"
    info "10) Enable web updates   11) Disable web updates" "10) 启用后台更新   11) 禁用后台更新"
    local choice
    read -r -p "$(message 'Choose: ' '请选择：')" choice
    case "$choice" in
        1) ACTION=install ;; 2) ACTION=upgrade ;; 3) ACTION=backup ;;
        4) ACTION=status ;; 5) ACTION=logs ;; 6) ACTION=restart ;;
        7) ACTION=start ;; 8) ACTION=stop ;; 9) ACTION=uninstall ;;
        10) ACTION=enable-updates ;; 11) ACTION=disable-updates ;;
        0) exit 0 ;; *) die "Invalid choice." "选项无效。" ;;
    esac
    if [[ "$ACTION" == install || "$ACTION" == upgrade ]]; then
        read -r -p "$(message 'Release tag (empty to select a channel): ' '发布版本标签（留空则选择通道）：')" VERSION
        if [[ -z "$VERSION" ]]; then
            read -r -p "$(message 'Channel [stable/preview; default stable]: ' '通道 [stable/preview；默认 stable]：')" choice
            CHANNEL="${choice:-stable}"
        fi
    fi
}

configure_listen() {
    has_terminal || return 0
    local choice
    if ((HOST_GIVEN == 0)); then
        info "Listen IP: 127.0.0.1 for local access, 0.0.0.0 for all IPv4 interfaces; numeric IPv6 is also supported." \
            "监听 IP：127.0.0.1 仅供本机访问，0.0.0.0 监听所有 IPv4 接口；也支持数字形式的 IPv6 地址。"
        while true; do
            read -r -p "$(message 'Listen IP [%s]: ' '监听 IP [%s]：' "$HOST")" choice ||
                die "Input closed; installation canceled." "输入已结束，已取消安装。"
            choice="${choice:-$HOST}"
            if metadata validate "$choice" "$PORT" ""; then HOST="$choice"; break; fi
        done
    fi
    if ((PORT_GIVEN == 0)); then
        while true; do
            read -r -p "$(message 'Listen port (1-65535) [%s]: ' '监听端口（1-65535）[%s]：' "$PORT")" choice ||
                die "Input closed; installation canceled." "输入已结束，已取消安装。"
            choice="${choice:-$PORT}"
            if metadata validate "$HOST" "$choice" ""; then PORT="$choice"; break; fi
        done
    fi
}

listen_address() {
    local host="$HOST"
    [[ "$host" != *:* ]] || host="[$host]"
    printf '%s:%s' "$host" "$PORT"
}

confirm() {
    info "$@"
    ((YES)) && return
    has_terminal || die "Use --yes for a non-interactive operation." "非交互操作请使用 --yes 确认。"
    local answer
    read -r -p "$(message 'Continue? [y/N]: ' '是否继续？[y/N]：')" answer
    [[ "$answer" == y || "$answer" == Y ]] || die "Canceled." "已取消。"
}

preflight() {
    [[ "$(uname -s)" == Linux ]] ||
        die "This installer supports Linux with systemd only." "此安装脚本仅支持使用 systemd 的 Linux。"
    ((BASH_VERSINFO[0] >= 4)) || die "Bash 4 or later is required." "需要 Bash 4 或更高版本。"
    ((EUID == 0)) || die "Run with sudo or as root." "请使用 sudo 或以 root 身份运行。"
    local command
    for command in curl python3 systemctl journalctl flock tar install cp mv rm mktemp stat getent useradd chown chmod head id; do
        command -v "$command" >/dev/null ||
            die "Missing %s. Install curl, CA certificates, Python 3, util-linux, coreutils, tar and user-management tools first." \
                "缺少 %s。请先安装 curl、CA 证书、Python 3、util-linux、coreutils、tar 和用户管理工具。" "$command"
    done
    python3 -c 'import sys; sys.exit(0 if sys.version_info >= (3, 8) else 1)' ||
        die "Python 3.8+ is required." "需要 Python 3.8 或更高版本。"
    if [[ ! -d /run/systemd/system ]] || ! systemctl show-environment >/dev/null; then
        die "systemd is not running. Use the manual or Docker deployment instead." \
            "systemd 未运行。请使用手动部署或 Docker 部署。"
    fi
}

plain_directory() {
    [[ ! -L "$1" && ( ! -e "$1" || -d "$1" ) ]] ||
        die "Refusing a symlink or non-directory: %s" "拒绝使用符号链接或非目录路径：%s" "$1"
}

root_owned() {
    [[ "$(stat -c '%u' "$1")" == 0 ]] || die "Must be owned by root: %s" "必须由 root 拥有：%s" "$1"
    local mode
    mode="$(stat -c '%a' "$1")"
    (((8#$mode & 0022) == 0)) ||
        die "Must not be writable by group/other: %s" "不得允许用户组或其他用户写入：%s" "$1"
}

check_layout() {
    local path
    for path in "$INSTALL_DIR" "$DATA_DIR" "$BACKUP_DIR"; do plain_directory "$path"; done
    [[ ! -L "$UNIT_FILE" ]] || die "Refusing a symlink service file." "拒绝使用符号链接形式的服务文件。"
    [[ ! -e "$INSTALL_DIR" ]] || root_owned "$INSTALL_DIR"
    [[ ! -e "$BACKUP_DIR" ]] || root_owned "$BACKUP_DIR"
    [[ -z "$(systemctl show "$SERVICE" --property=DropInPaths --value)" ]] ||
        die "Service overrides exist. This script cannot safely manage a customized layout." \
            "存在服务覆盖配置，此脚本无法安全管理自定义部署布局。"
}

load_installation() {
    [[ -f "$INSTALL_DIR/.installer.json" && -f "$UNIT_FILE" ]] ||
        die "No script-managed installation found. Existing manual deployments are not overwritten." \
            "未找到由此脚本管理的安装。不会覆盖现有的手动部署。"
    root_owned "$UNIT_FILE"
    metadata read-state "$INSTALL_DIR/.installer.json" "$UNIT_FILE" > "$WORK_DIR/state"
    local state
    mapfile -t state < "$WORK_DIR/state"
    INSTALLED_VERSION="${state[0]}"
    HOST="${state[1]}"; PORT="${state[2]}"; SECURE_COOKIES="${state[3]}"
    info "Installed version: %s" "已安装版本：%s" "${state[0]}"
}

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64) printf 'amd64\n' ;;
        aarch64|arm64) printf 'arm64\n' ;;
        *) die "Only Linux amd64 and arm64 release packages are supported." \
            "仅支持 Linux amd64 和 arm64 发布包。" ;;
    esac
}

download_file() {
    curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
        --connect-timeout 10 --max-time 300 --retry 2 \
        --header 'Accept: application/vnd.github+json' --user-agent 'AI-Model-Connectivity-Installer' \
        --output "$2" "$1"
}

prepare_release() {
    local arch asset api
    arch="$(detect_arch)"
    asset="model-connectivity-linux-$arch.tar.gz"
    api="https://api.github.com/repos/$REPO/releases"
    if [[ -n "$VERSION" ]]; then api="$api/tags/$VERSION"
    elif [[ "$CHANNEL" == stable ]]; then api="$api/latest"
    else api="$api?per_page=100"; fi
    info "Resolving the %s release..." "正在查询 %s 通道的发布版本..." "$CHANNEL"
    download_file "$api" "$WORK_DIR/release.json" ||
        die "Cannot fetch the release. Check connectivity/API rate limits; if there is no stable release, use --channel preview or --version TAG." \
            "无法获取发布版本。请检查网络或 API 限流；若尚无稳定版，请使用 --channel preview 或 --version TAG。"
    VERSION="$(metadata release "$WORK_DIR/release.json" "$CHANNEL" "$VERSION" "$asset")"
    [[ "$VERSION" != *-* ]] || warn "%s is a prerelease, not a stable version." "%s 是预发布版本，并非稳定版。" "$VERSION"
    info "Downloading %s (%s), including the web interface..." "正在下载 %s（%s），包含 Web 界面..." "$VERSION" "$arch"
    progress downloading
    download_file "https://github.com/$REPO/releases/download/$VERSION/$asset" "$WORK_DIR/$asset"
    download_file "https://github.com/$REPO/releases/download/$VERSION/SHA256SUMS.txt" "$WORK_DIR/SHA256SUMS.txt"
    STAGE="$(mktemp -d "$(dirname "$INSTALL_DIR")/.model-connectivity-stage.XXXXXX")"
    progress verifying
    metadata extract "$WORK_DIR/$asset" "$WORK_DIR/SHA256SUMS.txt" "$asset" "$STAGE" "$arch"
}

ensure_service_user() {
    if getent passwd "$SERVICE_USER" >/dev/null; then
        [[ "$(id -u "$SERVICE_USER")" != 0 ]] || die "The service account must not be root." "服务账户不能是 root。"
        getent group "$SERVICE_USER" >/dev/null || die "Missing group: %s" "缺少用户组：%s" "$SERVICE_USER"
        local entry
        entry="$(getent passwd "$SERVICE_USER")"
        case "${entry##*:}" in
            */nologin|*/false) ;;
            *) die "The service account must not have an interactive shell." "服务账户不得使用可交互登录的 shell。" ;;
        esac
        [[ "$(id -gn "$SERVICE_USER")" == "$SERVICE_USER" &&
           "$(id -G "$SERVICE_USER")" == "$(id -g "$SERVICE_USER")" ]] ||
            die "The service account must use its own group without supplementary groups." \
                "服务账户必须使用专属用户组，且不得加入附加用户组。"
    else
        local nologin
        nologin="$(command -v nologin)" || die "Install the nologin utility first." "请先安装 nologin 工具。"
        useradd --system --user-group --home-dir "$DATA_DIR" --shell "$nologin" "$SERVICE_USER"
    fi
}

write_unit() {
    cat <<EOF
# Managed by install-model-connectivity.sh
[Unit]
Description=AI Model Connectivity
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
User=$SERVICE_USER
Group=$SERVICE_USER
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/model-connectivity serve
Environment="APP_HOST=$HOST"
Environment="APP_PORT=$PORT"
Environment="WEB_DIR=$INSTALL_DIR/web"
Environment="DATA_DIR=$DATA_DIR"
Environment="DATABASE_PATH=$DATA_DIR/cg.sqlite"
Environment="SECURE_COOKIES=$SECURE_COOKIES"
Environment="STATUS_LOGIN_REQUIRED=true"
Environment="AUTO_CHECK_RUN_ON_START=false"
Restart=on-failure
RestartSec=5
TimeoutStopSec=30
UMask=0077
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$DATA_DIR

[Install]
WantedBy=multi-user.target
EOF
}

check_updater_layout() {
    local path
    for path in "$UPDATER_DIR" "$UPDATE_STATE_DIR" "$UPDATE_STATE_DIR/requests"; do
        plain_directory "$path"
    done
    for path in "$UPDATER_DIR" "$UPDATE_STATE_DIR"; do
        if [[ -e "$path" ]]; then
            root_owned "$path"
            [[ -f "$path/.installer-owned" && "$(< "$path/.installer-owned")" == "$OWNER_MARKER" ]] ||
                die "Unmanaged updater directory: %s" "非此脚本管理的更新目录：%s" "$path"
        fi
    done
    for path in "$UPDATE_UNIT" "$UPDATE_PATH_UNIT"; do
        [[ ! -L "$path" ]] || die "Refusing a symlink service file." "拒绝使用符号链接形式的服务文件。"
        if [[ -e "$path" ]]; then
            root_owned "$path"
            [[ "$(head -n 1 "$path")" == "# $OWNER_MARKER" ]] ||
                die "Unmanaged updater service: %s" "非此脚本管理的更新服务：%s" "$path"
        else
            [[ -z "$(systemctl show "$(basename "$path")" --property=FragmentPath --value)" ]] ||
                die "An updater service exists outside the installer layout." "安装器管理范围之外已存在同名更新服务。"
        fi
        [[ -z "$(systemctl show "$(basename "$path")" --property=DropInPaths --value)" ]] ||
            die "Updater overrides exist." "更新服务存在自定义覆盖配置。"
    done
    # A Type=oneshot worker stays "activating" while its command runs.
    case "$(systemctl show model-connectivity-update.service --property=ActiveState --value)" in
        active|activating|deactivating|reloading)
            die "An update worker is active. Wait until it exits." "更新服务正在执行，请等待其结束。" ;;
    esac
}

verify_update_protocol() {
    [[ "$("$INSTALL_DIR/model-connectivity" --update-protocol)" == 1 ]] ||
        die "This version does not support web updates. Upgrade with the installer first." \
            "此版本不支持后台更新，请先通过安装脚本升级。"
}

enable_updates() {
    load_installation
    check_updater_layout
    [[ ! -e "$UPDATE_STATE_DIR/requests/request.json" && ! -L "$UPDATE_STATE_DIR/requests/request.json" ]] ||
        die "An update request is pending. Inspect it before enabling the watcher." "存在待处理更新请求，请先检查，再启用更新服务。"
    confirm "Enable a separate privileged updater for administrator-confirmed official releases? The service will briefly stop." \
        "是否启用独立特权更新服务，允许后台管理员确认安装官方发布版本？服务将短暂停止。"
    stop_for_backup
    take_backup
    CHANGING=1
    verify_update_protocol
    install -d -m 0755 "$UPDATER_DIR" "$UPDATE_STATE_DIR"
    printf '%s\n' "$OWNER_MARKER" > "$UPDATER_DIR/.installer-owned"
    printf '%s\n' "$OWNER_MARKER" > "$UPDATE_STATE_DIR/.installer-owned"
    # The app can enqueue requests, but cannot replace the directory or root status.
    install -d -m 0770 "$UPDATE_STATE_DIR/requests"
    chown "root:$SERVICE_USER" "$UPDATE_STATE_DIR/requests"
    install -m 0755 "$INSTALL_DIR/model-connectivity" "$UPDATER_DIR/.worker-new"
    mv -f -- "$UPDATER_DIR/.worker-new" "$UPDATER_DIR/model-connectivity"
    install -m 0644 "${BASH_SOURCE[0]}" "$UPDATER_DIR/.installer-new"
    mv -f -- "$UPDATER_DIR/.installer-new" "$UPDATER_DIR/install.sh"
    cat > "$WORK_DIR/update.service" <<EOF
# $OWNER_MARKER
[Unit]
Description=AI Model Connectivity independent updater
Wants=network-online.target
After=network-online.target

[Service]
Type=oneshot
ExecStart=$UPDATER_DIR/model-connectivity update-worker
WorkingDirectory=/
UMask=0077
TimeoutStartSec=0
TimeoutStopSec=360
KillMode=mixed
StandardInput=null
EOF
    cat > "$WORK_DIR/update.path" <<EOF
# $OWNER_MARKER
[Unit]
Description=AI Model Connectivity update request watcher

[Path]
PathExists=$UPDATE_STATE_DIR/requests/request.json
Unit=model-connectivity-update.service

[Install]
WantedBy=multi-user.target
EOF
    install -m 0644 "$WORK_DIR/update.service" "$UPDATE_UNIT"
    install -m 0644 "$WORK_DIR/update.path" "$UPDATE_PATH_UNIT"
    metadata enable-update-path "$UNIT_FILE" "$WORK_DIR/service" "$DATA_DIR" "$UPDATE_STATE_DIR/requests"
    install -m 0644 "$WORK_DIR/service" "$UNIT_FILE"
    metadata write-state "$INSTALL_DIR/.installer.json" "$UNIT_FILE" "$INSTALLED_VERSION" "$HOST" "$PORT" "$SECURE_COOKIES"
    systemctl daemon-reload
    systemctl enable --now model-connectivity-update.path
    printf '1\n' > "$UPDATE_STATE_DIR/enabled"
    chmod 0644 "$UPDATE_STATE_DIR/enabled"
    if ((WAS_ACTIVE)); then
        systemctl start "$SERVICE"
        wait_healthy || die "Service restart failed." "服务重启失败。"
    fi
    CHANGING=0; STOPPED=0
    info "Web updates enabled. Only official releases can be installed; inspect update service logs if a task is interrupted." \
        "后台更新已启用，仅允许安装官方发布版本；任务中断时请检查独立更新服务日志。"
}

disable_updates() {
    check_updater_layout
    confirm "Disable new web updates? Update records will be kept." "是否禁用新的后台更新？更新记录将保留。"
    if [[ -f "$UPDATE_PATH_UNIT" ]]; then systemctl disable --now model-connectivity-update.path; fi
    rm -f -- "$UPDATE_STATE_DIR/enabled"
    info "Web updates disabled. Existing request/state files were not deleted." \
        "后台更新已禁用。已有请求及状态文件未删除。"
}

stop_for_backup() {
    if [[ -f "$UNIT_FILE" ]]; then
        if systemctl is-active --quiet "$SERVICE"; then WAS_ACTIVE=1; fi
        if systemctl is-enabled --quiet "$SERVICE"; then WAS_ENABLED=1; fi
        STOPPED=1
        systemctl stop "$SERVICE"
        if systemctl is-active --quiet "$SERVICE"; then
            die "Service is still running; refusing an unsafe backup." "服务仍在运行，拒绝执行不安全的备份。"
        fi
    fi
}

take_backup() {
    install -d -m 0700 "$BACKUP_DIR"
    SNAPSHOT="$(mktemp -d "$BACKUP_DIR/$(date -u +%Y%m%dT%H%M%SZ)-XXXXXX")"
    [[ ! -d "$INSTALL_DIR" ]] || cp -a -- "$INSTALL_DIR" "$SNAPSHOT/program"
    [[ ! -d "$DATA_DIR" ]] || cp -a -- "$DATA_DIR" "$SNAPSHOT/data"
    [[ ! -f "$UNIT_FILE" ]] || cp -a -- "$UNIT_FILE" "$SNAPSHOT/service"
    printf 'was_active=%s\nwas_enabled=%s\n' "$WAS_ACTIVE" "$WAS_ENABLED" > "$SNAPSHOT/service-state.txt"
    printf '%s\n' "$OWNER_MARKER" > "$SNAPSHOT/complete"
    info "Offline backup: %s (keep private; includes credentials)" \
        "离线备份：%s（包含凭据，请妥善保管）" "$SNAPSHOT"
}

remove_directory() {
    local target="$1"
    [[ -n "$target" && "$target" != / && ! -L "$target" ]] || return 1
    case "$target" in
        "$INSTALL_DIR"|"$DATA_DIR"|"$WORK_DIR"|"$STAGE") rm -rf -- "$target" ;;
        *) return 1 ;;
    esac
}

wait_healthy() {
    local host="$HOST" _attempt good=0
    [[ "$host" != 0.0.0.0 ]] || host=127.0.0.1
    [[ "$host" != :: ]] || host=::1
    [[ "$host" != *:* ]] || host="[$host]"
    for _attempt in {1..30}; do
        if systemctl is-active --quiet "$SERVICE" &&
            curl --noproxy '*' --fail --silent --max-time 2 "http://$host:$PORT/health" >/dev/null; then
            good=$((good + 1))
            ((good < 3)) || return 0
        else good=0; fi
        sleep 1
    done
    return 1
}

restore_snapshot() {
    [[ -f "$SNAPSHOT/complete" ]] || return 1
    if [[ -f "$UNIT_FILE" ]]; then
        systemctl stop "$SERVICE" || return 1
        if systemctl is-active --quiet "$SERVICE"; then return 1; fi
        systemctl disable "$SERVICE" >/dev/null 2>&1 || return 1
    fi
    remove_directory "$INSTALL_DIR" || return 1
    remove_directory "$DATA_DIR" || return 1
    rm -f -- "$UNIT_FILE" || return 1
    if [[ -d "$SNAPSHOT/program" ]]; then cp -a -- "$SNAPSHOT/program" "$INSTALL_DIR" || return 1; fi
    if [[ -d "$SNAPSHOT/data" ]]; then cp -a -- "$SNAPSHOT/data" "$DATA_DIR" || return 1; fi
    if [[ -f "$SNAPSHOT/service" ]]; then cp -a -- "$SNAPSHOT/service" "$UNIT_FILE" || return 1; fi
    systemctl daemon-reload || return 1
    if ((WAS_ENABLED)); then systemctl enable "$SERVICE" >/dev/null || return 1; fi
    if ((WAS_ACTIVE)); then
        systemctl start "$SERVICE" || return 1
        wait_healthy || return 1
    fi
    STOPPED=0
}

finish() {
    local code=$?
    trap - EXIT INT TERM
    set +e
    if ((code != 0 && CHANGING)); then
        if [[ "$ACTION" == enable-updates ]]; then
            systemctl disable --now model-connectivity-update.path
            rm -f -- "$UPDATE_STATE_DIR/enabled"
        fi
        warn "Operation failed. Restoring the previous program, database and service..." \
            "操作失败，正在恢复之前的程序、数据库和服务..."
        progress restoring
        if restore_snapshot; then
            progress restored
            warn "Previous installation restored." "已恢复之前的安装。"
        else
            warn "Automatic recovery could not complete. Keep %s and recover manually; do not start another database writer." \
                "自动恢复未能完成。请保留 %s 并手动恢复；不要启动其他写入该数据库的进程。" "$SNAPSHOT"
        fi
    elif ((STOPPED && WAS_ACTIVE)); then
        if ! systemctl start "$SERVICE" || ! wait_healthy; then
            warn "The service could not be restarted. Inspect the logs and the backup's complete marker before recovery." \
                "服务无法重新启动。恢复前请检查日志及备份目录的 complete 标记。"
            code=1
        fi
    fi
    [[ -z "$STAGE" ]] || remove_directory "$STAGE"
    [[ -z "$WORK_DIR" ]] || remove_directory "$WORK_DIR"
    exit "$code"
}

install_release() {
    local requested="$VERSION"
    if [[ "$ACTION" == install ]]; then
        [[ ! -e "$INSTALL_DIR" && ! -e "$UNIT_FILE" ]] ||
            die "An installation already exists. Use upgrade for a script-managed installation." \
                "检测到已有安装。由此脚本管理的安装请使用 upgrade 升级。"
        [[ -z "$(systemctl show "$SERVICE" --property=FragmentPath --value)" ]] ||
            die "A service with this name already exists outside the installer layout." \
                "安装器管理范围之外已存在同名服务。"
        if [[ -d "$DATA_DIR" ]]; then
            [[ -f "$DATA_DIR/.installer-owned" && "$(< "$DATA_DIR/.installer-owned")" == "$OWNER_MARKER" ]] ||
                die "An unmanaged data directory exists. Back it up and use the manual migration instructions." \
                    "存在非此脚本管理的数据目录。请先备份，再按照手动迁移说明操作。"
        fi
        configure_listen
    else load_installation; fi
    info "Listen address: %s" "监听地址：%s" "$(listen_address)"
    if [[ "$ACTION" == install && ( "$HOST" == 0.0.0.0 || "$HOST" == :: ) ]]; then
        warn "This listens on all interfaces. Secure remote access with firewall rules and HTTPS; the installer does not configure either." \
            "此设置监听所有网络接口。请配置防火墙和 HTTPS 保护远程访问；安装器不会自动配置这些措施。"
    fi
    confirm "Install %s from channel %s? Upgrades briefly stop the service and back up all data." \
        "是否安装 %s（通道：%s）？升级会短暂停服并备份全部数据。" \
        "${VERSION:-$(message 'the latest available release' '最新可用版本')}" "$CHANNEL"
    prepare_release
    if [[ -n "$INSTALLED_VERSION" && ( -z "$requested" || "${CG_UPDATE_PROGRESS:-}" == 1 ) ]]; then
        metadata compare "$INSTALLED_VERSION" "$VERSION"
    fi
    ensure_service_user
    if [[ "$ACTION" == install ]]; then write_unit > "$WORK_DIR/service"
    else cp -- "$UNIT_FILE" "$WORK_DIR/service"; fi
    metadata write-state "$STAGE/.installer.json" "$WORK_DIR/service" "$VERSION" "$HOST" "$PORT" "$SECURE_COOKIES"
    progress backup
    stop_for_backup
    metadata port-free "$HOST" "$PORT"
    take_backup
    CHANGING=1
    progress installing
    remove_directory "$INSTALL_DIR"
    mv -- "$STAGE" "$INSTALL_DIR"
    STAGE=""
    install -d -m 0700 "$DATA_DIR"
    printf '%s\n' "$OWNER_MARKER" > "$DATA_DIR/.installer-owned"
    chown --no-dereference "$SERVICE_USER:$SERVICE_USER" "$DATA_DIR"
    install -m 0644 "$WORK_DIR/service" "$UNIT_FILE"
    systemctl daemon-reload
    if [[ "$ACTION" == install ]]; then systemctl enable "$SERVICE"; fi
    if [[ "$ACTION" == install ]] || ((WAS_ACTIVE)); then
        progress restarting
        systemctl start "$SERVICE"
        wait_healthy || die "The new service failed its local health check." "新服务未通过本机健康检查。"
    else
        info "The service was stopped before upgrading and remains stopped." "服务在升级前处于停止状态，升级后仍保持停止。"
    fi
    CHANGING=0; STOPPED=0
    info "Installed %s. Data: %s" "已安装 %s。数据目录：%s" "$VERSION" "$DATA_DIR"
    info "Listen address: %s; admin path: /admin" "监听地址：%s；管理入口：/admin" "$(listen_address)"
    info "Initial administrator password: sudo journalctl -u %s -n 50 --no-pager" \
        "查看初始管理员密码：sudo journalctl -u %s -n 50 --no-pager" "$SERVICE"
    info "Health checks do not call models. Existing scheduled checks resume when the service starts." \
        "健康检查不会调用模型。服务启动后，已有定时检测任务会继续运行。"
}

main() {
    choose_language
    parse_args "$@"
    [[ "$ACTION" != help ]] || { usage; return; }
    [[ -n "$ACTION" ]] || menu
    [[ "$CHANNEL" == stable || "$CHANNEL" == preview ]] ||
        die "Channel must be stable or preview." "通道必须为 stable 或 preview。"
    ((CONFIG_GIVEN == 0)) || [[ "$ACTION" == install ]] ||
        die "Listen/cookie options are install-only; upgrade preserves the existing service." \
            "监听地址、端口和 Cookie 选项仅适用于安装；升级会保留现有服务配置。"
    [[ -z "$VERSION" && "$CHANNEL" == stable || "$ACTION" == install || "$ACTION" == upgrade ]] ||
        die "Release options apply only to install/upgrade." "版本和通道选项仅适用于安装或升级。"
    preflight
    metadata validate "$HOST" "$PORT" "$VERSION"
    check_layout
    if [[ "$ACTION" != status && "$ACTION" != logs ]]; then
        [[ ! -L "$LOCK_FILE" ]] || die "Refusing a symlink lock file." "拒绝使用符号链接形式的锁文件。"
        exec 9>"$LOCK_FILE"
        flock -n 9 || die "Another installer operation is running." "另一个安装器操作正在运行。"
    fi
    WORK_DIR="$(mktemp -d)"
    trap finish EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM
    case "$ACTION" in
        install|upgrade) install_release ;;
        enable-updates) enable_updates ;;
        disable-updates) disable_updates ;;
        backup)
            load_installation
            confirm "Stop the service briefly and create a private offline backup?" "是否短暂停服并创建私有离线备份？"
            stop_for_backup
            take_backup ;;
        uninstall)
            load_installation
            check_updater_layout
            confirm "Remove the program and service? Data, backups and the service account will be kept." \
                "是否删除程序和服务？数据、备份和服务账户将保留。"
            stop_for_backup
            take_backup
            if [[ -f "$UPDATE_PATH_UNIT" ]]; then systemctl disable --now model-connectivity-update.path; fi
            rm -f -- "$UPDATE_STATE_DIR/enabled"
            CHANGING=1
            systemctl disable "$SERVICE"
            rm -f -- "$UNIT_FILE"
            remove_directory "$INSTALL_DIR"
            systemctl daemon-reload
            CHANGING=0; STOPPED=0
            info "Uninstalled. Data retained at %s; backups at %s." \
                "已卸载。数据保留在 %s；备份保留在 %s。" "$DATA_DIR" "$BACKUP_DIR" ;;
        status) load_installation; systemctl status "$SERVICE" --no-pager -l ;;
        logs)
            load_installation
            warn "Logs may contain passwords. Do not share them." "日志可能包含密码，请勿分享。"
            journalctl -u "$SERVICE" -n 50 -f ;;
        start|stop|restart) load_installation; systemctl "$ACTION" "$SERVICE" ;;
        *) die "Unknown command." "未知命令。" ;;
    esac
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then main "$@"; fi
