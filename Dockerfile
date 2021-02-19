  FROM golang:1.15.2-alpine as build
  ARG GOARCH=
  ARG GO_BUILD_ARGS=

  RUN mkdir /build
  WORKDIR /build
  RUN apk add --update --no-cache ca-certificates git \
    && go get git.sequentialread.com/forest/pkg-errors
  COPY . .
  RUN  go build -v $GO_BUILD_ARGS -o /build/sequentialread-caddy-config main.go

  FROM alpine
  WORKDIR /app
  # COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
  COPY --from=build /build/sequentialread-caddy-config /app/sequentialread-caddy-config
  RUN chmod +x /app/sequentialread-caddy-config
  ENTRYPOINT ["/app/sequentialread-caddy-config"]
 