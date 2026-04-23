FROM golang:1.26 AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/proxy-lb ./cmd/proxy-lb

FROM gcr.io/distroless/base-debian12

WORKDIR /app
COPY --from=builder /out/proxy-lb /app/proxy-lb
COPY config/config.example.yaml /app/config/config.example.yaml

EXPOSE 8088

ENTRYPOINT ["/app/proxy-lb"]
