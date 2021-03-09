#!/bin/bash -e

VERSION="1.4.2-15"

rm -rf dockerbuild || true
mkdir dockerbuild

#cp Dockerfile dockerbuild/Dockerfile-amd64
cp Dockerfile dockerbuild/Dockerfile-arm
#cp Dockerfile dockerbuild/Dockerfile-arm64

#sed -E 's|FROM alpine|FROM amd64/alpine|' -i dockerbuild/Dockerfile-amd64

sed -E 's|FROM alpine|FROM arm32v7/alpine|'   -i dockerbuild/Dockerfile-arm
sed -E 's/GOARCH=/GOARCH=arm/'   -i dockerbuild/Dockerfile-arm
sed -E 's/CC "\$CC"/CC "arm-linux-gnueabi-gcc"/'  -i dockerbuild/Dockerfile-arm
sed -E 's/build-essential/gcc-arm* build-essential/'  -i dockerbuild/Dockerfile-arm

#sed -E 's|FROM alpine|FROM arm64v8/alpine|' -i dockerbuild/Dockerfile-arm64
#sed -E 's/GOARCH=/GOARCH=amd64/' -i dockerbuild/Dockerfile-amd64
#aarch64-linux-musl-gcc


#cat dockerbuild/Dockerfile-arm
#sed -E 's/GOARCH=/GOARCH=arm64/' -i dockerbuild/Dockerfile-arm64

#docker build -f dockerbuild/Dockerfile-amd64 -t sequentialread/goatcounter:$VERSION-amd64 .
docker build -f dockerbuild/Dockerfile-arm   -t sequentialread/goatcounter:$VERSION-arm .
#docker build -f dockerbuild/Dockerfile-arm64 -t sequentialread/goatcounter:$VERSION-arm64 .

#docker push sequentialread/goatcounter:$VERSION-amd64
docker push sequentialread/goatcounter:$VERSION-arm
#docker push sequentialread/goatcounter:$VERSION-arm64

export DOCKER_CLI_EXPERIMENTAL=enabled

docker manifest create  sequentialread/goatcounter:$VERSION sequentialread/goatcounter:$VERSION-arm

# docker manifest create  sequentialread/goatcounter:$VERSION \
#   sequentialread/goatcounter:$VERSION-amd64 \
#   sequentialread/goatcounter:$VERSION-arm \
#   sequentialread/goatcounter:$VERSION-arm64 

#docker manifest annotate --arch amd64 sequentialread/goatcounter:$VERSION sequentialread/goatcounter:$VERSION-amd64
docker manifest annotate --arch arm sequentialread/goatcounter:$VERSION sequentialread/goatcounter:$VERSION-arm
#docker manifest annotate --arch arm64 sequentialread/goatcounter:$VERSION sequentialread/goatcounter:$VERSION-arm64

docker manifest push sequentialread/goatcounter:$VERSION

rm -rf dockerbuild || true