# zmount

A Fuse filesystem for compressed files. The tool currently supports Zip files in read-only mode.

To mount the zip file run `zmount -m /tmp/mountpoint -z /path/to/file.zip`. The command exits immediately after a successful mount to the mount point. To unmount  `zmount -stop`. The command unmounts the zip file from the mount point and stops the `zmount`.

## Usage

```
 -m string
       directory to mount compressed file
 -stop
       stop and unmount
 -z string
       path to compressed file
```
