package orchestrator

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/botlabs-gg/yagpdb/v2/lib/dshardorchestrator"
)

// NodeConn represents a connection from a master server to a slave
type NodeConn struct {
	Orchestrator *Orchestrator
	Conn         *dshardorchestrator.Conn

	connected      bool
	disconnectedAt time.Time

	// below fields are protected by this mutex
	mu sync.Mutex

	sessionEstablished bool
	version            string
	runningShards      []int

	shardMigrationOtherNodeID   string
	shardMigrationShard         int
	shardMigrationMode          dshardorchestrator.ShardMigrationMode
	processedUserEvents         int
	shardmigrationTotalUserEvts int

	shuttingDown bool
}

// NewNodeConn creates a new NodeConn (connection from master to slave) from a net.Conn
func (o *Orchestrator) NewNodeConn(netConn net.Conn) *NodeConn {
	sc := &NodeConn{
		Conn:                dshardorchestrator.ConnFromNetCon(netConn, o.Logger),
		Orchestrator:        o,
		connected:           true,
		shardMigrationShard: -1,
	}

	sc.Conn.MessageHandler = sc.handleMessage
	sc.Conn.ConnClosedHanlder = func() {
		sc.mu.Lock()
		sc.connected = false
		sc.disconnectedAt = time.Now()

		// a node that's gone can never finish a shard migration, so drop the migration state here.
		// if we didn't, this node and the one on the other end of the migration would both be stuck
		// in a migrating state forever, which blocks the monitor from restarting the shard and makes
		// every later migration involving them fail with ErrNodeBusy
		migratingShard := sc.shardMigrationShard
		wasMigrating := sc.shardMigrationMode != dshardorchestrator.ShardMigrationModeNone
		sc.clearShardMigrationState()
		sc.mu.Unlock()

		if wasMigrating {
			o.Log(dshardorchestrator.LogWarning, nil, fmt.Sprintf("node %q disconnected while migrating shard %d, aborting that migration", sc.Conn.GetID(), migratingShard))
		}

		// the node on the other end of any migration involving this one is stuck as well, clear it too
		o.abortShardMigrationsWith(sc.Conn.GetID())
	}

	return sc
}

func (nc *NodeConn) listen() {
	nc.Conn.Listen()
}

func (nc *NodeConn) removeShard(shardID int) {
	for i, v := range nc.runningShards {
		if v == shardID {
			nc.runningShards = append(nc.runningShards[:i], nc.runningShards[i+1:]...)
		}
	}
}

// Handle incoming messages
func (nc *NodeConn) handleMessage(msg *dshardorchestrator.Message) {
	switch msg.EvtID {
	case dshardorchestrator.EvtIdentify:
		nc.handleIdentify(msg.DecodedBody.(*dshardorchestrator.IdentifyData))

	case dshardorchestrator.EvtStartShards:
		data := msg.DecodedBody.(*dshardorchestrator.StartShardsData)
		nc.mu.Lock()

		// add the shards to the tracked shards for this node
		for _, v := range data.ShardIDs {
			if !dshardorchestrator.ContainsInt(nc.runningShards, v) {
				nc.runningShards = append(nc.runningShards, v)
			}

			if nc.shardMigrationShard == v && nc.shardMigrationMode != dshardorchestrator.ShardMigrationModeNone {
				// we were migrating this shard, mark it as done
				nc.shardMigrationMode = dshardorchestrator.ShardMigrationModeNone
			}
		}

		nc.mu.Unlock()

	case dshardorchestrator.EvtStopShard:
		data := msg.DecodedBody.(*dshardorchestrator.StopShardData)
		nc.mu.Lock()
		nc.removeShard(data.ShardID)
		nc.mu.Unlock()

	case dshardorchestrator.EvtPrepareShardmigration:
		data := msg.DecodedBody.(*dshardorchestrator.PrepareShardmigrationData)

		nc.mu.Lock()
		otherNodeID := nc.shardMigrationOtherNodeID
		nc.mu.Unlock()

		otherNode := nc.Orchestrator.FindNodeByID(otherNodeID)
		if otherNode == nil {
			nc.Conn.Log(dshardorchestrator.LogError, nil, "node dissapeared in the middle of shard migration")
			return
		}

		if data.Origin {
			nc.mu.Lock()
			nc.removeShard(data.ShardID)
			nc.mu.Unlock()

			data.Origin = false
			go otherNode.Conn.SendLogErr(dshardorchestrator.EvtPrepareShardmigration, data)
			return
		}
		// start sending state data
		go otherNode.Conn.SendLogErr(dshardorchestrator.EvtStartShardMigration, &dshardorchestrator.StartshardMigrationData{
			ShardID: data.ShardID,
		})

	case dshardorchestrator.EvtAllUserdataSent:
		nc.mu.Lock()
		otherNodeID := nc.shardMigrationOtherNodeID
		nc.shardMigrationMode = dshardorchestrator.ShardMigrationModeNone
		nc.mu.Unlock()

		otherNode := nc.Orchestrator.FindNodeByID(otherNodeID)
		if otherNode == nil {
			nc.Conn.Log(dshardorchestrator.LogError, nil, "node dissapeared in the middle of shard migration")
			return
		}

		go otherNode.Conn.SendLogErr(dshardorchestrator.EvtAllUserdataSent, msg.DecodedBody)

	default:
		if msg.EvtID < 100 {
			return
		}

		nc.mu.Lock()
		otherNodeID := nc.shardMigrationOtherNodeID
		nc.mu.Unlock()

		otherNode := nc.Orchestrator.FindNodeByID(otherNodeID)
		if otherNode == nil {
			nc.Conn.Log(dshardorchestrator.LogError, nil, "node dissapeared in the middle of shard migration")
			return
		}

		go otherNode.Conn.SendLogErr(msg.EvtID, msg.RawBody)
	}
}

func (nc *NodeConn) validateTotalShards(data *dshardorchestrator.IdentifyData) (ok bool, totalShards int) {
	nc.Orchestrator.mu.Lock()
	if data.TotalShards == 0 && nc.Orchestrator.totalShards == 0 {
		// we may need to fetch a fresh shard count, but wait 10 seconds to see if another node with already set shard count connects

		nc.Orchestrator.mu.Unlock()

		if !nc.Orchestrator.SkipSafeStartupDelayMaxShards {
			for i := 0; i < 100; i++ {
				time.Sleep(time.Millisecond * 100)

				nc.Orchestrator.mu.Lock()
				if nc.Orchestrator.totalShards != 0 {
					nc.Orchestrator.mu.Unlock()
					break
				}

				nc.Orchestrator.mu.Unlock()
			}
		}

		// we need to fetch a fresh total shard count
		for {
			nc.Orchestrator.mu.Lock()
			if nc.Orchestrator.totalShards != 0 {
				break
			}

			sc, err := nc.Orchestrator.getTotalShardCount()
			if err != nil {
				nc.Orchestrator.mu.Unlock()
				nc.Conn.Log(dshardorchestrator.LogError, err, "failed fetching total shard count, retrying in a second")
				time.Sleep(time.Second)
				continue
			}

			nc.Orchestrator.totalShards = sc
			break // keep it locked out of the loop
		}
	}

	totalShards = nc.Orchestrator.totalShards
	if data.TotalShards > 0 && nc.Orchestrator.totalShards == 0 {
		nc.Orchestrator.totalShards = data.TotalShards
		totalShards = data.TotalShards
	} else if data.TotalShards > 0 && data.TotalShards != nc.Orchestrator.totalShards {
		// in this case there isn't much we can do, in the current state the orchestrator does not support varying shard counts so if this were to happen then yeah...
		// in the future this will be handled, things like rescaling shard by doubling the count is a relatively easy process
		// (shut down 1 shard completely, start 2 shards that combined were holding the same servers as the one shut down, works since it's doubled)
		nc.Conn.Log(dshardorchestrator.LogError, nil, "NOT-MATCHING TOTAL SHARD COUNTS!")
		nc.Orchestrator.mu.Unlock()
		return false, totalShards
	}

	nc.Orchestrator.mu.Unlock()
	return true, totalShards
}

func (nc *NodeConn) handleIdentify(data *dshardorchestrator.IdentifyData) {
	if data.OrchestratorLogicVersion != 2 {
		// TODO: maybe disconnect instead? altough this is the best way of handling it imo
		// instead of accidentally launching multiple of already connected shards, just stop everything if this happens
		panic("Incompatible node behaviour/protocol version")
	}

	valid, totalShards := nc.validateTotalShards(data)
	if !valid {
		return
	}

	nc.Conn.ID.Store(data.NodeID)

	// check if were holding a duplicates
	nc.Orchestrator.mu.Lock()
	for _, n := range nc.Orchestrator.connectedNodes {
		if n.Conn.GetID() == data.NodeID && n != nc {
			go n.Conn.Close()
			nc.Orchestrator.removeNodeConnLocked(n)
			break
		}
	}
	nc.Orchestrator.mu.Unlock()

	// after this we have sucessfully established a session
	resp := &dshardorchestrator.IdentifiedData{
		NodeID:      nc.Conn.GetID(),
		TotalShards: totalShards,
	}

	nc.Conn.SendLogErr(dshardorchestrator.EvtIdentified, resp)

	nc.mu.Lock()
	nc.version = data.Version
	nc.sessionEstablished = true
	nc.runningShards = data.RunningShards
	nc.mu.Unlock()

	nc.Conn.Log(dshardorchestrator.LogInfo, nil, fmt.Sprintf("v%s - tot.shards: %d - running.shards: %v", data.Version, data.TotalShards, data.RunningShards))
}

// GetFullStatus returns the current status of the node
func (nc *NodeConn) GetFullStatus() *NodeStatus {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	status := &NodeStatus{
		ID:                 nc.Conn.GetID(),
		SessionEstablished: nc.sessionEstablished,
		MigratingShard:     nc.shardMigrationShard,
		Connected:          nc.connected,
		DisconnectedAt:     nc.disconnectedAt,
		Version:            nc.version,
	}

	status.Shards = make([]int, len(nc.runningShards))
	copy(status.Shards, nc.runningShards)

	if nc.shardMigrationMode == dshardorchestrator.ShardMigrationModeFrom {
		status.MigratingTo = nc.shardMigrationOtherNodeID
	} else if nc.shardMigrationMode == dshardorchestrator.ShardMigrationModeTo {
		status.MigratingFrom = nc.shardMigrationOtherNodeID
	}

	return status
}

// IsConnected returns whether the node is currently connected to the orchestrator
func (nc *NodeConn) IsConnected() bool {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	return nc.connected
}

// clearShardMigrationState resets any in progress shard migration on this node
// assumes nc.mu is held
func (nc *NodeConn) clearShardMigrationState() {
	nc.shardMigrationMode = dshardorchestrator.ShardMigrationModeNone
	nc.shardMigrationOtherNodeID = ""
	nc.shardMigrationShard = -1
	nc.processedUserEvents = 0
	nc.shardmigrationTotalUserEvts = 0
}

// StartShards tells the node to start the following shards
func (nc *NodeConn) StartShards(shards ...int) {
	go nc.Conn.SendLogErr(dshardorchestrator.EvtStartShards, &dshardorchestrator.StartShardsData{
		ShardIDs: shards,
	})
}

// StopShard tells the node to stop the provided shard
func (nc *NodeConn) StopShard(shard int) {
	go nc.Conn.SendLogErr(dshardorchestrator.EvtStopShard, &dshardorchestrator.StopShardData{
		ShardID: shard,
	})
}

// Shutdown tells the node to shut down compleely
func (nc *NodeConn) Shutdown() {
	nc.mu.Lock()
	nc.shuttingDown = true
	go nc.Conn.SendLogErr(dshardorchestrator.EvtShutdown, nil)
	nc.mu.Unlock()
	return
}
