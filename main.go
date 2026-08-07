// go-yaml 是一个轻量的 YAML 字段查询工具，不引入第三方库。
// 支持用点路径取值，例如 a.b.c；列表用 a.0.b 这种下标访问。
// 能处理最常见的写法：缩进层级、键值、列表（- 项）、嵌套、# 注释、引号包裹。
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// node 是解析出来的一个值，可能是标量、映射或列表。
type node struct {
	kind  string // "scalar" | "map" | "list"
	value string
	items []*node // list 用
	keys  map[string]*node
}

// indentOf 返回一行 leading 空格的数量，按 2 空格一个缩进层级也行，
// 这里直接数空格字符，混合 tab 的我们按 4 个空格算一个 tab。
func indentOf(line string) int {
	n := 0
	for _, r := range line {
		if r == ' ' {
			n++
		} else if r == '\t' {
			n += 4
		} else {
			break
		}
	}
	return n
}

// stripComment 去掉行内 # 注释，但要小心值本身就含 # 的情况（引号里的不删）。
func stripComment(s string) string {
	inSingle, inDouble := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\'' && !inDouble {
			inSingle = !inSingle
		} else if c == '"' && !inSingle {
			inDouble = !inDouble
		} else if c == '#' && !inSingle && !inDouble {
			if i == 0 || s[i-1] == ' ' || s[i-1] == '\t' {
				return s[:i]
			}
		}
	}
	return s
}

// unquote 去掉值两端的引号，并处理常见的转义。
func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			inner := s[1 : len(s)-1]
			inner = strings.ReplaceAll(inner, `\"`, `"`)
			inner = strings.ReplaceAll(inner, `\'`, `'`)
			return inner
		}
	}
	return s
}

// parseYAML 把文本解析成带缩进层级的节点树。
// 实现思路：按缩进层级用栈维护当前容器，遇到 key: 进 map，遇到 - 进 list。
func parseYAML(text string) (*node, error) {
	// Windows 编辑器常给文件加 UTF-8 BOM，去掉免得首行键名被污染成 ï»¿server
	text = strings.TrimPrefix(text, "\xef\xbb\xbf")
	root := &node{kind: "map", keys: map[string]*node{}}
	type frame struct {
		indent int
		node   *node
	}
	stack := []frame{{indent: -1, node: root}}

	lines := strings.Split(text, "\n")
	for _, raw := range lines {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		line := stripComment(raw)
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := indentOf(line)
		// 弹掉缩进更深或相等的层
		for len(stack) > 1 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
		cur := stack[len(stack)-1].node

		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") || trimmed == "-" {
			item := strings.TrimPrefix(trimmed, "- ")
			item = strings.TrimSpace(item)
			if cur.kind != "list" {
				cur.kind = "list"
				cur.items = nil
			}
			var child *node
			if i := strings.Index(item, ":"); i > 0 && item[i-1] != '\\' {
				// 列表项是带 key 的映射，例如 - name: foo
				child = &node{kind: "map", keys: map[string]*node{}}
			} else {
				child = &node{kind: "scalar", value: unquote(item)}
			}
			cur.items = append(cur.items, child)
			// 列表项可能是个映射，继续让它接收子键
			if child.kind == "map" {
				// 先把这一项的 key:value 就地解析进去
				if i := strings.Index(item, ":"); i > 0 {
					key := strings.TrimSpace(item[:i])
					val := strings.TrimSpace(item[i+1:])
					if strings.HasPrefix(val, "- ") {
						// 嵌套列表，简化：当成标量留着
						child.keys[key] = &node{kind: "scalar", value: unquote(strings.TrimPrefix(val, "- "))}
					} else if val == "" {
						// 值是下一层缩进给的，先留空待后续行填充
						child.keys[key] = &node{kind: "scalar", value: ""}
					} else {
						child.keys[key] = &node{kind: "scalar", value: unquote(val)}
					}
				}
				stack = append(stack, frame{indent: indent + 1, node: child})
			}
			continue
		}

		// 普通 key: value
		i := strings.Index(trimmed, ":")
		if i < 0 {
			continue
		}
		key := strings.TrimSpace(trimmed[:i])
		val := strings.TrimSpace(trimmed[i+1:])
		if cur.kind != "map" {
			cur.kind = "map"
			if cur.keys == nil {
				cur.keys = map[string]*node{}
			}
		}
		if val == "" {
			// 值在下层，先放一个空 map 占位
			child := &node{kind: "map", keys: map[string]*node{}}
			cur.keys[key] = child
			stack = append(stack, frame{indent: indent, node: child})
		} else if strings.HasPrefix(val, "- ") {
			// 行内列表：key: - a - b
			child := &node{kind: "list"}
			for _, part := range strings.Split(val, "- ") {
				part = strings.TrimSpace(part)
				if part != "" {
					child.items = append(child.items, &node{kind: "scalar", value: unquote(part)})
				}
			}
			cur.keys[key] = child
		} else {
			cur.keys[key] = &node{kind: "scalar", value: unquote(val)}
		}
	}
	return root, nil
}

// getPath 按点路径取值。支持 a.b.c 和列表下标 a.0.b。
func getPath(root *node, path string) (*node, bool) {
	parts := strings.Split(path, ".")
	cur := root
	for _, p := range parts {
		if cur == nil || cur.kind != "map" {
			// 试着当列表下标
			if idx, err := strconv.Atoi(p); err == nil && cur != nil && cur.kind == "list" {
				if idx >= 0 && idx < len(cur.items) {
					cur = cur.items[idx]
					continue
				}
			}
			return nil, false
		}
		next, ok := cur.keys[p]
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

func nodeToString(n *node) string {
	switch n.kind {
	case "scalar":
		return n.value
	case "map":
		// 打印成紧凑的 key=value 序列，便于一眼看
		var parts []string
		for k, v := range n.keys {
			parts = append(parts, fmt.Sprintf("%s=%s", k, nodeToString(v)))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case "list":
		var parts []string
		for _, it := range n.items {
			parts = append(parts, nodeToString(it))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
	return ""
}

func main() {
	path := flag.String("p", "", "要查询的点路径，例如 server.port 或 items.0.name")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "用法: go-yaml -p 路径 [文件]\n  不写文件则从标准输入读")
	}
	flag.Parse()

	var text string
	if flag.NArg() > 0 {
		data, err := os.ReadFile(flag.Arg(0))
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取失败:", err)
			os.Exit(1)
		}
		text = string(data)
	} else {
		sc := bufio.NewScanner(os.Stdin)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		var b strings.Builder
		for sc.Scan() {
			b.WriteString(sc.Text())
			b.WriteString("\n")
		}
		text = b.String()
	}

	root, err := parseYAML(text)
	if err != nil {
		fmt.Fprintln(os.Stderr, "解析失败:", err)
		os.Exit(1)
	}

	if *path == "" {
		// 没给路径就打印顶层所有键
		for k := range root.keys {
			fmt.Println(k)
		}
		return
	}
	n, ok := getPath(root, *path)
	if !ok {
		fmt.Fprintf(os.Stderr, "找不到路径: %s\n", *path)
		os.Exit(1)
	}
	fmt.Println(nodeToString(n))
}
