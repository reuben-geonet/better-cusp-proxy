# CUSP Browser Access Project

## Goal

Allow Windows users to access old CUSP device web interfaces from a modern browser while the device is reachable only from the user's local desk/laptop network.

This is a local-device access project. `jumpbox` was only used to prove how the existing `cusp-proxy` works and to inspect one reachable example device. The production tool must run locally on the Windows laptop or in WSL on that laptop, not on `jumpbox`.

The Windows delivery should not require administrator rights. Prefer a portable per-user executable/app that listens on `127.0.0.1` using a high port such as `4444`. Avoid designs that need:

- binding to privileged ports like `80` or `443`;
- installing a Windows service;
- changing the system hosts file;
- installing a machine-wide trusted root certificate;
- adding inbound firewall rules for non-loopback listeners.

## Current Findings

- Existing `cusp-proxy` on `jumpbox` is a Go HTTPS reverse proxy for devices that only support legacy TLS. Treat it as a behavioral reference, not as deployable infrastructure for this project.
- The binary at `/usr/local/bin/cusp-proxy` was built from `github.com/GeoNet/field/cmd/cusp-proxy`.
- Default usage is `cusp-proxy [options] host[:port]`, listening on `localhost:4444`.
- A live process was observed as `cusp-proxy 192.168.185.135`, listening on `127.0.0.1:4444`.
- Direct modern TLS negotiation with `192.168.185.135:443` fails.
- Forcing TLS 1.0 succeeds.
- The tested device certificate is self-signed, expired in 2003, signed with `md5WithRSAEncryption`, has a 1024-bit RSA key, and uses CN `172.16.0.5`.
- The device accepts weak TLS 1.0 ciphers including `DES-CBC3-SHA`, `IDEA-CBC-SHA`, and `RC4-SHA`; AES ciphers failed in testing.
- `cusp-proxy` forwards client HTTP requests to the device over TLS 1.0 and rewrites `Host` to the target address.

## Working Direction

The proxy must run on the Windows laptop or WSL environment that can directly route to the desk device. `jumpbox` cannot be assumed to have device reachability.

Prefer a local proxy approach over requiring users to weaken Firefox globally. Good candidates:

- A small portable Windows app wrapping a Go proxy executable with a simple tray/UI flow. It should run without admin.
- A Go command-line proxy distributed as a Windows binary. It should run without admin.
- A Nix flake or WSL install path for technical users who already use WSL.

GitHub Actions should build downloadable Windows executables rather than storing binaries in git. Release assets are the preferred distribution channel for non-technical Windows users.

The proxy should present modern TLS or plain HTTP on localhost and isolate all legacy TLS handling to the target-device side.

## Commit Style

Use Conventional Commits for commit messages, for example:

- `feat: add local CUSP proxy`
- `fix: handle legacy TLS handshake errors`
- `docs: document Windows no-admin workflow`
- `ci: build Windows release artifacts`

Do not force-push shared branches unless explicitly requested by the user. The initial `main` history rewrite to convert the first commit to a Conventional Commit was explicitly allowed once.
