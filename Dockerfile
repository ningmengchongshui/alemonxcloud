FROM node:22-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package.json frontend/yarn.lock ./
RUN corepack enable && yarn install --frozen-lockfile
COPY frontend/ ./
RUN yarn build

FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY src/ ./src/
COPY --from=frontend /app/src/dist ./src/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-X main.Version=${VERSION} -s -w" -o app ./src

FROM alpine:3.20
WORKDIR /app
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/app ./app
EXPOSE 8082
CMD ["./app"]
