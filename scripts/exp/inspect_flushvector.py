#!/usr/bin/env python

import os
import re
import subprocess
import sys


def main():
    s = sys.argv[1]
    regex = r"\{0x[0-9a-f]* [01]\}"
    kernel = os.path.join(
        os.environ["KERNELS_DIR"], "guest", "builds", "x86_64", "vmlinux"
    )
    for i, m in enumerate(re.findall(regex, s)):
        m = m[1:-1]
        toks = m.split()
        addr = hex(int(toks[0], 16) - 5)
        cmd = ["addr2line", "-ie", kernel, addr]
        res = subprocess.run(cmd, stdout=subprocess.PIPE)
        op = "Flushe" if toks[1].find("1") != -1 else "Not flushed"
        print("-----\n{}: {}\n{}".format(i, toks[1], res.stdout.decode("utf-8")))


if __name__ == "__main__":
    main()
