# NISSA — Native Inter-Software Signal API

NISSA is a small, versioned, machine-readable call interface for FPM and trusted sibling desktop applications such as Yelena Software. Calls are local `fpm nissa` invocations and responses are UTF-8 JSON Lines (one JSON object per line) on standard output. Standard error remains available for diagnostics. NISSA does not start a daemon, open a socket, elevate privileges, or change ordinary FPM CLI output.

## Version negotiation

```sh
fpm nissa v1 hello
fpm nissa v2 hello
```

NISSA v1 remains available for existing clients and advertises `search`. NISSA v2 advertises the complete read-only query set:

```json
{"protocol":"NISSA","version":2,"signal":"hello","payload":{"capabilities":["search","installed","updates","info"]},"ok":true}
```

Clients must validate protocol name, version, success flag, and required capabilities before enabling the relevant feature. An incomplete or incompatible stream must not replace cached state; clients may fall back to their native XBPS query implementation.

## Repository search

Both versions support search:

```sh
fpm nissa v1 search "firefox"
fpm nissa v2 search "firefox"
```

A call emits zero or more `search.result` records followed by exactly one `search.done` record. Example:

```json
{"protocol":"NISSA","version":2,"signal":"search.result","payload":{"installed":true,"name":"firefox","summary":"Fast web browser","version":"128.0_1"}}
{"protocol":"NISSA","version":2,"signal":"search.done","payload":{"count":1,"ok":true},"ok":true}
```

Each result contains `name`, `version`, `summary`, and `installed`. Results are sourced from XBPS repository search. Query length is limited to 512 Unicode characters and control characters are rejected. A failed query emits `search.error` and exits nonzero.

## Installed-package inventory (v2)

```sh
fpm nissa v2 installed
```

Emits one `installed.result` record per installed XBPS package, with `name`, `version`, `summary`, and `installed:true`, and concludes with `installed.done`. The inventory is read from `xbps-query -l`; NISSA does not maintain a parallel package database or infer installation state from search results.

## Pending update discovery (v2)

```sh
fpm nissa v2 updates
```

Emits `updates.result` records with `name`, `current_version`, `new_version`, `arch`, and `manager:"xbps"`, followed by `updates.done`. It uses XBPS's non-mutating `xbps-install -un` query and reads the repository index already present on disk; it does not refresh repositories or request privileges. The caller can refresh repository indexes through its normal privileged package operation before querying again.

## Package details (v2)

```sh
fpm nissa v2 info firefox
```

Emits one `info.result` record containing `name`, repository `version`, `summary`, `description`, `architecture`, `license`, `homepage`, `installed`, `installed_version`, and `installed_size`, then `info.done`. Package names are validated before being passed to XBPS.

## Transaction and update-manager integration

NISSA is intentionally read-only. It does not expose install/remove/update mutations, AppImage operations, arbitrary command execution, or privilege elevation. Use FPM's normal CLI for transactions (`fpm install`, `remove`, `update`, `upgrade`, `flat ...`, `aimg ...`); confirmation remains explicit, and `-y/--yes` is honored only when supplied.

After successful XBPS or Flatpak install/remove/update/upgrade operations, FPM checks the current user's Yelena applet PID file, verifies that the process belongs to that user and is actually the Yelena applet, then sends `SIGUSR2` so the applet immediately rechecks updates and refreshes Yelena's SQLite pending-update database. If the applet is not running, no signal is sent; its next normal check will refresh the list.

Yelena uses NISSA v2 for XBPS search, installed inventory, and update discovery when this engine is selected. System changes from Yelena itself continue through Yelena's existing polkit-controlled XBPS executor; NISSA is not a privilege bypass. If a NISSA response is unavailable or incomplete, Yelena falls back to its native XBPS adapter and does not overwrite a valid cached update list with partial data.

FPM maintains a separate global SQLite snapshot at `/var/lib/fpm/fpm.db`, refreshed with `sudo fpm db sync` and after successful FPM XBPS transactions. It is a rebuildable inventory cache, not an installation/dependency authority; all live operations continue to use XBPS at `/var/db/xbps`. `fpm db list` reads the cache without root, and `make uninstall` removes only the executable, leaving this data intact. Yelena's separate per-user SQLite database at `~/.config/cuerdtoken/updates.db` stores update candidates and UI/task status; it is neither FPM's inventory cache nor a replacement for XBPS.

## Compatibility

NISSA v1 remains search-only and its JSON field layout is unchanged. New capabilities are exposed via `nissa v2`, so clients that only understand v1 continue working. Existing FPM commands retain their ordinary behavior.
