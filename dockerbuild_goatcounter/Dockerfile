FROM golang:1.17-buster as build
ARG GOARCH=
ENV CGO_ENABLED 1
ENV CC "$CC"

RUN mkdir /build
WORKDIR /build
RUN apt update && apt install -y git build-essential
RUN git clone -v  https://github.com/zgoat/goatcounter

# v2.1.1
RUN cd goatcounter && git tag && git checkout 2461147f90cc340997710c307afe078548ffe7d6 

RUN cd goatcounter && echo "CC=$CC" && go get 
RUN cd goatcounter/cmd/goatcounter && go build -tags 'sqlite_omit_load_extension osusergo netgo'  -ldflags='-extldflags=-static'  -o ../../../goatcounter_binary -v .
COPY goatcounter-caddy-log-adapter /build/goatcounter-caddy-log-adapter
RUN cd /build/goatcounter-caddy-log-adapter && go get 
RUN cd /build/goatcounter-caddy-log-adapter && go build -tags 'osusergo netgo'  -ldflags='-extldflags=-static'  -o goatcounter-caddy-log-adapter -v .


FROM alpine
WORKDIR /app

RUN apk add --update --no-cache tzdata alpine-conf jq && setup-timezone -z UTC
COPY --from=build /build/goatcounter_binary /app/goatcounter
COPY --from=build /build/goatcounter-caddy-log-adapter/goatcounter-caddy-log-adapter /app/goatcounter-caddy-log-adapter

COPY goatcounter-caddy-log-adapter/config.json /app/config.json
RUN chmod +x /app/goatcounter && chmod +x /app/goatcounter-caddy-log-adapter 
ENTRYPOINT ["/app/goatcounter"]
CMD ["help"]
