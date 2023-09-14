package main

import (
	"archive/zip"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"syscall"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	zipfs "github.com/opcoder0/zmount/internal/zip"
	"github.com/sevlyar/go-daemon"
)

var (
	gMountPoint    string
	compressedFile string
	mountPoint     string
	stopFlag       bool
	foreground     bool
)

const (
	PidFile         = "/tmp/zmount.pid"
	LogFile         = "/tmp/zmount.log"
	StartDaemon     = 1
	StopDaemon      = 2
	StartForeground = 3
)

func termHandler(sig os.Signal) error {
	err := fuse.Unmount(gMountPoint)
	if err != nil {
		log.Println("Error unmounting filesystem:", err)
	}
	return err
}

func createMount() (*zip.ReadCloser, *fuse.Conn, *zipfs.ZipFS, error) {

	r, err := zip.OpenReader(compressedFile)
	if err != nil {
		return nil, nil, nil, err
	}
	zFuse := zipfs.NewZipFS(r)
	c, err := fuse.Mount(mountPoint, fuse.FSName("zfuse"), fuse.Subtype("zfuse"))
	if err != nil {
		return nil, nil, nil, err
	}
	gMountPoint = mountPoint
	return r, c, zFuse, nil
}

func startMount() {

	var child *os.Process
	var err error

	dctx := &daemon.Context{
		PidFileName: PidFile,
		PidFilePerm: 0644,
		LogFileName: LogFile,
		LogFilePerm: 0640,
		WorkDir:     "./",
		Umask:       027,
	}

	child, err = dctx.Reborn()
	if err != nil {
		log.Fatal("unable to start", err)
	}
	if child != nil {
		// in parent
		fmt.Fprintf(os.Stderr, "Check %s for zmount logs\n", LogFile)
		return
	}

	// --- in child process
	log.Println("Child daemon process has started")
	defer func() {
		err = dctx.Release()
		if err != nil {
			log.Println("Error releasing daemon resources:", err)
		}
	}()
	r, c, zFuse, err := createMount()
	if err != nil {
		log.Fatal(err)
	}
	defer r.Close()
	defer c.Close()
	log.Println("Mounted: ", gMountPoint)
	go func() {
		err := daemon.ServeSignals()
		if err != nil {
			log.Printf("Signal error: %v", err)
		}
	}()
	err = fs.Serve(c, zFuse)
	if err != nil {
		log.Fatal(err)
	}
}

func startMountFg() {

	r, c, zFuse, err := createMount()
	if err != nil {
		log.Fatal(err)
	}
	defer r.Close()
	defer c.Close()
	log.Println("Mounted: ", gMountPoint)
	err = fs.Serve(c, zFuse)
	if err != nil {
		log.Fatal(err)
	}
}

func stopMount() {

	dctx := &daemon.Context{
		PidFileName: PidFile,
		PidFilePerm: 0644,
		LogFileName: LogFile,
		LogFilePerm: 0640,
		WorkDir:     "./",
		Umask:       027,
	}

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

// check validates inputs and returns the operation to run
// if validation fails it returns an error
func check() (int, error) {

	var op int

	if stopFlag {
		if compressedFile != "" || mountPoint != "" {
			return -1, errors.New("options stop and flags -z and -m are mutually exclusive")
		} else {
			op = StopDaemon
		}
	} else {
		if compressedFile != "" || mountPoint != "" {
			if foreground {
				op = StartForeground
			} else {
				op = StartDaemon
			}
		} else {
			return -1, errors.New("required arguments are missing")
		}

		if fInfo, err := os.Stat(compressedFile); err != nil {
			return -1, fmt.Errorf("compressed file: %v", err)
		} else if fInfo.IsDir() {
			return -1, fmt.Errorf("%s not a file", compressedFile)
		}

		if mInfo, err := os.Stat(mountPoint); err != nil {
			return -1, fmt.Errorf("mount point: %v", err)
		} else if !mInfo.IsDir() {
			return -1, errors.New("mount point must be a directory")
		}
	}
	return op, nil
}

func main() {

	flag.StringVar(&compressedFile, "z", "", "path to compressed file")
	flag.StringVar(&mountPoint, "m", "", "directory to mount compressed file")
	flag.BoolVar(&foreground, "f", false, "run in foreground")
	flag.BoolVar(&stopFlag, "stop", false, "stop and unmount")
	flag.Parse()
	daemon.AddCommand(daemon.BoolFlag(&stopFlag), syscall.SIGTERM, termHandler)
	op, err := check()
	if err != nil {
		fmt.Println(err)
		flag.PrintDefaults()
		os.Exit(1)
	}
	switch op {
	case StartDaemon:
		startMount()
	case StopDaemon:
		stopMount()
	case StartForeground:
		startMountFg()
	default:
		panic("unrecognized operation")
	}
}
