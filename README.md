# lazycontainer

![Lazy Container](./.gitub/inmage)

A [lazydocker](https://github.com/jesseduffield/lazydocker)-style terminal UI for Apple's
[`container`](https://github.com/apple/container) CLI on macOS.

```
╭─ Status ──────────────────────╮╭─ web · Logs │ Stats │ Config ───────────────────────╮
│● running  container 1.3.1     ││2026-10-19T12:00:01Z GET / 200 3ms                    │
│images 21.6 GB · 20.1 GB recl… ││2026-10-19T12:00:02Z GET /health 200 1ms              │
│containers 8.1 GB · volumes 0 B││…                                                     │
╰───────────────────────────────╯│                                                      │
╭─ Containers (2) ──────────────╮│                                                      │
│● web      running nginx:latest││                                                      │
│● worker   stopped alpine      ││                                                      │
╰───────────────────────────────╯│                                                      │
╭─ Images (6) ──────────────────╮│                                                      │
…
```

## Install

```sh
go install github.com/owenrumney/container-tui/cmd/lazycontainer@latest
```

Requires the `container` CLI on your PATH with its services running (`container system start`).

## Keys

| Key             | Action                                                                                   |
| --------------- | ---------------------------------------------------------------------------------------- |
| `↑`/`k` `↓`/`j` | move selection                                                                           |
| `tab` / `1`–`4` | switch panel (Containers, Images, Volumes, Networks)                                     |
| `[` / `]`       | previous / next detail tab (Logs, Stats, Config)                                         |
| `enter` / `esc` | focus / leave the detail pane for scrolling                                              |
| `s` / `S`       | stop / start container                                                                   |
| `r`             | restart container                                                                        |
| `K`             | kill container                                                                           |
| `e`             | open a shell in the container (bash if present, else sh)                                 |
| `d`             | delete container, image, volume or network (confirms first)                              |
| `p` / `P`       | prune stopped containers, dangling images (`P` = all unused), unused volumes or networks |
| `R`             | refresh now                                                                              |
| `?`             | help                                                                                     |
| `q`             | quit                                                                                     |

Lists refresh every two seconds. Logs follow live. Stats sample every two seconds while the Stats tab is open.

## Development

```sh
go test ./...
go test ./... -tags integration -v   # drives the real container CLI
go run ./cmd/lazycontainer
```
