#!/bin/bash

# clone https://git.sequentialread.com/forest/gitea/
# checkout docker-armv7

GITEA_VERSION=1.13.2

docker build -t "sequentialread/gitea-armv7:$GITEA_VERSION" -f ./gitea/Dockerfile.armv7 ./gitea