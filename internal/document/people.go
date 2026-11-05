// SPDX-License-Identifier: Apache-2.0

package document

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/apperr"
)

// The kinds of mention.
const (
	MentionUser  = "user"
	MentionGroup = "group"
)

// Limits on a comment (spec 010).
const (
	MaxCommentBytes = 100 << 10
	maxCommentDepth = 12
)

// mentionText is what a mention says once reading has filled in the name. Stored text
// such as the search index has no name, so a mention says nothing there.
func mentionText(n Node) string {
	label, _ := n.Attrs["label"].(string)
	if label == "" {
		return ""
	}
	return "@" + label
}

// quoteLeaf stands for an inline node without text, as in the browser's index of a
// document (web/src/components/comments/anchor.ts).
const quoteLeaf = "￼"

func withoutLineBreaks(s string) string { return strings.ReplaceAll(s, "\n", "") }

// HoldsQuote reports whether a document has the words of a comment. The browser puts a
// line break between blocks, and line breaks do not count here.
func HoldsQuote(n Node, quote string) bool {
	wanted := withoutLineBreaks(quote)
	if wanted == "" {
		return false
	}
	var words strings.Builder
	Walk(n, func(c Node) {
		switch {
		case c.Type == TypeText:
			words.WriteString(c.Text)
		case contains(inlineTypes, c.Type):
			words.WriteString(quoteLeaf)
		}
	})
	return strings.Contains(withoutLineBreaks(words.String()), wanted)
}

// HoldsAttachment reports whether a document shows a file.
func HoldsAttachment(n Node, id uuid.UUID) bool {
	found := false
	Walk(n, func(c Node) {
		if s, _ := c.Attrs["attachment_id"].(string); s == id.String() {
			found = true
		}
	})
	return found
}

func mentionID(n Node, kind string) (uuid.UUID, bool) {
	if n.Type != TypeMention {
		return uuid.Nil, false
	}
	if k, _ := n.Attrs["kind"].(string); k != kind {
		return uuid.Nil, false
	}
	s, _ := n.Attrs["id"].(string)
	id, err := uuid.Parse(s)
	return id, err == nil
}

func mentioned(n Node, kind string) []uuid.UUID {
	seen := map[uuid.UUID]bool{}
	out := []uuid.UUID{}
	Walk(n, func(c Node) {
		if id, ok := mentionID(c, kind); ok && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	})
	return out
}

// MentionedUsers lists the people a document mentions, each once.
func MentionedUsers(n Node) []uuid.UUID { return mentioned(n, MentionUser) }

// MentionedGroups lists the groups a document mentions, each once.
func MentionedGroups(n Node) []uuid.UUID { return mentioned(n, MentionGroup) }

// HasMentions is a quick look at stored JSON, so that a document without mentions
// does not need parsing on every read.
func HasMentions(raw []byte) bool { return bytes.Contains(raw, []byte(`"mention"`)) }

// FillMentionNames writes today's names into a document for reading, so that a
// mention follows a rename while the stored version stays what it was.
func FillMentionNames(n *Node, names map[uuid.UUID]string) {
	if n.Type == TypeMention {
		s, _ := n.Attrs["id"].(string)
		if id, err := uuid.Parse(s); err == nil {
			if name, ok := names[id]; ok {
				n.Attrs["label"] = name
			}
		}
	}
	for i := range n.Content {
		FillMentionNames(&n.Content[i], names)
	}
}

// Task is one line of a task list, as the index holds it.
type Task struct {
	ID         uuid.UUID
	Text       string
	AssigneeID *uuid.UUID
	Due        string
	Done       bool
}

// ownLine is the first text block of a task: nested lists are tasks of their own.
func ownLine(item Node) (Node, bool) {
	for _, c := range item.Content {
		if c.Type == TypeParagraph || c.Type == TypeHeading {
			return c, true
		}
	}
	return Node{}, false
}

func taskID(n Node) (uuid.UUID, bool) {
	s, _ := n.Attrs["id"].(string)
	id, err := uuid.Parse(s)
	return id, err == nil
}

// Tasks lists the tasks of a document in order. The first person mentioned in a
// task's own line is its assignee, and the first date its due date (spec 010, D7).
func Tasks(n Node) []Task {
	out := []Task{}
	Walk(n, func(c Node) {
		if c.Type != TypeTaskItem {
			return
		}
		t := Task{}
		t.ID, _ = taskID(c)
		t.Done, _ = c.Attrs["checked"].(bool)
		line, ok := ownLine(c)
		if ok {
			readTaskLine(&t, line)
		}
		out = append(out, t)
	})
	return out
}

// readTaskLine takes the first person and date from a line. They have their own place
// in a list of tasks, so the words of the task go without them.
func readTaskLine(t *Task, line Node) {
	words := line
	words.Content = nil
	for _, part := range line.Content {
		if id, isPerson := mentionID(part, MentionUser); isPerson && t.AssigneeID == nil {
			t.AssigneeID = &id
			continue
		}
		if value, isDate := part.Attrs["value"].(string); part.Type == TypeDate && isDate && t.Due == "" {
			t.Due = value
			continue
		}
		words.Content = append(words.Content, part)
	}
	t.Text = strings.Join(strings.Fields(BlockText(words)), " ")
}

// AssignTaskIDs gives every task its own id. A task without one, a repeated one,
// or one that is taken by another page gets a new one from newID.
func AssignTaskIDs(n *Node, isTaken func(uuid.UUID) bool, newID func() uuid.UUID) {
	seen := map[uuid.UUID]bool{}
	var visit func(c *Node)
	visit = func(c *Node) {
		if c.Type == TypeTaskItem {
			id, ok := taskID(*c)
			if !ok || seen[id] || isTaken(id) {
				id = newID()
				if c.Attrs == nil {
					c.Attrs = map[string]any{}
				}
				c.Attrs["id"] = id.String()
			}
			seen[id] = true
		}
		for i := range c.Content {
			visit(&c.Content[i])
		}
	}
	visit(n)
}

// SetTaskDone ticks or unticks one task, and reports whether it was found.
func SetTaskDone(n *Node, id uuid.UUID, done bool) bool {
	if n.Type == TypeTaskItem {
		if got, ok := taskID(*n); ok && got == id {
			n.Attrs["checked"] = done
			return true
		}
	}
	for i := range n.Content {
		if SetTaskDone(&n.Content[i], id, done) {
			return true
		}
	}
	return false
}

// A comment goes through the same gate as a document, with a shorter list: one
// gate and one rule, and a second sanitiser cannot drift (spec 010, D4).
var (
	commentTypes = map[string]bool{TypeDoc: true, TypeParagraph: true, TypeBulletList: true, TypeOrderedList: true,
		TypeListItem: true, TypeCodeBlock: true, TypeHardBreak: true, TypeImage: true, TypeMention: true, TypeText: true}
	commentMarks = map[string]bool{MarkBold: true, MarkItalic: true, MarkCode: true, MarkLink: true}
)

func checkComment(n Node, depth int) error {
	if depth > maxCommentDepth {
		return errors.New("the comment is nested too deeply")
	}
	if !commentTypes[n.Type] {
		return fmt.Errorf("a comment cannot hold %q", truncate(n.Type, 40))
	}
	for _, m := range n.Marks {
		if !commentMarks[m.Type] {
			return fmt.Errorf("a comment cannot hold %q", truncate(m.Type, 40))
		}
	}
	for _, c := range n.Content {
		if err := checkComment(c, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// IsBlank reports whether a document says nothing at all.
func IsBlank(n Node) bool {
	blank := true
	Walk(n, func(c Node) {
		if (c.Type == TypeText && strings.TrimSpace(c.Text) != "") || c.Type == TypeImage || c.Type == TypeMention {
			blank = false
		}
	})
	return blank
}

// NormalizeComment parses and checks the body of a comment.
func NormalizeComment(raw json.RawMessage) (Node, error) {
	if len(raw) > MaxCommentBytes {
		return Node{}, apperr.TooLarge("A comment can be at most 100 KiB.")
	}
	n, err := Normalize(raw)
	if err != nil {
		return Node{}, err
	}
	if err := checkComment(n, 0); err != nil {
		return Node{}, apperr.Invalid(err.Error())
	}
	if IsBlank(n) {
		return Node{}, apperr.Invalid("A comment needs some words.")
	}
	return n, nil
}
