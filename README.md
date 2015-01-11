# Crash BoltDB
See https://github.com/boltdb/bolt/issues/277 for details.

I've managed to record all the transactions from my Camlistore
reindexing session, and this program downloads the starting
(last known good) BoltDB, the compressed transaction list,
and then applies those transactions in order.

This result in a panic, *but only on i386 architecture*!!!

## panic
```
panic: runtime error: index out of range

goroutine 1 [running]:
github.com/boltdb/bolt.(*Tx).page(0x186cae80, 0x1000f, 0x0, 0x1862e160)
        /home/gthomas/src/github.com/boltdb/bolt/tx.go:474 +0x9f
github.com/boltdb/bolt.(*Bucket).pageNode(0x186cae8c, 0x1000f, 0x0, 0x186f7d00, 0x200004c)
        /home/gthomas/src/github.com/boltdb/bolt/bucket.go:677 +0x195
github.com/boltdb/bolt.(*Cursor).search(0x186cd5d0, 0x83b8120, 0x1, 0x1, 0x1000f, 0x0)
        /home/gthomas/src/github.com/boltdb/bolt/cursor.go:230 +0x40
github.com/boltdb/bolt.(*Cursor).seek(0x186cd5d0, 0x83b8120, 0x1, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, ...)
        /home/gthomas/src/github.com/boltdb/bolt/cursor.go:144 +0xec
github.com/boltdb/bolt.(*Bucket).Bucket(0x186cae8c, 0x83b8120, 0x1, 0x1, 0x0)
        /home/gthomas/src/github.com/boltdb/bolt/bucket.go:111 +0x171
github.com/boltdb/bolt.(*Tx).Bucket(0x186cae80, 0x83b8120, 0x1, 0x1, 0x0)
        /home/gthomas/src/github.com/boltdb/bolt/tx.go:91 +0x4a
main.execute(0x186380e0, 0x18628040, 0x0, 0x0)
        /home/gthomas/src/github.com/tgulacsi/boltcrash/main.go:104 +0x129f
main.main()
        /home/gthomas/src/github.com/tgulacsi/boltcrash/main.go:50 +0x204

goroutine 10 [chan send]:
main.func·004()
        /home/gthomas/src/github.com/tgulacsi/boltcrash/main.go:209 +0x246
created by main.downloadAndOpen
        /home/gthomas/src/github.com/tgulacsi/boltcrash/main.go:211 +0x5e5

goroutine 17 [syscall, 4 minutes, locked to thread]:
runtime.goexit()
        /usr/local/go/src/runtime/asm_386.s:2287 +0x1
exit status 2
```
