# bootenv

`bootenv` creates, lists, and manages btrfs subvolume snapshots and keeps the
GRUB boot menu in sync with them.

Behaviour is driven by a **config file** that defines one or more *targets* —
each target is a subvolume to snapshot plus the directory where its snapshots
are stored. The `[root]` target is always present and is special: it feeds the
GRUB menu and records the running kernel version in each snapshot. Any number of
additional targets (e.g. `[home]`, `[var]`, `[srv]`) can be added.

Without `--target` every command operates on **all configured targets at once**.

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
go test ./...          # all packages
go test ./... -v       # with per-test output
```

Tests cover `config`, `snapstore`, and `grubgen` using temporary directories —
no real btrfs filesystem or root access is required.

---

## Config file

The config file is **optional**. When it is absent, bootenv behaves as if only
`[root]` were configured (source `/`, snapshot dir `/@snapshots/root`,
keep 10 auto snapshots).

Default location: `/etc/bootenv/bootenv.toml`  
Override with: `bootenv --config /path/to/file.toml <command>`

### Format

```toml
# Each top-level block is one snapshot target.
# The block name is the --target value on the CLI and determines the snapshot
# directory: /@snapshots/<name>.

[root]
# Snapshots /  →  /@snapshots/root  (source is implicit for [root])
keep_auto = 10

[home]
# Snapshots /home  →  /@snapshots/home  (source defaults to /home)
keep_auto = 5

[var]
keep_auto = 3
# source defaults to /var, snapshot dir is /@snapshots/var

[data]
source    = "/mnt/data"   # explicit source when the name doesn't match
keep_auto = 2
# snapshot dir is always /@snapshots/data
```

### Fields

| Field | Default for `[root]` | Default for any other `[name]` |
|---|---|---|
| `source` | `/` | `/<name>` |
| `keep_auto` | `10` | `10` |

The snapshot directory is not configurable — it is always `/@snapshots/<name>`.
The TOML block header **is** the directory name.

`keep_auto = 0` is valid and means "delete all auto snapshots on cleanup"
(useful for targets where you only want manual snapshots).

### Auto-creating snapshot directories

When you add a new target block to the config and run `bootenv snapshot`, the
`snapshot_dir/<kind>` directory is created automatically if it does not exist.
No manual `mkdir` step is needed.

### The `[root]` target is always present

Even if the config file omits `[root]` entirely, or if the config file does not
exist, bootenv always injects a `[root]` entry with its built-in defaults before
running any command.

---

## Commands

### Global flags

| Flag | Short | Default | Description |
|---|---|---|---|
| `--config` | `-c` | `/etc/bootenv/bootenv.toml` | Path to the TOML config file |

---

### `snapshot` — create a snapshot

```sh
bootenv snapshot <auto|manual> [name] [--target <name>]
```

Creates a read-write btrfs snapshot for each configured target (or just the one
named by `--target`). Snapshot directories are created automatically when they
do not exist.

**Auto snapshots** are named by timestamp including seconds
(`YYYY-MM-DD_HHMMSS`), e.g. `2025-05-20_090132`. The format is both
human-readable and sorts lexically in chronological order.

**Manual snapshots** require an explicit name:
```sh
bootenv snapshot manual before-upgrade
```

After snapshotting the `root` target a `.bootenv-kernel` marker file is written
into the snapshot recording the running kernel version. The GRUB menu is
regenerated after any root snapshot; it is skipped when `--target` points to a
non-root target.

**`--target` / `-T`** — act on one target instead of all:
```sh
bootenv snapshot auto                       # all configured targets
bootenv snapshot auto --target root         # root only
bootenv snapshot auto -T home               # home only
bootenv snapshot manual before-upgrade      # all configured targets
bootenv snapshot manual before-upgrade -T root
```

---

### `list` — list snapshots

```sh
bootenv list [--type auto|manual] [--target <name>]
```

Prints a table of all snapshots across configured targets, newest first.
Each row is one snapshot subvolume. The **TARGET** column shows which config
block it belongs to.

```
TARGET  TYPE    NAME                 CREATED               KERNEL          PATH
------  ----    ----                 -------               ------          ----
root    auto    2025-05-20_090132    2025-05-20 09:01:32   6.1.0-31-amd64  /@snapshots/root/auto/2025-05-20_090132
home    auto    2025-05-20_090132    2025-05-20 09:01:32   -               /@snapshots/home/auto/2025-05-20_090132
root    manual  before-upgrade       2025-05-18 14:22:05   6.1.0-30-amd64  /@snapshots/root/manual/before-upgrade
```

**`--type` / `-t`** — filter by kind:
```sh
bootenv list --type auto
bootenv list --type manual
```

**`--target` / `-T`** — show only one target:
```sh
bootenv list --target root
bootenv list -T home
```

---

### `cleanup` — prune old auto snapshots

```sh
bootenv cleanup [--target <name>] [--keep N] [--dry-run]
```

Deletes the oldest auto snapshots that exceed each target's `keep_auto` limit.
Each target is pruned independently using its own limit from the config. Manual
snapshots are never touched.

After pruning the GRUB menu is regenerated only if root snapshots were removed.

**`--target` / `-T`** — restrict to one target:
```sh
bootenv cleanup --target root
bootenv cleanup -T home
```

**`--keep` / `-k`** — override the keep limit for all targeted pools:
```sh
bootenv cleanup --keep 5           # keep only 5 for every targeted pool
bootenv cleanup -T root --keep 3   # keep 3 root auto snapshots
```

When `--keep` is omitted, each target uses its `keep_auto` from the config.

**`--dry-run` / `-n`** — preview without deleting:
```sh
bootenv cleanup --dry-run
bootenv cleanup -n --keep 3
```

---

### `delete` — delete a snapshot by name

```sh
bootenv delete <name> [--target <name>]
```

Deletes the named snapshot from all configured targets (or just `--target`).
The GRUB menu is regenerated only when the root target's snapshot is removed.

```sh
bootenv delete 2025-05-20_090132            # delete from all targets
bootenv delete before-upgrade --target root  # root only
bootenv delete before-upgrade -T home        # home only
```

---

### `restore` — promote a root snapshot to the live root

```sh
bootenv restore <name>
```

Replaces the current `/@` subvolume with the named root snapshot. Only the
`root` target is involved; other targets are not modified.

Steps performed:

1. Mount the btrfs top-level volume (`subvolid=5`) at `/run/bootenv/mnt`
2. Rename `/@` → `/@-pre-restore-<timestamp>` (safety backup)
3. Snapshot `/@snapshots/root/<kind>/<name>` → `/@`
4. Regenerate the GRUB menu

**A reboot is required** to enter the restored environment.

To remove the safety backup afterwards:
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

Scans `/@snapshots/root/{auto,manual}` (or whatever `snapshot_dir` the `root`
target is configured to use), writes `/etc/grub.d/42_bootenv_snapshots`, and
runs `update-grub`.

All root snapshots appear under a **"Bootenv Snapshots"** submenu in the boot
menu, newest first. Entries whose kernel (`/boot/vmlinuz-<ver>`) or initrd
(`/boot/initrd.img-<ver>`) is missing from the running system are silently
skipped.

This command is called automatically by `snapshot`, `cleanup`, `delete`, and
`restore` whenever they change root snapshots.

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
  var/                      ← any target added to the config appears here
    auto/
      2025-05-20_090132/
```
