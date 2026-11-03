#!/usr/bin/env python3
"""Generate a log-analyzer event CSV with brute-force IOC patterns.

Environment:
  EVENT_COUNT    total data rows to write (default 10000); must hold the drawn
                 IOC blocks plus the invalid rows
  IOC_COUNT      brute-force incidents to embed (default 3)
  IOC_MIN_MULTIPLIER
                 minimum failed logins per incident, as a multiple of the
                 brute-force threshold, 1 or greater (default 1)
  IOC_MAX_MULTIPLIER
                 maximum failed logins per incident, as a multiple of the
                 brute-force threshold (default 100)
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
MIN_IOC_ATTEMPTS = BRUTE_FORCE_THRESHOLD + 1
MIN_GAP_SECONDS = 0
MAX_GAP_SECONDS = 100

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
    ioc_min_attempts: int
    ioc_max_attempts: int
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
    min_multiplier = env_int("IOC_MIN_MULTIPLIER", 1)
    max_multiplier = env_int("IOC_MAX_MULTIPLIER", 100)

    config = Config(
        event_count=env_int("EVENT_COUNT", 10_000),
        ioc_count=env_int("IOC_COUNT", 3),
        ioc_min_attempts=max(
            MIN_IOC_ATTEMPTS, min_multiplier * BRUTE_FORCE_THRESHOLD
        ),
        ioc_max_attempts=max_multiplier * BRUTE_FORCE_THRESHOLD,
        invalid_ratio=env_float("INVALID_RATIO", 0.05),
        duplicate_ratio=env_float("DUPLICATE_RATIO", 0.05),
        output_path=Path(os.environ.get("OUTPUT_PATH", "events.csv")),
        seed=env_int("RANDOM_SEED", 42),
        start_epoch=env_int("START_EPOCH", 1_700_000_000),
    )

    if config.event_count < 0:
        raise SystemExit("EVENT_COUNT must be zero or greater")
    if config.ioc_count < 0:
        raise SystemExit("IOC_COUNT must be zero or greater")
    if min_multiplier < 1:
        raise SystemExit("IOC_MIN_MULTIPLIER must be 1 or greater")
    if max_multiplier < min_multiplier:
        raise SystemExit(
            f"IOC_MAX_MULTIPLIER={max_multiplier} must be at least "
            f"IOC_MIN_MULTIPLIER={min_multiplier}"
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


def duplicate_event_count(config: Config) -> int:
    return int(config.event_count * config.duplicate_ratio + 0.5)


def ioc_attempt_capacity(config: Config) -> int:
    """Rows available for IOC failures after reserving everything else.

    Invalid rows and duplicate targets are both taken from background rows, so
    one row is reserved for each.  Each IOC also spends one row on its trailing
    success event.
    """
    reserved = invalid_event_count(config) + duplicate_event_count(config)

    return max(0, config.event_count - config.ioc_count - reserved)


def draw_ioc_attempts(config: Config, rng: random.Random) -> list[int]:
    """Pick a failed-login count for each IOC.

    Counts are drawn uniformly between the configured multipliers of the
    brute-force threshold, so incidents vary in size instead of all sitting
    exactly on the threshold.  The upper bound shrinks when EVENT_COUNT cannot
    hold the maximum draw for every IOC, keeping the file size fixed.
    """
    if config.ioc_count == 0:
        return []

    capacity = ioc_attempt_capacity(config) // config.ioc_count
    upper = min(config.ioc_max_attempts, capacity)

    if upper < config.ioc_min_attempts:
        raise SystemExit(
            f"EVENT_COUNT={config.event_count} leaves room for only {capacity} failures "
            f"per IOC after reserving invalid and duplicate rows, but "
            f"IOC_MIN_MULTIPLIER={config.ioc_min_attempts // BRUTE_FORCE_THRESHOLD} needs "
            f"{config.ioc_min_attempts}.  Raise EVENT_COUNT or lower INVALID_RATIO, "
            "DUPLICATE_RATIO, or IOC_MIN_MULTIPLIER."
        )

    return [
        rng.randint(config.ioc_min_attempts, upper)
        for _ in range(config.ioc_count)
    ]


def build_rows(config: Config, rng: random.Random) -> tuple[list[EventRow], list[int]]:
    ioc_attempts = draw_ioc_attempts(config, rng)
    ioc_rows = sum(count + 1 for count in ioc_attempts)
    background_count = config.event_count - ioc_rows

    required = invalid_event_count(config)

    if background_count < required:
        raise SystemExit(
            f"drawn IOC sizes need {ioc_rows} events, leaving {background_count} "
            f"background rows, but INVALID_RATIO={config.invalid_ratio} needs "
            f"{required}.  Raise EVENT_COUNT or lower IOC_MAX_MULTIPLIER."
        )

    failures = {user: 0 for user in BACKGROUND_USERS}
    background = [background_row(rng, failures) for _ in range(background_count)]

    ioc_blocks: list[list[EventRow]] = []

    for ioc_index, attempts in enumerate(ioc_attempts):
        block = [
            ioc_row(rng, ioc_index, success=False) for _ in range(attempts)
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

    epoch = config.start_epoch

    for row in rows:
        row.set("epochtime", str(epoch))
        epoch += rng.randint(MIN_GAP_SECONDS, MAX_GAP_SECONDS)

    return rows, ioc_attempts


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

    candidates = [row for row in rows if row.background]

    if invalid_count > len(candidates):
        raise SystemExit(
            f"INVALID_RATIO={config.invalid_ratio} needs {invalid_count} invalid events, "
            f"but only {len(candidates)} background rows can be invalidated without "
            "corrupting IOC patterns or login-success boundaries"
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
    # are therefore restricted to non-login background rows, leaving every
    # login-failure bucket untouched.  Invalid rows are excluded on both sides so
    # duplication never adds another invalid row.
    candidates = [
        index
        for index, row in enumerate(rows)
        if index > 0
        and row.background
        and not row.invalid
        and row.action not in LOGIN_ACTIONS
        and not rows[index - 1].invalid
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

    rows, ioc_attempts = build_rows(config, rng)
    invalid_count = apply_invalid_events(config, rows, rng)
    duplicate_count = apply_duplicates(config, rows, rng)

    write_csv(config.output_path, rows)

    valid_count = len(rows) - invalid_count
    ratio = invalid_count / len(rows) if rows else 0.0

    print(f"wrote {len(rows)} events to {config.output_path}")
    print(f"  valid events: {valid_count}")
    print(f"  invalid events: {invalid_count} ({ratio:.2%})")
    print(f"  duplicate events: {duplicate_count}")
    print(f"  brute-force IOCs: {config.ioc_count}")
    print(f"  IOC failure counts: {ioc_attempts}")
    print(f"  IOC events: {sum(count + 1 for count in ioc_attempts)}")
    print(f"  seed: {config.seed}")


if __name__ == "__main__":
    try:
        main()
    except BrokenPipeError:
        sys.exit(1)
