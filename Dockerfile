FROM docker.io/alpine:edge AS build

RUN apk add --no-cache --update go gcc
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
    go build -trimpath -buildmode=pie -ldflags="-s -w" -o bin/dms .

FROM docker.io/alpine:edge AS prod

RUN addgroup -S user && \
    adduser -S -G user user
WORKDIR /dmsdir
VOLUME /dmsdir
RUN chown -R user:user /dmsdir
RUN apk add --no-cache \
    ffmpeg ffmpegthumbnailer mailcap
COPY --from=build /build/bin/dms /dms
USER user:user

ENTRYPOINT ["/dms"]
