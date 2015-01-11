# Crash BoltDB
See https://github.com/boltdb/bolt/issues/277 for details.

I've managed to record all the transactions from my Camlistore
reindexing session, and this program downloads the starting
(last known good) BoltDB, the compressed transaction list,
and then applies those transactions in order.

This result in a panic, *but only on i386 architecture*!!!
