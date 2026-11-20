FROM golang:1.26.5-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/authrail-mcp ./cmd/authrail-mcp && \
    CGO_ENABLED=0 go build -trimpath -o /out/authrail-gateway-adapter ./cmd/authrail-gateway-adapter

FROM alpine:3.23 AS mcp
ARG REVISION=ad71955-slice24-dirty
LABEL org.opencontainers.image.version="development-slice24" org.opencontainers.image.revision=$REVISION
RUN apk add --no-cache ca-certificates
COPY --from=build /out/authrail-mcp /usr/local/bin/authrail-mcp
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/authrail-mcp"]

FROM alpine:3.23 AS gateway-adapter
ARG REVISION=83981f7-slices14-17-dirty
LABEL org.opencontainers.image.version="development-slice17" org.opencontainers.image.revision=$REVISION
RUN apk add --no-cache ca-certificates && mkdir -p /var/lib/authrail-gateway && \
    chown 65532:65532 /var/lib/authrail-gateway && chmod 700 /var/lib/authrail-gateway
COPY --from=build /out/authrail-gateway-adapter /usr/local/bin/authrail-gateway-adapter
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/authrail-gateway-adapter"]

FROM cr.agentgateway.dev/agentgateway:v1.7.0@sha256:60216aafb03d37f94e952ff3d51d2f630a65b10ecaebdc0a864882f92610c45b AS gateway
ARG REVISION=fdbcb1c-slice8-dirty
LABEL org.opencontainers.image.version="1.7.0-authrail-slice7" org.opencontainers.image.revision=$REVISION
COPY --from=build /out/authrail-gateway-adapter /usr/local/bin/authrail-gateway-health
