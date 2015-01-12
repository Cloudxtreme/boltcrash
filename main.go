// Copyright 2015 Tamás Gulácsi
//
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package main

import (
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/boltdb/bolt"
	"gopkg.in/inconshreveable/log15.v2"
)

var Log = log15.New()

const URL = "http://git.gthomas.eu/gthomas/boltcrash/"

func main() {
	Log.SetHandler(log15.StderrHandler)
	flagWorkdir := flag.String("workdir", os.Getenv("TMPDIR"), "work dir to save downloaded files to")
	flagReplay := flag.Bool("replay", false, "download and replay the Camlistore session")
	flag.Parse()

	if !*flagReplay {
		if err := direct(*flagWorkdir); err != nil {
			os.Exit(2)
		}
		return
	}

	db, ops, err := downloadAndOpen(*flagWorkdir)
	if err != nil {
		Log.Crit("Download", "error", err)
		os.Exit(1)
	}

	defer db.Close()
	if err = execute(db, ops); err != nil {
		Log.Error("execute", "error", err)
		os.Exit(1)
	}
}

var bucketName = []byte("/")

func direct(workdir string) error {
	fn := filepath.Join(workdir, "direct.boltdb")
	_ = os.Remove(fn) // fresh start
	log.Printf("opening %q", fn)
	db, err := bolt.Open(fn, 0644, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			Log.Error("close DB", "error", err)
			if err != nil {
				err = closeErr
			}
		}
	}()
	if err = db.Update(func(tx *bolt.Tx) error {
		_, err = tx.CreateBucket(bucketName)
		return err
	}); err != nil {
		return err
	}

	var key [8]byte
	var value [1 << 20]byte // 1Mb
	E := func(off, count int) error {
		for i := off; i < off+count; i++ {
			log.Printf("i=%d", i)
			if err = db.Update(func(tx *bolt.Tx) error {
				binary.BigEndian.PutUint64(key[:], uint64(i))
				return tx.Bucket(bucketName).Put(key[:], value[:])
			}); err != nil {
				return err
			}
		}
		return err
	}

	for i := 0; i < 257; i += 16 {
		if err = E(i, 16); err != nil {
			return err
		}
	}

	return err
}

func execute(db *bolt.DB, ops <-chan operation) error {
	var (
		act string
		err error
	)
	batches := make(map[string]*bolt.Tx, 4)
	iters := make(map[string]*bolt.Cursor, 4)
	for op := range ops {
		//Log.Debug("execute", "op", op)
		if act == "" {
			act = op.ID
		} else if !strings.HasPrefix(op.ID, act) {
			return fmt.Errorf("database mismatch: wanted %q, got %q", act, op.ID)
		}
		done := true
		switch op.Op {
		case "dbOpen":
			//pass
		case "dbClose":
			if len(batches) > 0 {
				err = fmt.Errorf("dbClose with %d opened batches!", len(batches))
			}
			if len(iters) > 0 {
				err = fmt.Errorf("dbClose with %d opened iters!", len(iters))
			}

		case "delete":
			err = db.Update(func(tx *bolt.Tx) error {
				return tx.Bucket(bucketName).Delete([]byte(op.Key))
			})
		case "set":
			err = db.Update(func(tx *bolt.Tx) error {
				return tx.Bucket(bucketName).Put([]byte(op.Key), []byte(op.Value))
			})
		case "get":
			err = db.View(func(tx *bolt.Tx) error {
				_ = tx.Bucket(bucketName).Get([]byte(op.Key))
				return nil
			})

		case "iterBegin":
			tx, e := db.Begin(false)
			if e != nil {
				err = e
				break
			}
			iters[op.ID[12:]] = tx.Bucket(bucketName).Cursor()
		case "iterNext":
			cu, ok := iters[op.ID[12:]]
			if !ok {
				err = fmt.Errorf("cannot find iter %q", op.ID[12:])
				break
			}
			_, _ = cu.Next()
		case "iterClose":
			_, ok := iters[op.ID[12:]]
			if !ok {
				err = fmt.Errorf("cannot find iter %q", op.ID[12:])
				break
			}
			delete(iters, op.ID[12:])

		case "batchBegin":
			tx, e := db.Begin(true)
			if e != nil {
				err = e
				break
			}
			batches[op.ID[12:]] = tx
		case "batchCommit":
			tx, ok := batches[op.ID[12:]]
			if !ok {
				err = fmt.Errorf("cannot find batch %q", op.ID[12:])
				break
			}
			err = tx.Commit()
			delete(batches, op.ID[12:])
		case "batchDelete":
			tx, ok := batches[op.ID[12:]]
			if !ok {
				err = fmt.Errorf("cannot find batch %q", op.ID[12:])
				break
			}
			err = tx.Bucket(bucketName).Delete([]byte(op.Key))
		case "batchSet":
			tx, ok := batches[op.ID[12:]]
			if !ok {
				err = fmt.Errorf("cannot find batch %q", op.ID[12:])
				break
			}
			err = tx.Bucket(bucketName).Put([]byte(op.Key), []byte(op.Value))
		case "batchGet":
			tx, ok := batches[op.ID[12:]]
			if !ok {
				err = fmt.Errorf("cannot find batch %q", op.ID[12:])
				break
			}
			_ = tx.Bucket(bucketName).Get([]byte(op.Key))

		default:
			Log.Warn("Skip", "op", op)
			done = false
		}
		if done {
			Log.Debug("execute", "op", op)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func downloadAndOpen(workdir string) (*bolt.DB, <-chan operation, error) {
	if err := download(workdir); err != nil {
		return nil, nil, err
	}
	fn := filepath.Join(workdir, "crash.boltdb")
	if err := copyFile(fn, filepath.Join(workdir, "lng.boltdb")); err != nil {
		return nil, nil, err
	}

	db, err := bolt.Open(fn, 0640, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, nil, err
	}

	fh, err := os.Open(filepath.Join(workdir, "wal.json.gz"))
	if err != nil {
		return nil, nil, err
	}
	gzr, err := gzip.NewReader(fh)
	if err != nil {
		return nil, nil, err
	}
	dec := json.NewDecoder(gzr)
	opCh := make(chan operation, 16)
	go func() {
		defer gzr.Close()
		defer fh.Close()
		defer close(opCh)

		for {
			var op operation
			if err := dec.Decode(&op); err != nil {
				if err == io.EOF {
					break
				}
				Log.Error("Decode", "error", err)
				return
			}
			opCh <- op
		}
	}()

	return db, opCh, err
}

func copyFile(dFn, sFn string) error {
	src, err := os.Open(sFn)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.Create(dFn)
	if err != nil {
		return err
	}
	if _, err = io.Copy(dst, src); err != nil {
		dst.Close()
		return err
	}
	return dst.Close()
}

type operation struct {
	ID    string `json:"id"`
	Op    string `json:"op"`
	Key   string `json:"key,omitempty"`
	Value string `json:"value,omitempty"`
}

func download(workdir string) error {
	for _, name := range []string{"lng.boltdb", "wal.json.gz"} {
		fn := filepath.Join(workdir, name)
		if _, err := os.Stat(fn); err == nil {
			Log.Info("File " + fn + " already exist.")
			continue
		}
		url := URL + name
		Log.Info("Downloading " + url)
		resp, err := http.Get(url)
		if err != nil {
			Log.Error("Get", "url", url, "error", err)
			return err
		}
		if resp.StatusCode >= http.StatusBadRequest {
			Log.Error("Error", "status", resp.StatusCode)
			return fmt.Errorf("Bad response for %q: %v", url, resp.Status)
		}
		fh, err := os.Create(fn)
		if err != nil {
			return err
		}
		_, err = io.Copy(fh, resp.Body)
		_ = resp.Body.Close()
		if closeErr := fh.Close(); closeErr != nil {
			Log.Error("Close", "file", fh, "error", err)
			if err == nil {
				return err
			}
		}
		if err != nil {
			Log.Error("Copy", "src", resp, "dst", fh, "error", err)
			return err
		}
	}
	return nil
}
