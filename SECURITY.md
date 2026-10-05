# Security

recon reads your ledger with the DSN you give it, in a read-only transaction, and reads
provider exports from disk. It writes only the report files named in the configuration
and makes no network requests other than the database connection.

Use a database user that can only read the tables the query needs.

If you find a vulnerability, for example a way for an input file to make recon write
outside the report paths or run a statement that changes data, do not open a public
issue. Report it privately through
[GitHub](https://github.com/IanFoxDev/recon/security/advisories/new), or write to
ianfoxdeveloper@gmail.com.

Reports contain payment references and amounts from your data. Treat them as you treat
the exports they were built from.

## Supported versions

Fixes go into the latest release only. Until 1.0 that is the latest `0.x` tag.
