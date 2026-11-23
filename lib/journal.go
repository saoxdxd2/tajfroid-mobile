package lib

import "time"

// Transaction types shared with the desktop finance journal.
const (
	TxSale            = "Sale"
	TxPurchase        = "Purchase"
	TxExpense         = "Expense"
	TxPaymentReceived = "Payment Received"
	TxPaymentSent     = "Payment Sent"
	TxManualAdjust    = "Manual Adjustment"
)

// Journal appends one immutable financial event. Amounts are signed: positive
// is money in, negative is money out.
func Journal(db *TursoClient, transactionType, referenceID, description string, amountCents int64, at time.Time) error {
	_, err := db.Execute(`INSERT INTO journal_entries
		(id, transaction_type, reference_id, description, amount, is_reversed, created_at)
		VALUES (?,?,?,?,?,?,?)`,
		Text(NewID()), Text(transactionType), Text(referenceID), Text(description),
		Text(FormatMoney(amountCents)), Int(0), Int(at.Unix()))
	return err
}
