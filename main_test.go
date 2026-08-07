package main

import (
	"testing"
)

func TestSimpleKey(t *testing.T) {
	root, err := parseYAML("name: hello\nage: 3\n")
	if err != nil {
		t.Fatal(err)
	}
	if n, ok := getPath(root, "name"); !ok || n.value != "hello" {
		t.Errorf("name 应为 hello，实际 %v %v", n, ok)
	}
	if n, ok := getPath(root, "age"); !ok || n.value != "3" {
		t.Errorf("age 应为 3，实际 %v %v", n, ok)
	}
}

func TestNested(t *testing.T) {
	yaml := `
server:
  host: 127.0.0.1
  port: 8080
`
	root, _ := parseYAML(yaml)
	if n, ok := getPath(root, "server.port"); !ok || n.value != "8080" {
		t.Errorf("server.port 应为 8080，实际 %v %v", n, ok)
	}
	if n, ok := getPath(root, "server.host"); !ok || n.value != "127.0.0.1" {
		t.Errorf("server.host 应为 127.0.0.1，实际 %v %v", n, ok)
	}
}

func TestListOfScalars(t *testing.T) {
	yaml := `
fruits:
  - apple
  - banana
`
	root, _ := parseYAML(yaml)
	if n, ok := getPath(root, "fruits.0"); !ok || n.value != "apple" {
		t.Errorf("fruits.0 应为 apple，实际 %v %v", n, ok)
	}
	if n, ok := getPath(root, "fruits.1"); !ok || n.value != "banana" {
		t.Errorf("fruits.1 应为 banana，实际 %v %v", n, ok)
	}
}

func TestListOfMaps(t *testing.T) {
	yaml := `
items:
  - name: a
    val: 1
  - name: b
    val: 2
`
	root, _ := parseYAML(yaml)
	if n, ok := getPath(root, "items.0.name"); !ok || n.value != "a" {
		t.Errorf("items.0.name 应为 a，实际 %v %v", n, ok)
	}
	if n, ok := getPath(root, "items.1.val"); !ok || n.value != "2" {
		t.Errorf("items.1.val 应为 2，实际 %v %v", n, ok)
	}
}

func TestCommentIgnored(t *testing.T) {
	yaml := "name: hello # 这是注释\n# 整行注释\nage: 5"
	root, _ := parseYAML(yaml)
	if n, ok := getPath(root, "name"); !ok || n.value != "hello" {
		t.Errorf("name 应为 hello，实际 %q", n.value)
	}
	if n, ok := getPath(root, "age"); !ok || n.value != "5" {
		t.Errorf("age 应为 5，实际 %q", n.value)
	}
}

func TestQuotedHash(t *testing.T) {
	// 值里含 # 但被引号包住，不应当注释删
	yaml := `desc: "a # b"`
	root, _ := parseYAML(yaml)
	if n, ok := getPath(root, "desc"); !ok || n.value != "a # b" {
		t.Errorf("desc 应为 'a # b'，实际 %q", n.value)
	}
}

func TestMissingPath(t *testing.T) {
	root, _ := parseYAML("a: 1\n")
	if _, ok := getPath(root, "b.c"); ok {
		t.Errorf("不存在的路径应返回 false")
	}
}

func TestParseYAMLStripBOM(t *testing.T) {
	// 带 UTF-8 BOM 的文件，首行键名不能被污染成 ï»¿server
	yaml := "\xef\xbb\xbfserver:\n  port: 8080\n"
	root, err := parseYAML(yaml)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := getPath(root, "server.port"); !ok {
		t.Errorf("带 BOM 时 server.port 应可取，实际 root.keys=%v", root.keys)
	}
}
