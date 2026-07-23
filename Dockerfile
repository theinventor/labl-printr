# Stage 1: frontends
FROM node:22-alpine AS frontends
RUN corepack enable
WORKDIR /src
COPY web/package.json web/package-lock.json* web/
RUN cd web && npm install
COPY designer/package.json designer/pnpm-lock.yaml designer/pnpm-workspace.yaml designer/
COPY designer/packages designer/packages
RUN cd designer && pnpm install
COPY web web
COPY designer designer
RUN cd web && npm run build
RUN cd designer && pnpm run build

# Stage 2: Go binary with embedded assets
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontends /src/web/dist internal/server/dist
COPY --from=frontends /src/designer/dist internal/server/designerdist
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /labl-server ./cmd/labl-server

# Stage 3: runtime
FROM alpine:3.20
RUN adduser -D labl
USER labl
WORKDIR /home/labl
COPY --from=build /labl-server /usr/local/bin/labl-server
VOLUME /home/labl/data
# 5225 = web UI + API; 9100 = virtual printer (raw ZPL in, PNGs in the tray)
EXPOSE 5225 9100
ENTRYPOINT ["labl-server", "-data", "/home/labl/data"]
