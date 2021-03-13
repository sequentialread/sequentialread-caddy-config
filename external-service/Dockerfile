FROM golang:1.16-alpine as build
ARG GOARCH=
ARG GO_BUILD_ARGS=

RUN mkdir /build
WORKDIR /build
COPY . .
RUN  go get && go build -v $GO_BUILD_ARGS -o /build/external-service .


FROM alpine
WORKDIR /app
RUN apk --update --no-cache add curl 
COPY --from=build /build/external-service /app/external-service
RUN chmod +x /app/external-service
ENTRYPOINT ["/app/external-service"]
