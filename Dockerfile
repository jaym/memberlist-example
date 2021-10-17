# syntax = docker/dockerfile:1.3
FROM golang:1.17.1-alpine AS build
WORKDIR /build
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY main.go .
RUN --mount=type=cache,target=/root/.cache/go-build go build -o /app main.go 

FROM alpine:3.14 AS memberlist-example
COPY --from=build /app /usr/local/bin/app
CMD ["app"]
