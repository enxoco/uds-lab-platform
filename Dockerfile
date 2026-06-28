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
RUN CGO_ENABLED=0 GOOS=linux go build -o /lab-operator ./cmd/laboperator

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /lab-server /lab-server
COPY --from=build /lab-operator /lab-operator
EXPOSE 8080
CMD ["/lab-server"]
