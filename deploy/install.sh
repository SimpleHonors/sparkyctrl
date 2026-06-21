#!/usr/bin/env bash
#
# sparkyctrl installer (Linux). Installs /usr/local/bin/sparkyctrl + a systemd unit.
#
# Run it and it ASKS where to fence file operations — you don't pre-bake a path.
#   curl -fsSL <raw>/deploy/install.sh | sudo bash -s -- --start
#
# Flags (parallel to the Windows install.ps1 where the platform allows):
#   --fence DIR     confine file operations to DIR
#   --no-fence      full filesystem access (no fence)
#   --mode MODE     admin (default; root, exec/shell unfenced) | hardened (non-root, caps
#                   dropped, read-only FS except fence+audit; Linux-only)
#   --container     hardened mode inside an unprivileged LXC (drop mount-namespace isolation)
#   --addr H:P      listen address (default 0.0.0.0:7766)
#   --audit FILE    audit log path
#   --token T       explicit worker token (otherwise generate or reuse the token file)
#   --no-auth       disable token authentication entirely
#   --binary PATH   install this local binary instead of downloading the release
#   --version V     release version to download (default: latest)
#   --start         start the worker now
#   --no-enable     do not enable at boot
#   --uninstall     stop + remove the service (leaves the binary and audit log)
#
# If you pass neither --fence nor --no-fence, the installer prompts (even via curl | bash).
#
set -euo pipefail

# Save original arguments before shift consumes them — needed for the detach
# path that re-spawns the script with the same flags.
ORIG_ARGS=("$@")

MODE=admin
ADDR="0.0.0.0:7766"
FENCE=""
NO_FENCE=0
AUDIT="/var/log/sparkyctrl-audit.log"
TOKEN=""
NO_AUTH=0
BINARY=""
VERSION="${SPARKYCTRL_VERSION:-latest}"
REPO="${SPARKYCTRL_REPO:-SimpleHonors/sparkyctrl}"
SVC_USER="sparkyctrl"
UNIT="/etc/systemd/system/sparkyctrl.service"
DO_ENABLE=1
DO_START=0
CONTAINER=0
UNINSTALL=0
WAS_ACTIVE=0

die() { echo "install.sh: $*" >&2; exit 1; }
usage() { awk 'NR>2 && /^#/ {sub(/^# ?/,""); print; next} NR>2 {exit}' "$0"; }
have_command() { command -v "$1" >/dev/null 2>&1; }
gen_token() {
  if have_command openssl; then
    openssl rand -hex 32
  elif have_command od; then
    od -An -N32 -tx1 /dev/urandom | tr -d ' \n'
  else
    die "need openssl or od to generate a worker token"
  fi
}

download_url() {
  local url="$1"
  local dest="$2"

  if have_command curl; then
    curl -fsSL -H "Cache-Control: no-cache" -H "Pragma: no-cache" "$url" -o "$dest" \
      || die "download failed: ${url}"
    return
  fi

  if have_command wget; then
    wget -q --header="Cache-Control: no-cache" --header="Pragma: no-cache" \
      --output-document="$dest" "$url" || die "download failed: ${url}"
    return
  fi

  die "need curl or wget to download release binaries; install one and retry"
}

prompt_for_fence() {
  local need="$1"
  local ans=""

  if ! { : > /dev/tty; } 2>/dev/null; then
    if [ -n "$need" ]; then
      die "$need; pass --fence <dir> explicitly when running non-interactively"
    fi
    die "no terminal to prompt for fence choice: pass --fence <dir> or --no-fence explicitly"
  fi

  while :; do
    {
      echo "Confine the worker's file operations to a directory (the \"fence\")?"
      echo "  - enter an ABSOLUTE path to confine to it (recommended)"
      [ -z "$need" ] && echo "  - type 'none' for FULL filesystem access (dangerous)"
      printf 'fence path%s: ' "$([ -z "$need" ] && echo ' [or none]')"
    } > /dev/tty
    read -r ans < /dev/tty || die "no fence chosen"
    case "$ans" in
      none|NONE) [ -z "$need" ] && { NO_FENCE=1; break; } || echo "hardened mode needs a path." > /dev/tty ;;
      /*) FENCE="$ans"; break ;;
      "") echo "Please enter an absolute path${need:+ }." > /dev/tty ;;
      *)  echo "Enter an ABSOLUTE path (starting with /)${need:+, no 'none' in hardened mode}." > /dev/tty ;;
    esac
  done
}

while [ $# -gt 0 ]; do
  case "$1" in
    --mode)      MODE="${2:-}"; shift 2 ;;
    --addr)      ADDR="${2:-}"; shift 2 ;;
    --fence)     FENCE="${2:-}"; shift 2 ;;
    --no-fence)  NO_FENCE=1; shift ;;
    --audit)     AUDIT="${2:-}"; shift 2 ;;
    --token)     TOKEN="${2:-}"; shift 2 ;;
    --no-auth)   NO_AUTH=1; shift ;;
    --binary)    BINARY="${2:-}"; shift 2 ;;
    --version)   VERSION="${2:-}"; shift 2 ;;
    --repo)      REPO="${2:-}"; shift 2 ;;
    --user)      SVC_USER="${2:-}"; shift 2 ;;
    --start)     DO_START=1; shift ;;
    --no-enable) DO_ENABLE=0; shift ;;
    --container) CONTAINER=1; shift ;;
    --uninstall) UNINSTALL=1; shift ;;
    -h|--help)   usage; exit 0 ;;
    *) die "unknown argument: $1 (try --help)" ;;
  esac
done

[ "$(id -u)" -eq 0 ] || die "must run as root (writes /usr/local/bin and /etc/systemd/system)"

if [ "$UNINSTALL" -eq 1 ]; then
  systemctl stop sparkyctrl 2>/dev/null || true
  systemctl disable sparkyctrl 2>/dev/null || true
  rm -f "$UNIT"
  systemctl daemon-reload 2>/dev/null || true
  echo "==> removed the sparkyctrl service (left /usr/local/bin/sparkyctrl and the audit log in place)"
  exit 0
fi

case "$MODE" in admin|hardened) ;; *) die "--mode must be 'admin' or 'hardened'" ;; esac

# The fence is a deliberate choice, never a silent default. Confine file operations to a
# directory, or grant full filesystem access. Hardened mode always needs a fence (it is the
# only writable area). Otherwise: use the flags if given, else ASK — reading the terminal
# directly so this works even when piped (curl | bash). Only error when there is no terminal.
if [ -z "$FENCE" ] && [ "$NO_FENCE" -ne 1 ]; then
  if [ "$MODE" = hardened ]; then
    NEED="hardened mode requires a fence (the only writable area)"
  else
    NEED=""
  fi
  prompt_for_fence "$NEED"
fi

# Resolve the binary. Pass --binary to use a local file; otherwise download the
# release. We deliberately never auto-pick an already-installed binary — that
# silently reuses the old version, defeating upgrades. Always fetch fresh.
if [ -z "$BINARY" ]; then
  case "$(uname -m)" in
    x86_64|amd64)  ARCH=amd64 ;;
    aarch64|arm64) ARCH=arm64 ;;
    *) die "unsupported architecture: $(uname -m) — build locally and pass --binary <path>" ;;
  esac
  if [ "$VERSION" = latest ]; then
    URL="https://github.com/${REPO}/releases/latest/download/sparkyctrl-linux-${ARCH}"
  else
    URL="https://github.com/${REPO}/releases/download/${VERSION}/sparkyctrl-linux-${ARCH}"
  fi
  TMP="$(mktemp)"
  echo "==> downloading ${URL}"
  # Bypass intermediate caches so re-runs always fetch the current release.
  download_url "$URL" "$TMP"
  chmod +x "$TMP"
  BINARY="$TMP"
  # Verify the binary is executable and print its version.
  if version=$("$BINARY" --version 2>/dev/null); then
    echo "==> downloaded sparkyctrl version: ${version}"
  else
    die "downloaded binary does not appear to be executable"
  fi
fi
[ -n "$BINARY" ] && [ -x "$BINARY" ] || die "no binary found — build with deploy/build.sh, or pass --binary <path>"

DEST="/usr/local/bin/sparkyctrl"
if [ "$(readlink -f "$BINARY")" = "$(readlink -f "$DEST" 2>/dev/null || echo "$DEST")" ]; then
  echo "==> binary already in place: $DEST (skipping copy)"
else
  # Stop a running worker before overwriting its binary: Linux refuses to overwrite a
  # running executable (ETXTBSY), Windows locks it. Remember it was running so we restart it.
  # If this script is itself running as a child of sparkyctrl (e.g. `sparkyctrl shell
  # "curl ... | bash"`), stopping the service kills our parent and takes us with it
  # mid-install. Detect that case and re-spawn ourselves detached before the stop.
  # Walk up to two levels: when sparkyctrl runs a shell, the process tree is
  #   sparkyctrl serve → sh -c "..." → bash install.sh
  # so $PPID may be the intermediate shell, not sparkyctrl itself.
  if systemctl is-active --quiet sparkyctrl 2>/dev/null; then
    WAS_ACTIVE=1
    DETACH=0
    if [ -r "/proc/$PPID/cmdline" ] && tr '\0' ' ' < "/proc/$PPID/cmdline" 2>/dev/null | grep -q 'sparkyctrl serve'; then
      DETACH=1
    elif [ -r "/proc/$PPID/cmdline" ]; then
      GPID=$(awk '{print $4}' "/proc/$PPID/stat" 2>/dev/null)
      if [ -n "$GPID" ] && [ -r "/proc/$GPID/cmdline" ] && tr '\0' ' ' < "/proc/$GPID/cmdline" 2>/dev/null | grep -q 'sparkyctrl serve'; then
        DETACH=1
      fi
    fi
    if [ "$DETACH" -eq 1 ]; then
      UPGRADE_SCRIPT="/tmp/sparkyctrl-upgrade-$$.sh"
      download_url "https://raw.githubusercontent.com/${REPO}/master/deploy/install.sh" "$UPGRADE_SCRIPT"
      if [ -s "$UPGRADE_SCRIPT" ]; then
        chmod +x "$UPGRADE_SCRIPT"
        # Run detached in a new systemd scope to escape the sparkyctrl service
        # cgroup — otherwise systemctl stop kills us along with the worker.
        systemd-run --quiet --no-block --scope --collect \
          bash "$UPGRADE_SCRIPT" "${ORIG_ARGS[@]}" \
          > /tmp/sparkyctrl-upgrade.log 2>&1
        echo "==> running under sparkyctrl — install detached to background"
        echo "==> sparkyctrl will restart in a few seconds (check /tmp/sparkyctrl-upgrade.log)"
        exit 0
      fi
      # Fall through if download fails — best-effort inline upgrade.
    fi
    systemctl stop sparkyctrl 2>/dev/null || true
    echo "==> stopped running worker to replace its binary"
  fi
  install -m 0755 "$BINARY" "$DEST"
  echo "==> installed binary: $BINARY -> $DEST"
fi

# Resolve the worker auth material. The token file is the source of truth when auth is enabled.
TOKEN_PATH="/etc/sparkyctrl/token"
TOKEN_DIR="$(dirname "$TOKEN_PATH")"
TOKEN_OWNER="root:root"
TOKEN_MODE="0600"
if [ "$MODE" = hardened ]; then
  TOKEN_OWNER="${SVC_USER}:${SVC_USER}"
fi

if [ "$NO_AUTH" -eq 0 ]; then
  if [ -n "$TOKEN" ]; then
    WORKER_TOKEN="$TOKEN"
    TOKEN_SOURCE="override"
  elif [ -s "$TOKEN_PATH" ]; then
    WORKER_TOKEN="$(tr -d '\r\n' < "$TOKEN_PATH")"
    TOKEN_SOURCE="existing"
  else
    WORKER_TOKEN="$(gen_token)"
    TOKEN_SOURCE="generated"
  fi
  [ -n "$WORKER_TOKEN" ] || die "worker token was empty"
  mkdir -p "$TOKEN_DIR"
  printf '%s' "$WORKER_TOKEN" > "$TOKEN_PATH"
  chmod "$TOKEN_MODE" "$TOKEN_PATH"
  echo "==> auth token ($TOKEN_SOURCE): written to $TOKEN_PATH"
  echo "==> set SPARKYCTRL_TOKEN from $TOKEN_PATH for client use (do NOT pass on the command line)"
fi

# Assemble the serve arguments.
SERVE_ARGS="--addr ${ADDR} --audit ${AUDIT}"
[ -n "$FENCE" ] && SERVE_ARGS="${SERVE_ARGS} --fence ${FENCE}"
[ "$NO_AUTH" -eq 1 ] && SERVE_ARGS="${SERVE_ARGS} --no-auth"

[ -n "$FENCE" ] && mkdir -p "$FENCE"
touch "$AUDIT"

if [ "$MODE" = admin ]; then
  chown root:root "$AUDIT" 2>/dev/null || true
  cat > "$UNIT" <<EOF
[Unit]
Description=sparkyctrl worker (admin mode — root; exec/shell unfenced)
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/sparkyctrl serve ${SERVE_ARGS}
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF
else
  if ! id -u "$SVC_USER" >/dev/null 2>&1; then
    useradd --system --no-create-home --shell /usr/sbin/nologin "$SVC_USER"
    echo "==> created system user: $SVC_USER"
  fi
  chown -R "$SVC_USER":"$SVC_USER" "$FENCE" "$AUDIT"

  # Mount-namespace isolation can't be set up in an unprivileged LXC (the worker dies with
  # 226/NAMESPACE). --container omits it while keeping the user/capability/seccomp hardening.
  FS_ISOLATION=""
  if [ "$CONTAINER" -eq 0 ]; then
    FS_ISOLATION="# Whole filesystem read-only except the fence + audit log.
ProtectSystem=strict
ReadWritePaths=${FENCE} ${AUDIT}
ProtectHome=yes
PrivateTmp=yes
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectControlGroups=yes
"
  fi

  cat > "$UNIT" <<EOF
[Unit]
Description=sparkyctrl worker (hardened file-only mode — non-root, caps dropped)
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/sparkyctrl serve ${SERVE_ARGS}
Restart=on-failure
RestartSec=3

# Identity: run as a dedicated unprivileged user, never root.
User=${SVC_USER}
Group=${SVC_USER}

# Hand the process zero Linux capabilities; never gain privileges via execve().
CapabilityBoundingSet=
AmbientCapabilities=
NoNewPrivileges=yes

${FS_ISOLATION}# Network: only the families the listener needs.
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
# No W+X memory pages (safe for a stdlib Go binary; no JIT).
MemoryDenyWriteExecute=yes
RestrictSUIDSGID=yes
LockPersonality=yes
RestrictRealtime=yes
SystemCallArchitectures=native

[Install]
WantedBy=multi-user.target
EOF
fi

if [ "$NO_AUTH" -eq 0 ]; then
  chown "$TOKEN_OWNER" "$TOKEN_DIR" "$TOKEN_PATH" 2>/dev/null || true
  chmod 0700 "$TOKEN_DIR" 2>/dev/null || true
fi

if [ -n "$FENCE" ]; then FENCE_DESC="fence=$FENCE"; else FENCE_DESC="no fence (FULL access)"; fi
echo "==> wrote unit: $UNIT (mode=$MODE, $FENCE_DESC)"
systemctl daemon-reload
if [ "$DO_ENABLE" -eq 1 ]; then systemctl enable sparkyctrl >/dev/null 2>&1 && echo "==> enabled at boot"; fi
if [ "$DO_START" -eq 1 ] || [ "$WAS_ACTIVE" -eq 1 ]; then systemctl restart sparkyctrl && echo "==> started: $(systemctl is-active sparkyctrl)"; fi
echo "==> done (mode=$MODE)."
