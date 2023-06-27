#!/usr/bin/env python

import os
import sys


def do_addr2line(addrs, verbose):
    import subprocess

    if len(addrs) == 0:
        return

    print("run do_addr2line")

    vmlinux = os.path.join(os.environ["KERNEL_X86_64"], "vmlinux")
    cmd = ["addr2line", "-e", vmlinux]
    if verbose:
        cmd = cmd + ["-fi"]
    res = subprocess.run(cmd + addrs, capture_output=True).stdout
    print(res.decode("utf-8"))


def main():
    if len(sys.argv) < 2:
        exit(1)

    fn = sys.argv[1]
    verbose = len(sys.argv) > 2

    with open(fn) as f:
        lines = f.readlines()
    addrs = []
    for line in lines:
        toks = line.split()
        if len(toks) < 1:
            continue
        f1 = toks[0]
        if not f1.startswith("0xffffffff") and not f1.startswith("ffffffff"):
            do_addr2line(addrs, verbose)
            addrs = []
        else:
            addrs.append(hex(int(f1, 16) - 5))

    if len(addrs) != 0:
        do_addr2line(addrs, verbose)


if __name__ == "__main__":
    main()
