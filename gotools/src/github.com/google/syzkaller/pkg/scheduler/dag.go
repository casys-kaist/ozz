package scheduler

type dag struct {
	nodes map[node]struct{}
	edges edge
}

func newDAG() dag {
	return dag{
		nodes: make(map[node]struct{}),
		edges: make(map[node]map[node]struct{}),
	}
}

func (d *dag) addEdge(src, dst interface{}) {
	d.nodes[src] = struct{}{}
	d.nodes[dst] = struct{}{}
	if _, ok := d.edges[src]; !ok {
		d.edges[src] = make(map[node]struct{})
	}
	d.edges[src][dst] = struct{}{}
}

func (d dag) topologicalSort() ([]node, bool) {
	res := make([]node, 0, len(d.nodes))
	q, head := make([]node, 0, len(d.nodes)), 0
	inbounds := make(map[node]int)
	visited := make(map[node]struct{})
	// Preprocessing: calculating in-bounds
	for v := range d.nodes {
		inbounds[v] = 0
	}

	for _, dsts := range d.edges {
		for dst := range dsts {
			inbounds[dst]++
		}
	}

	// step 1: queue all nodes with 0 inbound
	for n, inbound := range inbounds {
		if inbound == 0 {
			q = append(q, n)
		}
	}

	// step 2: iteratively infd a vertex with 0 inbound
	for head < len(q) {
		v := q[head]
		head++
		visited[v] = struct{}{}
		res = append(res, v)
		for dst := range d.edges[v] {
			inbounds[dst]--
			if inbounds[dst] == 0 {
				q = append(q, dst)
			}
		}
	}
	return res, len(visited) == len(d.nodes)
}

type node interface{}

type edge map[node]map[node]struct{}
