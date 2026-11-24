FROM golang:1.25.5-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/otlp-ingress ./cmd/otlp-ingress
COPY internal/otlp/ingress ./internal/otlp/ingress
RUN CGO_ENABLED=0 go build -buildvcs=false -trimpath -o /out/otlp-ingress ./cmd/otlp-ingress

FROM alpine:3.22
RUN apk add --no-cache ca-certificates && addgroup -S ingress && adduser -S -G ingress ingress
COPY --from=build /out/otlp-ingress /usr/local/bin/otlp-ingress
USER ingress
ENTRYPOINT ["/usr/local/bin/otlp-ingress"]
