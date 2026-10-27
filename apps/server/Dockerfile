# Image of the muserver service (ConnectServer + GameServer instances).
# Build context: apps/server. Used by docker-compose.yml at the repository root.

FROM golang:1.27.2 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
# Every dependency is pure Go (pgx, go-lua), so the binary is static.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/muserver ./cmd/muserver

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /srv/mu
COPY --from=build /out/muserver /usr/local/bin/muserver
# The server rewrites config/data/xshop.toml, the Castle Siege reset in
# config/gameserver/gameserver-cs.toml and images in data/Data/Custom, so both
# folders belong to the runtime user.
COPY --chown=nonroot:nonroot config ./config
COPY --chown=nonroot:nonroot data ./data
# ConnectServer, GameServer "Principal", GameServer "Castle Siege".
EXPOSE 44405 55900 55919
ENTRYPOINT ["muserver", "-config", "config", "-data", "data"]
