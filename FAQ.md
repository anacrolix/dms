# FAQ

## Installation

### How do I install dms?

```
go install github.com/anacrolix/dms@latest
```

Go must be installed and `$GOPATH/bin` (or `$HOME/go/bin`) must be in your `PATH`.

### How do I run it as a Docker container?

```bash
docker pull ghcr.io/anacrolix/dms:latest
docker run -d --network host -v /path/to/media:/dmsdir ghcr.io/anacrolix/dms:latest
```

`--network host` is required for SSDP/UPnP discovery to work.

### How do I run it as a systemd service?

A sample service file is provided at `helpers/systemd/dms.service`. Copy it to `/etc/systemd/system/` and adjust the `ExecStart` path and media directory as needed.

### How do I run it as a FreeBSD service?

Install the service file at `helpers/bsd/dms` to `/etc/rc.d` or `/usr/local/etc/rc.d`, then add to `/etc/rc.conf`:

```
dms_enable="YES"
dms_root="/path/to/my/media"   # optional
dms_user="myuser"              # optional
```

### The binary doesn't work on NixOS.

Pre-built binaries use dynamic linking that doesn't work on NixOS. Install from source instead:

```
go install github.com/anacrolix/dms@latest
```

---

## Configuration

### How do I use a config file instead of command-line flags?

Pass `-config /path/to/config.json`. Example:

```json
{
  "path": "/path/to/media",
  "friendlyName": "My Media Server",
  "noTranscode": true,
  "deviceIcon": "/path/to/icon.png",
  "deviceIconSizes": ["48:512", "128:512"]
}
```

Config file keys match the flag names (camelCase). Note: `allowedIps` in config files was broken before v1.7.2 — upgrade if you see issues.

### How do I serve multiple directories?

dms serves a single root path. To serve multiple directories, create a parent directory and use symlinks:

```bash
mkdir ~/media
ln -s /path/to/movies ~/media/movies
ln -s /path/to/music ~/media/music
dms -path ~/media
```

### How do I restrict which clients can connect?

Use `-allowedIps` with a comma-separated list of IPs or CIDR ranges:

```
dms -allowedIps 192.168.1.0/24,10.0.0.1
```

### How do I change the HTTP port?

Use `-http :8080` (default is `:1338`). To listen on IPv6 only: `-http [::]:1338`. To listen on all interfaces including IPv6: `-http [::]:1338`.

### How do I set a custom server name?

```
dms -friendlyName "Living Room Server"
```

### How do I ignore hidden files and dot-directories?

```
dms -ignoreHidden -ignoreUnreadable
```

### How do I ignore specific directories (e.g. thumbnails)?

```
dms -ignore thumbnails,thumbs,.git
```

### How do I configure the SSDP announce interval?

```
dms -notifyInterval 60s
```

Default is 30s.

### How do I restrict dms to a specific network interface?

```
dms -ifname eth0
```

---

## Transcoding and Media

### Does dms require ffmpeg?

ffmpeg (or avconv) is optional. Without it, dms serves files directly. With it, dms can:
- Transcode video to mpeg2 PAL-DVD or WebM/VP8 (e.g. for Chromecast)
- Generate thumbnails (requires `ffmpegthumbnailer` separately)

### How do I disable transcoding?

```
dms -noTranscode
```

Or in config: `"noTranscode": true`

### How do I force transcoding to a specific format?

```
dms -forceTranscodeTo chromecast   # WebM/VP8
dms -forceTranscodeTo vp8          # VP8
```

### How do I disable ffprobe media scanning?

```
dms -noProbe
```

This disables bitrate/duration metadata but improves startup time on large libraries.

### Why is the first browse slow on a large library?

dms scans directories on demand rather than at startup, but older versions had a bug where `childCount` triggered a full recursive scan. This was fixed — upgrade to v1.7.0 or later.

### Does dms support subtitles?

Subtitle files are served, but client support varies. Most DLNA renderers need to handle subtitle loading themselves. This is a known limitation — see issue #140.

### Does dms support FLAC and MP3?

Yes, audio files are served directly. If they're not appearing, check that the files have standard media extensions and are readable. Run with `-logHeaders` to see what the client is requesting.

### How do I use ffmpeg instead of avconv?

dms prefers `ffmpeg`/`ffprobe` and falls back to `avconv`/`avprobe`. Ensure `ffmpeg` is in your `PATH` and it will be used automatically.

### How do I serve a live stream (e.g. RTSP camera)?

Enable dynamic streams with `-allowDynamicStreams` and create a `.dms.json` file in your media directory:

```json
{
  "Title": "My Camera",
  "Resources": [
    {
      "MimeType": "video/webm",
      "Command": "ffmpeg -i rtsp://camera-ip:554/stream -c:v copy -c:a copy -f matroska -"
    }
  ]
}
```

---

## Compatibility

### Which players and renderers are known to work?

- Panasonic Viera TVs
- BubbleUPnP and AirWire (Android)
- Chromecast
- VLC (desktop and mobile)
- LG Smart TVs (with varying success)
- Roku devices
- Apple TV 4K (via VLC or 8player)
- iOS VLC and 8player

### Samsung TV: video is found but cannot be played.

Samsung TVs can be picky about DLNA conformance. Try:
1. Running without transcoding: `dms -noTranscode`
2. If pause/seek doesn't work, this is a known issue (#165) — Samsung requires specific DLNA headers for transport control.
3. For Samsung Frame TVs, ensure you're on v1.6.0 or later (#110).

### LG TV: stalled subscribe connection messages in logs.

Try the `-stallEventSubscribe` workaround:

```
dms -stallEventSubscribe
```

This was added specifically for older LG TVs that send malformed event subscription requests.

### Windows 10 cannot discover the DMS server.

Windows 10's DLNA discovery uses UPnP multicast. Make sure:
- Your firewall allows UDP port 1900 (SSDP) and TCP port 1338 (HTTP)
- dms is bound to the correct network interface (`-ifname`)
- You're on the same subnet (DLNA does not cross routers by default)

### How do I access dms across different network segments (VLANs)?

DLNA relies on multicast which doesn't cross network boundaries. Use [dlna-proxy](https://github.com/fenio/dlna-proxy) to bridge between segments.

### BubbleUPnP is slow or crashes with a large music library.

With very large libraries (e.g. 26k+ tracks), each browse triggers ffprobe on new files, which accumulates processes. Use `-noProbe` to disable ffprobe entirely, or use `-noTranscode -noProbe` if you don't need transcoding features. See issue #105.

---

## Troubleshooting

### dms is not discovered on the network.

1. Ensure dms is running and listening: `curl http://localhost:1338/rootDesc.xml`
2. Check that UDP port 1900 is not blocked by a firewall
3. Try specifying the interface: `dms -ifname eth0`
4. On multi-homed hosts, try each interface name until one works
5. If using Docker, ensure `--network host` is set

### I see "broken pipe" errors from ffmpeg when transcoding.

This is normal — it happens when a client stops playback before the transcode is finished. The errors are benign. See issue #129.

### Symlinked directories are not visible.

This was fixed in a recent version. Upgrade to the latest release. If still broken, ensure the symlink target is readable by the user running dms.

### The path flag requires an absolute path.

Always use an absolute path with `-path`:

```
dms -path /home/user/media   # correct
dms -path ~/media            # may not work in all shells
```

### How do I get more verbose log output?

There is no `-verbose` flag currently (#126). Run with `-logHeaders` to see HTTP request/response headers. Transcode logs go to `~/.dms/log/` by default; change with `-transcodeLogPattern`.

### How do I disable transcode logging?

```
dms -transcodeLogPattern /dev/null
```

### The server freezes or panics on close.

A `close of closed channel` panic on `Close()` was fixed in v1.7.0. Upgrade if you're hitting this.
