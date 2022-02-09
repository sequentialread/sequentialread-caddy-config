FROM golang:1.16-alpine as build
ARG GOARCH=
ARG GO_BUILD_ARGS=

RUN mkdir /build
WORKDIR /build
RUN apk add --update --no-cache ca-certificates git 
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download
COPY main.go main.go
RUN  go build -v $GO_BUILD_ARGS -o /build/sequentialread-caddy-config .




FROM alpine
WORKDIR /app
# COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /build/sequentialread-caddy-config /app/sequentialread-caddy-config
RUN chmod +x /app/sequentialread-caddy-config
ENTRYPOINT ["/app/sequentialread-caddy-config"]
