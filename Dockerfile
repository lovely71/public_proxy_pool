FROM golang:1.24-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/proxypool ./cmd/proxypool

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /out/proxypool /app/proxypool

ENV HTTP_ADDR=":8080"
ENV SQLITE_PATH="/data/proxypool.db"

VOLUME ["/data"]
EXPOSE 8080

ENTRYPOINT ["/app/proxypool"]
