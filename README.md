# bootenv

`bootenv` creates, lists, and manages btrfs subvolume snapshots and keeps the
GRUB menu in sync with them.

## Build

```sh
go build -o bootenv .
```

## Install

Build the binary, then install it somewhere on your `PATH`:

```sh
sudo install -m 755 bootenv /usr/local/bin/bootenv
```

## Usage

```text
bootenv creates, lists, and manages btrfs subvolume snapshots
and keeps the GRUB menu in sync with them.

Usage:
  bootenv [command]

Available Commands:
  cleanup     Show which auto snapshots would be pruned (dry-run)
  completion  Generate the autocompletion script for the specified shell
  delete      Delete a snapshot by name
  grub        Regenerate /etc/grub.d/42_bootenv_snapshots and run update-grub
  help        Help about any command
  list        List all bootenv snapshots
  restore     Promote a snapshot to the live root subvolume (/@)
  snapshot    Create a btrfs snapshot of / and /home

Flags:
  -h, --help   help for bootenv

Use "bootenv [command] --help" for more information about a command.
```
