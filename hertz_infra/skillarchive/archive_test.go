package skillarchive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"testing"
)

func TestZipAndTarImportsNormalizeWrapperAndExecutable(t *testing.T) {
	instructions := []byte("---\r\nname: demo\r\ndescription: Example\r\n---\r\nHello")
	tree := Tree{"demo/SKILL.md": {Content: instructions}, "demo/scripts/run.sh": {Content: []byte("#!/bin/sh\nprintf ok"), Executable: true}}
	zipped, err := Pack(tree)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	tw := tar.NewWriter(gz)
	for name, file := range tree {
		mode := int64(0644)
		if file.Executable {
			mode = 0755
		}
		if err := tw.WriteHeader(&tar.Header{Name: "./" + name, Mode: mode, Size: int64(len(file.Content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		_, _ = tw.Write(file.Content)
	}
	_ = tw.Close()
	_ = gz.Close()
	for _, body := range [][]byte{zipped, out.Bytes()} {
		imported, err := Import(body)
		if err != nil {
			t.Fatal(err)
		}
		normalized, err := Pack(imported)
		if err != nil {
			t.Fatal(err)
		}
		final, err := Unpack(normalized)
		if err != nil {
			t.Fatal(err)
		}
		h, err := Validate(final)
		if err != nil || h.Name != "demo" || !final["scripts/run.sh"].Executable {
			t.Fatal(h, err)
		}
		if bytes.Contains(final["SKILL.md"].Content, []byte("\r\n")) {
			t.Fatal("CRLF was not normalized for Agent")
		}
	}
}
func TestDraftCanBeIncompleteButPublicationRequiresValidHeader(t *testing.T) {
	body, err := Pack(Tree{"notes.txt": {Content: []byte("draft")}})
	if err != nil {
		t.Fatal(err)
	}
	tree, err := Unpack(body)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Validate(tree); err == nil {
		t.Fatal("missing entry published")
	}
	tree["SKILL.md"] = File{Content: []byte("---\nname: INVALID NAME\ndescription: Test\n---\n")}
	if _, err := Validate(tree); err == nil {
		t.Fatal("invalid Runtime slug published")
	}
}
func TestUnsafePathsLinksAndConflictingFileTreesAreRejected(t *testing.T) {
	for _, name := range []string{"../secret", "/root/file", "a/../b", "a\\b", "a//b", "a/./b"} {
		if _, err := Pack(Tree{name: {Content: []byte("x")}}); err == nil {
			t.Fatal(name)
		}
	}
	if _, err := Pack(Tree{"file": {}, "file/child": {}}); err == nil {
		t.Fatal("file/directory collision")
	}
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	h := &zip.FileHeader{Name: "link"}
	h.SetMode(os.ModeSymlink | 0777)
	entry, _ := writer.CreateHeader(h)
	_, _ = entry.Write([]byte("../outside"))
	_ = writer.Close()
	if _, err := Unpack(out.Bytes()); err == nil {
		t.Fatal("symlink accepted")
	}
}
