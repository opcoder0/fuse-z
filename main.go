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

func (zf ZFuse) Root() (fs.Node, error) {
	log.Println("Root: zf = ", zf)
	return ZDir{}, nil
}

func (zd ZDir) Attr(ctx context.Context, a *fuse.Attr) error {
	log.Println("Attr: zd = ", zd)
	a.Inode = 1
	a.Mode = os.ModeDir | 0o555
	return nil
}

func (zd ZDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	log.Println("Lookup: zd = ", zd)
	if name == "hello" {
		return ZFile{}, nil
	}
	return nil, syscall.ENOENT
}

func (zd ZDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	log.Println("ReadDirAll: zd = ", zd)
	return dirs, nil
}

func (zf ZFile) Attr(ctx context.Context, a *fuse.Attr) error {

	log.Println("Attr: zf = ", zf)
	a.Inode = 2
	a.Mode = 0o444
	a.Size = uint64(len(greeting))
	return nil
}

func (zf ZFile) ReadAll(ctx context.Context) ([]byte, error) {
	log.Println("ReadAll: zf = ", zf)
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
	if err != nil {
		log.Fatal(err)
	}
}
