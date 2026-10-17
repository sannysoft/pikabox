package dialstats

import (
	"bytes"
	"io"
	"os"
	"time"
)

// Follow tails the log file at path forever, handing each complete line to
// handle. It starts at the current end (lines already counted by an earlier
// daemon are not replayed), starts over when the file shrinks or is replaced,
// and empties it once it passes maxSize. sing-box opens its log for append, so
// it keeps writing at the new end.
func Follow(path string, maxSize int64, handle func(string)) {
	var (
		file    *os.File
		info    os.FileInfo
		offset  int64
		pending []byte
		buf     = make([]byte, 64*1024)
	)
	for {
		if file == nil {
			f, err := os.Open(path)
			if err != nil {
				time.Sleep(2 * time.Second)
				continue
			}
			file = f
			info, _ = f.Stat()
			if info != nil {
				offset = info.Size()
			}
			pending = pending[:0]
		}

		current, err := os.Stat(path)
		if err != nil || (info != nil && !os.SameFile(info, current)) {
			file.Close()
			file = nil
			continue
		}
		if current.Size() < offset {
			offset, pending = 0, pending[:0] // truncated
		}
		for {
			n, err := file.ReadAt(buf, offset)
			offset += int64(n)
			pending = append(pending, buf[:n]...)
			for {
				i := bytes.IndexByte(pending, '\n')
				if i < 0 {
					break
				}
				handle(string(pending[:i]))
				pending = pending[i+1:]
			}
			if err == io.EOF || n == 0 {
				break
			}
			if err != nil {
				break
			}
		}
		if offset > maxSize && len(pending) == 0 {
			if os.Truncate(path, 0) == nil {
				offset = 0
			}
		}
		time.Sleep(time.Second)
	}
}
