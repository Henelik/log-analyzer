#!/usr/bin/env python3
"""Generate a log-analyzer event CSV with brute-force, port-scan, and
exfiltration IOC patterns.

Environment:
  EVENT_COUNT    total data rows to write (default 10000); must hold the drawn
                 IOC blocks plus the invalid rows
  BRUTE_FORCE_COUNT
                 brute-force incidents to embed (default 3)
  PORT_SCAN_COUNT
                 port-scan incidents to embed (default 3)
  BRUTE_FORCE_MIN_MULTIPLIER
                 minimum failed logins per brute-force incident, as a multiple
                 of the brute-force threshold, 1 or greater (default 1)
  BRUTE_FORCE_MAX_MULTIPLIER
                 maximum failed logins per brute-force incident, as a multiple
                 of the brute-force threshold (default 100)
  PORT_SCAN_MIN_PORTS
                 minimum unique destination ports per port-scan incident, 16 or
                 greater (default 16)
  PORT_SCAN_MAX_PORTS
                 maximum unique destination ports per port-scan incident
                 (default 200)
  EXFIL_COUNT    data-exfiltration incidents to embed (default 3)
  EXFIL_MIN_MULTIPLIER
                 minimum request bytes per exfiltration incident, as a multiple
                 of the exfiltration threshold, 1 or greater (default 1)
  EXFIL_MAX_MULTIPLIER
                 maximum request bytes per exfiltration incident, as a multiple
                 of the exfiltration threshold (default 100)
  INVALID_RATIO  fraction of rows with invalid field data, 0..1 (default 0.05)
  DUPLICATE_RATIO
                 fraction of rows replaced by a verbatim copy of the previous
                 row, 0..1 (default 0.05); invalid rows are never duplicated
  OUTPUT_PATH    destination CSV path (default events.csv)
  RANDOM_SEED    PRNG seed (default 42)
  START_EPOCH    first event epoch second (default 1700000000); subsequent
                 events advance by a random 0..100 seconds each
"""

import csv
import os
import random
import sys
from dataclasses import dataclass
from pathlib import Path

HEADERS = (
    "epochtime",
    "login",
    "cip",
    "sip",
    "srv_dport",
    "url",
    "reqmethod",
    "respcode",
    "action",
    "appname",
    "reqdatasize",
    "respdatasize",
)

FIELD_INDEX = {name: index for index, name in enumerate(HEADERS)}

BRUTE_FORCE_THRESHOLD = 10
# The detector fires only when a failure bucket is larger than the threshold.
MIN_BRUTE_FORCE_ATTEMPTS = BRUTE_FORCE_THRESHOLD + 1
# Background activity stays at or below half the threshold so it never becomes
# an IOC on its own; only the drawn brute-force blocks exceed it.
MAX_BACKGROUND_FAILURES = BRUTE_FORCE_THRESHOLD // 2

PORT_SCAN_THRESHOLD = 15
# The detector fires only when a source uses more than the threshold ports.
MIN_PORT_SCAN_PORTS = PORT_SCAN_THRESHOLD + 1

# The exfiltration detector in worker/detectors.go flags a single Egress event
# whose reqdatasize (BytesIn) is at or above exfiltraionThresh.
EXFIL_THRESHOLD = int(10e6)
EXFIL_ACTION = "Egress"

MIN_GAP_SECONDS = 0
MAX_GAP_SECONDS = 100
MAX_SERVER_PORT = 65535
# Each port-scan IOC gets a distinct client IP within 198.18.0.0/16 and each
# exfiltration IOC within 198.19.0.0/16.
MAX_IOC_CLIENTS = 254 * 254

LOGIN_FAIL = "Login Fail"
LOGIN_SUCCESS = "Login Success"
LOGIN_ACTIONS = (LOGIN_FAIL, LOGIN_SUCCESS)

BACKGROUND_USERS = (
    "alice",
    "bob",
    "carol",
    "dave",
    "erin",
    "frank",
    "grace",
    "heidi",
    "ivan",
    "judy",
    "mallory",
    "niaj",
    "olivia",
    "peggy",
    "rupert",
    "sybil",
    "trent",
    "victor",
    "walter",
    "wendy",
)

BACKGROUND_ACTIONS = (
    "allow",
    "deny",
    "blocked",
    "Logout",
    "read",
    "write",
)

ENDPOINTS = (
    "/",
    "/dashboard",
    "/api/v1/events",
    "/search?q=login,failure",
    "/files?sort=name,asc",
    "/admin",
    "/reports?id=42,43",
)

METHODS = ("GET", "POST", "PUT", "PATCH", "DELETE")

SERVER_IPS = (
    "10.0.0.5",
    "10.0.0.6",
    "192.168.1.10",
    "192.168.1.11",
)

APP_NAMES = ("vpn", "portal", "admin", "api")

RESPONSE_CODES = {
    LOGIN_FAIL: "401 Unauthorized",
    LOGIN_SUCCESS: "200 OK",
    "allow": "200 OK",
    "deny": "403 Forbidden",
    "blocked": "403 Forbidden",
    "Logout": "200 OK",
    "read": "200 OK",
    "write": "201 Created",
    EXFIL_ACTION: "200 OK",
}

DEFECTS = (
    "epochtime",
    "srv_dport",
    "respcode",
    "reqdatasize",
    "respdatasize",
    "field_count",
)

INVALID_EPOCHTIMES = ("", "not-a-number", "1700000000.5", "2026-01-01T00:00:00Z")
INVALID_PORTS = ("443.0", "port-443", "four-four-three", "443/tcp")
INVALID_RESPONSE_CODES = ("OK - 200", "two-hundred", "status=200", "abc")
INVALID_SIZES = ("1.5MB", "many", "1,024", "unknown")


@dataclass
class Config:
    event_count: int
    brute_force_count: int
    port_scan_count: int
    exfil_count: int
    brute_force_min_attempts: int
    brute_force_max_attempts: int
    port_scan_min_ports: int
    port_scan_max_ports: int
    exfil_min_bytes_in: int
    exfil_max_bytes_in: int
    invalid_ratio: float
    duplicate_ratio: float
    output_path: Path
    seed: int
    start_epoch: int


@dataclass
class EventRow:
    values: list[str]
    background: bool
    invalid: bool = False

    def get(self, name: str) -> str:
        return self.values[FIELD_INDEX[name]]

    def set(self, name: str, value: str) -> None:
        self.values[FIELD_INDEX[name]] = value

    @property
    def action(self) -> str:
        return self.get("action")


def env_int(name: str, default: int) -> int:
    raw = os.environ.get(name)

    if raw is None or raw.strip() == "":
        return default

    try:
        return int(raw)
    except ValueError:
        raise SystemExit(f"{name} must be an integer, got {raw!r}")


def env_float(name: str, default: float) -> float:
    raw = os.environ.get(name)

    if raw is None or raw.strip() == "":
        return default

    try:
        return float(raw)
    except ValueError:
        raise SystemExit(f"{name} must be a number, got {raw!r}")


def load_config() -> Config:
    min_multiplier = env_int("BRUTE_FORCE_MIN_MULTIPLIER", 1)
    max_multiplier = env_int("BRUTE_FORCE_MAX_MULTIPLIER", 100)

    port_scan_min_ports = env_int("PORT_SCAN_MIN_PORTS", MIN_PORT_SCAN_PORTS)
    port_scan_max_ports = env_int("PORT_SCAN_MAX_PORTS", 200)

    exfil_min_multiplier = env_int("EXFIL_MIN_MULTIPLIER", 1)
    exfil_max_multiplier = env_int("EXFIL_MAX_MULTIPLIER", 100)

    config = Config(
        event_count=env_int("EVENT_COUNT", 10_000),
        brute_force_count=env_int("BRUTE_FORCE_COUNT", 3),
        port_scan_count=env_int("PORT_SCAN_COUNT", 3),
        exfil_count=env_int("EXFIL_COUNT", 3),
        brute_force_min_attempts=max(
            MIN_BRUTE_FORCE_ATTEMPTS, min_multiplier * BRUTE_FORCE_THRESHOLD
        ),
        brute_force_max_attempts=max_multiplier * BRUTE_FORCE_THRESHOLD,
        port_scan_min_ports=port_scan_min_ports,
        port_scan_max_ports=port_scan_max_ports,
        exfil_min_bytes_in=max(EXFIL_THRESHOLD, exfil_min_multiplier * EXFIL_THRESHOLD),
        exfil_max_bytes_in=exfil_max_multiplier * EXFIL_THRESHOLD,
        invalid_ratio=env_float("INVALID_RATIO", 0.05),
        duplicate_ratio=env_float("DUPLICATE_RATIO", 0.05),
        output_path=Path(os.environ.get("OUTPUT_PATH", "events.csv")),
        seed=env_int("RANDOM_SEED", 42),
        start_epoch=env_int("START_EPOCH", 1_700_000_000),
    )

    if config.event_count < 0:
        raise SystemExit("EVENT_COUNT must be zero or greater")
    if config.brute_force_count < 0:
        raise SystemExit("BRUTE_FORCE_COUNT must be zero or greater")
    if config.port_scan_count < 0:
        raise SystemExit("PORT_SCAN_COUNT must be zero or greater")
    if config.exfil_count < 0:
        raise SystemExit("EXFIL_COUNT must be zero or greater")
    if min_multiplier < 1:
        raise SystemExit("BRUTE_FORCE_MIN_MULTIPLIER must be 1 or greater")
    if max_multiplier < min_multiplier:
        raise SystemExit(
            f"BRUTE_FORCE_MAX_MULTIPLIER={max_multiplier} must be at least "
            f"BRUTE_FORCE_MIN_MULTIPLIER={min_multiplier}"
        )
    if exfil_min_multiplier < 1:
        raise SystemExit("EXFIL_MIN_MULTIPLIER must be 1 or greater")
    if exfil_max_multiplier < exfil_min_multiplier:
        raise SystemExit(
            f"EXFIL_MAX_MULTIPLIER={exfil_max_multiplier} must be at least "
            f"EXFIL_MIN_MULTIPLIER={exfil_min_multiplier}"
        )
    if port_scan_min_ports < MIN_PORT_SCAN_PORTS:
        raise SystemExit(
            f"PORT_SCAN_MIN_PORTS must be {MIN_PORT_SCAN_PORTS} or greater; "
            f"the port-scan detector fires above {PORT_SCAN_THRESHOLD} unique ports"
        )
    if port_scan_max_ports < port_scan_min_ports:
        raise SystemExit(
            f"PORT_SCAN_MAX_PORTS={port_scan_max_ports} must be at least "
            f"PORT_SCAN_MIN_PORTS={port_scan_min_ports}"
        )
    if port_scan_max_ports > MAX_SERVER_PORT:
        raise SystemExit(
            f"PORT_SCAN_MAX_PORTS={port_scan_max_ports} must be at most "
            f"{MAX_SERVER_PORT}"
        )
    if config.port_scan_count > MAX_IOC_CLIENTS:
        raise SystemExit(
            f"PORT_SCAN_COUNT={config.port_scan_count} exceeds the "
            f"{MAX_IOC_CLIENTS} distinct client IPs available for port-scan IOCs"
        )
    if config.exfil_count > MAX_IOC_CLIENTS:
        raise SystemExit(
            f"EXFIL_COUNT={config.exfil_count} exceeds the "
            f"{MAX_IOC_CLIENTS} distinct client IPs available for exfiltration IOCs"
        )
    if not 0 <= config.invalid_ratio <= 1:
        raise SystemExit("INVALID_RATIO must be between 0 and 1")
    if not 0 <= config.duplicate_ratio <= 1:
        raise SystemExit("DUPLICATE_RATIO must be between 0 and 1")
    if config.start_epoch < 0:
        raise SystemExit("START_EPOCH must be zero or greater")

    return config


def invalid_event_count(config: Config) -> int:
    return int(config.event_count * config.invalid_ratio + 0.5)


def login_row(
    *,
    epoch: int,
    user: str,
    client_ip: str,
    server_ip: str,
    server_port: int,
    url: str,
    method: str,
    response_code: str,
    action: str,
    app_name: str,
    bytes_in: int,
    bytes_out: int,
) -> list[str]:
    return [
        str(epoch),
        user,
        client_ip,
        server_ip,
        str(server_port),
        url,
        method,
        response_code,
        action,
        app_name,
        str(bytes_in),
        str(bytes_out),
    ]


def background_row(
    rng: random.Random,
    failures: dict[str, int],
) -> EventRow:
    user = rng.choice(BACKGROUND_USERS)

    if failures[user] >= MAX_BACKGROUND_FAILURES:
        action = LOGIN_SUCCESS
    else:
        roll = rng.random()

        if roll < 0.35:
            action = LOGIN_FAIL
        elif roll < 0.50:
            action = LOGIN_SUCCESS
        else:
            action = rng.choice(BACKGROUND_ACTIONS)

    if action == LOGIN_FAIL:
        failures[user] += 1
    elif action == LOGIN_SUCCESS:
        failures[user] = 0

    if action in LOGIN_ACTIONS:
        url = "/login"
        method = "POST"
        server_port = rng.choice((22, 443))
        client_ip = f"10.0.{rng.randrange(0, 16)}.{rng.randrange(1, 255)}"
    else:
        url = rng.choice(ENDPOINTS)
        method = "GET" if action == "read" else rng.choice(METHODS)
        server_port = rng.choice((80, 443))
        client_ip = f"10.0.{rng.randrange(0, 16)}.{rng.randrange(1, 255)}"

    return EventRow(
        values=login_row(
            epoch=0,
            user=user,
            client_ip=client_ip,
            server_ip=rng.choice(SERVER_IPS),
            server_port=server_port,
            url=url,
            method=method,
            response_code=RESPONSE_CODES[action],
            action=action,
            app_name=rng.choice(APP_NAMES),
            bytes_in=rng.randrange(0, 8192),
            bytes_out=rng.randrange(0, 1_000_000),
        ),
        background=True,
    )


def brute_force_row(rng: random.Random, ioc_index: int, success: bool) -> EventRow:
    action = LOGIN_SUCCESS if success else LOGIN_FAIL

    return EventRow(
        values=login_row(
            epoch=0,
            user=f"ioc-user-{ioc_index:04d}",
            client_ip=f"198.51.100.{(ioc_index % 254) + 1}",
            server_ip=rng.choice(SERVER_IPS),
            server_port=rng.choice((22, 443)),
            url="/login",
            method="POST",
            response_code=RESPONSE_CODES[action],
            action=action,
            app_name="vpn",
            bytes_in=rng.randrange(64, 2048),
            bytes_out=rng.randrange(128, 4096),
        ),
        background=False,
    )


def port_scan_row(
    rng: random.Random,
    ioc_index: int,
    server_port: int,
) -> EventRow:
    action = "blocked"
    block, offset = divmod(ioc_index, 254)

    return EventRow(
        values=login_row(
            epoch=0,
            user="",
            client_ip=f"198.18.{block}.{offset + 1}",
            server_ip=rng.choice(SERVER_IPS),
            server_port=server_port,
            url=rng.choice(ENDPOINTS),
            method=rng.choice(METHODS),
            response_code=RESPONSE_CODES[action],
            action=action,
            app_name=rng.choice(APP_NAMES),
            bytes_in=rng.randrange(0, 512),
            bytes_out=rng.randrange(0, 512),
        ),
        background=False,
    )


def exfil_row(rng: random.Random, ioc_index: int, bytes_in: int) -> EventRow:
    block, offset = divmod(ioc_index, 254)

    return EventRow(
        values=login_row(
            epoch=0,
            user="",
            client_ip=f"198.19.{block}.{offset + 1}",
            server_ip=rng.choice(SERVER_IPS),
            server_port=443,
            url=rng.choice(ENDPOINTS),
            method=rng.choice(("POST", "PUT")),
            response_code=RESPONSE_CODES[EXFIL_ACTION],
            action=EXFIL_ACTION,
            app_name=rng.choice(APP_NAMES),
            bytes_in=bytes_in,
            bytes_out=rng.randrange(0, 8192),
        ),
        background=False,
    )


def duplicate_event_count(config: Config) -> int:
    return int(config.event_count * config.duplicate_ratio + 0.5)


def ioc_row_capacity(config: Config) -> int:
    """Rows available for IOC events after reserving non-IOC rows.

    Invalid rows and duplicate targets are both taken from background rows, so
    one row is reserved for each.
    """
    reserved = invalid_event_count(config) + duplicate_event_count(config)

    return max(0, config.event_count - reserved)


def draw_port_scan_ports(config: Config, rng: random.Random) -> list[int]:
    """Pick a unique destination-port count for each port-scan IOC.

    Counts are drawn uniformly between the configured port limits.  The upper
    bound shrinks when EVENT_COUNT cannot hold the maximum draw for every
    incident, keeping the file size fixed.  Brute-force minimums and the
    exfiltration rows are reserved first so port-scan draws cannot crowd them
    out.
    """
    if config.port_scan_count == 0:
        return []

    brute_force_reserve = config.brute_force_count * (
        config.brute_force_min_attempts + 1
    )
    exfil_reserve = config.exfil_count

    capacity = max(
        0,
        (ioc_row_capacity(config) - brute_force_reserve - exfil_reserve)
        // config.port_scan_count,
    )
    upper = min(config.port_scan_max_ports, capacity)

    if upper < config.port_scan_min_ports:
        raise SystemExit(
            f"EVENT_COUNT={config.event_count} leaves room for only {capacity} "
            f"port-scan events per IOC after reserved rows, minimum brute-force "
            f"events, and exfiltration events, but PORT_SCAN_MIN_PORTS needs "
            f"{config.port_scan_min_ports}.  "
            "Raise EVENT_COUNT or lower INVALID_RATIO, DUPLICATE_RATIO, "
            "PORT_SCAN_COUNT, EXFIL_COUNT, or PORT_SCAN_MIN_PORTS."
        )

    return [
        rng.randint(config.port_scan_min_ports, upper)
        for _ in range(config.port_scan_count)
    ]


def draw_brute_force_attempts(
    config: Config,
    port_scan_rows: int,
    rng: random.Random,
) -> list[int]:
    """Pick a failed-login count for each brute-force IOC.

    Counts are drawn uniformly between the configured multipliers of the
    brute-force threshold, so incidents vary in size instead of all sitting
    exactly on the threshold.  The upper bound shrinks when EVENT_COUNT cannot
    hold the maximum draw for every IOC after the port-scan and exfiltration
    rows, keeping the file size fixed.
    """
    if config.brute_force_count == 0:
        return []

    available = ioc_row_capacity(config) - port_scan_rows - config.exfil_count
    capacity = max(0, available // config.brute_force_count - 1)
    upper = min(config.brute_force_max_attempts, capacity)

    if upper < config.brute_force_min_attempts:
        raise SystemExit(
            f"EVENT_COUNT={config.event_count} leaves room for only {capacity} "
            f"brute-force failures per IOC after reserved rows, port-scan events, "
            f"and exfiltration events, but BRUTE_FORCE_MIN_MULTIPLIER="
            f"{config.brute_force_min_attempts // BRUTE_FORCE_THRESHOLD} needs "
            f"{config.brute_force_min_attempts}.  Raise EVENT_COUNT or lower "
            "INVALID_RATIO, DUPLICATE_RATIO, PORT_SCAN_COUNT, EXFIL_COUNT, or "
            "BRUTE_FORCE_MIN_MULTIPLIER."
        )

    return [
        rng.randint(config.brute_force_min_attempts, upper)
        for _ in range(config.brute_force_count)
    ]


def draw_exfil_bytes_in(config: Config, rng: random.Random) -> list[int]:
    """Pick a request byte count for each exfiltration IOC.

    Each incident is a single egress event whose reqdatasize (BytesIn) is at or
    above the exfiltration threshold.  Counts are drawn uniformly between the
    configured multipliers of the threshold so incidents vary in size instead
    of all sitting exactly on the threshold.
    """
    if config.exfil_count == 0:
        return []

    return [
        rng.randint(config.exfil_min_bytes_in, config.exfil_max_bytes_in)
        for _ in range(config.exfil_count)
    ]


def build_rows(
    config: Config,
    rng: random.Random,
) -> tuple[list[EventRow], list[int], list[int], list[int]]:
    port_scan_ports = draw_port_scan_ports(config, rng)
    port_scan_rows = sum(port_scan_ports)
    brute_force_attempts = draw_brute_force_attempts(config, port_scan_rows, rng)
    brute_force_rows = sum(count + 1 for count in brute_force_attempts)
    exfil_bytes_in = draw_exfil_bytes_in(config, rng)
    exfil_rows = len(exfil_bytes_in)

    ioc_rows = port_scan_rows + brute_force_rows + exfil_rows
    background_count = config.event_count - ioc_rows

    required = invalid_event_count(config)

    if background_count < required:
        raise SystemExit(
            f"drawn IOC sizes need {ioc_rows} events, leaving {background_count} "
            f"background rows, but INVALID_RATIO={config.invalid_ratio} needs "
            f"{required}.  Raise EVENT_COUNT or lower IOC sizes."
        )

    failures = {user: 0 for user in BACKGROUND_USERS}
    background = [background_row(rng, failures) for _ in range(background_count)]

    ioc_blocks: list[list[EventRow]] = []

    for ioc_index, ports in enumerate(port_scan_ports):
        ioc_blocks.append(
            [
                port_scan_row(rng, ioc_index, server_port)
                for server_port in rng.sample(range(1, MAX_SERVER_PORT + 1), ports)
            ]
        )

    for ioc_index, attempts in enumerate(brute_force_attempts):
        block = [
            brute_force_row(rng, ioc_index, success=False) for _ in range(attempts)
        ]
        block.append(brute_force_row(rng, ioc_index, success=True))
        ioc_blocks.append(block)

    for ioc_index, bytes_in in enumerate(exfil_bytes_in):
        ioc_blocks.append([exfil_row(rng, ioc_index, bytes_in)])

    blocks_at: dict[int, list[list[EventRow]]] = {
        position: [] for position in range(background_count + 1)
    }

    for block in ioc_blocks:
        blocks_at[rng.randrange(background_count + 1)].append(block)

    rows: list[EventRow] = []

    for position in range(background_count + 1):
        for block in blocks_at[position]:
            rows.extend(block)

        if position < background_count:
            rows.append(background[position])

    epoch = config.start_epoch

    for row in rows:
        row.set("epochtime", str(epoch))
        epoch += rng.randint(MIN_GAP_SECONDS, MAX_GAP_SECONDS)

    return rows, brute_force_attempts, port_scan_ports, exfil_bytes_in


def invalidate(row: EventRow, defect: str, rng: random.Random) -> None:
    if defect == "epochtime":
        row.set("epochtime", rng.choice(INVALID_EPOCHTIMES))
    elif defect == "srv_dport":
        row.set("srv_dport", rng.choice(INVALID_PORTS))
    elif defect == "respcode":
        row.set("respcode", rng.choice(INVALID_RESPONSE_CODES))
    elif defect == "reqdatasize":
        row.set("reqdatasize", rng.choice(INVALID_SIZES))
    elif defect == "respdatasize":
        row.set("respdatasize", rng.choice(INVALID_SIZES))
    elif defect == "field_count":
        if rng.random() < 0.5:
            row.values.append("unexpected")
        else:
            row.values.pop(rng.randrange(len(row.values)))
    else:
        raise ValueError(f"unknown defect: {defect}")


def apply_invalid_events(
    config: Config,
    rows: list[EventRow],
    rng: random.Random,
) -> int:
    invalid_count = invalid_event_count(config)

    if invalid_count == 0:
        return 0

    # Rejecting a login row would change failure buckets: a lost success event
    # merges its neighboring failure runs, which can push background activity
    # over the brute-force threshold.  Only non-login background rows are made
    # invalid so the failure-cap invariant survives parsing.
    candidates = [
        row for row in rows if row.background and row.action not in LOGIN_ACTIONS
    ]

    if invalid_count > len(candidates):
        raise SystemExit(
            f"INVALID_RATIO={config.invalid_ratio} needs {invalid_count} invalid events, "
            f"but only {len(candidates)} non-login background rows can be invalidated "
            "without corrupting IOC patterns or login-failure buckets"
        )

    selected = rng.sample(candidates, invalid_count)

    for index, row in enumerate(selected):
        invalidate(row, DEFECTS[index % len(DEFECTS)], rng)
        row.invalid = True

    return invalid_count


def apply_duplicates(
    config: Config,
    rows: list[EventRow],
    rng: random.Random,
) -> int:
    duplicate_count = duplicate_event_count(config)

    if duplicate_count == 0:
        return 0

    # A duplicate overwrites its own values with those of the row directly above
    # it, so the output contains a run of identical neighboring rows.  Applying
    # targets in file order means the source already holds its final value, even
    # when it is itself a duplicate.
    #
    # Upload deduplication keeps the first row of a run and drops the copy, so a
    # duplicate's only lasting effect is that the target row disappears.  Targets
    # are therefore restricted to non-login background rows, so no login event
    # leaves the analyzed stream.  A target also never copies a Login Fail row:
    # doing so would add a failure to the raw file, and a run of duplicated rows
    # could stack failures past the background cap before deduplication.  The
    # source must be a background row too, so duplication can never clone an IOC
    # event (an exfiltration copy would add a detection).  Invalid rows are
    # excluded on both sides so duplication never adds another invalid row.
    candidates = [
        index
        for index, row in enumerate(rows)
        if index > 0
        and row.background
        and not row.invalid
        and row.action not in LOGIN_ACTIONS
        and rows[index - 1].background
        and not rows[index - 1].invalid
        and rows[index - 1].action != LOGIN_FAIL
    ]

    if duplicate_count > len(candidates):
        raise SystemExit(
            f"DUPLICATE_RATIO={config.duplicate_ratio} needs {duplicate_count} duplicate rows, "
            f"but only {len(candidates)} eligible background rows are available.  Lower "
            "DUPLICATE_RATIO or raise EVENT_COUNT."
        )

    for index in sorted(rng.sample(candidates, duplicate_count)):
        rows[index].values = list(rows[index - 1].values)

    return duplicate_count


def write_csv(path: Path, rows: list[EventRow]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)

    with path.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle, lineterminator="\n")
        writer.writerow(HEADERS)

        for row in rows:
            writer.writerow(row.values)


def main() -> None:
    config = load_config()
    rng = random.Random(config.seed)

    rows, brute_force_attempts, port_scan_ports, exfil_bytes_in = build_rows(
        config, rng
    )
    invalid_count = apply_invalid_events(config, rows, rng)
    duplicate_count = apply_duplicates(config, rows, rng)

    write_csv(config.output_path, rows)

    ratio = invalid_count / len(rows) if rows else 0.0

    print(f"wrote {len(rows)} events to {config.output_path}")
    print(f"  total events: {len(rows)}")
    print(f"  invalid events: {invalid_count} ({ratio:.2%})")
    print(f"  duplicate events: {duplicate_count}")
    print(f"  brute-force IOCs: {config.brute_force_count}")
    print(f"  brute-force failure counts: {brute_force_attempts}")
    print(f"  brute-force events: {sum(count + 1 for count in brute_force_attempts)}")
    print(f"  port-scan IOCs: {config.port_scan_count}")
    print(f"  port-scan port counts: {port_scan_ports}")
    print(f"  port-scan events: {sum(port_scan_ports)}")
    print(f"  exfiltration IOCs: {config.exfil_count}")
    print(f"  exfiltration request bytes: {exfil_bytes_in}")
    print(f"  seed: {config.seed}")


if __name__ == "__main__":
    try:
        main()
    except BrokenPipeError:
        sys.exit(1)
