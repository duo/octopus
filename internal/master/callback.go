package master

import (
	"fmt"
	"hash/fnv"
	"strconv"
)

// Telegram command callback
type Callback struct {
	Category string
	Acction  string
	Query    string
	Page     int
	Data     string
}

var cbMap = map[string]Callback{}

func putCallback(cb Callback) string {
	h := fnv.New64a()
	h.Write([]byte(fmt.Sprintf("%v", cb)))
	hash := strconv.FormatUint(h.Sum64(), 10)
	cbMap[hash] = cb
	return hash
}

func getCallback(hash string) (Callback, bool) {
	cb, ok := cbMap[hash]
	return cb, ok
}
