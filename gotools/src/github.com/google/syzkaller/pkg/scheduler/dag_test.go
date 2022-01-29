package scheduler

import "testing"

func buildDAGforTest(edges [][2]int) dag {
	dag := newDAG()
	for _, edge := range edges {
		dag.addEdge(edge[0], edge[1])
	}
	return dag
}

func TestDAGTopologicalSort(t *testing.T) {
	edges := [][2]int{
		{1, 2},
		{1, 3},
		{4, 3},
		{2, 3},
		{2, 5},
		{3, 6},
		{3, 6},
		{6, 7},
		{5, 8},
	}
	dag := buildDAGforTest(edges)
	nodes, ok := dag.topologicalSort()
	t.Logf("%d", len(nodes))
	if !ok {
		t.Errorf("wrong. detected a cycle")
	}
	for _, edge := range edges {
		src, dst := edge[0], edge[1]
		srcFound, dstFound := false, false
		for _, node0 := range nodes {
			node := node0.(int)
			if node == dst {
				if !srcFound {
					t.Errorf("dst appears before src, dst=%v, src=%v, nodes=%v",
						dst, src, nodes)
				}
				dstFound = true
			}
			if node == src {
				srcFound = true
			}
		}
		if !srcFound || !dstFound {
			t.Errorf("missing node, src=%v, dst=%v, nodes=%v",
				src, dst, nodes)
		}
	}
}

func TestCyclicGraphTopologicalSort(t *testing.T) {
	edges := [][2]int{
		{1, 2},
		{2, 3},
		{3, 1},
	}
	dag := buildDAGforTest(edges)
	_, ok := dag.topologicalSort()
	if ok {
		t.Errorf("wrong. failed to detect a cycle")
	}
}
