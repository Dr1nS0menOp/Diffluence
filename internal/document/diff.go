// SPDX-License-Identifier: Apache-2.0

package document

import "strings"

// Diff operations.
const (
	OpEqual   = "equal"
	OpAdded   = "added"
	OpRemoved = "removed"
	OpChanged = "changed"
)

// WordOp is one run of words in a changed block.
type WordOp struct {
	Op   string `json:"op"` // equal, added, removed
	Text string `json:"text"`
}

// DiffBlock is one top-level block in a comparison.
type DiffBlock struct {
	Op     string   `json:"op"`
	Type   string   `json:"type"`
	Before string   `json:"before,omitempty"`
	After  string   `json:"after,omitempty"`
	Words  []WordOp `json:"words,omitempty"`
}

// maxDiffCells caps the comparison table. Beyond it blocks are shown as
// replaced instead of aligned, so a huge page cannot exhaust the server.
const maxDiffCells = 16_000_000

// lcs returns, for two sequences, the pairs of indexes that stay the same.
func lcs(n, m int, equal func(i, j int) bool) [][2]int {
	if n == 0 || m == 0 || n*m > maxDiffCells {
		return nil
	}
	table := make([][]int32, n+1)
	for i := range table {
		table[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if equal(i, j) {
				table[i][j] = table[i+1][j+1] + 1
			} else {
				table[i][j] = max(table[i+1][j], table[i][j+1])
			}
		}
	}
	var out [][2]int
	for i, j := 0, 0; i < n && j < m; {
		switch {
		case equal(i, j):
			out = append(out, [2]int{i, j})
			i++
			j++
		case table[i+1][j] >= table[i][j+1]:
			i++
		default:
			j++
		}
	}
	return out
}

// Compare aligns the top-level blocks of two documents. A removed block directly
// followed by an added block of the same type is shown as one changed block.
func Compare(before, after Node) []DiffBlock {
	a, b := before.Content, after.Content
	texts := func(ns []Node) []string {
		out := make([]string, len(ns))
		for i, n := range ns {
			out[i] = n.Type + "\x00" + compareKey(n)
		}
		return out
	}
	ta, tb := texts(a), texts(b)
	pairs := lcs(len(a), len(b), func(i, j int) bool { return ta[i] == tb[j] })

	var out []DiffBlock
	i, j := 0, 0
	flush := func(toI, toJ int) {
		for i < toI && j < toJ && a[i].Type == b[j].Type {
			before, after := BlockText(a[i]), BlockText(b[j])
			out = append(out, DiffBlock{Op: OpChanged, Type: a[i].Type, Before: before, After: after, Words: compareWords(before, after)})
			i++
			j++
		}
		for ; i < toI; i++ {
			out = append(out, DiffBlock{Op: OpRemoved, Type: a[i].Type, Before: BlockText(a[i])})
		}
		for ; j < toJ; j++ {
			out = append(out, DiffBlock{Op: OpAdded, Type: b[j].Type, After: BlockText(b[j])})
		}
	}
	for _, p := range pairs {
		flush(p[0], p[1])
		out = append(out, DiffBlock{Op: OpEqual, Type: a[i].Type, Before: BlockText(a[i]), After: BlockText(b[j])})
		i++
		j++
	}
	flush(len(a), len(b))
	return out
}

// compareKey is what makes two blocks the same. An image with the same caption
// can still be another image, and it must show as changed (PAG-092).
func compareKey(n Node) string {
	files := []string{BlockText(n)}
	Walk(n, func(c Node) {
		if id, ok := c.Attrs["attachment_id"].(string); ok && pointsToAttachment(c) {
			files = append(files, id)
		}
	})
	return strings.Join(files, "\x00")
}

func compareWords(before, after string) []WordOp {
	a, b := strings.Fields(before), strings.Fields(after)
	pairs := lcs(len(a), len(b), func(i, j int) bool { return a[i] == b[j] })
	var out []WordOp
	add := func(op, word string) {
		if n := len(out); n > 0 && out[n-1].Op == op {
			out[n-1].Text += " " + word
			return
		}
		out = append(out, WordOp{Op: op, Text: word})
	}
	i, j := 0, 0
	flush := func(toI, toJ int) {
		for ; i < toI; i++ {
			add(OpRemoved, a[i])
		}
		for ; j < toJ; j++ {
			add(OpAdded, b[j])
		}
	}
	for _, p := range pairs {
		flush(p[0], p[1])
		add(OpEqual, a[i])
		i++
		j++
	}
	flush(len(a), len(b))
	return out
}
