# Nebi Server

The Nebi server is a hosted web interface to manage Nebi workspaces in a team. It has a similar interface as the local desktop, but with more features for teams and organizations.

This page covers how to run and configure it.

{/* TODO: Embed video walkthrough of server UI, created with https://github.com/nebari-dev/nebi-video-demo-automation. Update the link in the following iframe. */}

{/* <iframe width="560" height="315" src="" title="YouTube video player" frameborder="0" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share" referrerpolicy="strict-origin-when-cross-origin" allowfullscreen></iframe> */}

## Admin Credentials

Before starting the server for the first time, set `ADMIN_USERNAME` and `ADMIN_PASSWORD`. Nebi uses these to create the initial admin account for authentication.

![Nebi login screen](/img/login-nebi.png)

You (and your team) will use these credentials to log in via `nebi login` or the web UI.

Export the variables in your terminal session before starting the server:

```bash
export ADMIN_USERNAME=admin
export ADMIN_PASSWORD=your-password
```

## Running the Server

Start the server:

```bash
nebi serve
```

By default (`--host` unset), Nebi binds all interfaces on port `8460` in team mode. Local mode (`NEBI_MODE=local`) is a single-user, on-device setup, so the server binds only the loopback interface (`127.0.0.1`) and only accepts requests addressed to a local host/origin. To bind a local-mode server to another interface, set `--host` (or `NEBI_SERVER_HOST`) explicitly.

To use a different port:

```bash
nebi serve --port 9000
```

To explicitly bind a host/interface, use `--host` (or `NEBI_SERVER_HOST`):

```bash
nebi serve --host 127.0.0.1 --port 8460
```

Once the server is running, authenticate from any client machine with [`nebi login`](./cli-team.md#connect-to-a-server).

## Build Environment Variables

Users can store encrypted build variables for package-manager operations, such as private Git or Python package indexes. To keep team servers from accepting unsafe process-level environment overrides, the server only accepts names allowed by admin configuration.

By default, Nebi accepts the exact names `GITLAB_TOKEN`, `GITHUB_TOKEN`, and `GH_TOKEN`, plus names starting with `NEBI_`. Add only the exact names or prefixes your package manager needs:

```yaml
build_env:
  allowed_names:
    - GITLAB_TOKEN
    - UV_INDEX_USERNAME
    - UV_INDEX_PASSWORD
  allowed_prefixes:
    - NEBI_
    - PIP_
```

The same settings can be supplied with `NEBI_BUILD_ENV_ALLOWED_NAMES` and `NEBI_BUILD_ENV_ALLOWED_PREFIXES` as comma-separated lists. Nebi always rejects reserved names and prefixes such as `PATH`, `HOME`, `LD_`, `DYLD_`, `XDG_`, and `PIXI_`, even if they appear in the allow-list.

Before relying on a new private package index in production, run a smoke test against the real index by saving the needed variable in Nebi and triggering a workspace build that resolves from that index.

## API Documentation

The Swagger API docs are available at [http://localhost:8460/docs](http://localhost:8460/docs).

## Groups

### OIDC group sync

When OIDC authentication is configured, nebi requests the `groups` scope alongside `openid profile email`. The IdP must return a `groups` claim in the ID token (a JSON array of strings). On every login, nebi reconciles the user's group memberships:

- For each name in the claim, an OIDC-source group is created (if missing) and the user is added to it.
- Memberships in OIDC-source groups that aren't in this login's claim are removed.
- Native groups (created via the admin UI) are **never** modified by OIDC sync — even if a claim name happens to collide with a native group name.

OIDC groups with zero members are kept so existing workspace shares survive temporary churn. There is no background reconcile worker; all updates happen at login time.

## What's Next

- See the [CLI Team Workflows](./cli-team.md) for push/pull examples
- Check the [CLI Reference](./cli-reference.md) for all available commands
