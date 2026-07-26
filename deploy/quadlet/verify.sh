#!/bin/sh

set -eu

unit_path=${1:-deploy/quadlet/phantom.container}
unit_dir=$(CDPATH= cd -- "$(dirname -- "$unit_path")" && pwd)
work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT HUP INT TERM

if podman quadlet --help >/dev/null 2>&1; then
    XDG_CONFIG_HOME="$work_dir/config" \
        podman quadlet install --reload-systemd=false "$unit_path" >/dev/null
    unit_dir=$work_dir/config/containers/systemd
fi

quadlet_bin=
for candidate in /usr/libexec/podman/quadlet /usr/lib/podman/quadlet; do
    if [ -x "$candidate" ]; then
        quadlet_bin=$candidate
        break
    fi
done
if [ -z "$quadlet_bin" ]; then
    echo "Podman Quadlet generator was not found" >&2
    exit 1
fi
QUADLET_UNIT_DIRS="$unit_dir" \
    "$quadlet_bin" -user -dryrun >"$work_dir/phantom.service"

grep -q '^ExecStart=.*podman run ' "$work_dir/phantom.service"
grep -q -- '--network host' "$work_dir/phantom.service"
grep -q -- 'io.containers.autoupdate=registry' "$work_dir/phantom.service"
grep -q -- '%h/.local/share/phantom:/etc/x-ui:Z' "$work_dir/phantom.service"
grep -Fq -- '--health-cmd "[\"CMD\",\"/app/x-ui\",\"healthcheck\"]"' \
    "$work_dir/phantom.service"
