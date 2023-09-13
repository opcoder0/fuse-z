package main

import (
	"archive/zip"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"strings"
	"syscall"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/opcoder0/zmount/zfs"
	"github.com/sevlyar/go-daemon"
)

var (
	gMountPoint    string
	done           = make(chan struct{})
	serviceDone    = make(chan struct{})
	compressedFile string
	mountPoint     string
	stopFlag       bool
)

const (
	PidFile = "/tmp/zmount.pid"
	LogFile = "/tmp/zmount.log"
)

type ZFuse struct {
	source *zip.ReadCloser
	tree   *zfs.Tree[ZEntry]
}

type ZEntry struct {
	Entry fuse.Dirent
	FP    *zip.File
	tree  *zfs.Tree[ZEntry]
}

func NewZEntry(parentInode uint64, name string, entType fuse.DirentType, fp *zip.File, tree *zfs.Tree[ZEntry]) ZEntry {
	if entType != fuse.DT_Dir && entType != fuse.DT_File {
		panic("unsupported directory entry type")
	}
	node := ZEntry{
		Entry: fuse.Dirent{
			Inode: fs.GenerateDynamicInode(parentInode, name),
			Type:  entType,
			Name:  name,
		},
		FP:   fp,
		tree: tree,
	}
	return node
}

func (zf *ZFuse) Load() {

	var parent, name string

	tree := zfs.NewTree[ZEntry]()
	zf.tree = &tree
	rootZEntry := NewZEntry(0, ".", fuse.DT_Dir, nil, zf.tree)
	rootNode := zfs.NewNode[ZEntry](&rootZEntry, true)
	err := zf.tree.Add(".", rootNode, rootNode, rootNode.Value.Entry.Inode, rootNode.Value.Entry.Inode, true)
	if err != nil {
		panic("error adding root node to the tree")
	}
	for _, f := range zf.source.File {
		if strings.HasSuffix(f.Name, "/") {
			parent = path.Dir(path.Dir(f.Name))
			name = path.Base(f.Name)
			parentNode, err := zf.tree.Get(parent)
			if err != nil {
				panic(fmt.Sprintf("Error while adding %s - %v", f.Name, err))
			}
			childZEntry := NewZEntry(parentNode.Value.Entry.Inode, name, fuse.DT_Dir, f, zf.tree)
			childNode := zfs.NewNode[ZEntry](&childZEntry, true)
			err = zf.tree.Add(f.Name, parentNode, childNode, parentNode.Value.Entry.Inode, childNode.Value.Entry.Inode, true)
			if err != nil {
				log.Println("Error adding ", f.Name, err)
			}
		} else {
			parent = path.Dir(f.Name)
			name = path.Base(f.Name)
			parentNode, err := zf.tree.Get(parent)
			if err != nil {
				panic(fmt.Sprintf("Error while adding %s - %v", f.Name, err))
			}
			childZEntry := NewZEntry(parentNode.Value.Entry.Inode, name, fuse.DT_File, f, zf.tree)
			childNode := zfs.NewNode[ZEntry](&childZEntry, false)
			err = zf.tree.Add(f.Name, parentNode, childNode, parentNode.Value.Entry.Inode, childNode.Value.Entry.Inode, false)
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

	if ze.Entry.Name == "." {
		a.Inode = ze.Entry.Inode
		a.Mode = os.ModeDir | 0o555
		return nil
	}
	if ze.FP != nil {
		stat := ze.FP.FileInfo()
		a.Inode = ze.Entry.Inode
		a.Size = uint64(stat.Size())
		a.Atime = ze.FP.Modified
		a.Mtime = ze.FP.Modified
		a.Ctime = ze.FP.Modified
		a.Mode = stat.Mode()
		a.Nlink = 1
		a.Uid = uint32(os.Getuid())
		a.Gid = uint32(os.Getgid())
	}
	return nil
}

func (ze ZEntry) Access(ctx context.Context, req *fuse.AccessRequest) error {
	log.Println("Access: ", ze)
	return nil
}

func (ze ZEntry) Lookup(ctx context.Context, name string) (fs.Node, error) {
	// In the directory from ZEntry look for file with "name" if it exists
	// return node else ENOENT.
	entries, err := ze.tree.ListByInode(ze.Entry.Inode)
	if err != nil {
		log.Println("No entries found for ", ze, err)
		return nil, syscall.ENOENT
	}
	for _, entry := range entries {
		if entry.Entry.Name == name {
			return entry, nil
		}
	}
	return nil, syscall.ENOENT
}

func (ze ZEntry) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	log.Println("ReadDirAll: ", ze)
	dirents := make([]fuse.Dirent, 0)
	entries, err := ze.tree.ListByInode(ze.Entry.Inode)
	if err != nil {
		log.Println("ReadDirAll: ", err)
		return nil, err
	}
	for _, entry := range entries {
		dirents = append(dirents, entry.Entry)
	}
	log.Println("ReadDirAll returning ", len(dirents), " entries")
	return dirents, nil
}

func (ze ZEntry) ReadAll(ctx context.Context) ([]byte, error) {
	log.Println("ReadAll: ", ze)
	reader, err := ze.FP.Open()
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func termHandler(sig os.Signal) error {
	err := fuse.Unmount(gMountPoint)
	log.Println("termHandler: Unmounting done: err =", err)
	err = os.Remove(PidFile)
	log.Println("termHandler: Remove PID file done: err =", err)
	// FIXME handle the clean up;
	// done <- struct{}{}
	// log.Println("termHandler: Write to done channel complete")
	os.Exit(1)
	return err
}

func startMount(dctx *daemon.Context) {
	if fInfo, err := os.Stat(compressedFile); err != nil {
		fmt.Println()
		fmt.Println("compressed file: ", compressedFile, err)
		flag.PrintDefaults()
		os.Exit(1)
	} else if fInfo.IsDir() {
		fmt.Println("compressed file: ", compressedFile, "must be a file")
		os.Exit(1)
	}

	if mInfo, err := os.Stat(mountPoint); err != nil {
		fmt.Println()
		fmt.Println("mount point: ", mountPoint, err)
		flag.PrintDefaults()
		os.Exit(1)
	} else if !mInfo.IsDir() {
		fmt.Println("mount point must be a directory")
		os.Exit(1)
	}

	d, err := dctx.Reborn()
	if err != nil {
		log.Fatal("unable to start", err)
	}
	if d != nil {
		// returning from parent
		return
	}

	log.Println("Child Daemon Process: Started")
	defer func() {
		err = dctx.Release()
		log.Println("Released daemon resources: err = ", err)
	}()

	r, err := zip.OpenReader(compressedFile)
	if err != nil {
		log.Fatal(err)
	}

	defer r.Close()
	zFuse := ZFuse{
		source: r,
	}

	c, err := fuse.Mount(mountPoint, fuse.FSName("zfuse"), fuse.Subtype("zfuse"))
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()
	log.Println("Mounted on: ", mountPoint)
	gMountPoint = mountPoint

	go func() {
		err = fs.Serve(c, &zFuse)
		serviceDone <- struct{}{}
		if err != nil {
			log.Fatal(err)
		}
	}()
	err = daemon.ServeSignals()
	if err != nil {
		log.Printf("daemon error: %v", err)
	}
	log.Println("Waiting on done")
	select {
	case <-done:
		log.Println("done: received")
	case <-serviceDone:
		log.Println("serviceDone: received")
	}
	log.Println("Done. Exiting")
}

func stopMount(dctx *daemon.Context) {
	if len(daemon.ActiveFlags()) > 0 {
		d, err := dctx.Search()
		if err != nil {
			log.Fatalf("Unable send signal to the daemon: %s", err.Error())
		}
		err = daemon.SendCommands(d)
		if err != nil {
			log.Printf("Error sending stop signal %v", err)
		}
		return
	}
}

func main() {

	flag.StringVar(&compressedFile, "z", "", "path to compressed file")
	flag.StringVar(&mountPoint, "m", "", "directory to mount compressed file")
	flag.BoolVar(&stopFlag, "stop", false, "stop and unmount")
	flag.Parse()
	daemon.AddCommand(daemon.BoolFlag(&stopFlag), syscall.SIGTERM, termHandler)
	dctx := &daemon.Context{
		PidFileName: PidFile,
		PidFilePerm: 0644,
		LogFileName: LogFile,
		LogFilePerm: 0640,
		WorkDir:     "./",
		Umask:       027,
	}

	// can't specify both stop and mount together
	if stopFlag {
		if compressedFile != "" || mountPoint != "" {
			fmt.Println()
			fmt.Println("Options stop and flags -z and -m are mutually exclusive")
			flag.PrintDefaults()
			os.Exit(1)
		} else {
			stopMount(dctx)
		}
	} else {
		if compressedFile != "" || mountPoint != "" {
			startMount(dctx)
		} else {
			fmt.Println()
			fmt.Println("Required arguments are missing")
			flag.PrintDefaults()
			os.Exit(1)
		}
	}
}
