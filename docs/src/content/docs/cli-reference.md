---
title: "CLI Reference"
---

Nebi's CLI is `nebi-cli`. It is organized into command groups: **Workspace**, **Sync**, and **Connection**.

## Specs vs. Bundles

Two terms appear throughout these commands:

- **Workspace spec**: `pixi.toml` and `pixi.lock`. The minimal environment definition. Stored on the Nebi server.
- **Workspace bundle**: `pixi.toml` + `pixi.lock` + any project files (READMEs, source code, data), packaged for an OCI registry.

## Workspace Commands

| Command | Description |
|---------|-------------|
| `nebi-cli init` | Track current directory as a workspace (runs `pixi init` if needed) |
| `nebi-cli status` | Show workspace sync status |
| `nebi-cli workspace list` | List tracked workspaces |
| `nebi-cli workspace install <name>` | Install a server workspace's environment from its lockfile (local mode) |
| `nebi-cli workspace uninstall <name>` | Remove a server workspace's installed environment (local mode) |
| `nebi-cli workspace remove <name>` | Remove a workspace from tracking |
| `nebi-cli workspace prune` | Remove workspaces whose paths no longer exist |
| `nebi-cli shell [name] [pixi-args...]` | Activate a pixi shell |
| `nebi-cli run [name] [pixi-args...]` | Run a command or task via pixi |

## Sync Commands

| Command | Description |
|---------|-------------|
| `nebi-cli push [<name>][:<tag>]` | Push workspace specs to a server (tag optional, auto-tags with content hash + latest) |
| `nebi-cli pull [<name>[:<tag>]]` | Pull workspace specs from a server |
| `nebi-cli diff [<ref-a>] [<ref-b>]` | Compare workspace specs |
| `nebi-cli publish [name]` | Publish a workspace bundle to an OCI registry (uses content hash tag by default) |
| `nebi-cli import <oci-reference>` | Import a workspace bundle from an OCI registry, restoring pixi files and asset layers |

## Connection Commands

| Command | Description |
|---------|-------------|
| `nebi-cli login <server-url>` | Authenticate with a server |
| `nebi-cli registry list` | List available OCI registries |
| `nebi-cli registry add` | Add an OCI registry |
| `nebi-cli registry remove <name>` | Remove an OCI registry |

## Flags

**`publish`**

- `--local`: Publish directly to registry without a server
- `--tag <tag>`: Set the OCI tag (default: content hash with `--local`, auto-incrementing `v1`, `v2`, ... otherwise)
- `--repo <name>`: Set the OCI repository name (defaults to the workspace name)
- `--registry <name>`: Registry name or ID to publish to (defaults to the configured default registry)
- `--concurrency N`: Number of files uploaded at the same time (only with `--local`, default 8)

**`import`**

- `-o, --output <dir>`: Output directory (defaults to current directory)
- `--concurrency N`: Number of files downloaded at the same time (default 8)
- `--force`: Overwrite an existing `pixi.toml` without asking. Only applies when the bundle contains just pixi files; bundles with other files always refuse to overwrite.

**`workspace list`, `workspace remove`**

- `-r, --remote`: Use workspaces from the Nebi server instead of local workspaces
- `--installed` (`workspace list` only): Only list server workspaces with an installed environment; implies `--remote`
