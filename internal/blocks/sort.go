// SPDX-License-Identifier: Apache-2.0

package blocks

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// factValue is what a cell is worth when sorting: a date or a number by its real value,
// and text after numbers (BLK-041).
type factValue struct {
	rank   int
	number float64
	text   string
}

func valueOf(cell string) factValue {
	text := strings.ToLower(strings.TrimSpace(cell))
	if d, err := time.Parse(time.DateOnly, text); err == nil {
		return factValue{number: float64(d.Unix())}
	}
	if n, err := strconv.ParseFloat(strings.ReplaceAll(text, ",", "."), 64); err == nil {
		return factValue{number: n}
	}
	return factValue{rank: 1, text: text}
}

func (a factValue) less(b factValue) bool {
	if a.rank != b.rank {
		return a.rank < b.rank
	}
	if a.rank == 0 {
		return a.number < b.number
	}
	return a.text < b.text
}

func sortFacts(rows []Row, column int, descending bool) {
	if column < 0 {
		return
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := valueOf(rows[i].Cells[column]), valueOf(rows[j].Cells[column])
		if descending {
			return b.less(a)
		}
		return a.less(b)
	})
}
