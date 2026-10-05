package config

import "sort"

// profile fills in the columns of a known export, so a config only names the key.
type profile struct {
	columns Columns
	amounts string
	filter  []Filter
}

// profiles of provider exports. stripe_balance is the itemized Balance report
// (balance_change_from_activity.itemized.*): gross, fee and net in major units.
var profiles = map[string]profile{
	"stripe_balance": {
		columns: Columns{Amount: "gross", Fee: "fee", Currency: "currency"},
		amounts: "major",
	},
}

func (p profile) apply(s *Source) {
	if s.Columns.Amount == "" {
		s.Columns.Amount = p.columns.Amount
	}
	if s.Columns.Fee == "" {
		s.Columns.Fee = p.columns.Fee
	}
	if s.Columns.Currency == "" {
		s.Columns.Currency = p.columns.Currency
	}
	if s.Amounts == "" {
		s.Amounts = p.amounts
	}
	if s.Filter == nil {
		s.Filter = p.filter
	}
}

func profileNames() []string {
	names := make([]string, 0, len(profiles))
	for n := range profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
