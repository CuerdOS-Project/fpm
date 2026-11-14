# PLUTO — communication protocol

PLUTO is a small, versioned, machine-readable communication protocol between FPM and trusted sibling desktop applications such as Yelena Software. Calls are local `fpm pluto` invocations and responses are UTF-8 JSON Lines (one JSON object per line) on standard output. Standard error remains available for diagnostics. PLUTO does not start a daemon, open a socket, elevate privileges, or change ordinary FPM CLI output.

## Version negotiation

```sh
fpm pluto v1 hello
fpm pluto v2 hello
```

PLUTO v1 remains available for existing clients and advertises `search`. PLUTO v2 advertises the complete read-only query set:

```json
{"protocol":"PLUTO","version":2,"signal":"hello","payload":{"capabilities":["search","search-am","installed","updates","info","mirrors","kernels"]},"ok":true}
```

Clients must validate protocol name, version, success flag, and required capabilities before enabling the relevant feature. An incomplete or incompatible stream must not replace cached state; clients may fall back to their native XBPS query implementation.

## Repository search

Both versions support search. PLUTO v1 keeps its existing XBPS-only result shape. PLUTO v2 combines XBPS results with AM AppImage results when `am-FUSION` is enabled and AM/AppMan is available. The `search-am` capability advertises this optional source.

```sh
fpm pluto v1 search "firefox"
fpm pluto v2 search "firefox"
```

A call emits zero or more `search.result` records followed by exactly one `search.done` record. Example:

```json
{"protocol":"PLUTO","version":2,"signal":"search.result","payload":{"installed":true,"manager":"xbps","name":"firefox","summary":"Fast web browser","version":"128.0_1"}}
{"protocol":"PLUTO","version":2,"signal":"search.result","payload":{"installed":false,"manager":"am","name":"Firefox-AppImage","repository":"am-FUSION","summary":"Firefox AppImage","version":""}}
{"protocol":"PLUTO","version":2,"signal":"search.done","payload":{"complete":true,"count":2,"ok":true,"sources":["xbps","am"]},"ok":true}
```

Each result contains `name`, `version`, `summary`, and `installed`. PLUTO v2 also supplies `manager` (`xbps` or `am`); AM results include `repository:"am-FUSION"`. These fields let a graphical client route an explicit user-approved install to the right FPM frontend. PLUTO stays read-only; install an AM result through `fpm aimg install --repo am-FUSION <name>`, not through a PLUTO mutation call.

When an enabled source fails, v2 emits `search.source_error` and finishes with `search.done` containing `complete:false` and the list of successful `sources`; a usable source can still return results. If no source can be queried, FPM emits `search.error` and exits nonzero. Query length is limited to 512 Unicode characters and control characters are rejected.

## Installed-package inventory (v2)

```sh
fpm pluto v2 installed
```

Emits one `installed.result` record per installed XBPS package, with `name`, `version`, `summary`, and `installed:true`, and concludes with `installed.done`. The inventory is read from `xbps-query -l`; PLUTO does not maintain a parallel package database or infer installation state from search results.

## Pending update discovery (v2)

```sh
fpm pluto v2 updates
```

Emits `updates.result` records with `name`, `current_version`, `new_version`, `arch`, and `manager:"xbps"`, followed by `updates.done`. It uses XBPS's non-mutating `xbps-install -un` query and reads the repository index already present on disk; it does not refresh repositories or request privileges. The caller can refresh repository indexes through its normal privileged package operation before querying again.

## Package details (v2)

```sh
fpm pluto v2 info firefox
```

Emits one `info.result` record containing `name`, repository `version`, `summary`, `description`, `architecture`, `license`, `homepage`, `installed`, `installed_version`, and `installed_size`, then `info.done`. Package names are validated before being passed to XBPS.

## Void mirror status and fallback (v2)

```sh
fpm pluto v2 mirrors
```

This read-only call reports the active official Void repository URLs, the corresponding `repo-default.voidlinux.org` URL when applicable, whether that fallback is configured by FPM, and whether a row is the managed fallback. The stream consists of `mirrors.result` records followed by one `mirrors.done`, whose payload includes `count`, `fallback_enabled`, and `fallback_urls`. Custom repositories (including CuerdOS) are omitted rather than rewritten or treated as Void mirrors. Example:

```json
{"protocol":"PLUTO","version":2,"signal":"mirrors.result","payload":{"repository_url":"https://repo-de.voidlinux.org/current","fallback_url":"https://repo-default.voidlinux.org/current","fallback_configured":true,"is_fallback":false,"official_void":true}}
{"protocol":"PLUTO","version":2,"signal":"mirrors.done","payload":{"ok":true,"count":1,"fallback_enabled":true,"fallback_urls":["https://repo-default.voidlinux.org/current"]},"ok":true}
```

The client should first call `hello`, check for the optional `mirrors` capability, then validate the protocol/version and the complete result stream. FPM's `repos fallback enable|disable` commands manage this system-wide, opt-in configuration through the normal privileged CLI; PLUTO itself remains read-only. The fallback file is sorted after existing XBPS configuration and retains the user's current repository order. Consequently it is a resilience fallback, not a fastest-mirror selector: if the configured mirror responds, XBPS continues to prefer it. `repo-default.voidlinux.org` can map to any Void Tier 1 mirror and does not promise the lowest latency. See [Void's mirror list](https://xmirror.voidlinux.org/) and the [XBPS configuration format](https://man.voidlinux.org/xbps.d.5).

## Kernels (v2, optional `kernels` capability)

```sh
fpm pluto v2 kernels
```

Read-only; does not require `vkpurge`. The inventory comes from `/boot/vmlinuz-*`, `/usr/lib/modules` and the installed `linuxX.Y` XBPS packages. Emits one `kernels.result` per kernel, oldest first, with `version`, `running`, `removable` (false for the running kernel), `package` (owning `linuxX.Y` package, or `""`) and `leftover` (true when no package owns it, i.e. what `vkpurge list` reports), then `kernels.done` with `ok`, `count` and `running` (`uname -r`). Errors: `kernel_query_failed` (neither `/boot` nor `/usr/lib/modules` is readable), `invalid_arguments`. Removal is not a PLUTO call: run `fpm kernel rm -y <version|all>` through the caller's trusted authorization UI; `all` removes only leftovers, while a package-owned kernel is removed by name with `xbps-remove`. Clients must check the `kernels` capability in `hello` first and must ignore unknown fields.

## Transaction and update-manager integration

PLUTO is intentionally read-only. It does not expose install/remove/update mutations, AppImage install/remove/update operations, arbitrary command execution, or privilege elevation. Use FPM's normal CLI for transactions (`fpm install`, `remove`, `update`, `upgrade`, `flat ...`, `aimg ...`); confirmation remains explicit, and `-y/--yes` is honored only when supplied.

After successful XBPS or Flatpak install/remove/update/upgrade operations, FPM checks the current user's Yelena applet PID file, verifies that the process belongs to that user and is actually the Yelena applet, then sends `SIGUSR2` so the applet immediately rechecks updates and refreshes Yelena's SQLite pending-update database. If the applet is not running, no signal is sent; its next normal check will refresh the list.

Yelena uses PLUTO v2 for XBPS search, installed inventory, and update discovery when this engine is selected. System changes from Yelena itself continue through Yelena's existing polkit-controlled XBPS executor; PLUTO is not a privilege bypass. If a PLUTO response is unavailable or incomplete, Yelena falls back to its native XBPS adapter and does not overwrite a valid cached update list with partial data.

FPM maintains a separate global SQLite snapshot at `/var/lib/fpm/fpm.db`, refreshed with `sudo fpm db sync` and after successful FPM XBPS transactions. It is a rebuildable inventory cache, not an installation/dependency authority; all live operations continue to use XBPS at `/var/db/xbps`. `fpm db list` reads the cache without root, and `make uninstall` removes only the executable, leaving this data intact. Yelena's separate per-user SQLite database at `~/.config/cuerdtoken/updates.db` stores update candidates and UI/task status; it is neither FPM's inventory cache nor a replacement for XBPS.

## Compatibility

PLUTO v1 remains XBPS-only, search-only, and its JSON field layout is unchanged. AM search is exposed via v2 with an explicit manager/repository label, so clients that only understand v1 continue working. Existing FPM commands retain their ordinary behavior.
