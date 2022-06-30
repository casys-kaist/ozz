#!/usr/bin/env python

import os
import shutil
import sys


def scrap_crash(crash, report_dir):
    crash_hash = os.path.basename(crash)
    outdir = os.path.join(report_dir, crash_hash)
    os.makedirs(outdir)
    for root, _, files in os.walk(crash):
        for file in files:
            if file.startswith("machineInfo"):
                continue
            src = os.path.join(root, file)
            dst = os.path.join(outdir, file)
            shutil.copyfile(src, dst)


def scrap_crashes(crashes):
    exp_dir = os.environ["EXP_DIR"]
    report_dir = os.path.join(exp_dir, "report")
    for crash in crashes:
        scrap_crash(crash, report_dir)


def main():
    if len(sys.argv) < 2:
        exit(1)

    scrap_crashes(sys.argv[1:])


if __name__ == "__main__":
    main()
