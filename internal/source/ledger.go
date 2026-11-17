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

// ledgerQuery reads every posting on one account of ianfoxdev/ledger. The amount is on
// the account's normal side, as balance_after is: for a cash account a deposit is
// positive and a refund negative. The key is the provider's id from the transaction's
// meta, or the idempotency key when the meta has none, so a posting nobody can match
// (an opening balance) is reported instead of stopping the run.
const ledgerQuery = `
SELECT COALESCE(NULLIF(t.meta::jsonb ->> $2, ''), t.idempotency_key) AS key,
    CASE WHEN (p.side = 'D') = (a.type IN ('asset', 'expense')) THEN p.amount ELSE -p.amount END AS amount,
    0 AS fee,
    a.currency
FROM ledger_postings p
JOIN ledger_transactions t ON t.id = p.transaction_id
JOIN ledger_accounts a ON a.code = p.account
WHERE p.account = $1
ORDER BY p.sequence`

func readLedger(ctx context.Context, side config.Source, currencies money.Currencies) ([]Row, []Input, error) {
	l := side.Ledger
	dsn := os.Getenv(l.DSNEnv)
	if dsn == "" {
		return nil, nil, fmt.Errorf("ianfoxdev_ledger: environment variable %s is empty", l.DSNEnv)
	}
	// Tells a misspelled account from one with no postings yet.
	exists := func(tx pgx.Tx) error {
		var code string
		err := tx.QueryRow(ctx, `SELECT code FROM ledger_accounts WHERE code = $1`, l.Account).Scan(&code)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("ianfoxdev_ledger: account %q is not in ledger_accounts", l.Account)
		}
		if err != nil {
			return fmt.Errorf("ianfoxdev_ledger: %w", err)
		}
		return nil
	}
	return query(ctx, dsn, side, currencies, "ledger:"+l.Account, exists, ledgerQuery, l.Account, l.Key)
}
