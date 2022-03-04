#!/bin/sh -e

# NOTE: This environment setup is for my emacs usage and not necessary
# for the project.

export EMACS_SOCKET_NAME="relrazzer"
emacs --daemon="$EMACS_SOCKET_NAME"
