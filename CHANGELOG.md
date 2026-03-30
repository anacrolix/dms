# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added
- Justfile with `run` and `browse-root` recipes for development convenience

### Changed
- Migrated logging from `anacrolix/log` to standard library `log/slog`
- HTTP listener now uses `net.ParseIP` to correctly select `tcp4` or `tcp6` network

### Fixed
- `allowedIps` config file parsing bug (#178)
- `::` in `-http` argument now correctly produces an IPv6 socket (#171)

---

## [v1.7.2] — 2025-06-17

### Added
- IPv6 support for SSDP (#162)
- Samsung TV fast-forward/rewind support (#169)
- `deviceIconSizes` flag for configuring device icon sizes (e.g. `"48:512,128:512"`) (#146)
- Example JSON config file

### Fixed
- Memory leak from `time.After` — replaced with proper timer management
- Slow browse performance on large directories caused by directory walk and ffprobe during `childCount` calculation
- `ssdp.Server.Serve` not returning when closed
- Docker image improvements (#168)

---

## [v1.7.1] — 2024-07-04

### Fixed
- Concurrent map read/write race condition

### Changed
- Config file is now loaded before config vars are dumped to logs (#145)

---

## [v1.7.0] — 2024-06-16

### Added
- `-ignore` flag to skip named directories during browsing (#139)
- Dynamic audio streams support (#137)

### Fixed
- SSDP panic with `Int63n` when delay time calculation overflowed (#136)
- SSDP panic when client reports `MX` value of 0 (#124)
- Security fix: bump `golang.org/x/net` to address CVE (#141)

### Changed
- Empty directories are no longer shown in browse results
- Confirmed compatibility with Apple TV 4K and iOS VLC (#127)

---

## [v1.6.0] — 2023-05-10

### Added
- Live streams generated on the fly via `.dms.json` config files (#112)
- Docker image and corresponding GitHub Action (#113)
- Thumbnail parameters configurable via environment variables (#114)
- `content-disposition` header on media responses (#115)
- `-transcodeLogPattern` flag to customize transcode log location (#117)
- Samsung Frame TV compatibility (#110)

### Fixed
- Log level for ignored media files reduced to debug, reducing noise (#121)

---

## [v1.5.0] — 2022-07-06

### Added
- Docker image published via GitHub Actions
- `OnBrowseDirectChildren` and `OnBrowseMetadata` optional interfaces for custom browse handling (#95)
- IP filter callback for SSDP (#96)
- Subtitle support (#79)

### Fixed
- Uninitialized logger on SSDP error (#94)

---

## [v1.4.0] — 2022-01-26

### Added
- Required `connectionManagerService` and `mediaReceiverRegistrarService` UPnP service implementations (#86)
- `audio/ogg` MIME type support (#87)
- Default device icon embedded via `go:embed`
- goreleaser-based release workflow

### Fixed
- Race condition when serving device icons
- Multicast loopback disabled to prevent duplicate SSDP packets
- Skip notifying on link-local unicast addresses

---

## [v1.3.0] — 2021-09-09

### Fixed
- Correct `childCount` reported for containers (affected browse performance on some clients)
- SOAP response elements sorted for spec compliance
- `dc:title` marshalled first in DIDL-Lite responses

---

## [v1.2.2] — 2021-03-22

### Fixed
- Support IPv6 host names with the `address&zone` form

---

## [v1.2.1] — 2021-03-22

### Fixed
- Build constraints for Unix targets

---

## [v1.2.0] — 2020-12-21

### Added
- Initial Dockerfile
- Sample systemd `.service` file
- Web browser compatible transcoding scheme
- Logging of the browse root path

### Fixed
- Images no longer transcoded when transcoding is forced to a specific format
- `SoapAction` header validation tightened

---

## [v1.1.0] — 2019-11-07

### Added
- IPv6 support
- `-force-transcode-to` CLI flag
- CIDR notation support for IP allowlists (#59)
- IP whitelisting mechanism
- Configurable device icon via `-deviceIcon` flag

### Fixed
- Chromecast transcode (#61)
- Race condition in server `Close`

---

## [v1.0.0] — 2019-04-16

### Added
- Go modules support
- Initial tagged release

[Unreleased]: https://github.com/anacrolix/dms/compare/v1.7.2...HEAD
[v1.7.2]: https://github.com/anacrolix/dms/compare/v1.7.1...v1.7.2
[v1.7.1]: https://github.com/anacrolix/dms/compare/v1.7.0...v1.7.1
[v1.7.0]: https://github.com/anacrolix/dms/compare/v1.6.0...v1.7.0
[v1.6.0]: https://github.com/anacrolix/dms/compare/v1.5.0...v1.6.0
[v1.5.0]: https://github.com/anacrolix/dms/compare/v1.4.0...v1.5.0
[v1.4.0]: https://github.com/anacrolix/dms/compare/v1.3.0...v1.4.0
[v1.3.0]: https://github.com/anacrolix/dms/compare/v1.2.2...v1.3.0
[v1.2.2]: https://github.com/anacrolix/dms/compare/v1.2.1...v1.2.2
[v1.2.1]: https://github.com/anacrolix/dms/compare/v1.2.0...v1.2.1
[v1.2.0]: https://github.com/anacrolix/dms/compare/v1.1.0...v1.2.0
[v1.1.0]: https://github.com/anacrolix/dms/compare/v1.0.0...v1.1.0
[v1.0.0]: https://github.com/anacrolix/dms/releases/tag/v1.0.0
