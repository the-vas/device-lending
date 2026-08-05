# Dockerfile
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/device-lending .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/device-lending ./device-lending
VOLUME ["/app/pb_data"]
EXPOSE 8090
ENTRYPOINT ["./device-lending", "serve", "--http=0.0.0.0:8090", "--dir=/app/pb_data"]
