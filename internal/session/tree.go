package session

import "strings"

type Kind int

const (
	KindGroup Kind = iota
	KindSession
)

// Node is one entry in the session tree. Group nodes carry children; session
// nodes carry the session itself.
type Node struct {
	Kind     Kind
	Path     string
	Label    string
	Session  *Session
	Children []*Node
	Pinned   bool
}

// BuildTree turns the flat session list into a group tree. Groups are formed
// from the "/"-delimited Group field; empty group means the root.
func BuildTree(sessions []Session) *Node {
	root := &Node{Kind: KindGroup, Label: ""}

	for i := range sessions {
		s := &sessions[i]
		cur := root
		for _, part := range s.GroupPath() {
			cur = childGroup(cur, part)
		}
		cur.Children = append(cur.Children, &Node{
			Kind:    KindSession,
			Path:    s.ID,
			Label:   s.Name,
			Session: s,
			Pinned:  s.Pinned,
		})
	}

	sortTree(root)
	return root
}

func childGroup(parent *Node, label string) *Node {
	path := label
	if parent.Path != "" {
		path = parent.Path + "/" + label
	}
	for _, c := range parent.Children {
		if c.Kind == KindGroup && c.Path == path {
			return c
		}
	}
	g := &Node{Kind: KindGroup, Path: path, Label: label}
	parent.Children = append(parent.Children, g)
	return g
}

// sortTree orders each group: pinned sessions first, then subgroups, then the
// remaining sessions. Ties break alphabetically, case-insensitively.
func sortTree(n *Node) {
	for _, c := range n.Children {
		sortTree(c)
	}
	if n.Kind == KindSession {
		return
	}
	less := func(a, b *Node) bool {
		if a.Pinned != b.Pinned {
			return a.Pinned
		}
		if (a.Kind == KindGroup) != (b.Kind == KindGroup) {
			return a.Kind == KindGroup
		}
		return strings.ToLower(a.Label) < strings.ToLower(b.Label)
	}
	for i := 1; i < len(n.Children); i++ {
		for j := i; j > 0 && less(n.Children[j], n.Children[j-1]); j-- {
			n.Children[j], n.Children[j-1] = n.Children[j-1], n.Children[j]
		}
	}
	for _, c := range n.Children {
		c.Pinned = c.Pinned || hasPinned(c)
	}
}

func hasPinned(n *Node) bool {
	for _, c := range n.Children {
		if c.Pinned || hasPinned(c) {
			return true
		}
	}
	return false
}

// Row is a flattened, renderable line of the tree.
type Row struct {
	Node     *Node
	Depth    int
	Prefix   string
	Expanded bool
}

// Flatten walks the tree in display order, honouring collapsed groups and an
// active filter. A non-empty query keeps only matching sessions and the groups
// that contain them, and forces those groups open.
func (n *Node) Flatten(collapsed map[string]bool, query string) []Row {
	rows := make([]Row, 0, len(n.Children))
	n.flatten(&rows, "", true, 0, collapsed, query)
	return rows
}

func (n *Node) flatten(rows *[]Row, prefix string, top bool, depth int, collapsed map[string]bool, query string) {
	children := n.Children
	if query != "" {
		children = nil
		for _, c := range n.Children {
			if c.matches(query) {
				children = append(children, c)
			}
		}
	}

	for i, c := range children {
		last := i == len(children)-1
		connector := ""
		if !top {
			if last {
				connector = "└─ "
			} else {
				connector = "├─ "
			}
		}

		isCollapsed := c.Kind == KindGroup && collapsed[c.Path] && query == ""
		*rows = append(*rows, Row{
			Node:     c,
			Depth:    depth,
			Prefix:   prefix + connector,
			Expanded: c.Kind == KindGroup && !isCollapsed,
		})

		if c.Kind == KindGroup && !isCollapsed {
			childPrefix := prefix
			if !top {
				if last {
					childPrefix += "   "
				} else {
					childPrefix += "│  "
				}
			}
			c.flatten(rows, childPrefix, false, depth+1, collapsed, query)
		}
	}
}

// matches reports whether a node is kept under query. Groups match when any
// descendant session does.
func (n *Node) matches(query string) bool {
	if n.Kind == KindSession {
		return n.Session.Matches(query)
	}
	for _, c := range n.Children {
		if c.matches(query) {
			return true
		}
	}
	return false
}

// GroupPaths lists every group path, for expand-all and collapse-all.
func (n *Node) GroupPaths() []string {
	var out []string
	var walk func(*Node)
	walk = func(cur *Node) {
		for _, c := range cur.Children {
			if c.Kind == KindGroup {
				out = append(out, c.Path)
				walk(c)
			}
		}
	}
	walk(n)
	return out
}
