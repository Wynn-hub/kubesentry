// Package dbmodel provides the shared snowflake ID generator and the base
// GORM model embedded by every table in the kubesentry schema (spec D4/D5/D6).
package dbmodel

import (
	"hash/fnv"

	"github.com/bwmarrin/snowflake"
)

var node *snowflake.Node

// Init sets up the package-level snowflake ID generator. Call once at
// process startup (cmd/gateway, cmd/console, cmd/db) before any GORM
// BeforeCreate hook fires.
func Init(nodeID int64) error {
	n, err := snowflake.NewNode(nodeID)
	if err != nil {
		return err
	}
	node = n
	return nil
}

// NewID returns a freshly generated snowflake ID.
func NewID() int64 {
	if node == nil {
		panic("dbmodel: Init must be called before NewID")
	}
	return node.Generate().Int64()
}

// NodeIDFromPodName hashes POD_NAME (injected via Downward API) into
// snowflake's 10-bit node id range (0-1023), so multiple gateway/console
// replicas get distinct machine bits without a coordination service
// (spec D5). Empty input (single-replica dev/test) returns 0.
func NodeIDFromPodName(podName string) int64 {
	if podName == "" {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(podName))
	return int64(h.Sum32() % 1024)
}
