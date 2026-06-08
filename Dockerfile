FROM node:20-alpine AS ide-build
WORKDIR /app/web/ide-src
COPY web/ide-src/package*.json ./
RUN npm ci
COPY web/ide-src/ ./
RUN npm run build

FROM golang:1.26-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
COPY --from=ide-build /app/web/static/ide-assets/ ./web/static/ide-assets/
RUN CGO_ENABLED=0 GOOS=linux go build -o /lab-server ./cmd/labserver

FROM debian:bookworm-slim
RUN apt-get update -q && apt-get install -y -q ca-certificates shellcheck && rm -rf /var/lib/apt/lists/* \
    && mkdir /data && chown nobody:nogroup /data
COPY --from=build /lab-server /lab-server
EXPOSE 8080
USER nobody
CMD ["/lab-server"]
