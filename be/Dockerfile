FROM golang:1.25-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o server ./cmd/server
FROM alpine:3.22
RUN apk add --no-cache ca-certificates espeak-ng ffmpeg poppler-utils
WORKDIR /app
COPY --from=build /app/server .
COPY migrations ./migrations
COPY internal/phonetics/data/CMUDICT-LICENSE ./licenses/CMUDICT-LICENSE
RUN mkdir -p /uploads && chown 10001:10001 /uploads
USER 10001
EXPOSE 8080
CMD ["./server"]
