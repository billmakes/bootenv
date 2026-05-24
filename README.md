# bootenv

`bootenv` creates, lists, and manages btrfs subvolume snapshots and keeps the
GRUB boot menu in sync with them.

Snapshots come in two kinds:

- **auto** — timestamp-named (e.g. `2025-05-20_090132`), created by scripts or
  system hooks and pruned automatically by `cleanup`
- **manual** — user-chosen names (e.g. `before-upgrade`), never pruned
  automatically

Each snapshot can have a **root** component (`/@snapshots/root/<kind>/<name>`),
a **home** component (`/@snapshots/home/<kind>/<name>`), or both.  All commands
that operate on snapshots accept `--target root|home|both` to restrict which
side they touch.

---

## Build

```sh
go build -o bootenv .
```

## Install

```sh
sudo install -m 755 bootenv /usr/local/bin/bootenv
```

## Test

```sh
go test ./...
```

Tests cover the core packages (`config`, `snapstore`, `grubgen`) using temporary
directories — no real btrfs filesystem or root access required.  Run with `-v`
for per-test output:

```sh
go test ./... -v
```

---

## Config file

`bootenv` reads an optional TOML config file (default
`/etc/bootenv/bootenv.toml`) to control retention limits for the `cleanup`
command.  The file is optional — built-in defaults are used when it is absent.

```toml
# /etc/bootenv/bootenv.toml

[root]
keep_auto = 10   # number of auto snapshots of / to keep

[home]
keep_auto = 5    # number of auto snapshots of /home to keep
```

Only the keys you specify are overridden; omitted keys retain their built-in
defaults (`keep_auto = 10` for root, `keep_auto = 5` for home).

Pass `--config <path>` (or `-c <path>`) to any command to use a different file:

```sh
bootenv --config /etc/bootenv/custom.toml cleanup
```

---

## Commands

### Global flags

| Flag | Short | Default | Description |
|---|---|---|---|
| `--config` | `-c` | `/etc/bootenv/bootenv.toml` | Path to the TOML config file |
| `--help` | `-h` | — | Help for any command |

---

### `snapshot` — create a snapshot

```sh
bootenv snapshot <auto|manual> [name] [--target root|home|both]
```

Creates a read-write btrfs snapshot of the root subvolume (`/`), the home
subvolume (`/home`), or both.

**`--target` / `-T`** (default `both`)  
Controls which subvolumes are snapshotted:
- `both` — snapshot root **and** home (default)
- `root` — snapshot root only
- `home` — snapshot home only

The root snapshot receives a `.bootenv-kernel` marker file containing the
running kernel version, which is later used by `grub` and `list` to associate
each snapshot with its kernel.  The GRUB menu is regenerated automatically
after any root snapshot is created; it is skipped when `--target home` is used.

**Auto snapshots** are named by timestamp including seconds
(`YYYY-MM-DD_HHMMSS`, e.g. `2025-05-20_090132`).  The format is both
human-readable and lexically sortable in chronological order.

**Manual snapshots** require an explicit name:

```sh
bootenv snapshot manual before-upgrade
```

#### Examples

```sh
# Both root and home (default)
bootenv snapshot auto
bootenv snapshot manual before-upgrade

# Root only
bootenv snapshot auto --target root
bootenv snapshot manual before-upgrade -T root

# Home only
bootenv snapshot auto --target home
```

---

### `list` — list snapshots

```sh
bootenv list [--type auto|manual] [--target root|home|both]
```

Prints a table of all known snapshots, newest first.  The **ROOT** and **HOME**
columns show whether each subvolume component actually exists on disk (`✓`/`✗`),
making it easy to spot asymmetric or orphaned snapshots.

```
TYPE    NAME                CREATED               ROOT  HOME  KERNEL          ROOT PATH
----    ----                -------               ----  ----  ------          ---------
auto    2025-05-20_090132   2025-05-20 09:01:32   ✓     ✓     6.1.0-31-amd64  /@snapshots/root/auto/2025-05-20_090132
manual  before-upgrade      2025-05-18 14:22:05   ✓     ✗     6.1.0-30-amd64  /@snapshots/root/manual/before-upgrade
```

**`--type` / `-t`** — filter by kind:
```sh
bootenv list --type auto
bootenv list --type manual
```

**`--target` / `-T`** — filter to entries where the specified component exists:
```sh
bootenv list --target root   # only entries that have a root snapshot
bootenv list --target home   # only entries that have a home snapshot
bootenv list --target both   # only entries that have both
```

Omitting `--target` shows everything, including entries that exist on only one
side.

---

### `cleanup` — prune old auto snapshots

```sh
bootenv cleanup [--target root|home|both] [--keep N] [--dry-run]
```

Deletes the oldest auto snapshots that exceed the configured keep limit.  Root
and home pools are pruned **independently**, each with its own limit from the
config file.  The GRUB menu is regenerated only when root snapshots are removed.

Manual snapshots are never touched by `cleanup`.

**`--target` / `-T`** (default `both`) — restrict which pool(s) to clean:
```sh
bootenv cleanup --target root   # only prune the root pool
bootenv cleanup --target home   # only prune the home pool
```

**`--keep` / `-k`** — override the keep limit for all targeted pools (ignores
config values):
```sh
bootenv cleanup --keep 5          # keep 5 root auto and 5 home auto
bootenv cleanup --keep 3 -T root  # keep 3 root auto; home untouched
```

When `--keep` is omitted, limits come from the config file
(`[root] keep_auto` and `[home] keep_auto`).

**`--dry-run` / `-n`** — preview what would be deleted without removing
anything:
```sh
bootenv cleanup --dry-run
bootenv cleanup --dry-run --keep 3
```

---

### `delete` — delete a snapshot by name

```sh
bootenv delete <name> [--target root|home|both]
```

Removes the named snapshot's subvolume(s).  By default both root and home are
deleted.  Use `--target` to remove only one side, for example when a snapshot
exists on one side only:

```sh
bootenv delete 2025-05-20_090132            # delete both sides
bootenv delete before-upgrade --target root  # root only, leave home
bootenv delete before-upgrade -T home        # home only
```

The GRUB menu is regenerated only when the root subvolume is removed.

---

### `restore` — promote a snapshot to the live root

```sh
bootenv restore <name>
```

Replaces the current `/@` subvolume with the named root snapshot.  The existing
`/@` is renamed to `/@-pre-restore-<timestamp>` as a safety backup rather than
deleted (btrfs cannot delete a subvolume that contains nested subvolumes).

Steps performed:

1. Mount the btrfs top-level volume (`subvolid=5`) at `/run/bootenv/mnt`
2. Rename `/@` → `/@-pre-restore-<timestamp>`
3. Snapshot `/@snapshots/root/<kind>/<name>` → `/@`
4. Regenerate the GRUB menu

`/home` is not modified.  **A reboot is required** to enter the restored
environment.

To clean up the safety backup after confirming the restore:

```sh
sudo mount -o subvolid=5 $(findmnt -no SOURCE / | cut -d'[' -f1) /run/bootenv/mnt
sudo btrfs subvolume delete /run/bootenv/mnt/@-pre-restore-<timestamp>
sudo umount /run/bootenv/mnt
```

---

### `grub` — regenerate the GRUB menu

```sh
bootenv grub
```

Scans all snapshots under `/@snapshots/root/{auto,manual}`, writes
`/etc/grub.d/42_bootenv_snapshots`, and runs `update-grub`.

Snapshots appear under a **"Bootenv Snapshots"** submenu in the GRUB boot menu,
similar to the built-in "Advanced Options" submenu.  Entries are listed
newest-first.  Any snapshot whose kernel (`/boot/vmlinuz-<ver>`) or initrd
(`/boot/initrd.img-<ver>`) is missing from the running system is silently
skipped.

This command is called automatically by `snapshot`, `cleanup`, `delete`, and
`restore` when they change root snapshots.

---

## Snapshot directory layout

```
/@snapshots/
  root/
    auto/
      2025-05-20_090132/    ← btrfs subvolume; contains .bootenv-kernel
      2025-05-19_120045/
    manual/
      before-upgrade/
  home/
    auto/
      2025-05-20_090132/
      2025-05-19_120045/
    manual/
      before-upgrade/
```
