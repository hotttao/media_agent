// Package skillarchive normalizes editable Skill trees into Agent-compatible ZIPs.
package skillarchive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"gopkg.in/yaml.v3"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const MaxBytes = 8 << 20
const MaxExpanded = 32 << 20
const MaxFiles = 1024

var ErrInvalid = errors.New("invalid skill content")

type File struct {
	Content    []byte
	Executable bool
}
type Tree map[string]File
type Header struct{ Name, Description string }

var slug = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func Hash(body []byte) string { sum := sha256.Sum256(body); return hex.EncodeToString(sum[:]) }
func Path(name string) bool {
	if name == "" || strings.ContainsAny(name, "\\\x00") || path.IsAbs(name) || len(name) > 1024 {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}
func check(tree Tree) error {
	if len(tree) > MaxFiles {
		return ErrInvalid
	}
	total := 0
	for name, file := range tree {
		if !Path(name) {
			return ErrInvalid
		}
		total += len(file.Content)
		if len(file.Content) > MaxBytes || total > MaxExpanded {
			return ErrInvalid
		}
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if _, ok := tree[parent]; ok {
				return ErrInvalid
			}
		}
	}
	return nil
}
func Pack(tree Tree) ([]byte, error) {
	if err := check(tree); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	names := make([]string, 0, len(tree))
	for name := range tree {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		file := tree[name]
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		mode := fs.FileMode(0644)
		if file.Executable {
			mode |= 0100
		}
		header.SetMode(mode)
		target, err := writer.CreateHeader(header)
		if err != nil {
			return nil, err
		}
		content := file.Content
		if name == "SKILL.md" {
			content = bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
		}
		if _, err := target.Write(content); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	if out.Len() > MaxBytes {
		return nil, ErrInvalid
	}
	return out.Bytes(), nil
}
func readFile(reader io.Reader, size int64) ([]byte, error) {
	if size > MaxExpanded {
		return nil, ErrInvalid
	}
	body, err := io.ReadAll(io.LimitReader(reader, MaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > MaxBytes {
		return nil, ErrInvalid
	}
	return body, nil
}
func Unpack(body []byte) (Tree, error) {
	if len(body) == 0 || len(body) > MaxBytes {
		return nil, ErrInvalid
	}
	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, ErrInvalid
	}
	if len(reader.File) > MaxFiles*2 {
		return nil, ErrInvalid
	}
	tree := Tree{}
	seen := map[string]bool{}
	for _, entry := range reader.File {
		name := strings.TrimSuffix(entry.Name, "/")
		if !Path(name) || seen[name] || entry.Mode()&os.ModeSymlink != 0 || !entry.Mode().IsRegular() && !entry.FileInfo().IsDir() || entry.Flags&1 != 0 {
			return nil, ErrInvalid
		}
		seen[name] = true
		if entry.FileInfo().IsDir() {
			continue
		}
		input, err := entry.Open()
		if err != nil {
			return nil, ErrInvalid
		}
		content, err := readFile(input, int64(entry.UncompressedSize64))
		_ = input.Close()
		if err != nil {
			return nil, err
		}
		tree[name] = File{Content: content, Executable: entry.Mode()&0111 != 0}
		if err := check(tree); err != nil {
			return nil, err
		}
	}
	return tree, nil
}
func Import(body []byte) (Tree, error) {
	if len(body) == 0 || len(body) > MaxBytes {
		return nil, ErrInvalid
	}
	var tree Tree
	if bytes.HasPrefix(body, []byte{0x1f, 0x8b}) {
		reader, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, ErrInvalid
		}
		defer reader.Close()
		archive := tar.NewReader(io.LimitReader(reader, MaxExpanded+(1<<20)))
		tree = Tree{}
		entries := 0
		seen := map[string]bool{}
		for {
			header, err := archive.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, ErrInvalid
			}
			entries++
			if entries > MaxFiles*2 {
				return nil, ErrInvalid
			}
			name := strings.TrimPrefix(strings.TrimSuffix(header.Name, "/"), "./")
			if name == "" || name == "." {
				if header.Typeflag == tar.TypeDir {
					continue
				}
				return nil, ErrInvalid
			}
			if !Path(name) || seen[name] {
				return nil, ErrInvalid
			}
			seen[name] = true
			if header.Typeflag == tar.TypeDir {
				continue
			}
			if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
				return nil, ErrInvalid
			}
			content, err := readFile(archive, header.Size)
			if err != nil {
				return nil, err
			}
			tree[name] = File{Content: content, Executable: header.Mode&0111 != 0}
			if err := check(tree); err != nil {
				return nil, err
			}
		}
	} else {
		var err error
		tree, err = Unpack(body)
		if err != nil {
			return nil, err
		}
	}
	for name := range tree {
		for _, part := range strings.Split(name, "/") {
			if part == "__MACOSX" || part == ".DS_Store" || strings.HasPrefix(part, "._") || part == ".git" {
				delete(tree, name)
				break
			}
		}
	}
	if _, ok := tree["SKILL.md"]; !ok {
		roots := []string{}
		for name := range tree {
			if path.Base(name) == "SKILL.md" {
				roots = append(roots, path.Dir(name)+"/")
			}
		}
		if len(roots) != 1 {
			return nil, ErrInvalid
		}
		trimmed := Tree{}
		for name, file := range tree {
			if strings.HasPrefix(name, roots[0]) {
				trimmed[strings.TrimPrefix(name, roots[0])] = file
			}
		}
		tree = trimmed
	}
	if err := check(tree); err != nil {
		return nil, err
	}
	return tree, nil
}
func Validate(tree Tree) (Header, error) {
	if err := check(tree); err != nil {
		return Header{}, err
	}
	file, ok := tree["SKILL.md"]
	if !ok || !utf8.Valid(file.Content) {
		return Header{}, ErrInvalid
	}
	body := bytes.ReplaceAll(file.Content, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(body, []byte("---\n")) {
		return Header{}, ErrInvalid
	}
	end := bytes.Index(body[4:], []byte("\n---"))
	if end < 0 || end > 16384 {
		return Header{}, ErrInvalid
	}
	var values map[string]any
	if err := yaml.Unmarshal(body[4:4+end], &values); err != nil {
		return Header{}, ErrInvalid
	}
	name, nok := values["name"].(string)
	description, dok := values["description"].(string)
	if !nok || !dok || !slug.MatchString(name) || len(name) > 64 || strings.TrimSpace(description) == "" || utf8.RuneCountInString(description) > 1024 {
		return Header{}, ErrInvalid
	}
	return Header{Name: name, Description: description}, nil
}
func Initial(name, description string) (Tree, error) {
	if !slug.MatchString(name) || len(name) > 64 {
		return nil, ErrInvalid
	}
	header, err := yaml.Marshal(map[string]string{"name": name, "description": description})
	if err != nil {
		return nil, err
	}
	return Tree{"SKILL.md": {Content: append(append([]byte("---\n"), header...), []byte("---\n\n")...)}}, nil
}
func Directory(root string) (Tree, error) {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, ErrInvalid
	}
	tree := Tree{}
	err = filepath.WalkDir(root, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if current == root {
			return nil
		}
		if entry.Name() == ".git" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return ErrInvalid
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > MaxBytes {
			return ErrInvalid
		}
		name, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		tree[filepath.ToSlash(name)] = File{Content: body, Executable: info.Mode()&0111 != 0}
		return check(tree)
	})
	if err != nil {
		return nil, err
	}
	return tree, nil
}
