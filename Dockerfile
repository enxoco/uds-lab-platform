FROM golang:1.26-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /lab-server ./cmd/labserver
RUN CGO_ENABLED=0 GOOS=linux go build -o /lab-operator ./cmd/laboperator

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /lab-server /lab-server
COPY --from=build /lab-operator /lab-operator
EXPOSE 8080
CMD ["/lab-server"]
