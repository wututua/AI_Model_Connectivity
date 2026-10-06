import hashlib
import io
import json
import os
from pathlib import Path
import shutil
import socket
import struct
import subprocess
import sys
import tarfile
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "install-model-connectivity.sh"
BASH = os.environ.get("BASH_BIN") or shutil.which("bash")
ASSET = "model-connectivity-linux-amd64.tar.gz"

# All service/account/network operations are mocked. Files stay in a validated
# workspace temporary directory, including Bash's mktemp output.
HARNESS = r'''
source "$1"
shift
TEST_ROOT="$1"
shift
INSTALL_DIR="$TEST_ROOT/opt/model-connectivity"
DATA_DIR="$TEST_ROOT/data"
BACKUP_DIR="$TEST_ROOT/backups"
UNIT_FILE="$TEST_ROOT/systemd/model-connectivity.service"
LOCK_FILE="$TEST_ROOT/installer.lock"
UPDATER_DIR="$TEST_ROOT/updater"
UPDATE_STATE_DIR="$TEST_ROOT/update-state"
UPDATE_UNIT="$TEST_ROOT/systemd/model-connectivity-update.service"
UPDATE_PATH_UNIT="$TEST_ROOT/systemd/model-connectivity-update.path"
export TMPDIR="$TEST_ROOT/tmp"
python3() { "$CG_TEST_PYTHON" "$@" | tr -d '\r'; }
preflight() { :; }
root_owned() { :; }
ensure_service_user() { :; }
verify_update_protocol() { [[ "${FAIL_PROTOCOL:-0}" != 1 ]]; }
chown() { :; }
read() {
    printf '%s\n' "$*" >> "$TEST_ROOT/prompts"
    builtin read "$@"
}
cp() {
    if [[ "${FAIL_BACKUP:-0}" == 1 && "${*: -1}" == "$BACKUP_DIR/"* ]]; then return 1; fi
    command cp "$@"
}
detect_arch() { printf 'amd64\n'; }
flock() {
    [[ "${FAIL_LOCK:-0}" != 1 ]] || return 1
    if type -P flock >/dev/null; then command flock "$@"; fi
}
install() {
    if [[ "$CG_TEST_WINDOWS" != 1 ]]; then command install "$@"; return; fi
    python3 - "$@" <<'PY'
import shutil
import sys
from pathlib import Path
args = sys.argv[1:]
directory = "-d" in args
args = [a for a in args if a != "-d"]
assert args[0] == "-m"
mode, *paths = args[1:]
if directory:
    for path in paths:
        Path(path).mkdir(parents=True, exist_ok=True)
else:
    assert len(paths) == 2
    shutil.copyfile(paths[0], paths[1])
PY
}
systemctl() {
    printf '%s\n' "$*" >> "$TEST_ROOT/calls"
    if [[ "$*" == *model-connectivity-update.* ]]; then
        case "$1" in
            show)
                if [[ "$*" == *ActiveState* && "${UPDATER_ACTIVE:-0}" == 1 ]]; then printf 'activating\n'; fi ;;
            is-active) [[ "${UPDATER_ACTIVE:-0}" == 1 ]] ;;
            enable|disable) : ;;
            *) return 1 ;;
        esac
        return
    fi
    case "$1" in
        show)
            if [[ "$*" == *DropInPaths* ]]; then printf '%s' "${MOCK_DROPINS:-}"; fi
            if [[ "$*" == *FragmentPath* ]]; then printf '%s' "${MOCK_FRAGMENT:-}"; fi ;;
        daemon-reload) : ;;
        is-active) [[ -f "$TEST_ROOT/active" ]] ;;
        is-enabled) [[ -f "$TEST_ROOT/enabled" ]] ;;
        stop)
            [[ "${FAIL_STOP:-0}" != 1 ]] || return 1
            rm -f "$TEST_ROOT/active" ;;
        enable) touch "$TEST_ROOT/enabled" ;;
        disable) rm -f "$TEST_ROOT/enabled" ;;
        start|restart)
            [[ -f "$UNIT_FILE" ]] || return 1
            touch "$TEST_ROOT/active"
            if [[ ! -f "$DATA_DIR/cg.sqlite" ]]; then printf 'original database' > "$DATA_DIR/cg.sqlite"; fi
            if [[ "${FAIL_HEALTH:-0}" == 1 ]] && grep -q 'v1.0.0-rc.1' "$INSTALL_DIR/.installer.json"; then
                printf 'new schema and data' > "$DATA_DIR/cg.sqlite"
            fi ;;
        status) [[ -f "$TEST_ROOT/active" ]] ;;
        *) printf 'Unexpected systemctl: %s\n' "$*" >&2; return 1 ;;
    esac
}
wait_healthy() {
    [[ -f "$TEST_ROOT/active" ]] || return 1
    if [[ "${FAIL_HEALTH:-0}" == 1 ]] && grep -q 'v1.0.0-rc.1' "$INSTALL_DIR/.installer.json"; then return 1; fi
}
download_file() {
    if [[ "$1" == *api.github.com* ]]; then
        cp "$TEST_ROOT/fixtures/release.json" "$2"
    else
        [[ "${FAIL_DOWNLOAD:-0}" != 1 ]] || return 1
        cp "$TEST_ROOT/fixtures/${1##*/}" "$2"
    fi
}
if [[ "$CG_TEST_INTERACTIVE" == 1 ]]; then has_terminal() { return 0; }; fi
if [[ "${1:-}" == helper ]]; then shift; choose_language; metadata "$@"; else main "$@"; fi
'''


@unittest.skipUnless(BASH, "Bash is required; set BASH_BIN when it is not on PATH")
class InstallerTests(unittest.TestCase):
    def setUp(self):
        artifacts = ROOT / "dist" / "installer-tests"
        artifacts.mkdir(parents=True, exist_ok=True)
        self.temporary = tempfile.TemporaryDirectory(prefix="case-", dir=artifacts)
        self.root = Path(self.temporary.name).resolve()
        self.assertIn(artifacts.resolve(), self.root.parents)
        for name in ("opt", "systemd", "tmp", "fixtures"):
            (self.root / name).mkdir()
        self.addCleanup(self.temporary.cleanup)
        self.program = self.root / "opt/model-connectivity"
        self.data = self.root / "data"
        self.unit = self.root / "systemd/model-connectivity.service"
        with socket.socket() as sock:
            sock.bind(("127.0.0.1", 0))
            self.port = str(sock.getsockname()[1])
        self.release()

    def release(self, version="v1.0.0-beta.2", extra=None, bad_hash=False, arch=62):
        fixtures = self.root / "fixtures"
        header = bytearray(64)
        header[:6] = b"\x7fELF\x02\x01"
        struct.pack_into("<H", header, 18, arch)
        with tarfile.open(fixtures / ASSET, "w:gz") as bundle:
            for name, value in {
                "model-connectivity": bytes(header),
                "web/index.html": b'<div id="root"></div>',
                "web/assets/app.js": b"// fixture",
                "LICENSE": b"test fixture",
            }.items():
                member = tarfile.TarInfo(name)
                member.size = len(value)
                bundle.addfile(member, io.BytesIO(value))
            if extra:
                member = tarfile.TarInfo(extra[0])
                member.type = extra[1]
                member.linkname = "/etc/passwd" if member.issym() or member.islnk() else ""
                bundle.addfile(member)
        digest = hashlib.sha256((fixtures / ASSET).read_bytes()).hexdigest()
        if bad_hash:
            digest = "0" * 64
        (fixtures / "SHA256SUMS.txt").write_text(f"{digest}  {ASSET}\n", encoding="utf-8")
        self.release_data = {
            "tag_name": version, "draft": False, "prerelease": "-" in version,
            "published_at": "2026-10-06T00:00:00Z",
            "assets": [{"name": name, "state": "uploaded"} for name in (ASSET, "SHA256SUMS.txt")],
        }
        (fixtures / "release.json").write_text(json.dumps(self.release_data), encoding="utf-8")

    def invoke_script(self, *args, ok=True, interactive=False, input_text="", **variables):
        env = {**os.environ, "CG_TEST_PYTHON": sys.executable.replace("\\", "/"),
               "CG_TEST_WINDOWS": "1" if os.name == "nt" else "0",
               "CG_TEST_INTERACTIVE": "1" if interactive else "0",
               "PYTHONIOENCODING": "utf-8", "PYTHONUTF8": "1", "LC_ALL": "C", **variables}
        process = subprocess.run(
            [BASH, "--noprofile", "--norc", "-c", HARNESS, "installer-test",
             SCRIPT.as_posix(), self.root.as_posix(), *args],
            # Binary input preserves LF for Bash on Windows as well as Linux.
            input=input_text.encode("utf-8"), capture_output=True, env=env, timeout=30,
        )
        process.stdout = process.stdout.decode("utf-8").replace("\r\n", "\n")
        process.stderr = process.stderr.decode("utf-8").replace("\r\n", "\n")
        output = process.stdout + process.stderr
        if ok:
            self.assertEqual(process.returncode, 0, output)
        else:
            self.assertNotEqual(process.returncode, 0, output)
        return process

    def run_script(self, *args, **kwargs):
        process = self.invoke_script(*args, **kwargs)
        return process.stdout + process.stderr

    def install(self):
        return self.run_script("install", "--version", "v1.0.0-beta.2", "--port", self.port, "--yes")

    def calls(self):
        path = self.root / "calls"
        return path.read_text(encoding="utf-8") if path.exists() else ""

    def prompts(self):
        path = self.root / "prompts"
        return path.read_text(encoding="utf-8") if path.exists() else ""

    def test_help_and_bad_inputs_are_non_destructive(self):
        self.assertIn("Linux/systemd", self.run_script("--help"))
        for args in (("--unknown",), ("install", "--port", "0", "--yes"),
                     ("install", "--port", "65536", "--yes"),
                     ("install", "--host", "127.0.0.1\nExecStart=/bin/false", "--yes"),
                     ("install", "--version", "../../etc", "--yes"),
                     ("upgrade", "--port", "8081", "--yes"),
                     ("install", "--channel", "nightly", "--yes")):
            with self.subTest(args=args):
                self.run_script(*args, ok=False)
        self.assertFalse(self.program.exists())
        self.assertNotIn("stop ", self.calls())

    def test_language_menu_selects_chinese_english_and_default(self):
        for choice, expected, absent in (("1", "1) 安装", "1) Install"),
                                         ("2", "1) Install", "1) 安装"),
                                         ("", "1) 安装", "1) Install")):
            with self.subTest(choice=choice):
                process = self.invoke_script(interactive=True, input_text=f"{choice}\n0\n")
                self.assertIn("1. 简体中文\n2. English\n", process.stderr)
                self.assertIn(expected, process.stdout)
                self.assertNotIn(absent, process.stdout)
        self.assertFalse(self.program.exists())
        self.assertEqual(self.calls(), "")

    def test_invalid_language_retries_and_closed_input_exits(self):
        output = self.run_script(interactive=True, input_text="3\n2\n0\n")
        self.assertIn("请输入 1 或 2。 / Enter 1 or 2.", output)
        self.assertIn("1) Install", output)
        output = self.run_script(interactive=True, input_text="", ok=False)
        self.assertIn("输入已结束。 / Input closed.", output)
        self.assertFalse(self.program.exists())
        self.assertEqual(self.calls(), "")

    def test_selected_language_applies_to_help_and_argument_errors(self):
        for choice, help_text, error_text in (("1", "用法：", "错误：未知参数："),
                                              ("2", "Usage:", "ERROR: Unknown argument:")):
            with self.subTest(choice=choice):
                process = self.invoke_script("--help", interactive=True, input_text=f"{choice}\n")
                self.assertIn(help_text, process.stdout)
                self.assertNotIn("--lang", process.stdout)
                self.assertIn("1. 简体中文", process.stderr)
                # Literal values must not become printf format strings or shell commands.
                argument = r"--unknown-%s-\n-$(touch unwanted)"
                output = self.run_script(argument, interactive=True, input_text=f"{choice}\n", ok=False)
                self.assertIn(error_text + (" " if choice == "2" else "") + argument, output)
        self.assertFalse(self.program.exists())
        self.assertEqual(self.calls(), "")

    def test_noninteractive_runs_do_not_read_language_or_confirmations(self):
        process = self.invoke_script("--help", input_text="1\n")
        self.assertIn("Usage:", process.stdout)
        self.assertEqual(process.stderr, "")
        self.assertIn("Specify a command", self.run_script(ok=False))
        output = self.run_script("install", "--version", "v1.0.0-beta.2", input_text="1\ny\n", ok=False)
        self.assertIn("Use --yes", output)
        self.assertNotIn("Choose language", output)
        self.assertIn("Unknown argument: --lang", self.run_script("--lang", "zh", ok=False))
        self.assertFalse(self.program.exists())
        self.assertNotIn("stop ", self.calls())

    def test_chinese_validation_and_archive_errors(self):
        for arguments, expected in (
                (("--port", "0"), "端口必须在 1-65535 之间"),
                (("--host", "999.0.0.1"), "IPv4/IPv6 监听地址无效"),
                (("--host", "localhost"), "请使用数字形式的 IPv4/IPv6 监听地址"),
                (("--version", "../../etc"), "发布版本标签无效")):
            with self.subTest(arguments=arguments):
                output = self.run_script("install", *arguments, "--yes",
                                         interactive=True, input_text="1\n", ok=False)
                self.assertIn("错误：元数据操作失败：" + expected, output)
                self.assertNotIn("Traceback", output)
        self.release(bad_hash=True)
        output = self.run_script("install", "--version", "v1.0.0-beta.2", "--yes",
                                 interactive=True, input_text="1\n\n\n", ok=False)
        self.assertIn("SHA-256 校验失败", output)
        self.assertFalse(self.program.exists())
        self.assertNotIn("stop ", self.calls())

    def test_chinese_python_errors_use_utf8_even_with_ascii_environment(self):
        output = self.run_script("install", "--port", "0", "--yes", interactive=True,
                                 input_text="1\n", ok=False, PYTHONIOENCODING="ascii", PYTHONUTF8="0")
        self.assertIn("错误：元数据操作失败：端口必须在 1-65535 之间", output)
        self.assertNotIn("Traceback", output)
        self.assertFalse(self.program.exists())

    def test_chinese_install_then_english_upgrade_preserves_machine_data(self):
        output = self.run_script("install", "--version", "v1.0.0-beta.2", "--port", self.port,
                                 "--yes", interactive=True, input_text="1\n\n")
        self.assertIn("已安装 v1.0.0-beta.2", output)
        self.assertIn("警告：v1.0.0-beta.2 是预发布版本", output)
        self.assertIn("监听 IP [127.0.0.1]", self.prompts())
        self.assertNotIn("监听端口", self.prompts())
        original_unit = self.unit.read_bytes()
        self.assertTrue(original_unit.isascii())
        for choice in ("1", "2"):
            process = self.invoke_script("helper", "read-state", (self.program / ".installer.json").as_posix(),
                                         self.unit.as_posix(), interactive=True, input_text=f"{choice}\n")
            self.assertEqual(process.stdout, f"v1.0.0-beta.2\n127.0.0.1\n{self.port}\nfalse\n")
            self.assertIn("1. 简体中文", process.stderr)
            process = self.invoke_script("helper", "release", (self.root / "fixtures/release.json").as_posix(),
                                         "preview", "", ASSET, interactive=True, input_text=f"{choice}\n")
            self.assertEqual(process.stdout, "v1.0.0-beta.2\n")
        self.release("v1.0.0-rc.1")
        output = self.run_script("upgrade", "--version", "v1.0.0-rc.1", "--yes",
                                 interactive=True, input_text="2\n")
        self.assertIn("Installed v1.0.0-rc.1", output)
        self.assertEqual(self.unit.read_bytes(), original_unit)
        self.assertEqual((self.data / "cg.sqlite").read_text(), "original database")

    def test_chinese_cancellation_does_not_install(self):
        output = self.run_script("install", "--version", "v1.0.0-beta.2",
                                 interactive=True, input_text="1\n\n\nn\n", ok=False)
        self.assertIn("监听地址：127.0.0.1:8080", output)
        self.assertIn("是否安装 v1.0.0-beta.2", output)
        self.assertIn("错误：已取消。", output)
        self.assertFalse(self.program.exists())
        self.assertNotIn("stop ", self.calls())

    def test_menu_install_custom_listen_settings_are_preserved_on_upgrade(self):
        output = self.run_script(interactive=True,
                                 input_text=f"1\n1\nv1.0.0-beta.2\n0.0.0.0\n{self.port}\ny\n")
        self.assertIn(f"监听地址：0.0.0.0:{self.port}", output)
        self.assertIn("此设置监听所有网络接口", output)
        state = json.loads((self.program / ".installer.json").read_text())
        self.assertEqual((state["host"], state["port"]), ("0.0.0.0", self.port))
        original_unit = self.unit.read_bytes()
        self.assertIn(b'Environment="APP_HOST=0.0.0.0"', original_unit)
        self.assertIn(f'Environment="APP_PORT={self.port}"'.encode(), original_unit)
        before = self.prompts()
        self.release("v1.0.0-rc.1")
        self.run_script("upgrade", "--version", "v1.0.0-rc.1", "--yes",
                        interactive=True, input_text="2\n")
        self.assertEqual(self.unit.read_bytes(), original_unit)
        self.assertEqual((self.data / "cg.sqlite").read_text(), "original database")
        self.assertNotIn("Listen IP", self.prompts()[len(before):])
        self.assertNotIn("Listen port", self.prompts()[len(before):])

    def test_command_install_prompts_for_custom_address_and_port_in_english(self):
        output = self.run_script("install", "--version", "v1.0.0-beta.2",
                                 interactive=True, input_text=f"2\n127.0.0.1\n{self.port}\ny\n")
        self.assertIn(f"Listen address: 127.0.0.1:{self.port}", output)
        self.assertIn("Listen IP [127.0.0.1]", self.prompts())
        self.assertIn("Listen port (1-65535) [8080]", self.prompts())
        self.assertNotIn("This listens on all interfaces", output)
        state = json.loads((self.program / ".installer.json").read_text())
        self.assertEqual((state["host"], state["port"]), ("127.0.0.1", self.port))

    def test_interactive_listen_settings_retry_invalid_input(self):
        output = self.run_script("install", "--version", "v1.0.0-beta.2", "--yes", interactive=True,
                                 input_text=f"1\nlocalhost\n999.0.0.1\n127.0.0.1\n0\n65536\nabc\n{self.port}\n")
        self.assertIn("请使用数字形式的 IPv4/IPv6 监听地址", output)
        self.assertIn("IPv4/IPv6 监听地址无效", output)
        self.assertEqual(output.count("端口必须在 1-65535 之间"), 3)
        self.assertEqual(self.prompts().count("监听 IP [127.0.0.1]"), 3)
        self.assertEqual(self.prompts().count("监听端口（1-65535）[8080]"), 4)
        state = json.loads((self.program / ".installer.json").read_text())
        self.assertEqual((state["host"], state["port"]), ("127.0.0.1", self.port))

    def test_explicit_listen_options_skip_network_prompts_even_in_menu(self):
        output = self.run_script("--host", "0.0.0.0", "--port", self.port, interactive=True,
                                 input_text="1\n1\nv1.0.0-beta.2\ny\n")
        self.assertIn(f"监听地址：0.0.0.0:{self.port}", output)
        self.assertNotIn("监听 IP", self.prompts())
        self.assertNotIn("监听端口", self.prompts())
        state = json.loads((self.program / ".installer.json").read_text())
        self.assertEqual((state["host"], state["port"]), ("0.0.0.0", self.port))

    def test_explicit_host_only_prompts_for_port(self):
        output = self.run_script("install", "--version", "v1.0.0-beta.2", "--host", "0.0.0.0", "--yes",
                                 interactive=True, input_text=f"2\n{self.port}\n")
        self.assertIn(f"Listen address: 0.0.0.0:{self.port}", output)
        self.assertNotIn("Listen IP", self.prompts())
        self.assertIn("Listen port", self.prompts())
        state = json.loads((self.program / ".installer.json").read_text())
        self.assertEqual((state["host"], state["port"]), ("0.0.0.0", self.port))

    def test_closed_listen_input_cancels_without_installing(self):
        for input_text in ("1\n", "1\n127.0.0.1\n"):
            with self.subTest(input_text=input_text):
                output = self.run_script("install", "--version", "v1.0.0-beta.2",
                                         interactive=True, input_text=input_text, ok=False)
                self.assertIn("输入已结束，已取消安装。", output)
        self.assertFalse(self.program.exists())
        self.assertFalse(self.unit.exists())
        self.assertFalse(self.data.exists())
        self.assertNotIn("stop ", self.calls())

    def test_busy_custom_port_does_not_create_an_installation(self):
        with socket.socket() as sock:
            sock.bind(("127.0.0.1", int(self.port)))
            sock.listen()
            self.run_script("install", "--version", "v1.0.0-beta.2", "--yes",
                            interactive=True, input_text=f"2\n127.0.0.1\n{self.port}\n", ok=False)
        self.assertFalse(self.program.exists())
        self.assertFalse(self.unit.exists())
        self.assertFalse(self.data.exists())
        self.assertNotIn("stop ", self.calls())

    def test_ipv6_listen_summary_uses_brackets(self):
        output = self.run_script("install", "--version", "v1.0.0-beta.2", interactive=True,
                                 input_text=f"2\n::1\n{self.port}\nn\n", ok=False)
        self.assertIn(f"Listen address: [::1]:{self.port}", output)
        self.assertIn("Canceled.", output)
        self.assertFalse(self.program.exists())

    def test_chinese_failed_upgrade_restores_data_and_translates_recovery(self):
        self.install()
        original_unit = self.unit.read_bytes()
        self.release("v1.0.0-rc.1")
        output = self.run_script("upgrade", "--version", "v1.0.0-rc.1", "--yes", interactive=True,
                                 input_text="1\n", ok=False, FAIL_HEALTH="1")
        self.assertIn("新服务未通过本机健康检查。", output)
        self.assertIn("已恢复之前的安装。", output)
        self.assertEqual((self.data / "cg.sqlite").read_text(), "original database")
        self.assertEqual(self.unit.read_bytes(), original_unit)
        self.assertEqual(json.loads((self.program / ".installer.json").read_text())["version"], "v1.0.0-beta.2")
        self.assertTrue((self.root / "active").exists())

    def test_install_has_complete_web_and_private_service_defaults(self):
        self.install()
        self.assertTrue((self.program / "web/assets/app.js").is_file())
        unit = self.unit.read_text(encoding="utf-8")
        for text in ("User=model-connectivity", 'APP_HOST=127.0.0.1', "STATUS_LOGIN_REQUIRED=true",
                     "ProtectSystem=strict", "AUTO_CHECK_RUN_ON_START=false", f"APP_PORT={self.port}"):
            self.assertIn(text, unit)
        self.assertNotIn("EnvironmentFile", unit)
        self.assertTrue((self.root / "active").is_file())
        if os.name != "nt":
            self.assertEqual((self.program / "web/assets").stat().st_mode & 0o777, 0o755)
            self.assertEqual((self.program / "model-connectivity").stat().st_mode & 0o777, 0o755)
            self.assertEqual(self.data.stat().st_mode & 0o777, 0o700)

    def test_preview_requires_opt_in_and_can_select_the_newest_release(self):
        self.run_script("install", "--yes", ok=False)
        self.assertFalse(self.program.exists())
        self.assertNotIn("stop ", self.calls())
        releases = [self.release_data, {**self.release_data, "tag_name": "v1.0.0-rc.1",
                    "published_at": "2026-10-07T00:00:00Z"}]
        (self.root / "fixtures/release.json").write_text(json.dumps(releases), encoding="utf-8")
        self.run_script("install", "--channel", "preview", "--port", self.port, "--yes")
        state = json.loads((self.program / ".installer.json").read_text(encoding="utf-8"))
        self.assertEqual(state["version"], "v1.0.0-rc.1")

    def test_upgrade_keeps_data_port_service_and_complete_backup(self):
        self.install()
        original_unit = self.unit.read_bytes()
        self.release("v1.0.0-rc.1")
        self.run_script("upgrade", "--version", "v1.0.0-rc.1", "--yes")
        self.assertEqual(self.unit.read_bytes(), original_unit)
        self.assertEqual((self.data / "cg.sqlite").read_text(), "original database")
        state = json.loads((self.program / ".installer.json").read_text())
        self.assertEqual((state["version"], state["port"]), ("v1.0.0-rc.1", self.port))
        backups = list((self.root / "backups").glob("*/program/.installer.json"))
        self.assertEqual(len(backups), 1)
        backup = backups[0].parent.parent
        self.assertEqual((backup / "data/cg.sqlite").read_text(), "original database")
        self.assertEqual((backup / "service").read_bytes(), original_unit)
        self.assertTrue((backup / "complete").is_file())

    def test_failed_upgrade_restores_database_program_and_service(self):
        self.install()
        original_unit = self.unit.read_bytes()
        self.release("v1.0.0-rc.1")
        output = self.run_script("upgrade", "--version", "v1.0.0-rc.1", "--yes", ok=False, FAIL_HEALTH="1")
        self.assertIn("Previous installation restored", output)
        self.assertEqual((self.data / "cg.sqlite").read_text(), "original database")
        self.assertEqual(self.unit.read_bytes(), original_unit)
        self.assertEqual(json.loads((self.program / ".installer.json").read_text())["version"], "v1.0.0-beta.2")
        self.assertTrue((self.root / "active").exists())
        self.assertTrue((self.root / "enabled").exists())

    def test_download_and_checksum_failures_do_not_stop_the_old_service(self):
        self.install()
        before = self.calls()
        self.release("v1.0.0-rc.1")
        self.run_script("upgrade", "--version", "v1.0.0-rc.1", "--yes", ok=False, FAIL_DOWNLOAD="1")
        self.release("v1.0.0-rc.1", bad_hash=True)
        self.run_script("upgrade", "--version", "v1.0.0-rc.1", "--yes", ok=False)
        self.assertNotIn("stop ", self.calls()[len(before):])
        self.assertTrue((self.root / "active").exists())
        self.assertEqual((self.data / "cg.sqlite").read_text(), "original database")

    def test_unsafe_archives_and_wrong_architecture_are_rejected(self):
        for entry in (("../escape", tarfile.REGTYPE), ("/tmp/escape", tarfile.REGTYPE),
                      ("web/link", tarfile.SYMTYPE), ("web/hardlink", tarfile.LNKTYPE),
                      ("web/fifo", tarfile.FIFOTYPE), ("data/cg.sqlite", tarfile.REGTYPE),
                      ("web/../../escape", tarfile.REGTYPE)):
            with self.subTest(entry=entry):
                self.release(extra=entry)
                self.run_script("install", "--version", "v1.0.0-beta.2", "--yes", ok=False)
                self.assertFalse(self.program.exists())
        self.release(arch=183)
        self.run_script("install", "--version", "v1.0.0-beta.2", "--yes", ok=False)
        self.assertFalse(self.program.exists())

    def test_missing_checksum_asset_and_duplicate_checksum_are_rejected(self):
        self.release_data["assets"].pop()
        (self.root / "fixtures/release.json").write_text(json.dumps(self.release_data), encoding="utf-8")
        self.run_script("install", "--version", "v1.0.0-beta.2", "--yes", ok=False)
        self.release()
        checksums = self.root / "fixtures/SHA256SUMS.txt"
        checksums.write_text(checksums.read_text() * 2)
        self.run_script("install", "--version", "v1.0.0-beta.2", "--yes", ok=False)
        self.assertFalse(self.program.exists())

    def test_unmanaged_installations_and_overrides_are_not_modified(self):
        self.run_script("install", "--yes", ok=False, MOCK_FRAGMENT="/usr/lib/systemd/system/other.service")
        self.data.mkdir()
        (self.data / "cg.sqlite").write_text("manual data")
        self.run_script("install", "--yes", ok=False)
        self.assertEqual((self.data / "cg.sqlite").read_text(), "manual data")
        (self.data / ".installer-owned").write_text("AI_Model_Connectivity installer v1\n")
        self.install()
        self.run_script("upgrade", "--yes", ok=False, MOCK_DROPINS="/etc/systemd/system/custom.conf")
        unit = self.unit.read_text()
        self.unit.write_text(unit + "\nEnvironment=DATA_DIR=/another/database\n")
        self.run_script("upgrade", "--yes", ok=False)
        self.assertTrue(self.unit.read_text().endswith("DATA_DIR=/another/database\n"))

    def test_uninstall_retains_data_and_supports_reinstallation(self):
        self.install()
        self.run_script("uninstall", "--yes")
        self.assertFalse(self.unit.exists())
        self.assertFalse(self.program.exists())
        self.assertEqual((self.data / "cg.sqlite").read_text(), "original database")
        self.assertFalse((self.root / "active").exists())
        self.install()
        self.assertEqual((self.data / "cg.sqlite").read_text(), "original database")
        self.assertTrue((self.root / "active").exists())

    def test_backup_and_upgrade_preserve_stopped_state(self):
        self.install()
        self.run_script("stop")
        self.run_script("backup", "--yes")
        self.assertFalse((self.root / "active").exists())
        self.release("v1.0.0-rc.1")
        self.run_script("upgrade", "--version", "v1.0.0-rc.1", "--yes")
        self.assertFalse((self.root / "active").exists())
        self.assertTrue((self.root / "enabled").exists())

    def test_busy_port_or_failed_stop_preserve_the_installation(self):
        self.install()
        self.release("v1.0.0-rc.1")
        with socket.socket() as sock:
            sock.bind(("127.0.0.1", int(self.port)))
            sock.listen()
            self.run_script("upgrade", "--version", "v1.0.0-rc.1", "--yes", ok=False)
        self.assertTrue((self.root / "active").exists())
        self.run_script("upgrade", "--version", "v1.0.0-rc.1", "--yes", ok=False, FAIL_STOP="1")
        self.assertEqual(json.loads((self.program / ".installer.json").read_text())["version"], "v1.0.0-beta.2")
        self.assertEqual((self.data / "cg.sqlite").read_text(), "original database")

    def test_concurrent_operation_is_rejected_before_download_or_stop(self):
        self.install()
        before = self.calls()
        self.run_script("upgrade", "--yes", ok=False, FAIL_LOCK="1")
        self.assertNotIn("stop ", self.calls()[len(before):])
        self.assertTrue((self.root / "active").exists())

    def test_channel_change_cannot_silently_downgrade(self):
        self.install()
        before = self.calls()
        self.release("v1.0.0-beta.1")
        output = self.run_script("upgrade", "--channel", "preview", "--yes", ok=False)
        self.assertIn("automatic downgrade", output)
        self.assertNotIn("stop ", self.calls()[len(before):])
        self.run_script("upgrade", "--version", "v1.0.0-beta.1", "--yes")
        self.assertEqual(json.loads((self.program / ".installer.json").read_text())["version"], "v1.0.0-beta.1")

    def test_semantic_prerelease_order(self):
        self.run_script("helper", "compare", "v1.0.0-beta.2", "v1.0.0-rc.1")
        self.run_script("helper", "compare", "v1.0.0-rc.9", "v1.0.0-rc.10")
        self.run_script("helper", "compare", "v1.0.0-rc.10", "v1.0.0")
        self.run_script("helper", "compare", "v1.0.0", "v1.0.0-rc.1", ok=False)
        self.run_script("helper", "compare", "v1.0.0-rc.10", "v1.0.0-rc.9", ok=False)

    def test_failed_reinstall_preserves_retained_data(self):
        self.install()
        self.run_script("uninstall", "--yes")
        self.release("v1.0.0-rc.1")
        output = self.run_script("install", "--version", "v1.0.0-rc.1", "--port", self.port,
                                 "--yes", ok=False, FAIL_HEALTH="1")
        self.assertIn("Previous installation restored", output)
        self.assertFalse(self.unit.exists())
        self.assertFalse(self.program.exists())
        self.assertEqual((self.data / "cg.sqlite").read_text(), "original database")
        self.assertFalse((self.root / "active").exists())

    def test_offline_backup_restarts_a_previously_active_service(self):
        self.install()
        self.run_script("backup", "--yes")
        self.assertTrue((self.root / "active").exists())
        self.assertEqual(len(list((self.root / "backups").glob("*/data/cg.sqlite"))), 1)

    def test_backup_failure_restarts_without_replacing_the_old_installation(self):
        self.install()
        before = set((self.root / "backups").iterdir())
        self.release("v1.0.0-rc.1")
        self.run_script("upgrade", "--version", "v1.0.0-rc.1", "--yes", ok=False, FAIL_BACKUP="1")
        self.assertTrue((self.root / "active").exists())
        self.assertEqual((self.data / "cg.sqlite").read_text(), "original database")
        self.assertEqual(json.loads((self.program / ".installer.json").read_text())["version"], "v1.0.0-beta.2")
        partial = set((self.root / "backups").iterdir()) - before
        self.assertEqual(len(partial), 1)
        self.assertFalse((partial.pop() / "complete").exists())

    def test_enable_and_disable_web_updater_preserve_service_and_data(self):
        self.install()
        self.run_script("enable-updates", "--yes")
        state = self.root / "update-state"
        self.assertEqual((state / "enabled").read_text(), "1\n")
        self.assertTrue((self.root / "updater/model-connectivity").is_file())
        self.assertTrue((self.root / "updater/install.sh").is_file())
        unit = self.unit.read_text()
        self.assertIn(f"ReadWritePaths=-{state.as_posix()}/requests", unit)
        self.assertIn("update-worker", (self.root / "systemd/model-connectivity-update.service").read_text())
        self.assertIn("PathExists=", (self.root / "systemd/model-connectivity-update.path").read_text())
        self.assertNotIn("User=root", unit)
        self.assertEqual((self.data / "cg.sqlite").read_text(), "original database")
        self.assertTrue((self.root / "active").exists())
        self.run_script("enable-updates", "--yes")
        self.assertEqual(self.unit.read_text().count("ReadWritePaths=-"), 1)
        self.run_script("disable-updates", "--yes")
        self.assertFalse((state / "enabled").exists())
        self.assertTrue((self.root / "active").exists())

    def test_enable_failure_restores_previous_unit_and_data(self):
        self.install()
        unit = self.unit.read_bytes()
        self.run_script("enable-updates", "--yes", FAIL_PROTOCOL="1", ok=False)
        self.assertEqual(self.unit.read_bytes(), unit)
        self.assertEqual((self.data / "cg.sqlite").read_text(), "original database")
        self.assertTrue((self.root / "active").exists())

    def test_uninstall_refuses_running_updater(self):
        self.install()
        self.run_script("uninstall", "--yes", UPDATER_ACTIVE="1", ok=False)
        self.assertTrue(self.program.exists())
        self.assertTrue((self.root / "active").exists())

    def test_worker_progress_and_explicit_downgrade_protection(self):
        self.install()
        self.release("v1.0.0-beta.1")
        self.run_script("upgrade", "--version", "v1.0.0-beta.1", "--yes", CG_UPDATE_PROGRESS="1", ok=False)
        self.release("v1.0.0-rc.1")
        output = self.run_script("upgrade", "--version", "v1.0.0-rc.1", "--yes",
                                 CG_UPDATE_PROGRESS="1", FAIL_HEALTH="1", ok=False)
        for stage in ("downloading", "verifying", "backup", "installing", "restarting", "restoring", "restored"):
            self.assertIn("CG_UPDATE_STAGE=" + stage, output)

if __name__ == "__main__":
    unittest.main()
