package scheduler

import "github.com/google/syzkaller/pkg/primitive"

const (
	upperleft = iota + 1
	upper
	left
)

// NOTE: We infer the program order with at most two serial
// executions. Thus, we can use LCS for two sequences. Extend LCS to
// handle multiple sequences if we want to handle more complex cases
// (i.e., with more than two serials).

func lcs(serial1, serial2 primitive.SerialAccess) (int, [][]bool) {
	// TODO: The current implementation allocates lots of memory and
	// incurs the high overhead. In this project we need an
	// approximation of LCS, not an exact LCS, which can be possibly
	// obtained by a lightweight heuristic.
	d := make([][]struct{ val, dir int }, len(serial1)+1)
	for i := 0; i < len(serial1)+1; i++ {
		d[i] = make([]struct{ val, dir int }, len(serial2)+1)
	}

	// LCS is calculated based on instructions' addresses
	for i := 1; i <= len(serial1); i++ {
		for j := 1; j <= len(serial2); j++ {
			if serial1[i-1].Inst == serial2[j-1].Inst {
				d[i][j].val = d[i-1][j-1].val + 1
				d[i][j].dir = upperleft
			} else {
				if d[i-1][j].val > d[i][j-1].val {
					d[i][j].val = d[i-1][j].val
					d[i][j].dir = upper
				} else {
					d[i][j].val = d[i][j-1].val
					d[i][j].dir = left
				}
			}
		}
	}
	return lcsTrackBack(d, serial1, serial2)
}

func lcsTrackBack(d [][]struct{ val, dir int }, serial1, serial2 primitive.SerialAccess) (int, [][]bool) {
	l := d[len(serial1)][len(serial2)].val
	// bitmaps represent that each element is included in LCS
	bitmap1 := make([]bool, len(serial1))
	bitmap2 := make([]bool, len(serial2))
	for idx, i, j := l-1, len(serial1), len(serial2); idx >= 0; {
		switch d[i][j].dir {
		case upperleft:
			bitmap1[i-1] = true
			bitmap2[j-1] = true
			idx, i, j = idx-1, i-1, j-1
		case upper:
			i--
		case left:
			j--
		}
	}
	return l, [][]bool{bitmap1, bitmap2}
}
