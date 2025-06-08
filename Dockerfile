# syntax=docker/dockerfile:1

ARG ALPINE_VERSION=3.21.3
ARG GO_VERSION=1.24.3
ARG PORT=8080

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build

WORKDIR /src

RUN --mount=type=cache,target=/go/pkg/mod/ \
    --mount=type=bind,source=go.mod,target=go.mod \
    --mount=type=bind,source=go.sum,target=go.sum \
    go mod download -x

ARG TARGETARCH

RUN --mount=type=cache,target=/go/pkg/mod/ \
    --mount=type=bind,target=. \
    CGO_ENABLED=0 GOARCH=$TARGETARCH go build -o /bin/go-orchestrator ./cmd/go-orchestrator

FROM alpine:${ALPINE_VERSION} AS final

RUN --mount=type=cache,target=/var/cache/apk \
    apk --update add --no-cache \
        ca-certificates \
        tzdata \
        && \
        update-ca-certificates && \
        rm -rf /var/cache/apk/*

ARG UID=10001

RUN adduser \
    --disabled-password \
    --gecos "" \
    --home "/nonexistent" \
    --shell "/sbin/nologin" \
    --no-create-home \
    --uid "${UID}" \
    appuser

USER appuser

COPY --from=build /bin/go-orchestrator /bin/

EXPOSE ${PORT}

ENTRYPOINT [ "/bin/go-orchestrator" ]