# pixsurf

Surf the web in your terminal — no browser install, no JavaScript bloat.

pixsurf is a terminal web browser with its own pure-Go rendering engine:
it fetches pages, lays them out with colors and styles, draws images as
truecolor half-block pixels, and makes links clickable — all in a single
static binary. No Chrome, no Chromium, no dependencies. VPS-friendly:
starts instantly, uses a few dozen MB of RAM.

No JavaScript by design: content sites (news, docs, wikis, blogs) render
great; app-style sites that require JS will show their static content only.

## Requirements

- A terminal with truecolor support (most modern terminals)

That's it.

## Install

**Prebuilt binary (recommended):** grab the archive for your OS/arch from the
[latest release](https://github.com/MhmdShd/pixsurf/releases/latest), unpack,
and put `pixsurf` on your PATH. Linux, macOS, and Windows, amd64 + arm64.

**With Go:**

```sh
go install github.com/MhmdShd/pixsurf@latest
```

## Usage

```sh
pixsurf wikipedia.org
pixsurf --no-images news.ycombinator.com   # text only, minimal bandwidth
```

Site search works: click a search box, type, Enter (GET forms).

Images load asynchronously — pages appear instantly and images fill in.

## Controls

| Key | Action |
|-----|--------|
| ↑ / ↓ | scroll |
| PgUp / PgDn | scroll a page |
| mouse click | follow link |
| g | enter a URL |
| b / f | back / forward |
| r | reload |
| q | quit |

## How it works

Pure-Go pipeline: HTTP fetch (capped, cookie-aware) → HTML parse
(`x/net/html`) → semantic styling → flow layout at your terminal width →
styled cells drawn with tcell. Images are downscaled to `▀` half-blocks
with 24-bit color.

## License

MIT
