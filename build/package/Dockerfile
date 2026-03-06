# Stage 1: Build frontend
FROM node:20-alpine AS frontend
WORKDIR /app/web/frontend
COPY web/frontend/package*.json ./
RUN npm ci
COPY web/frontend/ ./
COPY tailflow-shared/web/src /tailflow-shared/web/src
RUN ln -s /app/web/frontend/node_modules /tailflow-shared/node_modules
RUN npx vite build

# Stage 2: Build Go binary
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache ca-certificates tzdata
ARG VERSION=dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" -o /tailflow ./cmd/tailflow/

# Stage 3: Final image
FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /tailflow /usr/local/bin/tailflow
WORKDIR /workflows
EXPOSE 8080
ENTRYPOINT ["tailflow"]
CMD ["serve"]
