package main

import (
	"archive/zip"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"syscall"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

type ZFuse struct{}
type ZDir struct {
	fuse.Dirent
}
type ZFile struct {
	fuse.Dirent
}
type ZEntry struct {
	ZFile
	ZDir
}

var (
	greeting = "Hello from the fuse file\n"
	zfs      map[string]ZEntry
)

func (ZFuse) Root() (fs.Node, error) {
	return ZDir{}, nil
}

func (zd ZDir) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Inode = 1
	a.Mode = os.ModeDir | 0o555
	return nil
}

func (zd ZDir) Access(ctx context.Context, req *fuse.AccessRequest) error {
	return nil
}

func (zd ZDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	if name == "hello" {
		return ZFile{}, nil
	}
	return nil, syscall.ENOENT
}

func (zd ZDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	return dirs, nil
}

func (zf ZFile) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Inode = zf.Inode
	a.Mode = 0o444
	a.Size = uint64(len(greeting))
	return nil
}

func (ZFile) Access(ctx context.Context, req *fuse.AccessRequest) error {
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

	r, err := zip.OpenReader(compressedFile)
	if err != nil {
		log.Fatal(err)
	}
	defer r.Close()

	for _, f := range r.File {
		if strings.HasSuffix(f.Name, "/") {
			// directory
			zDir := new(ZDir)
			zDir.Inode = 0
			zDir.Name = f.Name
			zDir.Type = fuse.DT_Dir
			dirs = append(dirs, *zDir)
		} else {
			// file
		}
		zDirent := new(fuse.Dirent)
		zDirent.Inode = fs.GenerateDynamicInode(1, f.Name)
		zDirent.Name = f.Name
		zDirent.Type = fuse.DT_File
		dirs = append(dirs, *zDirent)
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
