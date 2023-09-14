package main

import (
	"archive/zip"
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
	done           = make(chan struct{})
	serviceDone    = make(chan struct{})
	compressedFile string
	mountPoint     string
	stopFlag       bool
	foreground     bool
)

const (
	PidFile = "/tmp/zmount.pid"
	LogFile = "/tmp/zmount.log"
)

func termHandler(sig os.Signal) error {
	err := fuse.Unmount(gMountPoint)
	log.Println("termHandler: Unmounting done: err =", err)
	err = os.Remove(PidFile)
	log.Println("termHandler: Remove PID file done: err =", err)
	// FIXME handle the clean up via daemon's Release function
	// which makes PidFile cleanup unnecessary.
	// done <- struct{}{}
	// log.Println("termHandler: Write to done channel complete")
	os.Exit(1)
	return err
}

func startMount(dctx *daemon.Context) {

	var child *os.Process
	var err error

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

	child, err = dctx.Reborn()
	if err != nil {
		log.Fatal("unable to start", err)
	}
	if child != nil {
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
	zFuse := zipfs.NewZipFS(r)
	c, err := fuse.Mount(mountPoint, fuse.FSName("zfuse"), fuse.Subtype("zfuse"))
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()
	log.Println("Mounted on: ", mountPoint)
	gMountPoint = mountPoint

	go func() {
		err = fs.Serve(c, zFuse)
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

func startMountFg() {

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

	r, err := zip.OpenReader(compressedFile)
	if err != nil {
		log.Fatal(err)
	}

	defer r.Close()
	zFuse := zipfs.NewZipFS(r)
	c, err := fuse.Mount(mountPoint, fuse.FSName("zfuse"), fuse.Subtype("zfuse"))
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()
	log.Println("Mounted on: ", mountPoint)
	gMountPoint = mountPoint

	err = fs.Serve(c, zFuse)
	serviceDone <- struct{}{}
	if err != nil {
		log.Fatal(err)
	}
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
	flag.BoolVar(&foreground, "f", false, "run in foreground")
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
			if foreground {
				startMountFg()
			} else {
				startMount(dctx)
			}
		} else {
			fmt.Println()
			fmt.Println("Required arguments are missing")
			flag.PrintDefaults()
			os.Exit(1)
		}
	}
}
