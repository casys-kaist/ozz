#!/usr/bin/env python

import os
import shutil
import sys


def get_kernel_hash(crash):
    crashes_dir = os.path.basename(os.path.dirname(crash))
    PREFIX = "crashes-"
    if crashes_dir.startswith(PREFIX):
        kernel_hsh = crashes_dir[len(PREFIX) :]
    else:
        kernel_hsh = "unknown"

    return kernel_hsh


def get_kernel_info(crash, kernels_dir):
    kernel_hsh = get_kernel_hash(crash)

    try:
        with open(os.path.join(kernels_dir, "guest", "BUILD_HISTORY")) as f:
            build_history = f.readlines()
            assert len(build_history) > 0
    except:
        build_history = ["unknown"]

    kernel_info = ""
    kernel_info += build_history[0]
    for h in build_history[1:]:
        if h.find(kernel_hsh):
            kernel_info += h
    return kernel_info


def scrap_crash(crash, exp_dir, kernels_dir):
    crash = os.path.normpath(crash)

    report_dir = os.path.join(exp_dir, "report")

    crash_hash = os.path.basename(crash)
    outdir = os.path.join(report_dir, crash_hash)
    os.makedirs(outdir, exist_ok=True)
    for root, _, files in os.walk(crash):
        for file in files:
            if file.startswith("machineInfo"):
                continue
            src = os.path.join(root, file)
            dst = os.path.join(outdir, file)
            shutil.copyfile(src, dst)

    kernel_info = get_kernel_info(crash, kernels_dir)
    try:
        with open(os.path.join(outdir, "kernel_info"), "w") as f:
            f.write(kernel_info)
    except:
        pass


def scrap_crashes(crashes):
    exp_dir = os.environ["EXP_DIR"]
    kernels_dir = os.environ["KERNELS_DIR"]
    for crash in crashes:
        scrap_crash(crash, exp_dir, kernels_dir)


def main():
    if len(sys.argv) < 2:
        exit(1)

    scrap_crashes(sys.argv[1:])


if __name__ == "__main__":
    main()
