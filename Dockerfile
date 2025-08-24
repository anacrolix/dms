FROM docker.io/golang:1.19-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -buildmode=pie -ldflags="-s -w" -o dms

FROM docker.io/alpine:edge AS prod
WORKDIR /dmsdir
VOLUME /dmsdir
COPY --from=build /app/dms /dms
RUN apk add --no-cache ffmpeg ffmpegthumbnailer mailcap
RUN adduser user || true
USER user:user
SHELL ["/bin/sh", "-c"]
ENTRYPOINT exec /dms $DMS_FLAGS
