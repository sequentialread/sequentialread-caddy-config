#!/bin/bash

# this dockerfile was lifted from https://github.com/docker-library/ghost/blob/master/3/alpine/Dockerfile
# with the chown node and su-exec node stuff removed.
set -e

if [[ "$*" == node*current/index.js* ]]; then
	baseDir="$GHOST_INSTALL/content.orig"
	for src in "$baseDir"/*/ "$baseDir"/themes/*; do
		src="${src%/}"
		target="$GHOST_CONTENT/${src#$baseDir/}"
		mkdir -p "$(dirname "$target")"
		if [ ! -e "$target" ]; then
			tar -cC "$(dirname "$src")" "$(basename "$src")" | tar -xC "$(dirname "$target")"
		fi
	done
fi

exec "$@"
