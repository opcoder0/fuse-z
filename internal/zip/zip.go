package zip

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	zdaemon "github.com/opcoder0/zmount/internal/daemon"
	"github.com/opcoder0/zmount/internal/zfs"
)

const (
	PidFile = "/tmp/zmount.pid"
	LogFile = "/tmp/zmount.log"
)

type ZipFS struct {
	reader       *zip.ReadCloser
	writer       *zip.Writer
	mountPoint   string
	mountOptions []string
	conn         *fuse.Conn
	tree         *zfs.Tree[ZipEntry]
	sigChan      chan os.Signal
	daemon       bool
	Done         chan bool
}

type FileHandle struct {
	Inode     uint64
	FP        *zip.File
	NewFileFP *os.File
	tree      *zfs.Tree[ZipEntry] // FIXME: Duplicate from ZipEntry
}

type ZipEntry struct {
	Entry  fuse.Dirent
	Handle *FileHandle
	tree   *zfs.Tree[ZipEntry]
}

func (zipFS *ZipFS) termHandler(sig os.Signal) error {
	err := fuse.Unmount(zipFS.mountPoint)
	if err != nil {
		log.Println("Error unmounting filesystem:", err)
	}
	return err
}

func New(zipFile, mountPoint string, mountOptions []string) (*ZipFS, error) {

	var w *zip.Writer

	mountOptionHas := func(opt string) bool {
		for _, v := range mountOptions {
			if v == opt {
				return true
			}
		}
		return false
	}
	r, err := zip.OpenReader(zipFile)
	if err != nil {
		return nil, err
	}
	if mountOptionHas("w") {
		f, err := os.OpenFile(zipFile, os.O_RDWR, 0755)
		if err != nil {
			return nil, err
		}
		w = zip.NewWriter(f)
	}
	zipFS := ZipFS{
		reader:       r,
		writer:       w,
		sigChan:      make(chan os.Signal, 1),
		mountPoint:   mountPoint,
		mountOptions: mountOptions,
	}
	return &zipFS, nil
}

func (zipFS *ZipFS) Mount() {

	var err error

	mountOptionHas := func(opt string) bool {
		for _, v := range zipFS.mountOptions {
			if v == opt {
				return true
			}
		}
		return false
	}
	if mountOptionHas("f") {
		zipFS.Done = make(chan bool)
		signal.Notify(zipFS.sigChan, os.Interrupt)
		go func() {
			err = fs.Serve(zipFS.conn, zipFS)
			if err != nil {
				log.Fatal(err)
			}
		}()
		go func() {
			<-zipFS.sigChan
			zipFS.fuseUnmount()
			zipFS.Done <- true
		}()
	} else {
		zipFS.daemon = true
		err = zdaemon.Start(PidFile, LogFile, false, zipFS.termHandler, zipFS.fuseMount, zipFS.serve)
		if err != nil {
			log.Fatal("Error starting daemon", err)
		}
	}
}

func (zipFS *ZipFS) fuseMount() {
	conn, err := fuse.Mount(zipFS.mountPoint, fuse.FSName("zipfuse"), fuse.Subtype("zipfuse"))
	if err != nil {
		log.Fatalf("Error mounting on %s - %v", zipFS.mountPoint, err)
	}
	zipFS.conn = conn
}

func (zipFS *ZipFS) serve() {
	log.Println("Serving requests:")
	err := fs.Serve(zipFS.conn, zipFS)
	if err != nil {
		log.Fatal(err)
	}
}

func (zipFS *ZipFS) fuseUnmount() {
	err := fuse.Unmount(zipFS.mountPoint)
	if err != nil {
		log.Println("Error unmounting filesystem:", err)
	}
}

func Unmount() {
	zdaemon.Stop()
}

func NewZipEntry(parentInode uint64, name string, entType fuse.DirentType, tree *zfs.Tree[ZipEntry]) *ZipEntry {
	if entType != fuse.DT_Dir && entType != fuse.DT_File {
		panic("unsupported directory entry type")
	}
	node := ZipEntry{
		Entry: fuse.Dirent{
			Inode: fs.GenerateDynamicInode(parentInode, name),
			Type:  entType,
			Name:  name,
		},
		tree: tree,
		Handle: &FileHandle{
			tree: tree,
		},
	}
	node.Handle.Inode = node.Entry.Inode
	return &node
}

func (ze *ZipEntry) AddNewFile(fp *os.File) *ZipEntry {
	ze.Handle.NewFileFP = fp
	return ze
}

func (ze *ZipEntry) SetFP(fp *zip.File) *ZipEntry {
	ze.Handle.FP = fp
	return ze
}

func (zf *ZipFS) Load() {

	var parent, name string

	tree := zfs.NewTree[ZipEntry]()
	zf.tree = &tree
	rootZEntry := NewZipEntry(0, ".", fuse.DT_Dir, zf.tree)
	rootNode := zfs.NewNode[ZipEntry](rootZEntry, true)
	err := zf.tree.Add(".", rootNode, rootNode, rootNode.Value.Entry.Inode, rootNode.Value.Entry.Inode, true)
	if err != nil {
		panic("error adding root node to the tree")
	}
	for _, f := range zf.reader.File {
		if strings.HasSuffix(f.Name, "/") {
			parent = path.Dir(path.Dir(f.Name))
			name = path.Base(f.Name)
			parentNode, err := zf.tree.Get(parent)
			if err != nil {
				panic(fmt.Sprintf("Error while adding %s - %v", f.Name, err))
			}
			childZEntry := NewZipEntry(parentNode.Value.Entry.Inode, name, fuse.DT_Dir, zf.tree).SetFP(f)
			childNode := zfs.NewNode[ZipEntry](childZEntry, true)
			err = zf.tree.Add(f.Name, parentNode, childNode, parentNode.Value.Entry.Inode, childNode.Value.Entry.Inode, true)
			if err != nil {
				log.Println("Error adding ", f.Name, err)
			}
		} else {
			parent = path.Dir(f.Name)
			name = path.Base(f.Name)
			parentNode, err := zf.tree.Get(parent)
			if err != nil {
				// add all missing parent nodes
				pDir := path.Dir(parent)
				pNode, err := zf.tree.Get(pDir)
				if err != nil {
					panic(fmt.Sprintf("unable to determine ancestor %v", err))
				}
				parts := strings.Split(parent, "/")
				for idx, part := range parts {
					pName := path.Join(parts[0 : idx+1]...)
					pName = path.Join(pDir, pName)
					childZEntry := NewZipEntry(pNode.Value.Entry.Inode, part, fuse.DT_Dir, zf.tree)
					childNode := zfs.NewNode[ZipEntry](childZEntry, true)
					// intermediate nodes are directories
					err = zf.tree.Add(pName, pNode, childNode, pNode.Value.Entry.Inode, childNode.Value.Entry.Inode, true)
					if err != nil {
						log.Println("Error adding ", f.Name, err)
					}
					pNode = childNode
				}
				parentNode = pNode
			}
			childZEntry := NewZipEntry(parentNode.Value.Entry.Inode, name, fuse.DT_File, zf.tree).SetFP(f)
			childNode := zfs.NewNode[ZipEntry](childZEntry, false)
			err = zf.tree.Add(f.Name, parentNode, childNode, parentNode.Value.Entry.Inode, childNode.Value.Entry.Inode, false)
			if err != nil {
				log.Println("Error adding ", f.Name, err)
			}
		}
	}
}

func (zf *ZipFS) Root() (fs.Node, error) {
	zf.Load()
	v := zf.tree.Root.Value
	return v, nil
}

func (ze ZipEntry) Attr(ctx context.Context, a *fuse.Attr) error {

	if ze.Entry.Name == "." {
		a.Inode = ze.Entry.Inode
		a.Mode = os.ModeDir | 0o555
		return nil
	}
	switch {
	case ze.Handle.FP != nil:
		stat := ze.Handle.FP.FileInfo()
		a.Inode = ze.Entry.Inode
		a.Size = uint64(stat.Size())
		a.Atime = ze.Handle.FP.Modified
		a.Mtime = ze.Handle.FP.Modified
		a.Ctime = ze.Handle.FP.Modified
		a.Mode = stat.Mode()
		a.Nlink = 1
		a.Uid = uint32(os.Getuid())
		a.Gid = uint32(os.Getgid())
		return nil
	case ze.Handle.NewFileFP != nil:
		a.Inode = ze.Entry.Inode
		a.Mode = 0o644
		a.Uid = uint32(os.Getuid())
		a.Gid = uint32(os.Getgid())
		return nil
	default:
		// Intermediate entries can have nil FP and they are directories
		a.Inode = ze.Entry.Inode
		a.Mode = os.ModeDir | 0o555
		a.Uid = uint32(os.Getuid())
		a.Gid = uint32(os.Getgid())
		return nil
	}
}

func (ze ZipEntry) Access(ctx context.Context, req *fuse.AccessRequest) error {
	return nil
}

func (ze ZipEntry) getAbsolutePath() (string, error) {
	var absPath string
	node, err := ze.tree.GetByInode(ze.Entry.Inode)
	if err != nil {
		return absPath, err
	}
	for node.Children["."].Value.Entry.Inode != ze.tree.Root.Value.Entry.Inode {
		absPath = path.Join(node.Children["."].Value.Entry.Name, absPath)
		node = node.Children[".."]
	}
	return absPath, nil
}

func (ze *ZipEntry) Create(ctx context.Context, req *fuse.CreateRequest, resp *fuse.CreateResponse) (fs.Node, fs.Handle, error) {

	log.Println("Create ze:", ze)
	log.Println("Create req:", req)

	currentNode, err := ze.tree.GetByInode(ze.Entry.Inode)
	if err != nil {
		return nil, nil, err
	}
	newFileFP, err := os.CreateTemp("", "zfuse*")
	if err != nil {
		return nil, nil, err
	}
	childZEntry := NewZipEntry(currentNode.Value.Entry.Inode, req.Name, fuse.DT_File, ze.tree).AddNewFile(newFileFP)
	childNode := zfs.NewNode[ZipEntry](childZEntry, true)
	if err = ze.tree.Add(req.Name, currentNode, childNode, ze.Entry.Inode, childZEntry.Entry.Inode, false); err != nil {
		return nil, nil, err
	}
	return childZEntry, childZEntry.Handle, nil
}

func (ze ZipEntry) Open(ctx context.Context, req *fuse.OpenRequest, resp *fuse.OpenResponse) (fs.Handle, error) {
	log.Println("Open req:", req)
	log.Println("Open ze:", ze)
	if ze.Handle.FP != nil {
		return ze.Handle, nil
	}
	if ze.Handle.NewFileFP != nil {
		return ze.Handle.NewFileFP, nil
	}
	return &ze, nil
}

func (h *FileHandle) Write(ctx context.Context, req *fuse.WriteRequest, resp *fuse.WriteResponse) error {
	switch {
	case h.FP != nil:
		return errors.New("no implementation")
	case h.NewFileFP != nil:
		n, err := h.NewFileFP.WriteAt(req.Data, req.Offset)
		log.Printf("Write: wrote %d bytes. err = %v", n, err)
	}
	return errors.New("no implementation")
}

func (h *FileHandle) Flush(ctx context.Context, req *fuse.FlushRequest) error {
	log.Println("FlushRequest:", req)
	return nil
}

// func (h *FileHandle) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
// 	log.Println("Read: ", req)
// 	dirents := make([]string, 0)
// 	entries, err := h.tree.ListByInode(h.Inode)
// 	if err != nil {
// 		log.Println("Read: ", err)
// 		return err
// 	}
// 	for _, entry := range entries {
// 		dirents = append(dirents, entry.Entry.Name)
// 	}
// 	resp.Data = []byte(strings.Join(dirents[:], "\n"))
// 	return nil
// }

func (ze ZipEntry) Mkdir(ctx context.Context, req *fuse.MkdirRequest) (fs.Node, error) {
	log.Println("Mkdir ZipEntry:", ze)
	log.Println("Mkdir Request:", req)
	return nil, nil
}

func (ze ZipEntry) Lookup(ctx context.Context, name string) (fs.Node, error) {
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

func (ze ZipEntry) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
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

func (handle *FileHandle) ReadAll(ctx context.Context) ([]byte, error) {
	log.Println("ReadAll: ", handle)
	if handle.FP != nil {
		reader, err := handle.FP.Open()
		if err != nil {
			return nil, err
		}
		return io.ReadAll(reader)
	}
	return nil, errors.New("TODO: implement new file logic here")
}
