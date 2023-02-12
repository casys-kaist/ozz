#!/usr/bin/env python

import sys


def translate(fn):
    with open(fn) as f:
        lines = f.readlines()

    for line in lines:
        toks = line.split()
        if len(toks) < 10:
            print("Thread:")
        else:
            # thr = toks[4]
            # ctx = toks[6][:-2]
            inst = int(toks[7], 16) + 0xffffffff00000000
            addr = int(toks[9], 16) + 0xffffffff00000000
            size = toks[11][:-1]
            typ = int(toks[13][:-1])

            print(hex(inst)[2:], hex(addr), "R" if typ == 1 else "W", size)


translate(sys.argv[1])
