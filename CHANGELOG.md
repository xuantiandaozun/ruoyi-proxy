# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Hub remote-command queue with outbound Spoke polling, result persistence, and local confirmation.
- Standalone `spoke-agent` runtime with proxy/daemon ownership coordination.
- MySQL connection profiles, configuration discovery, schema browsing, and controlled SQL execution.
- SQLite-backed AI scheduled tasks with leases, permission boundaries, and run history.
- Shared CLI/AI command catalog and additional Hub v1 compatibility tests.
- Hub remote-job lifecycle with idempotency keys, claim leases, bounded recovery, cancellation, retry, and explicit running acknowledgements.
- Structured remote actions for service status, logs, restart, deployment, and read-only database queries, plus multi-Spoke batches and append-only job events.
- Spoke capability/resource/health reporting, Hub-managed node governance, maintenance gating, capability allowlists, and group-targeted jobs.

### Changed

- Documented anonymous short-lived Spoke registration as a trusted-network onboarding design.
- Updated the project roadmap around reliability, unified tasks, node governance, database safety, scheduling, sessions, channels, and Web management.
- Removed obsolete file-synchronization setup and documentation; file transport is delegated to deployment scripts or external artifact tooling.
- Hardened read-only Shell classification so compound commands and write-capable query options require confirmation.
- Changed proxy routing to short-lived immutable snapshots, copy-on-write configuration updates, and graceful process shutdown.
- Added CI checks for formatting, vet, tests, race detection, and Linux builds.

## [1.1.0] - 2026-04-22

### Added

- AI Agent-first CLI with OpenAI-compatible, Anthropic, Ollama, and Hub providers.
- Hub/Spoke packaging profiles and centralized AI credential relay.
- Conversation persistence, confirmation flow, adaptive health checks, and low-memory deployment.

## [1.0.0] - 2025-12-11

### Added

- Initial blue-green reverse proxy and multi-service configuration.
- Interactive service management, Nginx/HTTPS automation, embedded scripts, and cross-platform builds.
