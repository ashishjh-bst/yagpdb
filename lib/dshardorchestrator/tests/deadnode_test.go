package tests

import (
	"testing"
	"time"

	"github.com/botlabs-gg/yagpdb/v2/lib/dshardorchestrator/node"
	"github.com/botlabs-gg/yagpdb/v2/lib/dshardorchestrator/orchestrator"
)

// waitForNodeStatus polls the orchestrator until the provided check passes for the provided node,
// it returns nil if the node is gone entirely
func waitForNodeStatus(t *testing.T, orch *orchestrator.Orchestrator, nodeID string, timeout time.Duration, check func(*orchestrator.NodeStatus) bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		var status *orchestrator.NodeStatus
		for _, ns := range orch.GetFullNodesStatus() {
			if ns.ID == nodeID {
				status = ns
				break
			}
		}

		if check(status) {
			return
		}

		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting on the status of node %q, last status: %+v", nodeID, status)
		}

		time.Sleep(time.Millisecond * 100)
	}
}

// TestShardMigrationDestinationDies makes sure that a node dying in the middle of a shard migration
// doesn't leave the other node stuck in a migrating state forever, which used to keep the shard down
// permanently (the monitor skips shards that look like they're being migrated) and made every later
// migration involving that node fail with ErrNodeBusy
func TestShardMigrationDestinationDies(t *testing.T) {
	orch := CreateMockOrchestrator(10)
	// keep the dead node around so we can assert on its state
	orch.NodeReapDelay = -1

	err := orch.Start(testServerAddr)
	if err != nil {
		t.Fatal("failed starting orchestrator: ", err)
	}
	defer orch.Stop()

	sessionWaitChan := make(chan node.SessionInfo)
	shardsAddedChan := make(chan []int, 10)

	mockBot := func() *MockBot {
		return &MockBot{
			SessionEstablishedFunc: func(info node.SessionInfo) {
				sessionWaitChan <- info
			},
			AddNewShardFunc: func(shards ...int) {
				shardsAddedChan <- shards
			},
		}
	}

	n1, ok := startConnectNode(t, mockBot(), sessionWaitChan)
	if !ok {
		t.Fatal("failed connecting node 1")
	}
	defer n1.Close()

	n2, ok := startConnectNode(t, mockBot(), sessionWaitChan)
	if !ok {
		t.Fatal("failed connecting node 2")
	}

	on1 := orch.FindNodeByID(n1.GetIDLock())
	addWaitForShards(t, []int{0, 1, 2, 3, 4}, shardsAddedChan, on1)

	// give the orchestrator a moment to record the started shards
	time.Sleep(time.Millisecond * 250)

	err = orch.StartShardMigration(n2.GetIDLock(), 3)
	if err != nil {
		t.Fatal("failed starting the migration: ", err)
	}

	// the destination node dies mid migration
	n2.Close()

	// both ends should drop out of the migrating state on their own
	waitForNodeStatus(t, orch, n1.GetIDLock(), time.Second*15, func(status *orchestrator.NodeStatus) bool {
		return status != nil && status.MigratingTo == "" && status.MigratingFrom == ""
	})

	waitForNodeStatus(t, orch, n2.GetIDLock(), time.Second*15, func(status *orchestrator.NodeStatus) bool {
		return status == nil || (!status.Connected && status.MigratingTo == "" && status.MigratingFrom == "")
	})

	// and the origin node should no longer be considered busy
	n3, ok := startConnectNode(t, mockBot(), sessionWaitChan)
	if !ok {
		t.Fatal("failed connecting node 3")
	}
	defer n3.Close()

	err = orch.StartShardMigration(n3.GetIDLock(), 2)
	if err != nil {
		t.Fatal("origin node was still stuck after the destination died: ", err)
	}
}

// TestShutdownDisconnectedNode makes sure shutting down an already dead node reports what happened and
// drops the node instead of writing into a dead connection and leaving the entry in the status output
func TestShutdownDisconnectedNode(t *testing.T) {
	orch := CreateMockOrchestrator(10)
	orch.NodeReapDelay = -1

	err := orch.Start(testServerAddr)
	if err != nil {
		t.Fatal("failed starting orchestrator: ", err)
	}
	defer orch.Stop()

	sessionWaitChan := make(chan node.SessionInfo)
	n1, ok := startConnectNode(t, &MockBot{
		SessionEstablishedFunc: func(info node.SessionInfo) {
			sessionWaitChan <- info
		},
	}, sessionWaitChan)
	if !ok {
		t.Fatal("failed connecting node 1")
	}

	nodeID := n1.GetIDLock()
	n1.Close()

	waitForNodeStatus(t, orch, nodeID, time.Second*15, func(status *orchestrator.NodeStatus) bool {
		return status != nil && !status.Connected
	})

	if err := orch.ShutdownNode(nodeID); err != orchestrator.ErrNodeNotConnected {
		t.Fatalf("expected ErrNodeNotConnected, got: %v", err)
	}

	if err := orch.ShutdownNode(nodeID); err != orchestrator.ErrUnknownNode {
		t.Fatalf("node should be gone after the first shutdown, got: %v", err)
	}

	for _, ns := range orch.GetFullNodesStatus() {
		if ns.ID == nodeID {
			t.Fatal("disconnected node is still in the status output")
		}
	}
}

// TestReapDisconnectedNodes makes sure dead nodes don't linger in the status output forever
func TestReapDisconnectedNodes(t *testing.T) {
	orch := CreateMockOrchestrator(10)
	orch.NodeReapDelay = time.Millisecond * 100

	err := orch.Start(testServerAddr)
	if err != nil {
		t.Fatal("failed starting orchestrator: ", err)
	}
	defer orch.Stop()

	sessionWaitChan := make(chan node.SessionInfo)
	n1, ok := startConnectNode(t, &MockBot{
		SessionEstablishedFunc: func(info node.SessionInfo) {
			sessionWaitChan <- info
		},
	}, sessionWaitChan)
	if !ok {
		t.Fatal("failed connecting node 1")
	}

	nodeID := n1.GetIDLock()
	n1.Close()

	waitForNodeStatus(t, orch, nodeID, time.Second*15, func(status *orchestrator.NodeStatus) bool {
		return status != nil && !status.Connected
	})

	time.Sleep(orch.NodeReapDelay * 2)
	orch.ReapDisconnectedNodes()

	for _, ns := range orch.GetFullNodesStatus() {
		if ns.ID == nodeID {
			t.Fatal("disconnected node was not reaped")
		}
	}
}
