FROM docker.io/alpine:edge AS build
WORKDIR /dms
ADD . /dms
RUN apk add --no-cache go gcc musl-dev
RUN go mod tidy 
RUN go build -trimpath -buildmode=pie -ldflags="-s -w" -o dms


FROM docker.io/alpine:edge
COPY --from=build --chown=1000:1000 /dms/dms /dms
RUN apk add --no-cache ffmpeg ffmpegthumbnailer mailcap
RUN adduser user || true
USER user:user
WORKDIR /dmsdir
VOLUME /dmsdir
ENTRYPOINT ["/dms"]
