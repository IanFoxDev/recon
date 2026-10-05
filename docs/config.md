# Configuration

recon reads one YAML file, `recon.yaml` by default (`recon run -c path`). Relative
paths in it are resolved against the directory of the file. Unknown keys are errors
with the line they are on, so a typo does not silently change a run.

```yaml
version: 1

# Exponents for currencies ISO 4217 does not have, or that a provider treats differently.
currencies:
  USDT: 6

ledger:              # your side
  sql:
    dsn_env: LEDGER_DSN          # name of the environment variable with the DSN
    query: select ...            # or query_file: sql/payments.sql
  amounts: minor                 # minor (integers, default for sql) or major (decimals)
  columns:
    key: psp_reference           # one column, or a list: [account_id, psp_reference]
    amount: amount_minor
    fee: fee_minor               # optional; set on both sides or on neither
    currency: currency
  filter:                        # optional: keep only these rows
    - {column: status, in: [captured, refunded]}

provider:            # the payment provider's side
  csv:
    path: exports/stripe_*.csv   # a file or a glob; files are read in name order
    profile: stripe_balance      # optional, fills in columns and amounts
    delimiter: ","               # default ","
    decimal: "."                 # default "."
    thousands: ""                # default none; "," or "." or " "
  amounts: major                 # default for csv
  columns:
    key: payment_intent_id
  filter:
    - {column: reporting_category, in: [charge]}

match:
  group: false                   # true: sum rows with the same key on each side first
  tolerance:
    amount: 0                    # minor units; 0 is an exact match
    fee: 0

report:
  html: out/report.html          # at least one of html, json, csv
  json: out/report.json
  csv: out/differences.csv
  fail_on: any                   # any: exit 1 on differences; none: always exit 0
```

## Sides

Both sides take the same keys. `ledger` and `provider` only name them in the report:
a row only on the ledger side is `missing_in_provider`, and the other way round. Either
side can be `sql` or `csv`, so two CSV exports can be compared as well.

### sql

PostgreSQL. recon connects with the DSN from the environment variable named in
`dsn_env`, opens a read-only transaction and runs the query. Every value is read as
text: an integer column, `numeric` and `text` all work, and a `numeric` amount never
passes through a float.

Give the query an `ORDER BY` if you want row numbers in the report to point at the
same rows on every run. The differences themselves do not depend on the order.

### csv

The first line is the header. A byte order mark from a spreadsheet export is skipped.
Columns are found by name, so their order in the file does not matter.

## Amounts

`amounts: major` means decimal strings in major units, as in `"10.99"` for USD. They
are parsed with the currency's exponent from ISO 4217:

- `"10.9"` is 1090 cents; `"1100"` JPY is 1100 yen.
- Extra decimals that are zeros are fine: `"100.00"` for a currency without minor
  units.
- Other extra decimals are an error: `"10.999"` in EUR stops the run and names the
  file, line and column.
- A thousands separator must group digits by three.

`amounts: minor` means integers in minor units: 1099 for 10.99 USD.

Currencies are matched case-insensitively and reported in upper case. A currency recon
does not know stops the run; add it under `currencies` with its exponent.

## Keys and grouping

Rows are paired by `columns.key`. With several key columns, all of them have to match,
and both sides need the same number.

Without `match.group`, a key that appears more than once on one side is a `duplicate`
and its amounts are not compared: recon cannot tell which row goes with which. With
`match.group: true`, rows with the same key are summed per side, which is what you want
when one provider payment is several entries in your ledger.

## Profiles

| Profile | Export | Fills in |
|---|---|---|
| `stripe_balance` | Stripe Balance report, itemized (`balance_change_from_activity.itemized.*`) | `amount: gross`, `fee: fee`, `currency: currency`, `amounts: major` |

The key is yours to choose: `payment_intent_id` if your ledger stores PaymentIntent ids,
`charge_id` or `source_id` otherwise. Filter on `reporting_category` to leave out
payouts and other balance movements you do not book per payment.
