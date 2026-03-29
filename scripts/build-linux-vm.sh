#!/usr/bin/env bash
#
# Build Buildermark Linux CLI binaries inside the Debian UTM VM from macOS.
#
# Usage:
#   ./scripts/build-linux-vm.sh
#   ./scripts/build-linux-vm.sh --arch amd64
#   ./scripts/build-linux-vm.sh --arch arm64 --version 1.0.0
#
# Environment variables:
#   ARCH             - "amd64", "arm64", or "all" (default: "all")
#   VERSION          - version string baked into the binary (default: "dev")
#   VM_NAME          - UTM VM name (default: "Debian Desktop")
#   SSH_HOST         - SSH host alias for the VM (default: "debianvm")
#   REMOTE_REPO_DIR  - existing repo checkout inside Debian
#                      (default: "/home/debian/github/buildermark")
#   REMOTE_RSYNC     - rsync path on Debian (default: "/usr/bin/rsync")
#   SSH_WAIT_SECONDS - SSH readiness timeout in seconds (default: 120)
#

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
LOCAL_BUILD_DIR="$ROOT_DIR/apps/linux-cli/build"
UTMCTL="/Applications/UTM.app/Contents/MacOS/utmctl"
CHANGED_FILES_LIST=""
DELETED_FILES_LIST=""

ARCH="${ARCH:-all}"
VERSION="${VERSION:-dev}"
PUBLIC_READ_ONLY="${PUBLIC_READ_ONLY:-}"
VM_NAME="${VM_NAME:-Debian Desktop}"
SSH_HOST="${SSH_HOST:-debianvm}"
REMOTE_REPO_DIR="${REMOTE_REPO_DIR:-/home/debian/github/buildermark}"
REMOTE_RSYNC="${REMOTE_RSYNC:-/usr/bin/rsync}"
SSH_WAIT_SECONDS="${SSH_WAIT_SECONDS:-120}"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --arch)    ARCH="$2";    shift 2 ;;
        --version) VERSION="$2"; shift 2 ;;
        --read-only) PUBLIC_READ_ONLY="true"; shift ;;
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

remote_shell_preamble() {
    cat <<'EOF'
export PATH="$PATH:/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin"
if [[ -s "$HOME/.nvm/nvm.sh" ]]; then
    . "$HOME/.nvm/nvm.sh"
fi
EOF
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
        echo "Error: local checkout is in detached HEAD; cannot determine branch to pull in Debian." >&2
        exit 1
    fi
}

prepare_file_lists() {
    CHANGED_FILES_LIST="$(mktemp "${TMPDIR:-/tmp}/buildermark-linux-vm.changed.XXXXXX")"
    DELETED_FILES_LIST="$(mktemp "${TMPDIR:-/tmp}/buildermark-linux-vm.deleted.XXXXXX")"

    git -C "$ROOT_DIR" diff --name-only --no-renames -z --diff-filter=ACMRTUXB HEAD > "$CHANGED_FILES_LIST"
    git -C "$ROOT_DIR" ls-files --others --exclude-standard -z >> "$CHANGED_FILES_LIST"
    git -C "$ROOT_DIR" diff --name-only --no-renames -z --diff-filter=D HEAD > "$DELETED_FILES_LIST"
}

update_remote_checkout() {
    ssh -o BatchMode=yes -o ConnectTimeout=5 "$SSH_HOST" 'bash -s' -- "$REMOTE_REPO_DIR" "$LOCAL_BRANCH" <<'EOF'
set -euo pipefail

repo_dir="$1"
branch="$2"

if [[ ! -d "$repo_dir/.git" ]]; then
    echo "Error: expected existing git checkout at $repo_dir" >&2
    exit 1
fi

cd "$repo_dir"

git reset --hard HEAD
git clean -fd

git fetch origin "$branch" --prune

if git show-ref --verify --quiet "refs/heads/$branch"; then
    git checkout "$branch"
elif git show-ref --verify --quiet "refs/remotes/origin/$branch"; then
    git checkout -B "$branch" "origin/$branch"
else
    echo "Error: branch '$branch' not found in remote checkout." >&2
    exit 1
fi

git pull --ff-only origin "$branch"
EOF
}

sync_changed_files_to_remote() {
    ssh_cmd "$REMOTE_RSYNC --version >/dev/null 2>&1"

    if [[ -s "$CHANGED_FILES_LIST" ]]; then
        rsync -az \
            --from0 \
            --files-from="$CHANGED_FILES_LIST" \
            --rsync-path="$REMOTE_RSYNC" \
            --info=name \
            "$ROOT_DIR/" \
            "$SSH_HOST:$REMOTE_REPO_DIR/"
    fi
}

apply_deleted_files_remote() {
    local rel_path remote_path

    if [[ ! -s "$DELETED_FILES_LIST" ]]; then
        return 0
    fi

    while IFS= read -r -d '' rel_path; do
        remote_path="$(printf '%q' "$REMOTE_REPO_DIR/$rel_path")"
        ssh_cmd "rm -rf -- $remote_path"
    done < "$DELETED_FILES_LIST"
}

build_remote_cli() {
    local repo_q arch_q version_q preamble

    repo_q="$(printf '%q' "$REMOTE_REPO_DIR")"
    arch_q="$(printf '%q' "$ARCH")"
    version_q="$(printf '%q' "$VERSION")"
    read_only_q="$(printf '%q' "$PUBLIC_READ_ONLY")"
    preamble="$(remote_shell_preamble)"

    ssh_cmd "bash -lc '$preamble"$'\n'"cd $repo_q && ARCH=$arch_q VERSION=$version_q PUBLIC_READ_ONLY=$read_only_q ./scripts/build-linux.sh'"
}

copy_artifacts_back() {
    mkdir -p "$LOCAL_BUILD_DIR"
    for arch in "${ARCHES[@]}"; do
        rm -rf "$LOCAL_BUILD_DIR/$arch"
        mkdir -p "$LOCAL_BUILD_DIR/$arch"
        rsync -az \
            --rsync-path="$REMOTE_RSYNC" \
            "$SSH_HOST:$REMOTE_REPO_DIR/apps/linux-cli/build/$arch/" \
            "$LOCAL_BUILD_DIR/$arch/"
        echo "  OK: $LOCAL_BUILD_DIR/$arch/buildermark"
    done
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
        if ssh_cmd true >/dev/null 2>&1; then
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

resolve_arches() {
    case "$ARCH" in
        amd64) ARCHES=(amd64) ;;
        arm64) ARCHES=(arm64) ;;
        all)   ARCHES=(amd64 arm64) ;;
        *)
            echo "Error: unsupported arch '$ARCH' (expected amd64, arm64, or all)" >&2
            exit 1
            ;;
    esac
}

detect_remote_arch() {
    if [[ -n "$ARCH" ]]; then
        resolve_arches
        return
    fi

    step "Detecting Debian architecture"
    ARCH="$(ssh -o BatchMode=yes -o ConnectTimeout=5 "$SSH_HOST" 'bash -lc "go env GOARCH"')"
    resolve_arches
}

trap cleanup EXIT

step "Checking prerequisites"
check_tool git
check_tool rsync
check_tool ssh

if [[ ! -x "$UTMCTL" ]]; then
    echo "Error: UTM CLI not found at $UTMCTL" >&2
    exit 1
fi

require_local_branch
prepare_file_lists

step "Starting Debian VM"
start_vm || echo "VM start helper returned non-zero; continuing to SSH readiness check in case the VM is already running."

step "Waiting for SSH"
wait_for_ssh
detect_remote_arch

step "Updating Debian checkout"
update_remote_checkout

step "Syncing local uncommitted files to Debian"
sync_changed_files_to_remote
apply_deleted_files_remote

step "Building Linux CLI in Debian"
build_remote_cli

step "Copying artifacts back to macOS"
copy_artifacts_back

step "Build complete"
for arch in "${ARCHES[@]}"; do
    echo "  $arch : $LOCAL_BUILD_DIR/$arch/buildermark"
done
echo ""
echo "Debian VM left running."
