FROM golang:1.22 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/inskill ./cmd/server

FROM alpine:3.19
RUN apk add --no-cache tzdata
COPY --from=builder /out/inskill /usr/local/bin/inskill
EXPOSE 8081
ENTRYPOINT ["inskill"]
