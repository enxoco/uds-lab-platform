FROM golang:1.22-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /lab-server ./cmd/labserver

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /lab-server /lab-server
EXPOSE 8080
CMD ["/lab-server"]
