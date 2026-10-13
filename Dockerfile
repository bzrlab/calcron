FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json* ./
RUN npm ci
COPY web/ ./
# vite outDir is ../internal/server/dashboard/dist, i.e. /internal/server/dashboard/dist here.
RUN npm run build

FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
COPY --from=web /internal/server/dashboard/dist /src/internal/server/dashboard/dist
RUN CGO_ENABLED=0 go build -o /calcron ./cmd/calcron

FROM alpine:3.22
RUN apk add --no-cache tzdata && addgroup -S calcron && adduser -S -G calcron calcron
COPY --from=build /calcron /calcron
USER calcron
EXPOSE 8080
ENTRYPOINT ["/calcron"]
