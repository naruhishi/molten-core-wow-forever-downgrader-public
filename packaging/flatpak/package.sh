#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 2 ]; then
    echo "Usage: $0 LINUX_X64_EXECUTABLE OUTPUT.flatpak" >&2
    exit 2
fi
here=$(cd -- "$(dirname -- "$0")" && pwd)
binary=$1
output=$2
app=io.github.naruhishi.MoltenCore
stage=$(mktemp -d)
trap 'rm -rf -- "$stage"' EXIT

install -Dm755 "$binary" "$stage/app/files/bin/molten-core"
install -Dm644 "$here/$app.desktop" "$stage/app/files/share/applications/$app.desktop"
install -Dm644 "$here/../../assets/molten-core-crest.png" "$stage/app/files/share/icons/hicolor/512x512/apps/$app.png"
install -Dm644 "$here/../../LICENSE" "$stage/app/files/share/licenses/$app/LICENSE"
cat > "$stage/app/metadata" <<META
[Application]
name=$app
runtime=org.gnome.Platform/x86_64/49
sdk=org.gnome.Sdk/x86_64/49
command=molten-core
META

flatpak build-finish --share=network --share=ipc --socket=wayland --socket=fallback-x11 \
    --device=dri --filesystem=home --filesystem=/run/media "$stage/app"
flatpak build-export ${FLATPAK_EXPORT_OPTION:+"$FLATPAK_EXPORT_OPTION"} --arch=x86_64 "$stage/repo" "$stage/app" stable
flatpak build-bundle --arch=x86_64 --runtime-repo=https://dl.flathub.org/repo/flathub.flatpakrepo \
    "$stage/repo" "$output" "$app" stable
