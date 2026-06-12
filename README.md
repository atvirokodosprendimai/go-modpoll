# modpoll (Go)

A Go rewrite of [gavinying/modpoll](https://github.com/gavinying/modpoll). The
tool polls Modbus TCP/UDP/RTU/ASCII devices and forwards the decoded data over
[NATS](https://nats.io) subjects (the original tool used MQTT topics).

## Architecture

| Diagram | What it shows |
| ------- | ------------- |
| ![System architecture](docs/architecture.svg) | Modbus devices on the left, the modpoll process in the middle, NATS and consumers on the right. |
| ![Package layering](docs/packages.svg) | DDD layering: `domain` is pure Go and depends on nothing; adapters wrap I/O; `main.go` is the entry point. |
| ![Flow](docs/flow.svg) | One poll cycle plus an inbound write-command, as a sequence across the lanes. |

## Features

- Modbus master over TCP, UDP, RTU and ASCII (framer auto-selected per
  transport).
- CSV-driven device, poller and reference configuration — loaded from a local
  file or HTTP(S) URL.
- Endian-aware decoder for `uint16/int16/uint32/int32/uint64/int64/float32/float64`,
  `bool/bool8/bool16` and fixed-width `stringNN`. Bit references (`addr:bit`)
  on holding/input registers are supported.
- Publishes JSON payloads on per-device NATS subjects. Subscribes to a
  wildcard subject for write commands.
- Optional local JSON export, periodic diagnostics, daemon mode for headless
  use.

## Install

### Pre-built binaries

The `release` GitHub Actions workflow builds binaries for `linux/amd64`,
`linux/arm64`, `linux/arm` (Raspberry Pi 2+), `darwin/amd64`, `darwin/arm64`
and `windows/amd64`.

- **Tag pushes (`vX.Y.Z`)** first run the `docker` workflow; once it finishes
  successfully `release` is triggered (via `workflow_run`) and publishes all
  archives to the matching GitHub Release alongside a `SHA256SUMS` file. This
  serialisation keeps the two workflows from racing for runner capacity.
- **Manual runs** (`workflow_dispatch`) build the matrix on demand and store
  the archives as workflow artifacts on the run page — handy for grabbing a
  build off `main` without cutting a release.

```bash
# Linux amd64 example
curl -L -o modpoll.tar.gz \
  https://github.com/atvirokodosprendimai/go-modpoll/releases/latest/download/modpoll_vX.Y.Z_linux_amd64.tar.gz
tar -xzf modpoll.tar.gz
./modpoll-linux-amd64/modpoll --version
```

### From source

```bash
cd new/
go build -o modpoll .
```

`go test ./...` runs the unit tests for decoding, the CSV loader, the
subject helpers and the HTTP exporter.

## CLI reference

`modpoll --help` lists every flag. The table below groups them by purpose.

### General

| Flag                         | Default  | Description                                             |
| ---------------------------- | -------- | ------------------------------------------------------- |
| `--config, -f` *(required)*  | —        | Path or URL of the Modbus CSV config. Repeat for many.  |
| `--once, -1`                 | `false`  | Run a single poll cycle and exit.                       |
| `--daemon, -d`               | `false`  | Suppress the per-cycle result table.                    |
| `--rate, -r`                 | `10.0`   | Sampling rate in seconds.                               |
| `--interval`                 | `0.5`    | Pause (seconds) between two pollers in the same cycle.  |
| `--delay`                    | `0`      | Seconds to wait before the first poll.                  |
| `--export, -o`               | —        | Write decoded data to this JSON file each cycle.        |
| `--export-http`              | —        | POST decoded data as JSON to this URL each cycle.       |
| `--export-http-timeout`      | `10.0`   | Timeout (seconds) for `--export-http` POST requests.    |
| `--timestamp`                | `false`  | Add an RFC3339 timestamp to every payload/export row.   |
| `--diagnostics-rate`         | `0`      | Seconds between diagnostics publishes (0 disables).     |
| `--autoremove`               | `false`  | Disable a poller after 3 consecutive failures.          |
| `--loglevel`                 | `INFO`   | `DEBUG`, `INFO`, `WARN` or `ERROR`.                     |

### Modbus transport

Exactly one of `--tcp`, `--udp`, `--serial` (alias `--rtu`) is required.

| Flag                                          | Default  | Description                                  |
| --------------------------------------------- | -------- | -------------------------------------------- |
| `--tcp <host>`                                | —        | Modbus TCP host.                             |
| `--tcp-port`                                  | `502`    | Modbus TCP port.                             |
| `--udp <host>`                                | —        | Modbus UDP host.                             |
| `--udp-port`                                  | `502`    | Modbus UDP port.                             |
| `--serial <port>` (alias `--rtu`)             | —        | Serial device (e.g. `/dev/ttyUSB0`) or URL.  |
| `--serial-baud` (alias `--rtu-baud`)          | `9600`   | Serial baud rate.                            |
| `--serial-parity` (alias `--rtu-parity`)      | `none`   | `none`, `odd` or `even`.                     |
| `--timeout`                                   | `3.0`    | Modbus response timeout (seconds).           |
| `--framer`                                    | `default`| `default`, `ascii`, `rtu` or `socket`. Validated against the transport. |

### NATS

`--nats-url` is the master switch — if it is left empty modpoll runs locally
(prints/exports only). The URL can also be supplied via the `NATS_URL`
environment variable.

| Flag                                       | Default                            | Description                                              |
| ------------------------------------------ | ---------------------------------- | -------------------------------------------------------- |
| `--nats-url`                               | — (env `NATS_URL`)                 | NATS connection URL.                                     |
| `--nats-name`                              | `modpoll`                          | Client name reported to the server.                      |
| `--nats-user`                              | —                                  | Username.                                                |
| `--nats-pass`                              | —                                  | Password.                                                |
| `--nats-token`                             | —                                  | Auth token.                                              |
| `--nats-creds`                             | —                                  | Path to a NATS credentials file.                         |
| `--nats-tls`                               | `false`                            | Connect over TLS.                                        |
| `--nats-publish-subject-pattern`           | `modpoll.{device}.data`            | `{device}` is replaced with each device name.            |
| `--nats-subscribe-subject-pattern`         | `modpoll.*.set`                    | NATS wildcard; the `*` token is read as the device name. |
| `--nats-diagnostics-subject-pattern`       | `modpoll.{device}.diagnostics`     | Diagnostics subject pattern.                             |
| `--nats-single`                            | `false`                            | Publish each reference on its own subject.               |

## NATS subjects

Default mapping (all configurable via CLI flags):

| Direction | Subject pattern                            |
| --------- | ------------------------------------------ |
| publish   | `modpoll.<device>.data`                    |
| publish   | `modpoll.<device>.diagnostics`             |
| subscribe | `modpoll.*.set` (wildcard = device name)   |

When `--nats-single` is set, each reference is published on its own subject:

```
modpoll.<device>.data.<reference_name>
```

### Write commands

Publish a JSON message on the subscribe subject to write a Modbus value:

```bash
# Single holding register
nats pub 'modpoll.modsim01.set' \
  '{"object_type":"holding_register","address":40001,"value":12}'

# Multiple holding registers starting at 40001
nats pub 'modpoll.modsim01.set' \
  '{"object_type":"holding_register","address":40001,"value":[12,13,14,15]}'

# Single coil
nats pub 'modpoll.modsim01.set' \
  '{"object_type":"coil","address":0,"value":true}'
```

Supported `object_type` values: `coil`, `holding_register`.

For `holding_register` the `value` field is auto-detected:
- A scalar number → `WriteSingleRegister` (FC6).
- An array of numbers → `WriteMultipleRegisters` (FC16) starting at `address`.

Writes acquire the Modbus client briefly, so they interleave cleanly with
ongoing polling.

## CSV configuration

Each row is one of `device`, `poll` or `ref`. Lines starting with `#` and
blank lines are ignored.

```csv
# device,<name>,<unit_id>
# poll,<object_type>,<start_address>,<size>,<endian>
# ref,<name>,<address>,<dtype>,<rw>,<unit>,<scale>

device,modsim01,1,,
poll,coil,0,16,BE_BE
ref,coil01-08,0,bool8,rw
ref,coil09-16,1,bool8,rw
poll,holding_register,40000,44,BE_BE
ref,holding_reg01,40000,uint16,rw
ref,holding_reg10,40010,uint32,rw,,0.001
ref,holding_reg13,40016,float32,rw
ref,holding_reg19,40036,string16,rw
ref,alarm_bit_15,40019:15,bool,r,
```

Field reference:

- `<object_type>`: `coil`, `discrete_input`, `holding_register`, `input_register`.
- `<endian>`: `BE_BE` (default), `LE_BE`, `LE_LE`, `BE_LE` —
  *<byte_order>_<word_order>*.
- `<dtype>`: `uint16`, `int16`, `uint32`, `int32`, `uint64`, `int64`,
  `float32`, `float64`, `bool`, `bool8`, `bool16`, `stringN` (N = byte length).
- `<rw>`: `r`, `w` or `rw`. Anything containing `r` is polled.
- `<unit>` *(optional)*: free-form text included as `name|unit` in publishes.
- `<scale>` *(optional)*: float multiplier applied to numeric values.
- `<address>:bit` syntax: extract a single bit from a holding/input register.
  Only valid with `dtype=bool` (bit index 0..15, LSB-first).

See `examples/modsim.csv` and `examples/config_template.csv` for full samples.

## Examples

### Smallest possible run

```bash
./modpoll --once --tcp 127.0.0.1 --tcp-port 5020 \
  --config examples/modsim.csv
```

### Continuous polling

```bash
./modpoll --tcp modsim.topmaker.net \
  --rate 5 \
  --config examples/modsim.csv
```

### Poll a Modbus TCP device on a non-standard port

```bash
./modpoll --tcp 192.168.1.10 --tcp-port 1502 \
  --config examples/modsim.csv
```

### Poll a Modbus UDP device

```bash
./modpoll --udp 192.168.1.10 --udp-port 502 \
  --config examples/modsim.csv
```

### Poll a serial RTU device

```bash
./modpoll --serial /dev/ttyUSB0 \
  --serial-baud 19200 --serial-parity even \
  --config examples/modsim.csv
```

### Poll a serial ASCII device (explicit framer)

```bash
./modpoll --serial /dev/ttyUSB0 --framer ascii \
  --serial-baud 9600 \
  --config examples/modsim.csv
```

### Serial-over-TCP tunnel (rfc2217 / socket URL)

```bash
./modpoll --serial 'socket://gateway.local:7000' --framer rtu \
  --config examples/modsim.csv
```

### RTU over TCP (raw RTU frames in a TCP socket)

When a serial gateway speaks raw RTU frames over a TCP socket (no Modbus TCP
MBAP header), pass the `rtuovertcp://` URL directly to `--serial`. The library
detects the scheme and skips the local serial port driver entirely — no
socat needed.

```bash
./modpoll --serial 'rtuovertcp://192.168.255.1:5014' \
  --config examples/modsim.csv
```

This is different from `--tcp` (which speaks Modbus TCP) and from
`socket://` (which is a serial-over-TCP tunnel with rfc2217 negotiation).

### Virtual COM port (socat pty bound to a TCP gateway)

If another tool needs a real device file, expose the TCP gateway as a
pseudo-terminal with `socat` and point `--serial` at the pty path:

```bash
# In one terminal — keep socat running
socat -d -d -x pty,link=/tmp/com5014,raw tcp:192.168.255.1:5014

# In another terminal — poll through the pty
./modpoll --serial /tmp/com5014 --serial-baud 9600 \
  --config examples/modsim.csv
```

Internally modpoll wraps the path as `rtu:///tmp/com5014`. Prefer the
`rtuovertcp://` form above when only modpoll needs the link — fewer moving
parts and no extra process.

### Multiple RTU slaves on one serial bus

A single serial transport carries many slaves; each gets its own `device`
row with a distinct `unit_id`. modpoll polls them sequentially inside one
cycle (separated by `--interval`).

```csv
device,meter_a,1,,
poll,holding_register,40000,2,BE_BE
ref,kwh,40000,uint32,r,kWh

device,meter_b,2,,
poll,holding_register,40000,2,BE_BE
ref,kwh,40000,uint32,r,kWh
```

```bash
./modpoll --serial /dev/ttyUSB0 --serial-baud 9600 \
  --interval 0.25 --config bus.csv
```

### Poll discrete inputs and input registers (read-only registers)

```csv
device,plc01,1,,
poll,discrete_input,10000,16,BE_BE
ref,door_open,10000,bool,r
ref,alarm_block,10001,bool8,r

poll,input_register,30000,4,BE_BE
ref,temperature,30000,float32,r,degC,0.1
ref,humidity,30002,uint16,r,pct
```

`discrete_input` (FC2) and `input_register` (FC4) are read-only on the
Modbus side. `w` / `rw` flags on their refs are accepted but a write command
will fail — use `holding_register` (FC3) or `coil` (FC1) for writable points.

### Bit-pack a status word (`addr:bit` syntax)

A single register often holds 16 independent flags. Decode each bit as its
own boolean reference:

```csv
device,inverter,1,,
poll,holding_register,40020,1,BE_BE
ref,fault_overcurrent,40020:0,bool,r
ref,fault_overtemp,40020:1,bool,r
ref,run_state,40020:8,bool,r
ref,grid_ok,40020:15,bool,r
```

Bit index is `0..15`, LSB-first. Only `dtype=bool` is allowed with `:bit`.
Works on both `holding_register` and `input_register`.

### Endian variants (byte order × word order)

| String  | Byte order | Word order | Used by                                                            |
| ------- | ---------- | ---------- | ------------------------------------------------------------------ |
| `BE_BE` | big        | big        | Most PLCs / Siemens / Schneider                                    |
| `LE_BE` | little     | big        | Some inverter firmwares                                            |
| `BE_LE` | big        | little     | **Default.** EPEVER, many power meters, HVAC controllers           |
| `LE_LE` | little     | little     | Rare; some legacy gateways                                         |
| `BE`    | big        | little     | Short form of `BE_LE`                                              |
| `LE`    | little     | little     | Short form of `LE_LE`                                              |
| *empty* | big        | little     | Same as `BE_LE` — matches python `modpoll`'s pymodbus default       |

Set per-poller in the CSV:

```csv
poll,holding_register,40000,4,BE_LE
ref,energy_total,40000,uint32,r,Wh
```

### Scale factor and engineering units

Multiply raw integers into engineering units and tag the published payload
with a unit suffix (`name|unit`):

```csv
ref,current,40000,uint16,r,A,0.01     # 0..65535 → 0.00 .. 655.35 A
ref,energy,40010,uint32,r,kWh,0.001   # raw Wh → kWh
```

Numeric refs apply `value * scale` in Go; booleans, strings and bit refs
ignore scale.

### Fixed-length strings (`stringNN`)

`stringNN` decodes NN bytes from consecutive registers (2 bytes per
register), trims trailing NULs, and emits a Go string. Provide enough
poller size to cover the bytes — width is `ceil(NN / 2)` registers.

```csv
poll,holding_register,40050,8,BE_BE
ref,serial_no,40050,string16,r
ref,firmware,40058,string8,r
```

### Hex and binary addresses in CSV

Addresses, unit IDs, and poller starts accept Go's standard numeric
prefixes — handy when datasheets quote registers in hex:

```csv
device,plc02,0x01,,
poll,holding_register,0x9C40,4,BE_BE
ref,setpoint,0x9C40,uint16,rw
```

`0x...` (hex), `0b...` (binary) and `0o...` (octal) all parse. Bit indices
after `:` are decimal only.

### Initial delay before the first poll

Useful when launched at boot before a serial gateway is fully up:

```bash
./modpoll --delay 10 \
  --tcp 192.168.1.10 \
  --config examples/modsim.csv
```

### Slow link / tune `--interval` between pollers

`--interval` is the pause between *two pollers in the same cycle* (e.g.
between slaves on an RTU bus, or between two register blocks on the same
device). `--rate` is the time between full cycles.

```bash
./modpoll --rate 30 --interval 1.5 \
  --serial /dev/ttyUSB0 --serial-baud 9600 \
  --config bus.csv
```

### Publish to a local NATS server

```bash
./modpoll --tcp modsim.topmaker.net \
  --config examples/modsim.csv \
  --nats-url nats://127.0.0.1:4222

# In another terminal:
nats sub 'modpoll.*.data'
```

### Publish with custom subject patterns

```bash
./modpoll --tcp modsim.topmaker.net \
  --config examples/modsim.csv \
  --nats-url nats://127.0.0.1:4222 \
  --nats-publish-subject-pattern 'site.factory1.{device}.tlm' \
  --nats-diagnostics-subject-pattern 'site.factory1.{device}.health' \
  --nats-subscribe-subject-pattern 'site.factory1.*.write'
```

### Publish each reference on its own subject

```bash
./modpoll --tcp modsim.topmaker.net \
  --config examples/modsim.csv \
  --nats-url nats://127.0.0.1:4222 \
  --nats-single

# Each reference lands on:  modpoll.<device>.data.<reference_name>
nats sub 'modpoll.modsim01.data.>'
```

### NATS URL via environment variable

The `--nats-url` flag also reads from the `NATS_URL` env var, which keeps
secrets out of process listings and is handy in containers:

```bash
export NATS_URL=nats://broker.example.com:4222
./modpoll --tcp 192.168.1.10 --config examples/modsim.csv
```

### Custom client name in NATS monitoring

`--nats-name` controls how the modpoll process appears in `nats server
connections` output and `nats-top`:

```bash
./modpoll --nats-url nats://broker:4222 --nats-name modpoll-site42 \
  --tcp 192.168.1.10 --config examples/modsim.csv
```

### Authenticated NATS connections

```bash
# User/password
./modpoll --nats-url nats://broker.example.com:4222 \
  --nats-user alice --nats-pass s3cret \
  --tcp 192.168.1.10 --config examples/modsim.csv

# Token
./modpoll --nats-url nats://broker.example.com:4222 \
  --nats-token mY-T0Ken \
  --tcp 192.168.1.10 --config examples/modsim.csv

# NATS credentials file (operator/JWT)
./modpoll --nats-url tls://connect.ngs.global:4222 \
  --nats-creds /etc/modpoll/nats.creds \
  --tcp 192.168.1.10 --config examples/modsim.csv
```

### Use TLS

```bash
./modpoll --nats-url tls://broker.example.com:4222 --nats-tls \
  --tcp 192.168.1.10 --config examples/modsim.csv
```

### Add timestamps to every payload

```bash
./modpoll --tcp modsim.topmaker.net \
  --config examples/modsim.csv \
  --nats-url nats://127.0.0.1:4222 \
  --timestamp
```

### Periodic diagnostics

```bash
./modpoll --tcp modsim.topmaker.net \
  --config examples/modsim.csv \
  --nats-url nats://127.0.0.1:4222 \
  --diagnostics-rate 60

nats sub 'modpoll.*.diagnostics'
```

Diagnostics payload:

```json
{ "poll_count": 42, "error_count": 0, "last_poll_success": true }
```

### Auto-disable broken pollers

```bash
./modpoll --tcp 192.168.1.10 \
  --config examples/modsim.csv \
  --autoremove
```

Any poller that fails three cycles in a row is marked disabled for the rest
of the process lifetime.

### Daemon mode (no console table)

```bash
./modpoll -d \
  --tcp modsim.topmaker.net \
  --config examples/modsim.csv \
  --nats-url nats://127.0.0.1:4222
```

### Export decoded data to a JSON file

```bash
./modpoll --tcp modsim.topmaker.net \
  --export data.json \
  --config examples/modsim.csv
```

### POST decoded data to an HTTP endpoint

Each successful poll cycle results in one `POST` of the same JSON snapshot
that `--export` would write to disk. Non-2xx responses are logged as warnings
(the loop keeps running).

```bash
./modpoll --tcp modsim.topmaker.net \
  --export-http https://ingest.example.com/api/modpoll \
  --export-http-timeout 5 \
  --timestamp \
  --config examples/modsim.csv
```

Body shape (compact JSON):

```json
{
  "modsim01": {
    "holding_reg01": 1234,
    "holding_reg13": 3.14,
    "timestamp": "2026-05-20T08:42:11.123Z"
  }
}
```

`--export` and `--export-http` can be set together — they share the same
snapshot.

### Run all three sinks at once (file + HTTP + NATS)

The publisher, file exporter and HTTP poster are independent. Combine them
to mirror data to multiple consumers in one pass:

```bash
./modpoll --tcp 192.168.1.10 \
  --config examples/modsim.csv \
  --rate 5 --timestamp \
  --nats-url nats://broker:4222 \
  --export /var/lib/modpoll/latest.json \
  --export-http https://ingest.example.com/api/modpoll \
  --diagnostics-rate 60
```

### One-shot snapshot to a file

Combine `--once` with `--export` for a cron-friendly snapshot:

```bash
./modpoll --once --tcp 192.168.1.10 \
  --config examples/modsim.csv \
  --export /tmp/snapshot.json --timestamp
```

### Load multiple config files / multiple devices

```bash
./modpoll --tcp modsim.topmaker.net \
  --config examples/modsim.csv \
  --config examples/modsim2.csv
```

### Load a config from a URL

```bash
./modpoll --tcp modsim.topmaker.net \
  --config https://raw.githubusercontent.com/gavinying/modpoll/main/examples/modsim.csv
```

### Increase Modbus timeout / slow link

```bash
./modpoll --tcp slow.gateway.local --timeout 10 \
  --interval 1.5 \
  --config examples/modsim.csv
```

### Verbose logging

```bash
./modpoll --loglevel DEBUG \
  --tcp modsim.topmaker.net \
  --config examples/modsim.csv
```

### Write a value via NATS

```bash
# In one terminal, run modpoll connected to NATS
./modpoll --tcp 192.168.1.10 \
  --config examples/modsim.csv \
  --nats-url nats://127.0.0.1:4222

# In another terminal, send a write command
nats pub 'modpoll.modsim01.set' \
  '{"object_type":"holding_register","address":40001,"value":42}'
```

### Resilience and reconnect behaviour

- **NATS** reconnects automatically forever (`MaxReconnects = -1`,
  `ReconnectWait = 2s`). In-flight publishes during a disconnect are
  buffered by the NATS client; on shutdown modpoll calls `Drain` with a
  5-second budget to flush them.
- **Modbus** the client is opened lazily on every poll cycle and closed at
  the end, so a transient TCP/serial drop only loses the cycle in flight.
- **Per-poller backoff** with `--autoremove`: three consecutive failures
  disable that poller for the rest of the process lifetime (others keep
  running). Restart modpoll to re-enable.

### Run inside systemd

```ini
# /etc/systemd/system/modpoll.service
[Unit]
Description=modpoll Modbus->NATS bridge
After=network-online.target

[Service]
Environment=NATS_URL=nats://broker.internal:4222
ExecStart=/usr/local/bin/modpoll -d \
  --tcp 192.168.1.10 \
  --config /etc/modpoll/site.csv \
  --delay 5 --rate 5 --autoremove --diagnostics-rate 60
Restart=always
RestartSec=5
User=modpoll

[Install]
WantedBy=multi-user.target
```

## Docker

A multi-stage `Dockerfile` (distroless static, non-root) lives at the project
root.

```bash
# Build locally
docker build -t modpoll:dev .

# Run once against a public test device
docker run --rm modpoll:dev --once \
  --tcp modsim.topmaker.net \
  --config https://raw.githubusercontent.com/gavinying/modpoll/main/examples/modsim.csv
```

Published images: a GitHub Actions workflow (`.github/workflows/docker.yml`)
publishes multi-arch (`linux/amd64`, `linux/arm64`) images to GitHub Container
Registry **only when a `vX.Y.Z` git tag is pushed**. Each release is tagged
with `:latest`, `:X.Y.Z`, `:X.Y` and `:X`:

```bash
# Release flow
git tag v0.2.0
git push origin v0.2.0

# After the workflow finishes:
docker pull ghcr.io/<org>/modpoll:latest
docker pull ghcr.io/<org>/modpoll:0.2.0
```

Pre-release tags (e.g. `v0.2.0-rc1`) build the image but do not move the
`:latest` tag.

## Project layout

The package layout follows a small Domain-Driven layout:

```
new/
├── main.go                      CLI entry point (urfave/cli/v3)
├── examples/                    Sample CSV configs
└── internal/
    ├── domain/                  Pure value objects + decoders (no I/O)
    ├── config/                  CSV loader (local file or HTTP URL)
    ├── modbus/                  Modbus master facade
    ├── messaging/               NATS publisher / write-command subscriber
    ├── poller/                  Polling service + result printer
    └── exporter/                JSON file exporter
```

## Tests

```bash
go test ./...
```

The unit tests focus on the decoding behaviour (endianness, bit extraction,
strings, scale), the CSV loader, and the NATS subject helpers.
