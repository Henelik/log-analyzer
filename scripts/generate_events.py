#!/usr/bin/env python3
"""Generate a log-analyzer event CSV with brute-force IOC patterns.

Environment:
  EVENT_COUNT    total data rows to write (default 1000)
  IOC_COUNT      brute-force incidents to embed (default 3)
  IOC_ATTEMPTS   failed logins per incident; must exceed 10 (default 12)
  INVALID_RATIO  fraction of rows with invalid field data, 0..1 (default 0.05)
  OUTPUT_PATH    destination CSV path (default events.csv)
  RANDOM_SEED    PRNG seed (default 42)
  START_EPOCH    first event epoch second (default 1700000000)
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
LOGIN_FAIL = "Login Fail"
LOGIN_SUCCESS = "Login Success"

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
    ioc_count: int
    ioc_attempts: int
    invalid_ratio: float
    output_path: Path
    seed: int
    start_epoch: int


@dataclass
class EventRow:
    values: list[str]
    background: bool

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
    config = Config(
        event_count=env_int("EVENT_COUNT", 1000),
        ioc_count=env_int("IOC_COUNT", 3),
        ioc_attempts=env_int("IOC_ATTEMPTS", 12),
        invalid_ratio=env_float("INVALID_RATIO", 0.05),
        output_path=Path(os.environ.get("OUTPUT_PATH", "events.csv")),
        seed=env_int("RANDOM_SEED", 42),
        start_epoch=env_int("START_EPOCH", 1_700_000_000),
    )

    if config.event_count < 0:
        raise SystemExit("EVENT_COUNT must be zero or greater")
    if config.ioc_count < 0:
        raise SystemExit("IOC_COUNT must be zero or greater")
    if config.ioc_attempts <= BRUTE_FORCE_THRESHOLD:
        raise SystemExit(
            f"IOC_ATTEMPTS must exceed {BRUTE_FORCE_THRESHOLD} to trigger detection"
        )
    if not 0 <= config.invalid_ratio <= 1:
        raise SystemExit("INVALID_RATIO must be between 0 and 1")
    if config.start_epoch < 0:
        raise SystemExit("START_EPOCH must be zero or greater")

    ioc_rows = config.ioc_count * (config.ioc_attempts + 1)
    if ioc_rows > config.event_count:
        raise SystemExit(
            f"IOC_COUNT={config.ioc_count} with IOC_ATTEMPTS={config.ioc_attempts} "
            f"needs {ioc_rows} events, but EVENT_COUNT is {config.event_count}"
        )

    return config


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
    epoch: int,
    failures: dict[str, int],
) -> EventRow:
    user = rng.choice(BACKGROUND_USERS)

    if failures[user] >= BRUTE_FORCE_THRESHOLD:
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

    if action in (LOGIN_FAIL, LOGIN_SUCCESS):
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
            epoch=epoch,
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


def ioc_row(rng: random.Random, ioc_index: int, success: bool) -> EventRow:
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


def build_rows(config: Config, rng: random.Random) -> list[EventRow]:
    ioc_rows = config.ioc_count * (config.ioc_attempts + 1)
    background_count = config.event_count - ioc_rows

    failures = {user: 0 for user in BACKGROUND_USERS}
    background = [
        background_row(rng, config.start_epoch + index, failures)
        for index in range(background_count)
    ]

    ioc_blocks: list[list[EventRow]] = []

    for ioc_index in range(config.ioc_count):
        block = [
            ioc_row(rng, ioc_index, success=False)
            for _ in range(config.ioc_attempts)
        ]
        block.append(ioc_row(rng, ioc_index, success=True))
        ioc_blocks.append(block)

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

    for offset, row in enumerate(rows):
        row.set("epochtime", str(config.start_epoch + offset))

    return rows


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
    invalid_count = int(config.event_count * config.invalid_ratio + 0.5)

    if invalid_count == 0:
        return 0

    candidates = [
        row
        for row in rows
        if row.background and row.action != LOGIN_SUCCESS
    ]

    if invalid_count > len(candidates):
        raise SystemExit(
            f"INVALID_RATIO={config.invalid_ratio} needs {invalid_count} invalid events, "
            f"but only {len(candidates)} background rows can be invalidated without "
            "corrupting IOC patterns or login-success boundaries"
        )

    selected = rng.sample(candidates, invalid_count)

    for index, row in enumerate(selected):
        invalidate(row, DEFECTS[index % len(DEFECTS)], rng)

    return invalid_count


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

    rows = build_rows(config, rng)
    invalid_count = apply_invalid_events(config, rows, rng)

    write_csv(config.output_path, rows)

    valid_count = len(rows) - invalid_count
    ratio = invalid_count / len(rows) if rows else 0.0

    print(f"wrote {len(rows)} events to {config.output_path}")
    print(f"  valid events: {valid_count}")
    print(f"  invalid events: {invalid_count} ({ratio:.2%})")
    print(f"  brute-force IOCs: {config.ioc_count}")
    print(f"  IOC events: {config.ioc_count * (config.ioc_attempts + 1)}")
    print(f"  seed: {config.seed}")


if __name__ == "__main__":
    try:
        main()
    except BrokenPipeError:
        sys.exit(1)
