#!/usr/bin/env bash
#
# sparkyctrl installer — choose the worker's privilege mode at install time.
#
#   --mode admin     (default) runs the worker as root; exec/shell are unfenced.
#                    For trusted sysadmin use on a trusted LAN ("sharp tool").
#   --mode hardened  runs the worker as an unprivileged system user with zero Linux
#                    capabilities and a read-only filesystem except the fence + audit
#                    log. File-serving only — exec/shell no longer run as root.
#
# Installs the binary to /usr/local/bin/sparkyctrl and writes the systemd unit to
# /etc/systemd/system/sparkyctrl.service. Re-run any time to switch modes.
#
# Usage:
#   sudo ./deploy/install.sh --mode admin   (--fence DIR | --no-fence) [--addr H:P] [--audit FILE] [--token T] [--start]
#   sudo ./deploy/install.sh --mode hardened --fence DIR [--addr H:P] [--audit FILE] [--token T] [--container] [--start]
#
# Admin mode requires an explicit fence choice: --fence DIR confines file ops to DIR;
# --no-fence grants FULL filesystem access. Run interactively without either and it prompts.
#
# --container omits the mount-namespace filesystem isolation (ProtectSystem etc.)
# that an unprivileged LXC cannot set up; keeps the non-root / no-caps hardening.
#
set -euo pipefail

MODE=admin
ADDR="0.0.0.0:7766"
FENCE=""
AUDIT="/var/log/sparkyctrl-audit.log"
TOKEN=""
BINARY=""
SVC_USER="sparkyctrl"
UNIT="/etc/systemd/system/sparkyctrl.service"
DO_ENABLE=1
DO_START=0
CONTAINER=0
NO_FENCE=0

usage() { sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'; }

die() { echo "install.sh: $*" >&2; exit 1; }

while [ $# -gt 0 ]; do
  case "$1" in
    --mode)     MODE="${2:-}"; shift 2 ;;
    --addr)     ADDR="${2:-}"; shift 2 ;;
    --fence)    FENCE="${2:-}"; shift 2 ;;
    --audit)    AUDIT="${2:-}"; shift 2 ;;
    --token)    TOKEN="${2:-}"; shift 2 ;;
    --binary)   BINARY="${2:-}"; shift 2 ;;
    --user)     SVC_USER="${2:-}"; shift 2 ;;
    --start)    DO_START=1; shift ;;
    --no-enable) DO_ENABLE=0; shift ;;
    --container) CONTAINER=1; shift ;;
    --no-fence) NO_FENCE=1; shift ;;
    -h|--help)  usage; exit 0 ;;
    *) die "unknown argument: $1 (try --help)" ;;
  esac
done

case "$MODE" in admin|hardened) ;; *) die "--mode must be 'admin' or 'hardened'" ;; esac
[ "$MODE" = hardened ] && [ -z "$FENCE" ] && die "hardened mode requires --fence <dir> (it is the only writable area)"
[ "$(id -u)" -eq 0 ] || die "must run as root (writes /usr/local/bin and /etc/systemd/system)"

# Admin mode: the fence decision must be explicit — no fence means FULL filesystem
# access, so never default to that silently. Prompt when interactive; require an
# explicit --fence/--no-fence when piped (e.g. curl | bash), where we can't prompt.
if [ "$MODE" = admin ] && [ -z "$FENCE" ] && [ "$NO_FENCE" -ne 1 ]; then
  if [ -t 0 ]; then
    printf 'Fence file operations to a directory? Enter a path, or leave blank for FULL filesystem access: '
    read -r FENCE
  else
    die "no fence specified: pass --fence <dir> to confine file operations, or --no-fence for full access"
  fi
fi

# Reuse an already-installed binary at its absolute path if present, but NEVER auto-pick
# a binary from the current directory — a planted ./sparkyctrl would otherwise be installed
# and run as root. For a local build, pass --binary <path> explicitly.
if [ -z "$BINARY" ] && [ -x /usr/local/bin/sparkyctrl ]; then
  BINARY="/usr/local/bin/sparkyctrl"
fi
# If no local binary was found, download the published release binary.
# Public repo → no auth needed. Override repo/version via SPARKYCTRL_REPO / SPARKYCTRL_VERSION.
if [ -z "$BINARY" ]; then
  REPO="${SPARKYCTRL_REPO:-SimpleHonors/sparkyctrl}"
  VER="${SPARKYCTRL_VERSION:-latest}"
  case "$(uname -m)" in
    x86_64|amd64)  ARCH=amd64 ;;
    aarch64|arm64) ARCH=arm64 ;;
    *) die "unsupported architecture: $(uname -m) — build locally and pass --binary <path>" ;;
  esac
  if [ "$VER" = latest ]; then
    URL="https://github.com/${REPO}/releases/latest/download/sparkyctrl-linux-${ARCH}"
  else
    URL="https://github.com/${REPO}/releases/download/${VER}/sparkyctrl-linux-${ARCH}"
  fi
  command -v curl >/dev/null 2>&1 || die "curl is required to download the release binary"
  TMP="$(mktemp)"
  echo "==> downloading ${URL}"
  curl -fsSL "$URL" -o "$TMP" || die "download failed: ${URL}"
  chmod +x "$TMP"
  BINARY="$TMP"
fi
[ -n "$BINARY" ] && [ -x "$BINARY" ] || die "no binary found — build with deploy/build.sh, or pass --binary <path>"

DEST="/usr/local/bin/sparkyctrl"
if [ "$(readlink -f "$BINARY")" = "$(readlink -f "$DEST" 2>/dev/null || echo "$DEST")" ]; then
  echo "==> binary already in place: $DEST (skipping copy)"
else
  install -m 0755 "$BINARY" "$DEST"
  echo "==> installed binary: $BINARY -> $DEST"
fi

# Assemble the serve arguments shared by both modes.
SERVE_ARGS="--addr ${ADDR} --audit ${AUDIT}"
[ -n "$FENCE" ] && SERVE_ARGS="${SERVE_ARGS} --fence ${FENCE}"
[ -n "$TOKEN" ] && SERVE_ARGS="${SERVE_ARGS} --token ${TOKEN}"

# Ensure the fence dir and audit log exist.
[ -n "$FENCE" ] && mkdir -p "$FENCE"
touch "$AUDIT"

if [ "$MODE" = admin ]; then
  # Admin mode: root, full caps, exec/shell unfenced. Audit log owned by root.
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
  # Hardened mode: dedicated unprivileged user, zero caps, read-only FS except
  # the fence + audit log. Pre-create the user and hand it ownership of the
  # writable areas so a non-root worker can actually serve and audit.
  if ! id -u "$SVC_USER" >/dev/null 2>&1; then
    useradd --system --no-create-home --shell /usr/sbin/nologin "$SVC_USER"
    echo "==> created system user: $SVC_USER"
  fi
  chown -R "$SVC_USER":"$SVC_USER" "$FENCE" "$AUDIT"

  # The filesystem-isolation directives below use mount namespaces, which an
  # unprivileged LXC container cannot set up — the worker then dies with
  # 226/NAMESPACE. --container omits them while keeping the user / capability /
  # seccomp hardening. On a real host, VM, or privileged container, leave
  # --container off for the full read-only-filesystem posture.
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

echo "==> wrote unit: $UNIT (mode=$MODE)"
systemctl daemon-reload
if [ "$DO_ENABLE" -eq 1 ]; then systemctl enable sparkyctrl >/dev/null 2>&1 && echo "==> enabled at boot"; fi
if [ "$DO_START" -eq 1 ]; then systemctl restart sparkyctrl && echo "==> started: $(systemctl is-active sparkyctrl)"; fi
echo "==> done (mode=$MODE)."
