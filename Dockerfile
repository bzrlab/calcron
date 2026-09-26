FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json* ./
RUN npm ci || npm install
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
RUN apk add --no-cache tzdata
COPY --from=build /calcron /calcron
EXPOSE 8080
ENTRYPOINT ["/calcron"]
