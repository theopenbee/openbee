#!/usr/bin/env python3
import argparse
import json
import os
import re
import secrets
import shutil
import signal
import socket
import sqlite3
import subprocess
import sys
import time
import urllib.error
import urllib.request
from datetime import datetime
from pathlib import Path

E2E = Path(os.environ.get("OPENBEE_E2E_HOME", "~/.openbee-e2e")).expanduser()
ENVS = E2E / "envs"
TOOLS = E2E / "tools"
PORT_RANGE = range(12360, 12380)
DEFAULT_NAME = "e2e"
LINEAR_LABEL = "openbee-e2e"
STATE_DIR = ".openbee"

CHANNELS = {
    "telegram": {"keys": ["token"], "defaults": {"auth_code": ""}, "package": "@theopenbee/channel-telegram"},
    "discord": {"keys": ["token"], "defaults": {}, "package": "@theopenbee/channel-discord"},
    "feishu": {"keys": ["app_id", "app_secret"], "defaults": {}, "package": "@theopenbee/channel-lark"},
    "dingtalk": {"keys": ["client_id", "client_secret"], "defaults": {}, "package": "@theopenbee/channel-dingtalk"},
    "wecom": {"keys": ["bot_id", "secret"], "defaults": {}, "package": "@theopenbee/channel-wecom"},
    "linear": {"keys": ["api_key", "projects", "states"], "defaults": {"label_name": LINEAR_LABEL}, "package": "@theopenbee/channel-linear"},
    "weixin": {"keys": [], "defaults": {}, "package": "@theopenbee/channel-weixin"},
}

WATCH_TABLES = [
    ("bee_platform_messages", "received_at", "id, platform, status, session_key, substr(content, 1, 100)"),
    ("bee_tasks", "created_at", "id, status, type, worker_id, substr(instruction, 1, 100)"),
    ("bee_outbound_messages", "created_at", "id, status, source_type, error, substr(content, 1, 100)"),
]


def die(msg):
    print(f"error: {msg}", file=sys.stderr)
    sys.exit(1)


def log(msg):
    print(f"[testenv] {msg}", flush=True)


def node_bin():
    override = os.environ.get("OPENBEE_E2E_NODE_BIN")
    if override:
        return Path(override).expanduser()
    root = Path.home() / ".nvm" / "versions" / "node"
    versions = sorted(root.glob("v24.*"), key=lambda p: [int(x) for x in p.name[1:].split(".")])
    if versions:
        return versions[-1] / "bin"
    node = shutil.which("node")
    if not node:
        die("node not found; install node 24 or set OPENBEE_E2E_NODE_BIN")
    return Path(node).resolve().parent


def pi_bin():
    p = node_bin() / "pi"
    if p.exists():
        return p
    found = shutil.which("pi")
    if not found:
        die(f"pi not found in {node_bin()} or on PATH; npm install -g @earendil-works/pi-coding-agent")
    return Path(found)


def env_dir(name):
    return ENVS / name


def backend_dir(src):
    b = Path(src) / "backend"
    return b if b.is_dir() else Path(src)


def web_dist(src):
    d = backend_dir(src) / "internal" / "webui"
    return d / "dist" if d.is_dir() else Path(src) / "web" / "dist"


def state_dir(meta):
    return Path(meta["home"]) / STATE_DIR


def db_path(meta):
    return state_dir(meta) / "openbee.db"


def log_path(meta):
    return state_dir(meta) / "openbee.log"


def all_metas():
    out = []
    for p in sorted(ENVS.glob("*/meta.json")):
        try:
            out.append(json.loads(p.read_text()))
        except ValueError:
            continue
    return out


def load_meta(name):
    p = env_dir(name) / "meta.json"
    if not p.exists():
        die(f"env '{name}' does not exist")
    return json.loads(p.read_text())


def save_meta(meta):
    p = env_dir(meta["name"]) / "meta.json"
    p.write_text(json.dumps(meta, indent=2, ensure_ascii=False))
    p.chmod(0o600)


def instance_env(meta):
    env = {
        "HOME": meta["home"],
        "PATH": ":".join([str(node_bin()), "/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin", "/usr/sbin", "/sbin"]),
        "LANG": "en_US.UTF-8",
        "USER": os.environ.get("USER", ""),
        "LOGNAME": os.environ.get("LOGNAME", ""),
        "SHELL": "/bin/zsh",
        "TMPDIR": str(env_dir(meta["name"]) / "tmp"),
        "OPENBEE_LOG_LEVEL": "info",
    }
    env.update(meta.get("extra_env", {}))
    return env


def run(cmd, cwd=None, env=None, check=True, capture=False):
    r = subprocess.run([str(c) for c in cmd], cwd=cwd, env=env, text=True, capture_output=capture, stdin=subprocess.DEVNULL)
    if check and r.returncode != 0:
        if capture:
            sys.stderr.write(r.stdout + r.stderr)
        die(f"command failed ({r.returncode}): {' '.join(map(str, cmd))}")
    return r


def openbee(meta, *args, check=True, capture=False):
    return run([meta["bin"], *args], env=instance_env(meta), check=check, capture=capture)


def alive(pid):
    try:
        os.kill(pid, 0)
    except (ProcessLookupError, PermissionError):
        return False
    return True


def pid_of(meta):
    try:
        pid = int((state_dir(meta) / "openbee.pid").read_text().split()[0])
    except (OSError, ValueError, IndexError):
        return None
    return pid if alive(pid) else None


def port_open(port):
    with socket.socket() as s:
        s.settimeout(0.5)
        return s.connect_ex(("127.0.0.1", port)) == 0


def listener_pid(port):
    r = subprocess.run(["lsof", "-t", "-nP", f"-iTCP:{port}", "-sTCP:LISTEN"], text=True, capture_output=True)
    pids = [int(x) for x in r.stdout.split() if x.isdigit()]
    return pids[0] if pids else None


def http(meta, method, path, body=None, token=None, timeout=30):
    url = f"http://127.0.0.1:{meta['port']}{path}"
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return resp.status, decode_body(resp.read())
    except urllib.error.HTTPError as e:
        return e.code, decode_body(e.read())


def decode_body(raw):
    if not raw:
        return None
    try:
        return json.loads(raw)
    except ValueError:
        return raw.decode(errors="replace")


def login(meta):
    code, body = http(meta, "POST", "/api/auth/login", {"username": meta["admin_user"], "password": meta["admin_password"]})
    if code != 200:
        die(f"login failed: {code} {body}")
    return body["access_token"]


def yaml_dump(obj, indent=0):
    lines = []
    pad = "  " * indent
    for k, v in obj.items():
        if isinstance(v, dict):
            if v:
                lines.append(f"{pad}{k}:")
                lines.append(yaml_dump(v, indent + 1))
            else:
                lines.append(f"{pad}{k}: {{}}")
        elif isinstance(v, list):
            lines.append(f"{pad}{k}: {json.dumps(v, ensure_ascii=False)}")
        elif isinstance(v, bool):
            lines.append(f"{pad}{k}: {'true' if v else 'false'}")
        elif isinstance(v, (int, float)):
            lines.append(f"{pad}{k}: {v}")
        else:
            lines.append(f"{pad}{k}: {json.dumps(str(v), ensure_ascii=False)}")
    return "\n".join(lines)


def read_secret(name):
    p = E2E / name
    if not p.exists():
        die(f"missing secret file {p}")
    return p.read_text().strip()


def engine_bin(name):
    path = shutil.which(name)
    if not path:
        die(f"engine {name} not found on PATH")
    return path


def render_config(meta):
    cfg = {
        "language": "zh",
        "server": {
            "port": meta["port"],
            "host": "127.0.0.1",
            "debug": False,
            "auth": {"username": meta["admin_user"], "jwt_secret": meta["jwt_secret"]},
            "env_secret": meta["env_secret"],
        },
        "database": {"path": str(db_path(meta))},
        "bee": {
            "engine": {"default": "pi"},
            "engines": {
                "claude": {"enabled": False},
                "codex": {"enabled": False},
                "pi": {"enabled": True, "path": str(pi_bin()), "env": {"DEEPSEEK_API_KEY": read_secret("deepseek-key")}},
            },
            "rpc": {"token_secret": meta["rpc_secret"]},
        },
    }
    for name in meta.get("extra_engines", []):
        cfg["bee"]["engines"][name] = {"enabled": True, "path": engine_bin(name)}
    if meta.get("legacy_platforms"):
        cfg["bee"]["platforms"] = meta["legacy_platforms"]
    p = Path(meta["config"])
    p.write_text(yaml_dump(cfg) + "\n")
    p.chmod(0o600)


def git(src, *args):
    return run(["git", "-C", src, *args], capture=True, check=False).stdout.strip()


def build(meta, skip_web):
    src = Path(meta["src"])
    env = dict(os.environ)
    env["PATH"] = f"{node_bin()}:{env.get('PATH', '')}"
    if skip_web and (web_dist(src) / "index.html").exists():
        log("web build skipped, embedding the existing web build")
    else:
        log("building web (pnpm install --frozen-lockfile && pnpm build)")
        run(["pnpm", "install", "--frozen-lockfile"], cwd=src / "web", env=env, capture=True)
        run(["pnpm", "build"], cwd=src / "web", env=env, capture=True)
    log("building server")
    env["CGO_ENABLED"] = "0"
    run(["go", "build", "-o", meta["bin"], "./cmd/openbee"], cwd=backend_dir(src), env=env, capture=True)
    meta["commit"] = git(src, "rev-parse", "--short", "HEAD")
    branch = git(src, "rev-parse", "--abbrev-ref", "HEAD")
    if branch == "HEAD":
        branch = git(src, "name-rev", "--name-only", "--exclude=tags/*", "HEAD").removeprefix("remotes/") or "detached"
    meta["branch"] = branch
    meta["dirty"] = bool(git(src, "status", "--porcelain", "--untracked-files=no"))
    r = openbee(meta, "plugin", "--help", check=False, capture=True)
    meta["plugin_system"] = r.returncode == 0 and "install" in r.stdout


def install_skills(meta):
    src = backend_dir(meta["src"]) / "internal" / "infra" / "skillinstall" / "skills"
    if not src.is_dir():
        return
    for target in (Path(meta["home"]) / ".claude" / "skills", Path(meta["home"]) / ".agents" / "skills"):
        target.mkdir(parents=True, exist_ok=True)
        for skill in src.iterdir():
            if skill.is_dir():
                dest = target / skill.name
                if dest.exists():
                    shutil.rmtree(dest)
                shutil.copytree(skill, dest)


def write_pi_settings(meta):
    d = Path(meta["home"]) / ".pi" / "agent"
    d.mkdir(parents=True, exist_ok=True)
    (d / "settings.json").write_text(json.dumps({"defaultProvider": "deepseek", "defaultModel": meta["model"]}, indent=2))


def check_pi(meta):
    env = instance_env(meta)
    env["DEEPSEEK_API_KEY"] = read_secret("deepseek-key")
    try:
        r = subprocess.run([str(pi_bin()), "-p", "Reply with exactly: pong", "--no-session"], env=env, cwd=meta["home"], text=True, capture_output=True, timeout=120, stdin=subprocess.DEVNULL)
    except subprocess.TimeoutExpired:
        die("pi check timed out after 120s")
    if "pong" not in r.stdout.lower():
        die(f"pi check failed: {r.stdout.strip()} {r.stderr.strip()[-500:]}")
    log(f"pi + deepseek ({meta['model']}) ok")


def start(meta):
    if pid_of(meta):
        log(f"already running (pid {pid_of(meta)})")
        return
    if port_open(meta["port"]):
        die(f"port {meta['port']} is in use by pid {listener_pid(meta['port'])}")
    (env_dir(meta["name"]) / "tmp").mkdir(exist_ok=True)
    openbee(meta, "server", "-d", "-c", meta["config"], capture=True)
    deadline = time.time() + 60
    while time.time() < deadline:
        try:
            code, _ = http(meta, "GET", "/api/setup/status", timeout=2)
            if code == 200:
                log(f"server up at http://127.0.0.1:{meta['port']} (pid {pid_of(meta)})")
                return
        except (urllib.error.URLError, ConnectionError, socket.timeout):
            pass
        if time.time() > deadline - 55 and not pid_of(meta):
            break
        time.sleep(0.5)
    tail(meta, 40)
    die("server did not become healthy; log tail above")


def process_table():
    r = subprocess.run(["ps", "-axo", "pid=,ppid=,command="], text=True, capture_output=True)
    rows = []
    for line in r.stdout.splitlines():
        parts = line.split(None, 2)
        if len(parts) >= 2 and parts[0].isdigit() and parts[1].isdigit():
            rows.append((int(parts[0]), int(parts[1]), parts[2] if len(parts) > 2 else ""))
    return rows


def descendants(pid, table):
    kids = {}
    for p, pp, _ in table:
        kids.setdefault(pp, []).append(p)
    out, stack = [], [pid]
    while stack:
        for c in kids.get(stack.pop(), []):
            out.append(c)
            stack.append(c)
    return out


def ancestors(table):
    parent = {p: pp for p, pp, _ in table}
    out, cur = set(), os.getpid()
    while cur and cur not in out:
        out.add(cur)
        cur = parent.get(cur)
    return out


def env_processes(meta, table):
    marker = str(env_dir(meta["name"]))
    found = {p for p, _, cmd in table if marker in cmd}
    r = subprocess.run(["lsof", "-a", "-d", "cwd", "-Fpn"], text=True, capture_output=True)
    pid = None
    for line in r.stdout.splitlines():
        if line.startswith("p"):
            pid = int(line[1:])
        elif line.startswith("n") and pid and (line[1:] == marker or line[1:].startswith(marker + "/")):
            found.add(pid)
    return found - ancestors(table)


def kill_all(pids):
    pids = [p for p in pids if alive(p)]
    for p in pids:
        try:
            os.kill(p, signal.SIGTERM)
        except ProcessLookupError:
            pass
    deadline = time.time() + 5
    while time.time() < deadline and any(alive(p) for p in pids):
        time.sleep(0.2)
    for p in pids:
        if alive(p):
            try:
                os.kill(p, signal.SIGKILL)
            except ProcessLookupError:
                pass


def stop(meta):
    table = process_table()
    pid = pid_of(meta)
    tree = descendants(pid, table) if pid else []
    if pid:
        openbee(meta, "stop", check=False, capture=True)
    kill_all(tree + list(env_processes(meta, process_table())))
    for _ in range(30):
        if not port_open(meta["port"]):
            break
        time.sleep(0.5)
    if port_open(meta["port"]):
        die(f"port {meta['port']} still in use by pid {listener_pid(meta['port'])} after stop")
    left = env_processes(meta, process_table())
    if left:
        die(f"processes still running inside the env: {sorted(left)}")


def tail(meta, n):
    p = log_path(meta)
    if p.exists():
        print("\n".join(p.read_text(errors="replace").splitlines()[-n:]))


def pick_port():
    used = {m["port"] for m in all_metas()}
    for port in PORT_RANGE:
        if port not in used and not port_open(port):
            return port
    die(f"no free port in {PORT_RANGE.start}-{PORT_RANGE.stop - 1}")


def channel_values(ch, overrides):
    spec = CHANNELS[ch]
    values = dict(spec["defaults"])
    if spec["keys"]:
        values.update(json.loads(read_secret(f"{ch}.json")))
    values.update(overrides)
    bot_name = values.pop("bot_name", None)
    missing = [k for k in spec["keys"] if values.get(k) in (None, "", [])]
    if missing:
        die(f"{ch}: missing {', '.join(missing)} in {E2E / (ch + '.json')} (or pass --set)")
    if ch == "linear" and values.get("label_name") == "openbee":
        die("label 'openbee' is what a real openbee instance watches; use a temporary label")
    return {k: ",".join(map(str, v)) if isinstance(v, list) else v for k, v in values.items()}, bot_name


def channel_owner(ch, name):
    for m in all_metas():
        if m["name"] != name and ch in m.get("channels", []) and pid_of(m):
            return m["name"]
    return None


def add_channel(meta, ch, src, spec, overrides):
    if ch not in CHANNELS:
        die(f"unknown channel {ch}; known: {', '.join(CHANNELS)}")
    owner = channel_owner(ch, meta["name"])
    if owner:
        die(f"{ch} is already connected in running env '{owner}'; one bot cannot serve two instances")
    values, bot_name = channel_values(ch, overrides)
    if meta.get("plugin_system"):
        if src:
            p = Path(src).expanduser().resolve()
            if not (p / "openbee.plugin.json").exists():
                die(f"{p} has no openbee.plugin.json")
            if not (p / "dist" / "index.js").exists():
                die(f"{p}/dist/index.js is missing; build the plugin first")
            target = str(p)
        else:
            target = spec or f"npm:{CHANNELS[ch]['package']}"
        log(f"installing plugin {ch} from {target}")
        openbee(meta, "plugin", "install", target, "-c", meta["config"])
        if ch != "weixin":
            args = ["plugin", "configure", ch, "-c", meta["config"], "--enable"]
            for k, v in values.items():
                args += ["--set", f"{k}={v}"]
            if bot_name:
                args += ["--bot-name", bot_name]
            openbee(meta, *args, capture=True)
    else:
        if src or spec:
            die("this build has no plugin system; channels are built in")
        if ch == "weixin":
            die("weixin on a pre-plugin build needs the openbee config wizard; not supported by this script")
        block = {"enabled": True, **values}
        if bot_name:
            block["bot_name"] = bot_name
        for k in ("projects", "states"):
            if isinstance(block.get(k), str):
                block[k] = [x for x in block[k].split(",") if x]
        meta.setdefault("legacy_platforms", {})[ch] = block
        render_config(meta)
        log(f"built-in channel {ch} written to config, restarting")
        stop(meta)
        start(meta)
    if ch not in meta["channels"]:
        meta["channels"].append(ch)
    save_meta(meta)
    if ch == "weixin":
        log("weixin installed; run 'weixin-login' once the user is ready to scan")
    else:
        log(f"channel {ch} connected")


def cmd_up(a):
    name = a.name
    if (env_dir(name) / "meta.json").exists():
        m = load_meta(name)
        state = f"running on port {m['port']}" if pid_of(m) else "stopped"
        die(f"env '{name}' already exists ({state}); use restart, or down first")
    port = a.port or pick_port()
    if port_open(port):
        die(f"port {port} is in use by pid {listener_pid(port)}")
    src = Path(a.src or git(Path.cwd(), "rev-parse", "--show-toplevel") or Path.cwd()).expanduser().resolve()
    if not (backend_dir(src) / "cmd" / "openbee").is_dir():
        die(f"{src} is not an openbee checkout")
    read_secret("deepseek-key")
    d = env_dir(name)
    if d.exists():
        die(f"{d} exists without meta.json; remove it by hand after checking nothing runs from it")
    (d / "home" / STATE_DIR).mkdir(parents=True)
    (d / "tmp").mkdir()
    d.chmod(0o700)
    extra_env = {}
    for kv in a.env or []:
        k, sep, v = kv.partition("=")
        if not sep:
            die(f"bad --env {kv}")
        extra_env[k] = v
    meta = {
        "name": name,
        "port": port,
        "src": str(src),
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "home": str(d / "home"),
        "bin": str(d / "bin" / "openbee"),
        "config": str(d / "config.yaml"),
        "model": a.model,
        "admin_user": "admin",
        "admin_password": secrets.token_urlsafe(12),
        "jwt_secret": secrets.token_hex(32),
        "env_secret": secrets.token_hex(32),
        "rpc_secret": secrets.token_hex(32),
        "extra_env": extra_env,
        "extra_engines": a.engine or [],
        "channels": [],
        "legacy_platforms": {},
        "worker_id": None,
    }
    save_meta(meta)
    try:
        build(meta, a.skip_web)
        write_pi_settings(meta)
        install_skills(meta)
        render_config(meta)
        save_meta(meta)
        if not a.skip_pi_check:
            check_pi(meta)
        start(meta)
        code, body = http(meta, "POST", "/api/setup", {"username": meta["admin_user"], "password": meta["admin_password"], "display_name": "E2E Admin"})
        if code != 200:
            die(f"/api/setup failed: {code} {body}")
        if a.worker:
            code, w = http(meta, "POST", "/api/workers", {"name": a.worker, "engine": "pi", "description": "E2E test worker"}, token=body["access_token"])
            if code not in (200, 201):
                die(f"create worker failed: {code} {w}")
            meta["worker_id"] = w.get("id")
        save_meta(meta)
        for ch in a.channel or []:
            add_channel(meta, ch, None, None, {})
    except SystemExit:
        print(f"up failed; inspect with 'status -n {name}' / 'logs -n {name}', clean with 'down -n {name}'", file=sys.stderr)
        raise
    print_status(meta)


def cmd_channel(a):
    meta = load_meta(a.name)
    if not pid_of(meta):
        die(f"env '{a.name}' is not running")
    overrides = {}
    for s in a.set or []:
        k, sep, v = s.partition("=")
        if not sep:
            die(f"bad --set {s}")
        overrides[k] = v
    add_channel(meta, a.channel, a.src, a.spec, overrides)


def qr_png(url, path):
    if not (TOOLS / "node_modules" / "qrcode").exists():
        TOOLS.mkdir(parents=True, exist_ok=True)
        env = dict(os.environ, PATH=f"{node_bin()}:{os.environ.get('PATH', '')}")
        run(["npm", "install", "--prefix", TOOLS, "--no-save", "--silent", "qrcode"], env=env, capture=True)
    script = "require(process.argv[1]).toFile(process.argv[2], process.argv[3], {width: 480, margin: 2}, e => { if (e) { console.error(e); process.exit(1) } })"
    run([node_bin() / "node", "-e", script, TOOLS / "node_modules" / "qrcode", path, url], capture=True)


def cmd_weixin_login(a):
    meta = load_meta(a.name)
    if "weixin" not in meta.get("channels", []):
        add_channel(meta, "weixin", a.src, a.spec, {})
        meta = load_meta(a.name)
    token = login(meta)
    code, body = http(meta, "POST", "/api/plugins/weixin/setup", token=token)
    if code not in (200, 201):
        die(f"start setup failed: {code} {body}")
    sid = body["id"]
    meta["weixin_setup"] = sid
    save_meta(meta)
    log(f"setup session {sid}")
    req = urllib.request.Request(f"http://127.0.0.1:{meta['port']}/api/plugins/setup/{sid}/events")
    req.add_header("Authorization", f"Bearer {token}")
    n = 0
    with urllib.request.urlopen(req, timeout=600) as resp:
        for raw in resp:
            line = raw.decode(errors="replace").rstrip("\n")
            if not line.startswith("data: "):
                continue
            ev = json.loads(line[6:])
            if ev.get("type") == "done":
                if ev.get("ok"):
                    openbee(meta, "plugin", "enable", "weixin", "-c", meta["config"], check=False, capture=True)
                    log("weixin login ok, plugin enabled")
                    return
                die(f"weixin login failed: {ev.get('error')}")
            text = ev.get("text", "")
            if re.match(r"^https?://\S+$", text.strip()):
                n += 1
                png = env_dir(meta["name"]) / f"weixin-qr-{n}.png"
                qr_png(text.strip(), png)
                print(f"QR_PNG {png}", flush=True)
            elif text.strip() and not re.search(r"[▀▄█]", text):
                print(f"setup: {text}", flush=True)


def cmd_weixin_input(a):
    meta = load_meta(a.name)
    sid = meta.get("weixin_setup") or die("no weixin setup session")
    code, body = http(meta, "POST", f"/api/plugins/setup/{sid}/input", {"text": a.text}, token=login(meta))
    if code != 204:
        die(f"input failed: {code} {body}")
    log("sent")


def print_status(meta):
    pid = pid_of(meta)
    print(json.dumps({
        "name": meta["name"],
        "state": f"running (pid {pid})" if pid else "stopped",
        "web": f"http://127.0.0.1:{meta['port']}",
        "admin": f"{meta['admin_user']} / {meta['admin_password']}",
        "src": meta["src"],
        "build": f"{meta.get('branch')}@{meta.get('commit')}" + (" +uncommitted" if meta.get("dirty") else ""),
        "plugin_system": meta.get("plugin_system"),
        "channels": meta.get("channels", []),
        "worker_id": meta.get("worker_id"),
        "config": meta["config"],
        "db": str(db_path(meta)),
        "log": str(log_path(meta)),
    }, indent=2, ensure_ascii=False))


def cmd_status(a):
    if a.name:
        print_status(load_meta(a.name))
        return
    metas = all_metas()
    managed = set()
    for m in metas:
        pid = pid_of(m)
        if pid:
            managed.add(pid)
        print(f"{m['name']:<12} {'running' if pid else 'stopped':<8} :{m['port']:<6} {m.get('branch')}@{m.get('commit')} channels={','.join(m.get('channels', [])) or '-'}")
    if not metas:
        print("no test envs")
    for pid, _, cmd in process_table():
        if re.search(r"openbee\S*\s+server\b", cmd) and pid not in managed and pid != os.getpid():
            print(f"OTHER openbee server pid {pid}: {cmd}")


def busy(meta):
    if not db_path(meta).exists():
        return []
    conn = db_connect(meta)
    out = []
    try:
        n = conn.execute("SELECT count(*) FROM bee_platform_messages WHERE status IN ('received', 'feeding')").fetchone()[0]
        if n:
            out.append(f"{n} inbound message(s) still being handled")
        n = conn.execute("SELECT count(*) FROM bee_tasks WHERE status = 'running'").fetchone()[0]
        if n:
            out.append(f"{n} task(s) running")
    except sqlite3.Error:
        pass
    conn.close()
    return out


def cmd_restart(a):
    meta = load_meta(a.name)
    if a.src:
        if not a.rebuild:
            die("--src needs --rebuild")
        src = Path(a.src).expanduser().resolve()
        if not (backend_dir(src) / "cmd" / "openbee").is_dir():
            die(f"{src} is not an openbee checkout")
    work = busy(meta) if pid_of(meta) else []
    if work and not a.force:
        die("; ".join(work) + " - restarting now breaks the reply the user is waiting for; retry later or pass --force")
    stop(meta)
    if a.rebuild:
        if a.src:
            meta["src"] = str(src)
        build(meta, a.skip_web)
        install_skills(meta)
        save_meta(meta)
    if a.engine is not None:
        meta["extra_engines"] = [e for e in a.engine if e]
        render_config(meta)
        save_meta(meta)
    start(meta)
    print_status(meta)


def cmd_ctl(a):
    meta = load_meta(a.name)
    args = a.args[1:] if a.args[:1] == ["--"] else a.args
    os.execve(meta["bin"], [meta["bin"], "ctl", "-c", meta["config"], *args], instance_env(meta))


def cmd_api(a):
    meta = load_meta(a.name)
    body = json.loads(a.data) if a.data else None
    code, out = http(meta, a.method.upper(), a.path, body, token=login(meta))
    print(code)
    print(json.dumps(out, indent=2, ensure_ascii=False) if not isinstance(out, str) else out)


def db_connect(meta):
    p = db_path(meta)
    if not p.exists():
        die(f"{p} does not exist")
    wal = Path(f"{p}-wal").exists()
    conn = sqlite3.connect(f"file:{p}?mode=ro" + ("" if wal else "&immutable=1"), uri=True, timeout=5)
    return conn


def cmd_db(a):
    meta = load_meta(a.name)
    cur = db_connect(meta).execute(a.sql)
    cols = [c[0] for c in cur.description or []]
    if cols:
        print("\t".join(cols))
    for row in cur.fetchall():
        print("\t".join("" if v is None else str(v).replace("\n", " ") for v in row))


def cmd_watch(a):
    meta = load_meta(a.name)
    seen = {}
    first = True
    errors_at = log_path(meta).stat().st_size if log_path(meta).exists() else 0
    while True:
        try:
            conn = db_connect(meta)
            for table, order, cols in WATCH_TABLES:
                try:
                    rows = conn.execute(f"SELECT {cols} FROM {table} ORDER BY {order} DESC LIMIT 50").fetchall()
                except sqlite3.Error:
                    continue
                for row in reversed(rows):
                    key, state = (table, row[0]), row[1:3]
                    if seen.get(key) != state:
                        if not first:
                            vals = " | ".join("" if v is None else str(v).replace("\n", " ") for v in row)
                            print(f"{datetime.now():%H:%M:%S} {table.removeprefix('bee_')} {vals}", flush=True)
                        seen[key] = state
            conn.close()
        except sqlite3.Error as e:
            print(f"{datetime.now():%H:%M:%S} db error: {e}", flush=True)
        lp = log_path(meta)
        if lp.exists():
            with lp.open(errors="replace") as f:
                f.seek(errors_at)
                for line in f:
                    if '"level":"error"' in line or '"level":"warn"' in line:
                        try:
                            j = json.loads(line)
                            line = f"{j.get('level')} {j.get('msg')} {j.get('error', '')}"
                        except ValueError:
                            pass
                        print(f"{datetime.now():%H:%M:%S} log {line.strip()[:300]}", flush=True)
                errors_at = f.tell()
        first = False
        if not pid_of(meta):
            print(f"{datetime.now():%H:%M:%S} server stopped", flush=True)
            return
        time.sleep(a.interval)


def cmd_logs(a):
    meta = load_meta(a.name)
    if a.follow:
        os.execvp("tail", ["tail", "-n", str(a.lines), "-F", str(log_path(meta))])
    tail(meta, a.lines)


def cmd_down(a):
    names = [m["name"] for m in all_metas()] if a.all else [a.name]
    if not names:
        print("no test envs")
        return
    for name in names:
        meta = load_meta(name)
        stop(meta)
        if "linear" in meta.get("channels", []):
            log(f"linear was connected: delete the temporary issues and the '{LINEAR_LABEL}' label")
        if "weixin" in meta.get("channels", []):
            log("weixin was installed: its login token is deleted with the env")
        if a.keep:
            log(f"env '{name}' stopped, files kept at {env_dir(name)}")
            continue
        shutil.rmtree(env_dir(name))
        log(f"env '{name}' stopped and removed")


def main():
    ap = argparse.ArgumentParser(prog="testenv.py")
    sub = ap.add_subparsers(dest="cmd", required=True)

    def cmd(name, fn, default=DEFAULT_NAME):
        p = sub.add_parser(name)
        p.add_argument("--name", "-n", default=default)
        p.set_defaults(fn=fn)
        return p

    p = cmd("up", cmd_up)
    p.add_argument("--src")
    p.add_argument("--port", type=int)
    p.add_argument("--skip-web", action="store_true")
    p.add_argument("--skip-pi-check", action="store_true")
    p.add_argument("--model", default="deepseek-flash")
    p.add_argument("--worker", default="tester")
    p.add_argument("--channel", action="append")
    p.add_argument("--env", action="append")
    p.add_argument("--engine", action="append", choices=["claude", "codex"])

    p = cmd("channel", cmd_channel)
    p.add_argument("channel")
    p.add_argument("--src")
    p.add_argument("--spec")
    p.add_argument("--set", action="append")

    p = cmd("weixin-login", cmd_weixin_login)
    p.add_argument("--src")
    p.add_argument("--spec")

    p = cmd("weixin-input", cmd_weixin_input)
    p.add_argument("text")

    cmd("status", cmd_status, default=None)

    p = cmd("restart", cmd_restart)
    p.add_argument("--rebuild", action="store_true")
    p.add_argument("--src")
    p.add_argument("--skip-web", action="store_true")
    p.add_argument("--force", action="store_true")
    p.add_argument("--engine", action="append", choices=["claude", "codex", ""])

    p = cmd("ctl", cmd_ctl)
    p.add_argument("args", nargs=argparse.REMAINDER)

    p = cmd("api", cmd_api)
    p.add_argument("method")
    p.add_argument("path")
    p.add_argument("--data")

    p = cmd("db", cmd_db)
    p.add_argument("sql")

    p = cmd("watch", cmd_watch)
    p.add_argument("--interval", type=float, default=5)

    p = cmd("logs", cmd_logs)
    p.add_argument("--lines", type=int, default=80)
    p.add_argument("--follow", "-f", action="store_true")

    p = cmd("down", cmd_down)
    p.add_argument("--all", action="store_true")
    p.add_argument("--keep", action="store_true")

    a = ap.parse_args()
    ENVS.mkdir(parents=True, exist_ok=True)
    a.fn(a)


if __name__ == "__main__":
    main()
