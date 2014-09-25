// Copyright 2012, Google Inc. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Actions modify the state of a tablet, shard or keyspace.
//
// They are currenty managed through a series of queues stored in
// topology server, or RPCs. Switching to RPCs only now.

package actionnode

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"strings"
	"time"

	log "github.com/golang/glog"
	"github.com/youtube/vitess/go/jscfg"
	"github.com/youtube/vitess/go/vt/hook"
	myproto "github.com/youtube/vitess/go/vt/mysqlctl/proto"
)

const (
	// FIXME(msolomon) why is ActionState a type, but Action is not?

	//
	// Tablet actions. This first list is RPC only. In the process
	// of converting them all to RPCs.
	//

	// Sleep will sleep for a duration (used for tests)
	TABLET_ACTION_SLEEP = "Sleep"

	// SetReadOnly makes the mysql instance read-only
	TABLET_ACTION_SET_RDONLY = "SetReadOnly"

	// SetReadWrite makes the mysql instance read-write
	TABLET_ACTION_SET_RDWR = "SetReadWrite"

	// ChangeType changes the type of the tablet
	TABLET_ACTION_CHANGE_TYPE = "ChangeType"

	// Scrap scraps the live running tablet
	TABLET_ACTION_SCRAP = "Scrap"

	// DemoteMaster tells the current master it's about to not be a master
	// any more, and should go read-only.
	TABLET_ACTION_DEMOTE_MASTER = "DemoteMaster"

	// PromoteSlave tells the tablet it is going to be the master.
	TABLET_ACTION_PROMOTE_SLAVE = "PromoteSlave"

	// SlaveWasPromoted tells a tablet this previously slave
	// tablet is now the master. The tablet will update its
	// own topology record.
	TABLET_ACTION_SLAVE_WAS_PROMOTED = "SlaveWasPromoted"

	// RestartSlave tells the remote tablet it has a new master.
	TABLET_ACTION_RESTART_SLAVE = "RestartSlave"

	// SlaveWasRestarted tells a tablet the mysql master was changed.
	// The tablet will check it is indeed the case, and update its own
	// topology record.
	TABLET_ACTION_SLAVE_WAS_RESTARTED = "SlaveWasRestarted"

	// BreakSlaves will tinker with the replication stream in a way
	// that will stop all the slaves.
	TABLET_ACTION_BREAK_SLAVES = "BreakSlaves"

	// StopSlave will stop MySQL replication.
	TABLET_ACTION_STOP_SLAVE = "StopSlave"

	// StopSlave will stop MySQL replication after it reaches a
	// minimum point.
	TABLET_ACTION_STOP_SLAVE_MINIMUM = "StopSlaveMinimum"

	// StartSlave will start MySQL replication.
	TABLET_ACTION_START_SLAVE = "StartSlave"

	// TabletExternallyReparented is sent directly to the new master
	// tablet when it becomes the master. It is functionnaly equivalent
	// to calling "ShardExternallyReparented" on the topology.
	TABLET_ACTION_EXTERNALLY_REPARENTED = "TabletExternallyReparented"

	// MasterPosition returns the current master position
	TABLET_ACTION_MASTER_POSITION = "MasterPosition"

	// ReparentPosition returns the data for a slave to use to reparent
	// to the target tablet at the given position.
	TABLET_ACTION_REPARENT_POSITION = "ReparentPosition"

	// SlaveStatus returns the current slave status
	TABLET_ACTION_SLAVE_STATUS = "SlaveStatus"

	// WaitSlavePosition waits until the slave reaches a
	// replication position in MySQL replication
	TABLET_ACTION_WAIT_SLAVE_POSITION = "WaitSlavePosition"

	// WaitBlpPosition waits until the slave reaches a
	// replication position in filtered replication
	TABLET_ACTION_WAIT_BLP_POSITION = "WaitBlpPosition"

	// Stop and Start filtered replication
	TABLET_ACTION_STOP_BLP  = "StopBlp"
	TABLET_ACTION_START_BLP = "StartBlp"

	// RunBlpUntil will run filtered replication until it reaches
	// the provided stop position.
	TABLET_ACTION_RUN_BLP_UNTIL = "RunBlpUntil"

	// GetSchema returns the tablet current schema.
	TABLET_ACTION_GET_SCHEMA = "GetSchema"

	// ReloadSchema tells the tablet to reload its schema.
	TABLET_ACTION_RELOAD_SCHEMA = "ReloadSchema"

	// ExecuteFetch uses the DBA connection pool to run queries.
	TABLET_ACTION_EXECUTE_FETCH = "ExecuteFetch"

	// GetPermissions returns the mysql permissions set
	TABLET_ACTION_GET_PERMISSIONS = "GetPermissions"

	// GetSlaves returns the current set of mysql replication slaves.
	TABLET_ACTION_GET_SLAVES = "GetSlaves"

	//
	// Tablet actions. These are triggered by ActionNode only,
	// will be documented / converted to RPC soon.
	//

	TABLET_ACTION_PREFLIGHT_SCHEMA = "PreflightSchema"
	TABLET_ACTION_APPLY_SCHEMA     = "ApplySchema"

	TABLET_ACTION_EXECUTE_HOOK = "ExecuteHook"

	TABLET_ACTION_SNAPSHOT            = "Snapshot"
	TABLET_ACTION_SNAPSHOT_SOURCE_END = "SnapshotSourceEnd"
	TABLET_ACTION_RESERVE_FOR_RESTORE = "ReserveForRestore"
	TABLET_ACTION_RESTORE             = "Restore"
	TABLET_ACTION_MULTI_SNAPSHOT      = "MultiSnapshot"
	TABLET_ACTION_MULTI_RESTORE       = "MultiRestore"

	//
	// Tablet actions. These are triggered by both RPC and ActionNode,
	// will be documented / converted to RPC only soon.
	//

	TABLET_ACTION_PING = "Ping"

	//
	// Shard actions - involve all tablets in a shard.
	// These are just descriptive and used for locking / logging.
	//

	SHARD_ACTION_REPARENT              = "ReparentShard"
	SHARD_ACTION_EXTERNALLY_REPARENTED = "ShardExternallyReparented"
	// Recompute derived shard-wise data
	SHARD_ACTION_REBUILD = "RebuildShard"
	// Generic read lock for inexpensive shard-wide actions.
	SHARD_ACTION_CHECK = "CheckShard"
	// Apply a schema change on an entire shard
	SHARD_ACTION_APPLY_SCHEMA = "ApplySchemaShard"
	// Changes the ServedTypes inside a shard
	SHARD_ACTION_SET_SERVED_TYPES = "SetShardServedTypes"
	// Multi-restore on all tablets of a shard in parallel
	SHARD_ACTION_MULTI_RESTORE = "ShardMultiRestore"
	// Migrate served types from one shard to another
	SHARD_ACTION_MIGRATE_SERVED_TYPES = "MigrateServedTypes"
	// Update the Shard object (Cells, ...)
	SHARD_ACTION_UPDATE_SHARD = "UpdateShard"

	//
	// Keyspace actions - require very high level locking for consistency.
	// These are just descriptive and used for locking / logging.
	//

	KEYSPACE_ACTION_REBUILD             = "RebuildKeyspace"
	KEYSPACE_ACTION_APPLY_SCHEMA        = "ApplySchemaKeyspace"
	KEYSPACE_ACTION_SET_SHARDING_INFO   = "SetKeyspaceShardingInfo"
	KEYSPACE_ACTION_MIGRATE_SERVED_FROM = "MigrateServedFrom"

	//
	// SrvShard actions - very local locking, for consistency.
	// These are just descriptive and used for locking / logging.
	//

	SRV_SHARD_ACTION_REBUILD = "RebuildSrvShard"

	// all the valid states for an action

	ACTION_STATE_QUEUED  = ActionState("")        // All actions are queued initially
	ACTION_STATE_RUNNING = ActionState("Running") // Running inside vtaction process
	ACTION_STATE_FAILED  = ActionState("Failed")  // Ended with a failure
	ACTION_STATE_DONE    = ActionState("Done")    // Ended with no failure
)

// ActionState is the state an ActionNode
type ActionState string

// ActionNode describes a long-running action on a tablet, or an action
// on a shard or keyspace that locks it.
type ActionNode struct {
	Action     string
	ActionGuid string
	Error      string
	State      ActionState
	Pid        int // only != 0 if State == ACTION_STATE_RUNNING

	// do not serialize the next fields
	// path in topology server representing this action
	Path  string      `json:"-"`
	Args  interface{} `json:"-"`
	Reply interface{} `json:"-"`
}

// ActionNodeFromJson interprets the data from JSON.
func ActionNodeFromJson(data, path string) (*ActionNode, error) {
	decoder := json.NewDecoder(strings.NewReader(data))

	// decode the ActionNode
	node := &ActionNode{}
	err := decoder.Decode(node)
	if err != nil {
		return nil, err
	}
	node.Path = path

	// figure out our args and reply types
	switch node.Action {
	case TABLET_ACTION_PING:

	case TABLET_ACTION_PREFLIGHT_SCHEMA:
		node.Args = new(string)
		node.Reply = &myproto.SchemaChangeResult{}
	case TABLET_ACTION_APPLY_SCHEMA:
		node.Args = &myproto.SchemaChange{}
		node.Reply = &myproto.SchemaChangeResult{}
	case TABLET_ACTION_EXECUTE_HOOK:
		node.Args = &hook.Hook{}
		node.Reply = &hook.HookResult{}

	case TABLET_ACTION_SNAPSHOT:
		node.Args = &SnapshotArgs{}
		node.Reply = &SnapshotReply{}
	case TABLET_ACTION_SNAPSHOT_SOURCE_END:
		node.Args = &SnapshotSourceEndArgs{}
	case TABLET_ACTION_RESERVE_FOR_RESTORE:
		node.Args = &ReserveForRestoreArgs{}
	case TABLET_ACTION_RESTORE:
		node.Args = &RestoreArgs{}
	case TABLET_ACTION_MULTI_SNAPSHOT:
		node.Args = &MultiSnapshotArgs{}
		node.Reply = &MultiSnapshotReply{}
	case TABLET_ACTION_MULTI_RESTORE:
		node.Args = &MultiRestoreArgs{}

	case SHARD_ACTION_REPARENT,
		SHARD_ACTION_EXTERNALLY_REPARENTED,
		SHARD_ACTION_REBUILD,
		SHARD_ACTION_CHECK,
		SHARD_ACTION_APPLY_SCHEMA,
		SHARD_ACTION_SET_SERVED_TYPES,
		SHARD_ACTION_MULTI_RESTORE,
		SHARD_ACTION_MIGRATE_SERVED_TYPES,
		SHARD_ACTION_UPDATE_SHARD:
		return nil, fmt.Errorf("locking-only SHARD action: %v", node.Action)

	case KEYSPACE_ACTION_REBUILD,
		KEYSPACE_ACTION_APPLY_SCHEMA,
		KEYSPACE_ACTION_SET_SHARDING_INFO,
		KEYSPACE_ACTION_MIGRATE_SERVED_FROM:
		return nil, fmt.Errorf("locking-only KEYSPACE action: %v", node.Action)

	case SRV_SHARD_ACTION_REBUILD:
		return nil, fmt.Errorf("locking-only SRV_SHARD action: %v", node.Action)

	case TABLET_ACTION_SLEEP,
		TABLET_ACTION_SET_RDONLY,
		TABLET_ACTION_SET_RDWR,
		TABLET_ACTION_CHANGE_TYPE,
		TABLET_ACTION_SCRAP,
		TABLET_ACTION_GET_SCHEMA,
		TABLET_ACTION_RELOAD_SCHEMA,
		TABLET_ACTION_EXECUTE_FETCH,
		TABLET_ACTION_GET_PERMISSIONS,
		TABLET_ACTION_SLAVE_STATUS,
		TABLET_ACTION_WAIT_SLAVE_POSITION,
		TABLET_ACTION_MASTER_POSITION,
		TABLET_ACTION_REPARENT_POSITION,
		TABLET_ACTION_DEMOTE_MASTER,
		TABLET_ACTION_PROMOTE_SLAVE,
		TABLET_ACTION_SLAVE_WAS_PROMOTED,
		TABLET_ACTION_RESTART_SLAVE,
		TABLET_ACTION_SLAVE_WAS_RESTARTED,
		TABLET_ACTION_BREAK_SLAVES,
		TABLET_ACTION_STOP_SLAVE,
		TABLET_ACTION_STOP_SLAVE_MINIMUM,
		TABLET_ACTION_START_SLAVE,
		TABLET_ACTION_EXTERNALLY_REPARENTED,
		TABLET_ACTION_GET_SLAVES,
		TABLET_ACTION_WAIT_BLP_POSITION,
		TABLET_ACTION_STOP_BLP,
		TABLET_ACTION_START_BLP,
		TABLET_ACTION_RUN_BLP_UNTIL:
		return nil, fmt.Errorf("rpc-only action: %v", node.Action)

	default:
		return nil, fmt.Errorf("unrecognized action: %v", node.Action)
	}

	// decode the args
	if node.Args != nil {
		err = decoder.Decode(node.Args)
	} else {
		var a interface{}
		err = decoder.Decode(&a)
	}
	if err != nil {
		return nil, err
	}

	// decode the reply
	if node.Reply != nil {
		err = decoder.Decode(node.Reply)
	} else {
		var a interface{}
		err = decoder.Decode(&a)
	}
	if err != nil {
		return nil, err
	}

	return node, nil
}

// ToJson returns a JSON representation of the object.
func (n *ActionNode) ToJson() string {
	result := jscfg.ToJson(n) + "\n"
	if n.Args == nil {
		result += "{}\n"
	} else {
		result += jscfg.ToJson(n.Args) + "\n"
	}
	if n.Reply == nil {
		result += "{}\n"
	} else {
		result += jscfg.ToJson(n.Reply) + "\n"
	}
	return result
}

// SetGuid will set the ActionGuid field for the action node
// and return the action node.
func (n *ActionNode) SetGuid() *ActionNode {
	now := time.Now().Format(time.RFC3339)
	username := "unknown"
	if u, err := user.Current(); err == nil {
		username = u.Username
	}
	hostname := "unknown"
	if h, err := os.Hostname(); err == nil {
		hostname = h
	}
	n.ActionGuid = fmt.Sprintf("%v-%v-%v", now, username, hostname)
	return n
}

// ActionNodeCanBePurged returns true if that ActionNode can be purged
// from the topology server.
func ActionNodeCanBePurged(data string) bool {
	actionNode, err := ActionNodeFromJson(data, "")
	if err != nil {
		log.Warningf("bad action data: %v %#v", err, data)
		return true
	}

	if actionNode.State == ACTION_STATE_RUNNING {
		log.Infof("cannot remove running action: %v %v", actionNode.Action, actionNode.ActionGuid)
		return false
	}

	return true
}

// ActionNodeIsStale returns true if that ActionNode is not Running
func ActionNodeIsStale(data string) bool {
	actionNode, err := ActionNodeFromJson(data, "")
	if err != nil {
		log.Warningf("bad action data: %v %#v", err, data)
		return false
	}

	return actionNode.State != ACTION_STATE_RUNNING
}
