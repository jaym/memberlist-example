FROM golang:1.17.0-alpine AS build
WORKDIR /build
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY main.go .
RUN go build -o /app main.go 

FROM alpine:3.14 AS memberlist-example
COPY --from=build /app /usr/local/bin/app
CMD ["app"]
