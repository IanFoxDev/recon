# 0002. Reading the tables of ianfoxdev/ledger

Date: 2026-11-17. Status: accepted.

## Context

The `sql` source takes any query, so recon can already reconcile a ledger kept with
[ianfoxdev/ledger](https://github.com/IanFoxDev/ledger). In practice everyone writes
the same query: the postings of the cash account at the provider, signed, keyed by the
provider's id from the transaction's meta, joined to the account for the currency. The
ledger's payments example had exactly that in `recon/ledger.sql`, and it is easy to get
the sign wrong for anything but an asset account.

ledger 0.2 states that its tables are a contract for readers (its ADR 0002): table and
column names, the meaning of `side` and `amount`, `meta` as a JSON object, rows never
updated. A reader that relies only on that contract keeps working across minor
versions of the ledger.

## Decision

A source `ianfoxdev_ledger` with three settings: the DSN variable, the `account` to
reconcile and the meta `key` that holds the provider's id, plus an optional
`fee_account`. It fills in the columns
itself; a config that also sets `columns` or `amounts` for it is refused.

- **Sign.** A posting is positive on the account's normal side: a debit for `asset`
  and `expense`, a credit for `liability`, `equity` and `revenue`. This is how the ledger
  computes `balance_after`, so the sum of the rows is the change in the balance. For a
  cash account a deposit is positive and a refund negative, as in a provider's report.
- **Key.** The value of `key` in the transaction's meta, as text: an id written as a
  number matches one written as a string. A transaction without it is keyed by its
  idempotency key. An opening balance or a manual correction then shows up as
  `missing_in_provider` with a key someone can look up, instead of stopping the run on
  an empty key.
- **One row per transaction.** The postings of a transaction on the account and on
  the fee account are summed into one row, ordered by transaction id.
- **Fee.** Without `fee_account`, zero. When the other side compares fees (the Stripe
  profile does), the ledger side still gets a fee column of zeros, so fee differences
  are reported rather than the config being refused. With `fee_account`, an expense
  account in the same currency, the fee is what the transaction posted there and the
  amount is the cash plus the fee. A provider reports the gross and the fee; a ledger
  either books the net to cash and the fee to the expense in the capture, or books the
  gross and takes the fee from cash later. Both add up to the provider's gross and fee
  per payment. The fee account should hold the provider's fees and nothing else.
- **What is read.** `ledger_accounts.code`, `type`, `currency`;
  `ledger_transactions.id`, `idempotency_key`, `meta`; `ledger_postings.account`,
  `transaction_id`, `side`, `amount`. Nothing else, and never the hash,
  which the ledger keeps out of its contract.
- **How.** One read-only transaction, like the `sql` source. An account that is not in
  `ledger_accounts` stops the run with its name; an account with no postings gives no
  rows.
- **Databases.** PostgreSQL, like the `sql` source. MySQL comes with the `sql` source.

## Consequences

- A ledger kept with the library is reconciled with four lines of config and no SQL.
- recon depends on another project's schema. The coupling is to a documented contract,
  not to its code: ledger changes these columns only in a release marked BREAKING, with
  migration SQL. recon's tests run against a copy of the ledger's schema, refreshed
  when the ledger releases.
- Rows are ordered by transaction id, so the line numbers in a report point at the
  same transactions on every run.
- The query reads the whole history of the account. Reconciling one period needs a
  window by time, which neither source has yet.
