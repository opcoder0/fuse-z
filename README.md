# zmount [Work In Progress]

A Fuse filesystem for compressed files

## Usage

To mount the zip file - 

```
zmount -m /tmp/mountpoint -z /path/to/file.zip
```

To unmount - 

```
fusermount3 -u /tmp/mountpoint
```

