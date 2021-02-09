#!/bin/sh -e

[ -n "$__RELRAZZER_READY" ] || exit 1

. "$SCRIPTS_DIR/go/_envsetup.sh"

_F="$GO_VERSION.linux-amd64.tar.gz"
_DST="$TMP_DIR/$_F"

_download() {
	URL="https://golang.org/dl/$_F"
	wget "$URL" -O "$_DST"
}

_build() {
	tar xzf "$_DST" -C "$TOOLCHAINS_DIR"
}

_install() {
	: # We just extract the binary. Nothing to do.
}

_target="go-$GO_VERSION"
