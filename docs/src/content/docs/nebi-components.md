---
title: "Nebi components"
---

It has four executables: **nebi**, **nebi-server**, **nebi-web**, and **nebi-desktop**. They share the same core libraries but serve different use cases.

## System overview

<img src="/img/context-diagram.svg" alt="Nebi System Context diagram" style="max-width: 100%; height: auto;" />

## CLI

`nebi` is a standalone CLI tool for managing and tracking Pixi workspaces on your local machines, both for [solo use](/cli-local/) and for [team workflows](/cli-team/) backed by a Nebi server. Its source entry point lives in `cmd/nebi-cli`, and every command is documented in the [CLI reference](/cli-reference/).

- **Local database**: Track project names, paths, and versions in a local database
- **Pixi shell/run**: Open a pixi shell or run pixi tasks by project name
- **Publish/import - OCI registries**: Push project bundles (specs plus optional asset layers) directly to OCI registries, and import them on another machine
- **Push/pull - Nebi server**: Sync (push) versioned `pixi.toml` and `pixi.lock` specs to a Nebi server, and pull from a different machine
- **Track state (diff)**: Compare specs between local directories, project names, or server versions

## Desktop application

The [desktop app](/ui/) is another tool for managing Pixi workspaces on your local machines. It supports all the CLI workflows through a graphical / app interface.

## OCI Registries

Nebi can publish **project bundles** to any [OCI-compliant registry](/registry-setup/) such as GitHub Container Registry, Quay.io, or self-hosted registries. A bundle always contains `pixi.toml` and `pixi.lock`, and may also include other project files (READMEs, source code, data).

- Publishing can be done from the CLI (`nebi publish`) or triggered from the desktop app or server
- The desktop app and server UI include a registry browser for discovering and pulling published bundles

Bundles are packed into an OCI Image Manifest with custom media types for the spec and for each asset layer. Each push creates a content-addressed tag (`sha-<hash>`) plus a `latest` tag and any user-specified tags.

## Nebi Server

`nebi-server` is the team deployment of Nebi. It serves the REST API, bundled React frontend, and worker stack with full multi-user support.

- **Authentication**: JWT-based sessions with pluggable backends — basic auth, OIDC, or proxy auth
- **Role-based AC**: Apache Casbin-based access control with per-project permissions (read, write, admin) for users
- **API**: Git-based HTTP server serving the REST API and the bundled React frontend
- **Central database**: SQLite (default) or PostgreSQL for project and user tracking
- **Background worker**: Background processor for async operations like project creation and package installation
- **Job queue**: In-memory queue with an in-process worker that handles project creation and updates concurrently, keeping the API responsive.

Learn more: [Nebi server setup](/server-setup/).

## Local web app

`nebi-web` serves the bundled React frontend for a single-user local workflow. It uses local authentication bypass and binds to loopback by default.

## Managed project specifications

Nebi keeps managed project directories, `pixi.toml`, and `pixi.lock` read-only between its own write operations. This prevents external commands such as `pixi add` from changing a managed specification without recording a revision. Existing managed projects receive the same protection when the application starts. User-owned projects registered with `nebi init` remain writable for normal Pixi workflows.

Integrations that add packages to managed projects must use `POST /api/v1/projects/{id}/packages` with a body such as `{"packages": ["ipykernel"]}` and wait for the returned job to complete. The job creates a new revision and preserves the previous one. Environment installation uses the existing lockfile and rejects stale specifications; solve the project to record dependency changes first.

Filesystem permissions prevent accidental writes by external tools. They do not isolate the project from an administrator or its owner deliberately changing permissions. Applications such as Launchpad must use the versioned API for repair actions instead of invoking `pixi add` directly.
