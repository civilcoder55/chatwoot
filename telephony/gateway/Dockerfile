FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o /gateway ./cmd/gateway

FROM alpine:3.19

RUN apk add --no-cache ca-certificates

COPY --from=builder /gateway /gateway

EXPOSE 5060/udp 8080

ENTRYPOINT ["/gateway"]
