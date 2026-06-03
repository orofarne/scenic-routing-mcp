FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /scenic-routing-mcp ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /scenic-routing-mcp /usr/local/bin/scenic-routing-mcp
ENTRYPOINT ["/usr/local/bin/scenic-routing-mcp"]
