# recon

Reconciles your ledger with a payment provider's export: a SQL query against your
database on one side, the provider's CSV on the other, matching rules in YAML, and a
report of every difference.

> Status: in development, nothing released yet.

Amounts are integers in minor units from the file to the report, and the same input
always gives the same report: [docs/adr/0001-money-and-determinism.md](docs/adr/0001-money-and-determinism.md).

## License

[MIT](LICENSE)
