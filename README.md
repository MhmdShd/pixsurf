# pixsurf

Surf the web in pixels — a real web browser inside your terminal.

pixsurf drives a headless Chrome/Chromium instance and renders live page
screenshots as truecolor half-block "pixels" in your terminal. Full modern
web — JavaScript included — no GUI needed.

## Requirements

- A terminal with truecolor support (most modern terminals)
- Chrome or Chromium installed

## Install

```sh
go install github.com/MhmdShd/pixsurf@latest
```

## Usage

```sh
pixsurf wikipedia.org
```

## Controls

| Key | Action |
|-----|--------|
| ↑ / ↓ | scroll |
| PgUp / PgDn | scroll a page |
| mouse click | click links |
| g | enter a URL |
| b / f | back / forward |
| r | reload |
| q | quit |

## How it works

Headless Chrome renders the page at 1280px wide → screenshot → downscaled to
your terminal grid → each character cell becomes two pixels using `▀` with
24-bit foreground/background colors. Mouse clicks map back from cells to page
coordinates.

## License

MIT
