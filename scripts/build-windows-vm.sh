#!/usr/bin/env bash
#
# Build Buildermark Windows app binaries inside a Windows UTM VM from macOS.
#
# Usage:
#   ./scripts/build-windows-vm.sh
#   ./scripts/build-windows-vm.sh --arch amd64
#   ./scripts/build-windows-vm.sh --arch arm64
#   ./scripts/build-windows-vm.sh --self-sign
#
# Environment variables:
#   ARCH             - "amd64", "arm64", or "all" (default: "all")
#   SELF_SIGN        - set to "1" to pass the self-sign flag into the VM build
#   VM_NAME          - UTM VM name (default: "Windows Desktop")
#   SSH_HOST         - SSH host alias for the VM (default: "windowsvm")
#   REMOTE_REPO_DIR  - repo checkout inside Windows (cloned automatically if missing)
#                      (default: "C:/Users/Test/github/buildermark")
#   SSH_WAIT_SECONDS - SSH readiness timeout in seconds (default: 180)
#

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
LOCAL_BUILD_DIR="$ROOT_DIR/apps/windows/build"
UTMCTL="/Applications/UTM.app/Contents/MacOS/utmctl"
CHANGED_FILES_LIST=""
DELETED_FILES_LIST=""

ARCH="${ARCH:-all}"
SELF_SIGN="${SELF_SIGN:-0}"
VM_NAME="${VM_NAME:-Windows Desktop}"
SSH_HOST="${SSH_HOST:-windowsvm}"
REMOTE_REPO_DIR="${REMOTE_REPO_DIR:-C:/Users/Test/github/buildermark}"
SSH_WAIT_SECONDS="${SSH_WAIT_SECONDS:-180}"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --arch) ARCH="$2"; shift 2 ;;
        --self-sign) SELF_SIGN=1; shift ;;
        *)
            echo "Unknown argument: $1" >&2
            exit 1
            ;;
    esac
done

step() {
    echo ""
    echo "==> $1"
    echo ""
}

check_tool() {
    if ! command -v "$1" >/dev/null 2>&1; then
        echo "Error: $1 is not installed." >&2
        exit 1
    fi
}

ssh_cmd() {
    ssh -o BatchMode=yes -o ConnectTimeout=5 "$SSH_HOST" "$@"
}

cleanup() {
    if [[ -n "$CHANGED_FILES_LIST" && -f "$CHANGED_FILES_LIST" ]]; then
        rm -f "$CHANGED_FILES_LIST"
    fi
    if [[ -n "$DELETED_FILES_LIST" && -f "$DELETED_FILES_LIST" ]]; then
        rm -f "$DELETED_FILES_LIST"
    fi
}

require_local_branch() {
    LOCAL_BRANCH="$(git -C "$ROOT_DIR" branch --show-current)"
    if [[ -z "$LOCAL_BRANCH" ]]; then
        echo "Error: local checkout is in detached HEAD; cannot determine branch to pull in Windows." >&2
        exit 1
    fi
}

prepare_file_lists() {
    CHANGED_FILES_LIST="$(mktemp "${TMPDIR:-/tmp}/buildermark-windows-vm.changed.XXXXXX")"
    DELETED_FILES_LIST="$(mktemp "${TMPDIR:-/tmp}/buildermark-windows-vm.deleted.XXXXXX")"

    git -C "$ROOT_DIR" diff --name-only --no-renames -z --diff-filter=ACMRTUXB HEAD > "$CHANGED_FILES_LIST"
    git -C "$ROOT_DIR" ls-files --others --exclude-standard -z >> "$CHANGED_FILES_LIST"
    git -C "$ROOT_DIR" diff --name-only --no-renames -z --diff-filter=D HEAD > "$DELETED_FILES_LIST"
}

urlencode() {
    python3 -c 'import sys, urllib.parse; print(urllib.parse.quote(sys.argv[1]))' "$1"
}

start_vm() {
    if "$UTMCTL" start "$VM_NAME"; then
        return 0
    fi

    echo "UTM start returned non-zero; falling back to UTM URL scheme."
    check_tool python3
    open "utm://start?name=$(urlencode "$VM_NAME")"
}

wait_for_ssh() {
    local deadline now
    deadline=$((SECONDS + SSH_WAIT_SECONDS))

    while true; do
        if ssh_cmd "echo ok" >/dev/null 2>&1; then
            return 0
        fi

        now=$SECONDS
        if (( now >= deadline )); then
            echo "Error: SSH to $SSH_HOST did not become ready within ${SSH_WAIT_SECONDS}s." >&2
            return 1
        fi

        sleep 2
    done
}

pwsh_escape() {
    printf "%s" "$1" | sed "s/'/''/g"
}

run_remote_powershell() {
    local script="$1"
    ssh_cmd "powershell -NoProfile -ExecutionPolicy Bypass -Command \"$script\""
}

update_remote_checkout() {
    local repo branch repo_e branch_e script

    repo="$REMOTE_REPO_DIR"
    branch="$LOCAL_BRANCH"
    repo_e="$(pwsh_escape "$repo")"
    branch_e="$(pwsh_escape "$branch")"

    script="\$ErrorActionPreference='Stop'; \
if (-not (Test-Path -LiteralPath '$repo_e')) { \
  throw ('Remote repo not found at $repo_e. Clone it manually first.') \
}; \
Set-Location -LiteralPath '$repo_e'; \
git reset --hard HEAD; \
git clean -fd; \
git fetch origin '$branch_e' --prune; \
git show-ref --verify --quiet ('refs/heads/' + '$branch_e'); \
if (\$LASTEXITCODE -eq 0) { \
  git checkout '$branch_e' | Out-Null \
} else { \
  git show-ref --verify --quiet ('refs/remotes/origin/' + '$branch_e'); \
  if (\$LASTEXITCODE -eq 0) { \
    git checkout -B '$branch_e' ('origin/' + '$branch_e') | Out-Null \
  } else { \
    throw ('Branch $branch_e not found in remote checkout.') \
  } \
}; \
git pull --ff-only origin '$branch_e'"

    run_remote_powershell "$script"
}

sync_changed_files_to_remote() {
    if [[ ! -s "$CHANGED_FILES_LIST" ]]; then
        return 0
    fi

    local repo_e script
    repo_e="$(pwsh_escape "$REMOTE_REPO_DIR")"
    script="\$ErrorActionPreference='Stop'; New-Item -ItemType Directory -Force -Path '$repo_e' | Out-Null; tar -xzf - -C '$repo_e'"

    COPYFILE_DISABLE=1 tar -czf - --null -T "$CHANGED_FILES_LIST" -C "$ROOT_DIR" | \
        ssh_cmd "powershell -NoProfile -ExecutionPolicy Bypass -Command \"$script\""
}

apply_deleted_files_remote() {
    local rel_path repo target target_e script

    if [[ ! -s "$DELETED_FILES_LIST" ]]; then
        return 0
    fi

    repo="$REMOTE_REPO_DIR"
    while IFS= read -r -d '' rel_path; do
        target="$repo/$rel_path"
        target_e="$(pwsh_escape "$target")"
        script="\$ErrorActionPreference='Stop'; if (Test-Path -LiteralPath '$target_e') { Remove-Item -LiteralPath '$target_e' -Recurse -Force }"
        # Use ssh -n to prevent SSH from consuming stdin (the while-read pipe).
        ssh -n -o BatchMode=yes -o ConnectTimeout=5 "$SSH_HOST" "powershell -NoProfile -ExecutionPolicy Bypass -Command \"$script\""
    done < "$DELETED_FILES_LIST"
}

resolve_runtimes() {
    case "$ARCH" in
        amd64) RUNTIMES=(win-x64) ;;
        arm64) RUNTIMES=(win-arm64) ;;
        all)   RUNTIMES=(win-x64 win-arm64) ;;
        *)
            echo "Error: unsupported arch '$ARCH' (expected amd64, arm64, or all)" >&2
            exit 1
            ;;
    esac
}

build_remote_windows_app() {
    local runtime repo_e runtime_e self_sign_e self_sign_arg script

    for runtime in "${RUNTIMES[@]}"; do
        repo_e="$(pwsh_escape "$REMOTE_REPO_DIR")"
        runtime_e="$(pwsh_escape "$runtime")"
        self_sign_e="$(pwsh_escape "$SELF_SIGN")"
        self_sign_arg=""
        if [[ "$SELF_SIGN" == "1" ]]; then
            self_sign_arg="-SelfSign"
        fi
        script="\$ErrorActionPreference='Stop'; Set-Location -LiteralPath '$repo_e'; \$env:SELF_SIGN='$self_sign_e'; powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build-windows.ps1 -Runtime '$runtime_e' $self_sign_arg"
        run_remote_powershell "$script"
    done
}

copy_artifacts_back() {
    local runtime remote_dir

    mkdir -p "$LOCAL_BUILD_DIR"

    for runtime in "${RUNTIMES[@]}"; do
        remote_dir="$REMOTE_REPO_DIR/apps/windows/build/$runtime"
        rm -rf "$LOCAL_BUILD_DIR/$runtime"
        mkdir -p "$LOCAL_BUILD_DIR/$runtime"

        ssh_cmd "tar -czf - -C \"$remote_dir\" ." | tar -xzf - -C "$LOCAL_BUILD_DIR/$runtime"
        echo "  OK: $LOCAL_BUILD_DIR/$runtime"
    done
}

trap cleanup EXIT

step "Checking prerequisites"
check_tool git
check_tool ssh
check_tool tar

if [[ ! -x "$UTMCTL" ]]; then
    echo "Error: UTM CLI not found at $UTMCTL" >&2
    exit 1
fi

require_local_branch
prepare_file_lists
resolve_runtimes

step "Starting Windows VM"
if ssh_cmd "echo ok" >/dev/null 2>&1; then
    echo "VM is already running and SSH-reachable; skipping start."
else
    start_vm || echo "VM start helper returned non-zero; continuing to SSH readiness check in case the VM is already running."
    step "Waiting for SSH"
    wait_for_ssh
fi

step "Updating Windows checkout"
update_remote_checkout

step "Syncing local uncommitted files to Windows"
sync_changed_files_to_remote
apply_deleted_files_remote

step "Building Windows app in VM"
build_remote_windows_app

step "Copying artifacts back to macOS"
copy_artifacts_back

step "Build complete"
for runtime in "${RUNTIMES[@]}"; do
    echo "  $runtime : $LOCAL_BUILD_DIR/$runtime/installer"
done
echo ""
echo "Windows VM left running."
