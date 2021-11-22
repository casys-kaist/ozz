package affinity

/*
#include "affinity.h"
*/
import "C"

func RunOnCPU(mask uint) bool {
	affinity := uint(C.get_affinity())
	if affinity == 0 {
		return false
	}
	return mask|affinity == mask && mask&affinity == mask
}

func Affinity() uint {
	return uint(C.get_affinity())
}
