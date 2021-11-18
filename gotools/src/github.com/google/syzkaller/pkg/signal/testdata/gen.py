#!python

import re

class acc:
    def __init__(self, inst, addr, size, typ, ts):
        self.inst = int(inst, 16)
        self.addr = int(addr, 16)
        self.size = int(size, 16)
        self.typ = int(typ, 16)
        self.ts = int(ts, 16)

with open("data") as f:
    lines = f.readlines()

name = []
access = []
idx = -1
with open("accesses.dat", "w") as f:
    for line in lines:
        if not line.startswith('[FUZZER]'):
            continue
        if line.find("accesses") == -1:
            call = line.split()[-1]
            f.write('call: ' + call + '\n')
            name.append(call)
            access.append([])
            idx += 1
            continue
        line = re.sub(r',\)', '', line)
        toks = line.split()
        inst = toks[3]
        addr = toks[5]
        size = toks[7][:-1]
        typ = toks[9][:-1]
        ts = toks[10][10:-1]
        f.write(inst + ' ' + addr + ' ' +  size + ' ' + typ + ' ' + ts + '\n')
        a = acc(inst, addr, size, typ, ts)
        access[idx].append(a)

for i, acc1 in enumerate(access):
    store0 = {}
    for a1 in acc1:
        if a1.typ == 0:
            store0[a1.addr>>3] = a1

    for j, acc2 in enumerate(access):
        if i >= j:
            continue
        store = store0.copy()
        s = set()
        for a2 in acc2:
            # TODO: Since FromAccesses() in read_from.go does
            # not consider size of accesses at this point,
            # this script does not consider it too. Fix this
            # when FromAccesses() is fixed
            if a2.addr>>3 in store:
                a1 = store[a2.addr>>3]
                s.add(hex(a1.inst)[2:] + ' ' + hex(a2.inst)[2:] + '\n')
                if a2.typ == 0:
                    del store[a2.addr>>3]
        fn = name[i] + "_" + name[j] + "_rf.dat"
        with open(fn, 'w') as f:
            for a in sorted(list(s)):
                f.write(a)
