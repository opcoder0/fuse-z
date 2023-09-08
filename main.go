package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"syscall"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

type ZFuse struct{}
type ZDir struct{}
type ZFile struct{}

var (
	greeting = "Hello from the fuse file"
)

var dirs = []fuse.Dirent{
	{Inode: 2, Name: "hello", Type: fuse.DT_File},
}

func (ZFuse) Root() (fs.Node, error) {
	return ZDir{}, nil
}

func (ZDir) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Inode = 1
	a.Mode = os.ModeDir | 0o555
	return nil
}

func (ZDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	if name == "hello" {
		return ZFile{}, nil
	}
	return nil, syscall.ENOENT
}

func (ZDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	return dirs, nil
}

func (ZFile) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Inode = 2
	a.Mode = 0o444
	a.Size = uint64(len(greeting))
	return nil
}

func (ZFile) ReadAll(ctx context.Context) ([]byte, error) {
	return []byte(greeting), nil
}

func main() {

	var compressedFile string
	var mountPoint string

	flag.StringVar(&compressedFile, "z", "", "path to compressed file")
	flag.StringVar(&mountPoint, "d", "", "directory to mount compressed file")
	flag.Parse()
	if compressedFile == "" || mountPoint == "" {
		fmt.Println()
		fmt.Println("Required arguments are missing")
		flag.PrintDefaults()
		os.Exit(1)
	}

	c, err := fuse.Mount(mountPoint, fuse.FSName("zfuse"), fuse.Subtype("zfuse"))
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	err = fs.Serve(c, ZFuse{})
}
