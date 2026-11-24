# syntax=docker/dockerfile:1
# Mock DigiLocker — single image: API server + consent pages + React UI + docs.
#
#   docker build -t mock-digilocker .                       # uses the committed UI build
#   docker build --build-arg BUILD_UI=true -t mock-digilocker .   # rebuild the React UI too

ARG GO_VERSION=1.24
ARG NODE_VERSION=22

# ---- 1. (optional) build the React UI into backend/internal/webui/dist ----
FROM node:${NODE_VERSION}-alpine AS ui
ARG BUILD_UI=false
WORKDIR /src
COPY frontend/ frontend/
COPY docs/ docs/
COPY postman/ postman/
COPY CONTRIBUTING.md SECURITY.md ./
COPY backend/internal/webui/dist/ backend/internal/webui/dist/
RUN if [ "$BUILD_UI" = "true" ]; then cd frontend && npm install --no-audit --no-fund && npm run build; fi

# ---- 2. build the Go server (with the pgx PostgreSQL driver) ----
FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src/backend
ENV CGO_ENABLED=0 GOFLAGS=-trimpath
COPY backend/ ./
COPY --from=ui /src/backend/internal/webui/dist/ internal/webui/dist/
RUN go get github.com/jackc/pgx/v5@v5.7.2 && go mod tidy && \
    go build -tags pgx -ldflags="-s -w" -o /out/mock-digilocker ./cmd/server

# ---- 3. minimal runtime ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/mock-digilocker /mock-digilocker
COPY deploy/profiles/ /profiles/
ENV PORT=8080 PROFILES_DIR=/profiles
EXPOSE 8080
USER nonroot:nonroot
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=5 CMD ["/mock-digilocker", "healthcheck"]
ENTRYPOINT ["/mock-digilocker"]
