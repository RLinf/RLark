FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod main.go ./
RUN CGO_ENABLED=0 GOWORK=off go build -trimpath -o /station-forwarder .
FROM ubuntu:22.04
RUN apt-get update && apt-get install -y --no-install-recommends iptables && rm -rf /var/lib/apt/lists/*
COPY --from=build /station-forwarder /station-forwarder
ENTRYPOINT ["/station-forwarder"]
