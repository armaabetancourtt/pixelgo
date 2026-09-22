# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /pixelgo-api ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /pixelgo-api /pixelgo-api
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/pixelgo-api"]
