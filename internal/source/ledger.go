package source

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"

	"github.com/ianfoxdev/recon/internal/config"
	"github.com/ianfoxdev/recon/internal/money"
)

// ledgerQuery reads one row per transaction that touches the account or the fee
// account of ianfoxdev/ledger. Amounts are on the normal side, as balance_after is: for a
// cash account a deposit is positive and a refund negative. The fee account is an
// expense, so a fee is positive too. The amount is the cash plus the fee: whether the
// fee was taken from the capture (cash gets the net) or booked later against cash, the
// payment's rows add up to the gross and the fee, as in a provider's report.
//
// The key is the provider's id from the transaction's meta, or the idempotency key when
// the meta has none, so a posting nobody can match (an opening balance) is reported
// instead of stopping the run.
const ledgerQuery = `
SELECT COALESCE(NULLIF(t.meta::jsonb ->> $2, ''), t.idempotency_key) AS key,
    SUM(CASE WHEN (p.side = 'D') = (a.type IN ('asset', 'expense')) THEN p.amount ELSE -p.amount END) AS amount,
    SUM(CASE WHEN p.account <> $3 THEN 0 WHEN p.side = 'D' THEN p.amount ELSE -p.amount END) AS fee,
    MIN(a.currency) AS currency
FROM ledger_postings p
JOIN ledger_transactions t ON t.id = p.transaction_id
JOIN ledger_accounts a ON a.code = p.account
WHERE p.account IN ($1, $3)
GROUP BY t.id, t.meta, t.idempotency_key
ORDER BY t.id`

func readLedger(ctx context.Context, side config.Source, currencies money.Currencies) ([]Row, []Input, error) {
	l := side.Ledger
	dsn := os.Getenv(l.DSNEnv)
	if dsn == "" {
		return nil, nil, fmt.Errorf("ianfoxdev_ledger: environment variable %s is empty", l.DSNEnv)
	}
	// Tells a misspelled account from one with no postings yet, and keeps the fee in
	// the same currency and on the side the query expects.
	check := func(tx pgx.Tx) error {
		var currency string
		err := tx.QueryRow(ctx, `SELECT currency FROM ledger_accounts WHERE code = $1`, l.Account).Scan(&currency)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("ianfoxdev_ledger: account %q is not in ledger_accounts", l.Account)
		}
		if err != nil {
			return fmt.Errorf("ianfoxdev_ledger: %w", err)
		}
		if l.FeeAccount == "" {
			return nil
		}
		var feeType, feeCurrency string
		err = tx.QueryRow(ctx, `SELECT type, currency FROM ledger_accounts WHERE code = $1`, l.FeeAccount).Scan(&feeType, &feeCurrency)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("ianfoxdev_ledger: fee account %q is not in ledger_accounts", l.FeeAccount)
		case err != nil:
			return fmt.Errorf("ianfoxdev_ledger: %w", err)
		case feeType != "expense":
			return fmt.Errorf("ianfoxdev_ledger: fee account %q is %s, want expense", l.FeeAccount, feeType)
		case feeCurrency != currency:
			return fmt.Errorf("ianfoxdev_ledger: fee account %q is in %s, account %q in %s", l.FeeAccount, feeCurrency, l.Account, currency)
		}
		return nil
	}
	return query(ctx, dsn, side, currencies, "ledger:"+l.Account, check, ledgerQuery, l.Account, l.Key, l.FeeAccount)
}
