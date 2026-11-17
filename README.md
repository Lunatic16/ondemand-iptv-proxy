<div align="center">

# OnDemand IPTV

**A self-hosted IPTV playlist proxy for the OnDemand IPTV Service.**

One tiny Go binary that fetches your M3U playlist, rewrites every stream URL
to flow through it, repairs the quirks that make players choke — and serves a
fast web app with **multiview** (up to six live streams at once) right in your browser.

[![Go](https://img.shields.io/badge/Go-1.23%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-4f9dff.svg)](#-license)
[![Dependencies](https://img.shields.io/badge/dependencies-none-30a46c.svg)](#-how-it-works)
[![Platform](https://img.shields.io/badge/platform-linux%20%7C%20macOS%20%7C%20windows-lightgrey.svg)](#-quick-start)

<img src="docs/screenshot.png" alt="OnDemand IPTV web UI — 2×2 multiview with four live streams, channel sidebar with favorites and search" width="840">

</div>

---

## Why OnDemand IPTV?

If you self-host IPTV, you know the pain: your provider hands you an `m3u`
whose URLs only work with specific headers, break in VLC or `mpv` because the
segments are disguised as images, and leak the upstream provider (and its
token) to every device you load the playlist on.

**ondemand-iptv-proxy sits in the middle.** Your player only ever sees your server.
ondemand-iptv-proxy fetches the upstream playlist, rewrites every URL — variants,
segments, encryption keys, alternate audio tracks — into proxied paths,
forwards the right headers, and patches up broken streams on the fly. Change
providers once, in one environment variable; nothing on the player side ever
changes.

## ✨ Features

### Proxy
- **🔬 Zero dependencies** — pure Go standard library, single static binary, web UI and player library embedded via `go:embed`.
- **🔁 Full playlist rewriting** — understands HLS and M3U structures: `#EXT-X-STREAM-INF` variants, media segments, `#EXT-X-MAP` init sections, `#EXT-X-KEY` decryption keys, `#EXT-X-MEDIA` alternate audio and I-frame playlists. Nested playlists are detected by content sniffing and rewritten recursively.
- **🩹 MPEG-TS repair** — detects segments that are really TS data behind a fake `.png`/`.webp` extension, locates the first valid sync byte (`0x47` + two more at +188/+376), strips the junk prefix, fixes `Content-Length` and reports an honest `video/mp2t` type. Fixes "invalid data" errors in `mpv`, VLC and ffmpeg.
- **🏷️ Honest file extensions** — proxied URLs get the extension players expect (`.m3u8` for variants, `.ts` for segments, `.key` for keys), instead of whatever the upstream guessed.
- **⚡ Caching with stale fallback** — the playlist is cached with a configurable TTL; if the upstream hiccups, you still get the last good copy instead of a dead playlist.
- **📡 Player-agnostic** — the rewritten `playlist.m3u` is a standard M3U; point VLC, `mpv`, ffmpeg, TiviMate, Kodi or any smart-TV app at it.
- **🧭 Reverse-proxy friendly** — auto-detects scheme/host from `X-Forwarded-Proto`, or pin it with `PUBLIC_BASE`. Honors `HTTP_PROXY`/`HTTPS_PROXY` for upstream egress.

### Web app
- **🔊 Multi-audio** — unmute any combination of tiles, or solo one with a click.
- **🪟 Multiview** — watch 1, 2, 3, 4 or 6 streams at once in seven layouts (see [below](#-multiview)). Audio follows the tile you select, and you can unmute as many tiles as you like to mix them.
- **🔎 Fast channel browser** — virtualized list that stays smooth with thousands of channels, multi-word search, group picker with counts, **Favorites** and **Recent** tabs.
- **🖱️ Drag & drop** — drop a channel on any tile to load it, drag a tile's title onto another tile to swap.
- **🎛️ Per-tile controls** — quality picker, reload, picture-in-picture, expand to full stage, fullscreen, close.
- **🔁 Self-healing playback** — automatic reconnect with backoff, media-error recovery, a stall watchdog, and a Retry button on failed streams.
- **💾 Remembers your setup** — layout, slots, favorites, recents, volume and sidebar state persist in the browser.
- **⌨️ Keyboard-first** — press `?` in the app for the full list.
- **📱 Responsive** — the sidebar becomes a drawer on phones, and wide layouts re-flow in portrait.
- **📴 Works offline on your LAN** — [hls.js](https://github.com/video-dev/hls.js) is vendored and served by the binary itself (no CDN), with native HLS fallback for Safari.

## 🪟 Multiview

Pick a layout from the toolbar:

| Layout        | Streams | Description                         |
| ------------- | :-----: | ----------------------------------- |
| Single        | 1       | One full-stage player               |
| Side by side  | 2       | Two streams left / right            |
| Stacked       | 2       | Two streams top / bottom            |
| Main + 2      | 3       | One large tile with two on the side |
| Quad          | 4       | 2 × 2 grid                          |
| Main + 3 strip| 4       | One large tile with a strip below   |
| Six           | 6       | 3 × 2 grid                          |

**Filling slots.** Click a tile to make it the *target* (empty targets pulse),
then click a channel — it loads there and the target advances to the next empty
slot. Or use the **+** button on a channel row, or drag a channel onto a tile.

**Audio.** Audible tiles get an accent-colored border and a speaker badge.
- **Click a tile** (or press its number key, or `←` / `→`) to *solo* it: that tile is audible and all others are muted. Clicking a tile that is already audible leaves your mix alone.
- **Click the 🔊 button** on a tile, **Shift+click** it, or press `Shift`+`1`–`6` to add or remove just that tile from the mix, so any number of streams can play sound at once.
- Press `A` to listen to every visible stream (press again to go back to one), and `M` to mute or restore everything.
- Volume is shared by all audible tiles.

**Switching layouts** never loses your channels: slots that don't fit the
current layout are paused (to save bandwidth) and resume when you switch back.
Players size their quality to each tile, so small tiles pull lower renditions.

### Keyboard shortcuts

| Key                 | Action                              |
| ------------------- | ----------------------------------- |
| `1`–`6`             | Select slot and solo its audio      |
| `Shift`+`1`–`6`     | Add / remove a slot from the audio mix |
| `A`                 | Listen to all streams / back to one |
| `←` `→`             | Previous / next slot                |
| `L` / `Shift+L`     | Cycle layout forward / back         |
| `E` (or double-click) | Expand slot to full stage         |
| `Esc`               | Back to the grid                    |
| `X`                 | Close the selected slot             |
| `M`                 | Mute / restore all audio            |
| `↑` `↓`             | Volume                              |
| `F` / `Shift+F`     | Slot fullscreen / multiview fullscreen |
| `/`                 | Focus search                        |
| `[`                 | Toggle channel list                 |
| `?`                 | Shortcut help                       |

> **Bandwidth note:** every visible tile is a separate live stream through the
> proxy. A six-up grid at 1080p each needs a lot of upstream and LAN bandwidth —
> use the per-tile quality picker if your connection is limited.

## 🚀 Quick start

### From source

```bash
git clone https://github.com/Lunatic16/ondemand-iptv-proxy.git
cd ondemand-iptv-proxy
go build -o ondemand-iptv-proxy .

UPSTREAM_URL="https://your-provider.example/playlist.m3u?token=YOUR_TOKEN" ./ondemand-iptv-proxy
# listening on :8080 (playlist TTL 1m0s)
```

Open **http://localhost:8080** for the web app, or hand
`http://your-host:8080/playlist.m3u` to any player.

### With Docker

```bash
docker compose up -d --build
```

`docker-compose.yml` ships with all environment variables pre-wired — edit the
`environment:` block and you're done. The final image is built `FROM scratch`
and stays small (the binary embeds the UI and the ~400 KB hls.js bundle).

## ⚙️ Configuration

Everything is configured through environment variables — no config files.

| Variable       | Default                          | Description                                                                                                                                     |
| -------------- | -------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| `UPSTREAM_URL` | *(demo endpoint — set your own)* | Full URL of the upstream `m3u`/`m3u8` playlist. Required.                                                                                        |
| `UPSTREAM_UA`  | Safari-style `Mozilla/5.0 …`     | `User-Agent` sent to the upstream for **all** playlist and stream requests. Set this if your provider rejects unknown clients.                   |
| `PLAYLIST_TTL` | `60`                             | Playlist cache lifetime **in seconds**. Lower = fresher EPG/channel data; higher = fewer upstream hits.                                          |
| `PORT`         | `8080`                           | HTTP listen port.                                                                                                                               |
| `PUBLIC_BASE`  | *(auto-detect)*                  | Public origin used when rewriting URLs, e.g. `https://tv.example.com`. Leave empty to derive from the request's `Host` + `X-Forwarded-Proto`.    |

> **Tip:** if you run ondemand-iptv-proxy behind nginx/Caddy/Traefik, set `PUBLIC_BASE`
> to the public URL so rewritten stream URLs point at the proxy, not at an
> internal address.

## 🔌 Endpoints

| Path                 | Method | Description                                              |
| -------------------- | ------ | -------------------------------------------------------- |
| `/`                  | GET    | Web app (embedded, single page)                           |
| `/hls.min.js`        | GET    | Vendored hls.js player library (embedded, cached 24 h)    |
| `/playlist.m3u`      | GET    | Upstream playlist with all URLs rewritten to this proxy   |
| `/playlist.m3u8`     | GET    | Alias of `/playlist.m3u` (HLS-friendly name)              |
| `/s/{id}`            | GET    | Proxied stream: playlists are re-rewritten, media is piped through |
| `/api/channels`      | GET    | JSON: channel list + playlist URL + cache age/TTL         |
| `/api/refresh`       | POST   | Invalidate the playlist cache and re-fetch                |
| `/healthz`           | GET    | Liveness probe, returns `ok`                              |

<details>
<summary><code>GET /api/channels</code> response shape</summary>

```json
{
  "playlist": "http://localhost:8080/playlist.m3u",
  "age_seconds": 12,
  "ttl_seconds": 60,
  "channels": [
    {
      "name": "Aurora News HD",
      "group": "News",
      "logo": "https://…/logo.png",
      "url": "http://localhost:8080/s/aHR0cHM6Ly9…"
    }
  ]
}
```

</details>

## 🧠 How it works

```text
   ┌──────────────┐   playlist.m3u      ┌────────────────────┐
   │ your player  │◄────────────────────│ ondemand-iptv-proxy│
   │ (VLC, mpv,   │   /s/<base64-url>   │  · fetch + cache   │────► upstream
   │ Kodi, web UI)│────────────────────►│  · rewrite URLs    │      provider
   └──────────────┘   every segment &   │  · repair streams  │────► upstream
                      key flows through │  · forward headers │      streams
                                        └────────────────────┘
```

**Playlist rewriting.** Every URL in the upstream playlist is resolved against
its base, base64url-encoded, and replaced with a `/s/<id>` path on your
server. Attribute URIs (`URI="…"` on keys, maps and alternate renditions) are
rewritten in place, so nothing escapes the proxy — your player never learns
the upstream host.

**Stream proxying.** `/s/` decodes the target, applies the configured
`User-Agent`, and streams the response with chunked flushing (long-lived
connections are fine — there is no write timeout). A client `Range` request is
applied locally after any junk prefix is stripped, so offsets stay correct. If
the response is itself a playlist, it is rewritten recursively.

**TS repair.** When a "segment" arrives with a bogus image content type or a
junk prefix before the MPEG-TS data, ondemand-iptv-proxy scans for a valid TS sync
pattern (`0x47` at offsets *i*, *i+188*, *i+376*), discards everything before
it, and adjusts the headers. Your player just sees clean video.

**Web app.** A single embedded `index.html` (no build step, no framework)
talks to `/api/channels` and plays streams with the vendored hls.js. Each
multiview tile owns its own player instance, so one failing stream never
affects the others.

## 🐳 Docker

<details>
<summary>Dockerfile (multi-stage, scratch-based)</summary>

```dockerfile
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod main.go index.html hls.min.js ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /iptvproxy .

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /iptvproxy /iptvproxy
EXPOSE 8080
USER 65534:65534
ENTRYPOINT ["/iptvproxy"]
```

</details>

<details>
<summary>docker-compose.yml</summary>

```yaml
services:
  iptvproxy:
    build: .
    container_name: ondemand-iptvproxy
    restart: unless-stopped
    ports:
      - "8090:8080"
    environment:
      # Address TiviMate uses to reach this box (LAN IP or hostname)
      #PUBLIC_BASE: "http://localhost:8090"
      UPSTREAM_URL: "https://your-provider.example/playlist.m3u?token=YOUR_TOKEN"
      UPSTREAM_UA: "Mozilla/5.0 OnDemandIPTV AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/604.1"
      PLAYLIST_TTL: "60"   # seconds between upstream playlist refreshes
```

</details>

## 🧩 Project layout

```text
main.go          Go proxy: playlist rewriting, TS repair, API, static serving
index.html       Web app (multiview, channel browser) — embedded in the binary
hls.min.js       Vendored hls.js 1.5.x — embedded in the binary
Dockerfile       Multi-stage scratch build
docker-compose.yml
docs/screenshot.png
```

To upgrade the player library, replace `hls.min.js` with a newer
`dist/hls.min.js` from the [hls.js releases](https://github.com/video-dev/hls.js/releases)
and rebuild.

## 🛡️ Notes & responsible use

- ondemand-iptv-proxy is a **caching reverse proxy for playlists you already have
  access to**. It ships with no content and no default provider — point it at
  streams you are authorized to use, and respect your provider's terms and
  your local law.
- The server has **no built-in authentication**, and `/s/` will fetch whatever
  URL it is given. Anyone who can reach it can watch through it and use it as a
  generic HTTP fetcher. Keep it on your LAN/VPN, or put authentication in front
  of it before exposing it to the internet.
- Keep your provider token out of version control — pass `UPSTREAM_URL` via an
  `.env` file or your orchestrator's secrets rather than committing it.
- Upstream 403s? Try adjusting `UPSTREAM_UA` — many providers whitelist
  specific client user agents.
- Browsers may block autoplay with sound until you interact with the page; if a
  tile starts muted, click it to unmute.
- Some streams use codecs a given browser cannot decode (e.g. HEVC/H.265 in
  Firefox). Those tiles show an error; use a desktop player via `/playlist.m3u`
  instead.

## 🤝 Contributing

Issues and pull requests are welcome. For non-trivial changes, please open an
issue first to discuss what you'd like to change.

## 📄 License

Released under the [MIT License](LICENSE).
