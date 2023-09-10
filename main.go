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
	"github.com/opcoder0/zfuse/zfs"
)

type ZFuse struct {
	source *zip.ReadCloser
	tree   zfs.Tree[ZEntry]
}

type ZEntry struct {
	Entry fuse.Dirent
	FP    *zip.File
}

func NewZEntry(parentInode uint64, name string, entType fuse.DirentType, fp *zip.File) ZEntry {
	if entType != fuse.DT_Dir && entType != fuse.DT_File {
		panic("unsupported directory entry type")
	}
	node := ZEntry{
		Entry: fuse.Dirent{
			Inode: fs.GenerateDynamicInode(parentInode, name),
			Type:  entType,
			Name:  name,
		},
		FP: fp,
	}
	return node
}

func (zf *ZFuse) Load() {

	var parent, name string

	rootNode := NewZEntry(0, ".", fuse.DT_Dir, nil)
	zf.tree = zfs.NewTree(&rootNode)
	for _, f := range zf.source.File {
		if strings.HasSuffix(f.Name, "/") {
			parent = path.Dir(path.Dir(f.Name))
			name = path.Base(f.Name)
			node, err := zf.tree.Get(parent)
			if err != nil {
				panic(fmt.Sprintf("Error while adding %s - %v", f.Name, err))
			}
			childNode := NewZEntry(node.Entry.Inode, name, fuse.DT_Dir, f)
			err = zf.tree.Add(f.Name, &childNode, true)
			if err != nil {
				log.Println("Error adding ", f.Name, err)
			}
		} else {
			parent = path.Dir(f.Name)
			name = path.Base(f.Name)
			node, err := zf.tree.Get(parent)
			if err != nil {
				panic(fmt.Sprintf("Error while adding %s - %v", f.Name, err))
			}
			childNode := NewZEntry(node.Entry.Inode, name, fuse.DT_File, f)
			err = zf.tree.Add(f.Name, &childNode, false)
			if err != nil {
				log.Println("Error adding ", f.Name, err)
			}
		}
	}
}

func (zf *ZFuse) Root() (fs.Node, error) {
	zf.Load()
	v := zf.tree.Root.Value
	return v, nil
}

func (ze ZEntry) Attr(ctx context.Context, a *fuse.Attr) error {

	log.Println("Attr: ", ze)
	switch ze.Entry.Type {
	case fuse.DT_File:
		a.Inode = ze.Entry.Inode
		a.Mode = 0o444
		a.Size = 1024
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
	log.Println("Access: ", ze)
	return nil
}

func (ze ZEntry) Lookup(ctx context.Context, name string) (fs.Node, error) {
	log.Println("Lookup: ", ze)
	return nil, syscall.ENOENT
}

func (ze ZEntry) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	log.Println("ReadDirAll: ", ze)
	// TODO return all entries under this path
	return nil, nil
}

func (ze ZEntry) ReadAll(ctx context.Context) ([]byte, error) {
	log.Println("ReadAll: ", ze)
	return nil, nil
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

	err = fs.Serve(c, &zFuse)
	if err != nil {
		log.Fatal(err)
	}
}
