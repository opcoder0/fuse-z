package main

import (
	"archive/zip"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"syscall"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

type ZFuse struct {
	source   *zip.ReadCloser
	entries  map[string]*ZEntry
	iEntries map[uint64]*ZEntry
}

type ZEntry struct {
	zType fuse.DirentType
	entry fuse.Dirent
}

var (
	greeting = "Hello from the fuse file\n"
)

func (zf ZFuse) LoadZip() {
	zf.entries = make(map[string]*ZEntry)
	zf.iEntries = make(map[uint64]*ZEntry)
	for _, f := range zf.source.File {
		if strings.HasSuffix(f.Name, "/") {
			parent := path.Dir(path.Dir(f.Name)) + "/"
			name := path.Base(f.Name)
			parentZE, ok := zf.entries[parent]
			parentInode := uint64(0)
			if ok {
				parentInode = parentZE.entry.Inode
			}
			inode := fs.GenerateDynamicInode(parentInode, name)
			ze := ZEntry{
				zType: fuse.DT_Dir,
				entry: fuse.Dirent{
					Inode: inode,
					Type:  fuse.DT_Dir,
					Name:  name,
				},
			}
			zf.entries[f.Name] = &ze
			zf.iEntries[inode] = &ze
		} else {
			parent := path.Dir(f.Name) + "/"
			name := path.Base(f.Name)
			parentZE, ok := zf.entries[parent]
			if !ok {
				panic("Parent Inode information not found")
			}
			inode := fs.GenerateDynamicInode(parentZE.entry.Inode, name)
			ze := ZEntry{
				zType: fuse.DT_File,
				entry: fuse.Dirent{
					Inode: inode,
					Type:  fuse.DT_File,
					Name:  name,
				},
			}
			zf.entries[f.Name] = &ze
			zf.iEntries[inode] = &ze
		}
	}
}

func (zf ZFuse) Root() (fs.Node, error) {

	zf.LoadZip()
	rootDir := ZEntry{
		zType: fuse.DT_Dir,
		entry: fuse.Dirent{
			Inode: 0,
			Type:  fuse.DT_Dir,
			Name:  ".",
		},
	}
	return &rootDir, nil
}

func (ze ZEntry) Attr(ctx context.Context, a *fuse.Attr) error {
	switch ze.zType {
	case fuse.DT_File:
		a.Inode = ze.entry.Inode
		a.Mode = 0o444
		a.Size = uint64(len(greeting))
		return nil
	case fuse.DT_Dir:
		a.Inode = 1
		a.Mode = os.ModeDir | 0o555
		return nil
	default:
		log.Println("Invalid entry type")
		return errors.New("Invalid entry type")
	}
}

func (ze ZEntry) Access(ctx context.Context, req *fuse.AccessRequest) error {
	return nil
}

func (zd ZEntry) Lookup(ctx context.Context, name string) (fs.Node, error) {
	if name == "hello" {
		return ZEntry{
			zType: fuse.DT_File,
		}, nil
	}
	return nil, syscall.ENOENT
}

func (ze ZEntry) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	// TODO return all entries under this path
	return nil, nil
}

func (ze ZEntry) ReadAll(ctx context.Context) ([]byte, error) {
	switch ze.zType {
	case fuse.DT_File:
		return []byte(greeting), nil
	default:
		return nil, errors.New("Read error not a regular file")
	}
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

	// TODO handle close in ZFuse
	defer r.Close()
	zFuse := ZFuse{
		source: r,
	}

	c, err := fuse.Mount(mountPoint, fuse.FSName("zfuse"), fuse.Subtype("zfuse"))
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	err = fs.Serve(c, zFuse)
	if err != nil {
		log.Fatal(err)
	}
}
