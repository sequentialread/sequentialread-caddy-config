#!/bin/bash -e

VERSION="0.0.6"

rm -rf dockerbuild || true
mkdir dockerbuild

cp Dockerfile dockerbuild/Dockerfile-amd64
cp Dockerfile dockerbuild/Dockerfile-arm
cp Dockerfile dockerbuild/Dockerfile-arm64

sed -E 's|FROM alpine|FROM amd64/alpine|' -i dockerbuild/Dockerfile-amd64
sed -E 's|FROM alpine|FROM arm32v7/alpine|'   -i dockerbuild/Dockerfile-arm
sed -E 's|FROM alpine|FROM arm64v8/alpine|' -i dockerbuild/Dockerfile-arm64

sed -E 's/GOARCH=/GOARCH=amd64/' -i dockerbuild/Dockerfile-amd64
sed -E 's/GOARCH=/GOARCH=arm/'   -i dockerbuild/Dockerfile-arm
sed -E 's/GOARCH=/GOARCH=arm64/' -i dockerbuild/Dockerfile-arm64

docker build -f dockerbuild/Dockerfile-amd64 -t sequentialread/caddy-config:$VERSION-amd64 .
docker build -f dockerbuild/Dockerfile-arm   -t sequentialread/caddy-config:$VERSION-arm .
docker build -f dockerbuild/Dockerfile-arm64 -t sequentialread/caddy-config:$VERSION-arm64 .

docker push sequentialread/caddy-config:$VERSION-amd64
docker push sequentialread/caddy-config:$VERSION-arm
docker push sequentialread/caddy-config:$VERSION-arm64

export DOCKER_CLI_EXPERIMENTAL=enabled

docker manifest create  sequentialread/caddy-config:$VERSION \
  sequentialread/caddy-config:$VERSION-amd64 \
  sequentialread/caddy-config:$VERSION-arm \
  sequentialread/caddy-config:$VERSION-arm64 

docker manifest annotate --arch amd64 sequentialread/caddy-config:$VERSION sequentialread/caddy-config:$VERSION-amd64
docker manifest annotate --arch arm sequentialread/caddy-config:$VERSION sequentialread/caddy-config:$VERSION-arm
docker manifest annotate --arch arm64 sequentialread/caddy-config:$VERSION sequentialread/caddy-config:$VERSION-arm64

docker manifest push sequentialread/caddy-config:$VERSION

rm -rf dockerbuild || true