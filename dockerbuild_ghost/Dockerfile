# this dockerfile was lifted from https://github.com/docker-library/ghost/blob/master/3/alpine/Dockerfile
# with the chown node and su-exec node stuff removed.
FROM node:12-alpine3.12

RUN apk add --no-cache \
# add "bash" for "[["
		bash

ENV NODE_ENV production

ENV GHOST_CLI_VERSION 1.15.3
RUN set -eux; \
	npm install -g "ghost-cli@$GHOST_CLI_VERSION"; \
	npm cache clean --force

ENV GHOST_INSTALL /var/lib/ghost
ENV GHOST_CONTENT /var/lib/ghost/content

ENV GHOST_VERSION 3.41.6

RUN set -eux; \
	mkdir -p "$GHOST_INSTALL"; \
	\
# this stupid sed command is required to force ghost-cli to allow us to install as the root user
        sed -i "s/process.getuid() !== 0/true/" /usr/local/lib/node_modules/ghost-cli/lib/utils/check-root-user.js; \
	ghost install "$GHOST_VERSION" --db sqlite3 --allowRoot --no-prompt --no-stack --no-setup --dir "$GHOST_INSTALL"; \
	\
# Tell Ghost to listen on all ips and not prompt for additional configuration
	cd "$GHOST_INSTALL"; \
	ghost config --ip 0.0.0.0 --port 2368 --no-prompt --db sqlite3 --url http://localhost:2368 --dbpath "$GHOST_CONTENT/data/ghost.db"; \
	ghost config paths.contentPath "$GHOST_CONTENT"; \
	\
# make a config.json symlink for NODE_ENV=development (and sanity check that it's correct)
	ln -s config.production.json "$GHOST_INSTALL/config.development.json"; \
	readlink -f "$GHOST_INSTALL/config.development.json"; \
	\
# need to save initial content for pre-seeding empty volumes
	mv "$GHOST_CONTENT" "$GHOST_INSTALL/content.orig"; \
	mkdir -p "$GHOST_CONTENT"; \
	chmod 1777 "$GHOST_CONTENT"; \
	\
# force install "sqlite3" manually since it's an optional dependency of "ghost"
# (which means that if it fails to install, like on ARM/ppc64le/s390x, the failure will be silently ignored and thus turn into a runtime error instead)
# see https://github.com/TryGhost/Ghost/pull/7677 for more details
	cd "$GHOST_INSTALL/current"; \
# scrape the expected version of sqlite3 directly from Ghost itself
	sqlite3Version="$(node -p 'require("./package.json").optionalDependencies.sqlite3')"; \
	if ! yarn add "sqlite3@$sqlite3Version" --force; then \
# must be some non-amd64 architecture pre-built binaries aren't published for, so let's install some build deps and do-it-all-over-again
		apk add --no-cache --virtual .build-deps g++ gcc libc-dev make python3 vips-dev; \
		\
		npm_config_python='python3' yarn add "sqlite3@$sqlite3Version" --force --build-from-source; \
		\
		apk del --no-network .build-deps; \
	fi; \
	\
	yarn cache clean; \
	npm cache clean --force; \
	npm cache clean --force; \
	rm -rv /tmp/yarn* /tmp/v8*

WORKDIR $GHOST_INSTALL
VOLUME $GHOST_CONTENT

COPY docker-entrypoint.sh /usr/local/bin
ENTRYPOINT ["docker-entrypoint.sh"]

EXPOSE 2368
CMD ["node", "current/index.js"]
