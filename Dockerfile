FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/picfolderbot .

FROM alpine:3.21

RUN adduser -D -H appuser
USER appuser
WORKDIR /home/appuser

COPY --from=builder /bin/picfolderbot /usr/local/bin/picfolderbot

ENTRYPOINT ["/usr/local/bin/picfolderbot"]
