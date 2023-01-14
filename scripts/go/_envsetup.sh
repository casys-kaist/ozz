#!/bin/sh -e

# Be aware of conflictions with GO's environment variables
__export_envvar "GO" "$TOOLCHAINS_DIR/go"
__append_path "$GO_PATH/bin"
__append_path "$GOTOOLS_DIR/bin"
export GO="$GOROOT/bin/go"
export GOROOT="$GO_PATH"
export GOPATH="$GOTOOLS_DIR"
export GOOS="linux"
export GOARCH="amd64"
export GO_VERSION="go1.19.4"
