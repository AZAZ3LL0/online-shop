# syntax=docker/dockerfile:1

# Build stage: Tailwind -> templ -> go build, exactly the order of tech.md §10.1.
FROM golang:1.25-bookworm AS build

ARG TARGETARCH
ARG TAILWIND_VERSION=v3.4.17
ARG TEMPL_VERSION=v0.3.898

WORKDIR /src

RUN case "${TARGETARCH}" in \
      amd64) asset=tailwindcss-linux-x64 ;; \
      arm64) asset=tailwindcss-linux-arm64 ;; \
      *) echo "unsupported arch ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    curl -fsSL -o /usr/local/bin/tailwindcss \
      "https://github.com/tailwindlabs/tailwindcss/releases/download/${TAILWIND_VERSION}/${asset}" \
    && chmod +x /usr/local/bin/tailwindcss

COPY go.mod go.sum ./
RUN go mod download
RUN go install github.com/a-h/templ/cmd/templ@${TEMPL_VERSION}

COPY . .

RUN tailwindcss -c tailwind.config.js -i web/static/css/input.css -o web/static/css/app.css --minify
RUN templ generate
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/app

# Runtime stage: no shell, no package manager, non-root.
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=build /out/app /app/app
COPY --from=build /src/web/static /app/web/static

USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/app"]
CMD ["serve"]
