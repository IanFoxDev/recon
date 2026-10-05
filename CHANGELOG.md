# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/). Before 1.0, minor versions may break the
configuration or the report format; such changes are marked **BREAKING**.

## [Unreleased]

## [0.1.0] - 2026-10-05

The first release: a ledger in PostgreSQL or CSV against a provider export, six
categories of differences, HTML, JSON and CSV reports, and an exit code for CI.
Checked end to end on generated data with known differences, in CI and in the
docker compose example.

### Added

- `recon run -c recon.yaml`: compares a ledger with a provider export and writes HTML,
  JSON and CSV reports. Exit code 0 when everything reconciles, 1 on differences, 2 when
  the run fails.
- Sources: PostgreSQL (read-only, values read as text) and CSV files (globs,
  configurable separators). Profile `stripe_balance` for Stripe's itemized Balance
  report.
- Categories: `missing_in_ledger`, `missing_in_provider`, `amount_mismatch`,
  `fee_mismatch`, `currency_mismatch`, `duplicate`. Amount and fee tolerances, grouping
  of rows with the same key.
- Amounts as int64 minor units with ISO 4217 exponents; the same input gives the same
  report. See [docs/adr/0001-money-and-determinism.md](docs/adr/0001-money-and-determinism.md).
- Docker image `ghcr.io/ianfoxdev/recon`, `recon-gen` for test data with known
  differences, and an example in `examples/stripe`.

[Unreleased]: https://github.com/IanFoxDev/recon/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/IanFoxDev/recon/releases/tag/v0.1.0
