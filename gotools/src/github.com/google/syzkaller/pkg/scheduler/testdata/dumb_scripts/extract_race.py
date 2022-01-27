#!/usr/bin/env python

import sys

with open(sys.argv[1]) as f:
    lines = f.readlines()

import random

thrs = []
thr = -1
for line in lines:
    if line.startswith("Thread"):
        thr += 1
        thrs.append([])
        continue

    line = line.strip()
    toks = line.split()
    if len(toks) < 3:
        continue

    thrs[thr].append({
        'addr':int(toks[0], 16),
        'mem': int(toks[1], 16),
        'rw': toks[2],
    })

print(len(thrs))

for acc1 in thrs[0]:
    for acc2 in thrs[1]:
        if (((acc1['mem'] & 0xfffffffffffffffc) == (acc2['mem'] & 0xfffffffffffffffc)) and (acc1['rw'] == 'W' or acc2['rw'] == 'W')):
            print(acc1, acc2)

