package winapi

import (
	"fmt"
	"strings"
)

// Минимальный парсер/сериализатор Valve KeyValues (VDF) — для localconfig.vdf.

type KV struct {
	Key      string
	Value    string
	Children []*KV
	IsObj    bool
}

func (k *KV) Child(name string) *KV {
	for _, c := range k.Children {
		if strings.EqualFold(c.Key, name) {
			return c
		}
	}
	return nil
}

func (k *KV) Ensure(name string) *KV {
	if c := k.Child(name); c != nil {
		return c
	}
	c := &KV{Key: name, IsObj: true}
	k.Children = append(k.Children, c)
	return c
}

func (k *KV) Set(name, val string) {
	if c := k.Child(name); c != nil && !c.IsObj {
		c.Value = val
		return
	}
	k.Children = append(k.Children, &KV{Key: name, Value: val})
}

type vdfLexer struct {
	s   string
	pos int
}

func (l *vdfLexer) next() (tok string, quoted bool, ok bool) {
	for l.pos < len(l.s) {
		c := l.s[l.pos]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			l.pos++
		case c == '/' && l.pos+1 < len(l.s) && l.s[l.pos+1] == '/':
			for l.pos < len(l.s) && l.s[l.pos] != '\n' {
				l.pos++
			}
		case c == '{' || c == '}':
			l.pos++
			return string(c), false, true
		case c == '"':
			l.pos++
			var b strings.Builder
			for l.pos < len(l.s) && l.s[l.pos] != '"' {
				if l.s[l.pos] == '\\' && l.pos+1 < len(l.s) {
					b.WriteByte(l.s[l.pos])
					l.pos++
				}
				b.WriteByte(l.s[l.pos])
				l.pos++
			}
			l.pos++
			return b.String(), true, true
		default:
			start := l.pos
			for l.pos < len(l.s) && !strings.ContainsRune(" \t\r\n{}\"", rune(l.s[l.pos])) {
				l.pos++
			}
			return l.s[start:l.pos], true, true
		}
	}
	return "", false, false
}

// ParseVDF разбирает текст в корневой узел (корень — безымянный объект).
func ParseVDF(s string) (*KV, error) {
	l := &vdfLexer{s: s}
	root := &KV{IsObj: true}
	stack := []*KV{root}
	for {
		tok, quoted, ok := l.next()
		if !ok {
			break
		}
		cur := stack[len(stack)-1]
		if !quoted && tok == "}" {
			if len(stack) == 1 {
				return nil, fmt.Errorf("vdf: лишняя }")
			}
			stack = stack[:len(stack)-1]
			continue
		}
		if !quoted {
			return nil, fmt.Errorf("vdf: неожиданный %q", tok)
		}
		val, vq, ok := l.next()
		if !ok {
			return nil, fmt.Errorf("vdf: нет значения для %q", tok)
		}
		if !vq && val == "{" {
			n := &KV{Key: tok, IsObj: true}
			cur.Children = append(cur.Children, n)
			stack = append(stack, n)
		} else {
			cur.Children = append(cur.Children, &KV{Key: tok, Value: val})
		}
	}
	if len(stack) != 1 {
		return nil, fmt.Errorf("vdf: не закрыт блок")
	}
	return root, nil
}

func (k *KV) write(b *strings.Builder, depth int) {
	ind := strings.Repeat("\t", depth)
	for _, c := range k.Children {
		if c.IsObj {
			fmt.Fprintf(b, "%s\"%s\"\n%s{\n", ind, c.Key, ind)
			c.write(b, depth+1)
			fmt.Fprintf(b, "%s}\n", ind)
		} else {
			fmt.Fprintf(b, "%s\"%s\"\t\t\"%s\"\n", ind, c.Key, c.Value)
		}
	}
}

func (k *KV) String() string {
	var b strings.Builder
	k.write(&b, 0)
	return b.String()
}
