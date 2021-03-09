FROM golang:1.16-buster as build
ARG GOARCH=
ENV CGO_ENABLED 1
ENV CC "$CC"

RUN mkdir /build
WORKDIR /build
RUN apt update && apt install -y git build-essential
RUN git clone -v https://github.com/zgoat/goatcounter
RUN cd goatcounter && git checkout be3116c320b40f1161584e78329d0ca6cb5f2dcf
RUN cd goatcounter && echo "CC=$CC" && go get 
RUN cd goatcounter/cmd/goatcounter && go build -tags 'sqlite_omit_load_extension osusergo netgo'  -ldflags='-extldflags=-static'  -o ../../../goatcounter_binary -v .
COPY goatcounter-caddy-log-adapter /build/goatcounter-caddy-log-adapter
RUN cd /build/goatcounter-caddy-log-adapter && go get 
RUN cd /build/goatcounter-caddy-log-adapter && go build -tags 'osusergo netgo'  -ldflags='-extldflags=-static'  -o ../../../goatcounter-caddy-log-adapter -v .


FROM alpine
WORKDIR /app

RUN apk add --update --no-cache tzdata alpine-conf jq && setup-timezone -z UTC
COPY --from=build /build/goatcounter_binary /app/goatcounter
COPY --from=build /build/goatcounter-caddy-log-adapter /app/goatcounter-caddy-log-adapter

COPY goatcounter-caddy-log-adapter/config.json /app/config.json
RUN chmod +x /app/goatcounter && chmod +x /app/goatcounter-caddy-log-adapter 
ENTRYPOINT ["/app/goatcounter"]
CMD ["help"]
