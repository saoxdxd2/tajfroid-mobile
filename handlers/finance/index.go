package handler

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"time"

	"tajfroid-mobile/lib"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	db := lib.NewTurso()
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Unix()
	end := now.Unix() + 1
	if v := r.URL.Query().Get("start"); v != "" {
		if n, err := parseUnix(v); err == nil {
			start = n
		}
	}
	if v := r.URL.Query().Get("end"); v != "" {
		if n, err := parseUnix(v); err == nil {
			end = n
		}
	}

	// Cash flow from the immutable journal (reversed entries excluded).
	flowRes, err := db.Execute(`SELECT transaction_type, amount FROM journal_entries
		WHERE is_reversed = 0 AND created_at >= ? AND created_at < ?`, lib.Int(start), lib.Int(end))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	inflow, outflow := int64(0), int64(0)
	breakdown := map[string]int64{}
	for _, row := range flowRes.Rows {
		txType := lib.ToString(row[0])
		amount := lib.ParseMoney(lib.ToString(row[1]))
		breakdown[txType] += amount
		if amount >= 0 {
			inflow += amount
		} else {
			outflow += -amount
		}
	}

	// Debtors: invoiced (validated/issued/paid) minus payments, per customer.
	// Aggregate in SQL so both time and space scale with customers (C), not
	// invoices/payments (I/P).
	invRes, err := db.Execute(`SELECT i.customer_id, c.company_name, SUM(i.total_amount)
		FROM invoices i LEFT JOIN customers c ON i.customer_id = c.id
		WHERE i.state IN ('Validated','Issued','Paid')
		GROUP BY i.customer_id, c.company_name`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type debtor struct {
		name        string
		invoiced    int64
		paid        int64
		outstanding int64
	}
	debtors := map[string]*debtor{}
	for _, row := range invRes.Rows {
		customerID := lib.ToString(row[0])
		debtors[customerID] = &debtor{
			name:     lib.ToString(row[1]),
			invoiced: lib.ParseMoney(lib.ToString(row[2])),
		}
	}
	payRes, err := db.Execute(`SELECT i.customer_id, SUM(p.amount)
		FROM payments p JOIN invoices i ON p.invoice_id = i.id
		WHERE i.state IN ('Validated','Issued','Paid')
		GROUP BY i.customer_id`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, row := range payRes.Rows {
		if d := debtors[lib.ToString(row[0])]; d != nil {
			d.paid = lib.ParseMoney(lib.ToString(row[1]))
		}
	}

	type debtorRow struct {
		outstanding int64
		data        map[string]interface{}
	}
	rows := make([]debtorRow, 0, len(debtors))
	totalOutstanding := int64(0)
	for customerID, d := range debtors {
		d.outstanding = d.invoiced - d.paid
		if d.outstanding <= 0 {
			continue
		}
		totalOutstanding += d.outstanding
		rows = append(rows, debtorRow{
			outstanding: d.outstanding,
			data: map[string]interface{}{
				"customerId": customerID, "companyName": d.name,
				"totalInvoiced": lib.FormatMoney(d.invoiced),
				"totalPaid":     lib.FormatMoney(d.paid),
				"outstanding":   lib.FormatMoney(d.outstanding),
			},
		})
	}
	// O(C log C) with integer cents; no ParseMoney inside comparisons.
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].outstanding > rows[j].outstanding })
	debtorList := make([]map[string]interface{}, 0, len(rows))
	for _, r := range rows {
		debtorList = append(debtorList, r.data)
	}

	// Checks due in the next 7 days, including overdue pending ones.
	horizon := now.AddDate(0, 0, 7).Unix()
	checkRes, err := db.Execute(`SELECT id, check_number, direction, amount, due_date, party_name
		FROM checks WHERE status = 'Pending' AND due_date <= ? ORDER BY due_date ASC LIMIT 50`, lib.Int(horizon))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	upcoming := []map[string]interface{}{}
	for _, row := range checkRes.Rows {
		upcoming = append(upcoming, map[string]interface{}{
			"id": row[0], "checkNumber": row[1], "direction": row[2],
			"amount": row[3], "dueDate": row[4], "partyName": row[5],
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"start": start, "end": end,
		"inflow":           lib.FormatMoney(inflow),
		"outflow":          lib.FormatMoney(outflow),
		"net":              lib.FormatMoney(inflow - outflow),
		"breakdown":        toStringMap(breakdown),
		"debtors":          debtorList,
		"totalOutstanding": lib.FormatMoney(totalOutstanding),
		"upcomingChecks":   upcoming,
	})
}

func parseUnix(v string) (int64, error) {
	return strconv.ParseInt(v, 10, 64)
}

func toStringMap(m map[string]int64) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		out[k] = lib.FormatMoney(v)
	}
	return out
}
