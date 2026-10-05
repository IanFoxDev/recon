# 0001. Money as integers, and the same input gives the same report

Date: 2026-10-05. Status: accepted.

## Context

recon compares two lists of money movements: rows from the application's own
database and rows from a provider's export. A reconciliation tool that is wrong by a
cent is worse than none, because people stop reading its reports, and a tool that
gives a different report for the same files cannot be used in CI or trusted in an
audit.

Providers export amounts in different shapes. Stripe's Balance report has
`gross`, `fee` and `net` as decimal strings in major units (`"10.99"` for USD, `"1100"`
for JPY). Banks and other providers use a comma as the decimal separator, thousands
separators, or minor units. The application's database usually has integers in minor
units or a `numeric` column.

## Decision

**Money is an `int64` count of minor units plus a currency code.** Nothing in recon
holds an amount in a float.

- A decimal string is parsed digit by digit into minor units using the currency's
  exponent from ISO 4217 (JPY 0, EUR 2, KWD 3). recon ships the exponent table and
  allows overrides in the configuration for currencies it does not know.
- A string with more decimal places than the currency allows is an error, not
  rounded: `"10.999"` in EUR stops the run and names the file, line and column. Extra
  places that are zeros do not change the amount and are accepted: some exports write
  `"100.00"` for a currency without minor units.
- Amounts outside the `int64` range are an error.
- The decimal separator and the thousands separator are set per source. Nothing is
  guessed from the data.
- A database value is read as an integer in minor units, or as `numeric` text and
  parsed like a CSV value. It is never read through a float type.

**The same input gives the same report.**

- Rows are matched and reported in an order defined by the data (key, then source
  position), never by map iteration or by the order rows arrive from a database
  without `ORDER BY`.
- The report contains no wall-clock time unless asked for (`--stamp`). It records the
  configuration and a SHA-256 of each input file, so a report says what it was built
  from.
- Tolerances are explicit in the configuration (`amount: 1` minor unit, `date: 2d`).
  The default is exact.

## Consequences

- A provider file with a malformed amount fails the run loudly instead of producing a
  report with a silent rounding. The error message has to be good enough to fix the
  file or the configuration in one go.
- Two runs on the same files can be compared byte for byte, which makes the report
  usable as a CI artifact and in tests: the end-to-end tests compare reports to
  golden files.
- Currencies with an exponent other than 2 work from the first release, because the
  table is part of parsing, not an afterthought.
