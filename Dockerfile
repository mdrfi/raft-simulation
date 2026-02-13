FROM golang:1.21-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/raft-node       ./cmd/node
RUN CGO_ENABLED=0 go build -o /bin/raft-controller  ./cmd/controller

FROM alpine:3.19 AS node
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/raft-node /usr/local/bin/raft-node
RUN mkdir -p /data
VOLUME /data
EXPOSE 8080 7000
ENTRYPOINT ["raft-node"]

FROM alpine:3.19 AS controller
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/raft-controller /usr/local/bin/raft-controller
RUN mkdir -p /data
VOLUME /data
EXPOSE 9090
ENTRYPOINT ["raft-controller"]
