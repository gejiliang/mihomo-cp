# Stage 1: Build frontend
FROM node:lts-alpine AS frontend
WORKDIR /app/web
RUN corepack enable && corepack prepare pnpm@latest --activate
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ .
RUN pnpm build

# Stage 2: Build backend
FROM golang:alpine AS backend
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/dist web/dist
RUN CGO_ENABLED=0 go build -o mihomo-cp ./cmd/mihomo-cp

# Stage 3: Runtime
FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=backend /app/mihomo-cp /usr/local/bin/
EXPOSE 8080
ENTRYPOINT ["mihomo-cp"]
