package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	zipfs "github.com/opcoder0/zmount/internal/zip"
)

// check validates inputs and returns the operation to run
// if validation fails it returns an error
func check(compressedFile, mountPoint string, mountOption []string, stopFlag bool) error {

	if stopFlag {
		if mountPoint == "" {
			return errors.New("stop must specify mount point")
		}
	} else {
		if compressedFile == "" || mountPoint == "" {
			return errors.New("required arguments are missing")
		}

		if fInfo, err := os.Stat(compressedFile); err != nil {
			return fmt.Errorf("compressed file: %v", err)
		} else if fInfo.IsDir() {
			return fmt.Errorf("%s not a file", compressedFile)
		}

		if mInfo, err := os.Stat(mountPoint); err != nil {
			return fmt.Errorf("mount point: %v", err)
		} else if !mInfo.IsDir() {
			return errors.New("mount point must be a directory")
		}
		mOptHas := func(val string) bool {
			for _, v := range mountOption {
				if v == val {
					return true
				}
			}
			return false
		}
		if !mOptHas("r") && !mOptHas("w") && !mOptHas("f") {
			return errors.New("invalid mount option")
		}
	}
	return nil
}

func main() {

	var (
		mOption        string
		compressedFile string
		mountPoint     string
		stopFlag       bool
		mountOptions   []string
	)

	flag.StringVar(&compressedFile, "z", "", "path to compressed file")
	flag.StringVar(&mountPoint, "m", "", "directory to mount compressed file")
	flag.StringVar(&mOption, "o", "r", "mount option values are comma separated combination of [r,w,f]")
	flag.BoolVar(&stopFlag, "stop", false, "stop and unmount")
	flag.Parse()
	mountOptions = strings.Split(mOption, ",")
	err := check(compressedFile, mountPoint, mountOptions, stopFlag)
	if err != nil {
		fmt.Println(err)
		flag.PrintDefaults()
		os.Exit(1)
	}
	if !stopFlag {
		zipFS, err := zipfs.New(compressedFile, mountPoint, mountOptions)
		if err != nil {
			log.Fatal("Error creating ZipFS", err)
		}
		zipFS.Mount()
		if zipFS.Done != nil {
			log.Println("Press ctrl-c to unmount")
			<-zipFS.Done
		}
	} else {
		zipfs.Unmount(mountPoint)
	}
}
